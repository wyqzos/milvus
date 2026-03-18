package translator

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"
	"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"
	"google.golang.org/protobuf/proto"
)

// SelectExecutor handles SELECT query execution.
type SelectExecutor struct {
	translator *Translator
}

// NewSelectExecutor creates a new SELECT executor.
func NewSelectExecutor(t *Translator) *SelectExecutor {
	return &SelectExecutor{translator: t}
}

// Execute runs a SELECT query.
func (e *SelectExecutor) Execute(ctx context.Context, q *ParsedQuery) (*Result, error) {
	if q.OrderBy != nil && q.OrderBy.IsVectorDistance {
		return e.executeVectorSearch(ctx, q)
	}
	return e.executeQuery(ctx, q)
}

// executeQuery handles regular SELECT queries (maps to Milvus Query).
// Not yet implemented — Phase 2.
func (e *SelectExecutor) executeQuery(ctx context.Context, q *ParsedQuery) (*Result, error) {
	_ = ctx
	_ = q
	return nil, fmt.Errorf("SELECT without vector search is not yet supported (Phase 2)")
}

// executeVectorSearch handles SELECT with vector similarity ordering.
//
// SQL: SELECT id, name FROM my_collection ORDER BY embedding <-> '[0.1, 0.2, ...]' LIMIT 10
// Milvus: Search(collection, vectors=[[0.1, 0.2, ...]], anns_field="embedding", topk=10)
func (e *SelectExecutor) executeVectorSearch(ctx context.Context, q *ParsedQuery) (*Result, error) {
	if e.translator.proxy == nil {
		return nil, fmt.Errorf("no Milvus proxy connected")
	}

	topK := q.Limit
	if topK <= 0 {
		topK = 10
	}

	// Build PlaceholderGroup with the query vector
	plgBytes, err := buildPlaceholderGroup(q.VectorQuery)
	if err != nil {
		return nil, fmt.Errorf("build placeholder group: %w", err)
	}

	searchParams := []*commonpb.KeyValuePair{
		{Key: "anns_field", Value: q.VectorField},
		{Key: "topk", Value: strconv.FormatInt(topK, 10)},
		{Key: "params", Value: `{}`},
		{Key: "round_decimal", Value: "-1"},
	}
	// Only send metric_type if explicitly specified via operator (<-> L2, <=> COSINE, <#> IP).
	// Otherwise let Milvus auto-detect from the index.
	if q.MetricType != "" {
		searchParams = append(searchParams, &commonpb.KeyValuePair{Key: "metric_type", Value: q.MetricType})
	}

	req := &milvuspb.SearchRequest{
		DbName:         e.translator.database,
		CollectionName: q.Collection,
		Dsl:            q.Where,
		DslType:        commonpb.DslType_BoolExprV1,
		OutputFields:   q.Columns,
		SearchParams:   searchParams,
		Nq: 1,
	}
	req.SearchInput = &milvuspb.SearchRequest_PlaceholderGroup{
		PlaceholderGroup: plgBytes,
	}

	resp, err := e.translator.proxy.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if resp.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
		return nil, fmt.Errorf("search error: %s", resp.GetStatus().GetReason())
	}

	return convertSearchResults(q.Columns, resp)
}

// buildPlaceholderGroup serializes a float32 vector into a PlaceholderGroup protobuf.
func buildPlaceholderGroup(vec []float32) ([]byte, error) {
	// Encode vector as little-endian bytes
	bs := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(bs[i*4:], math.Float32bits(f))
	}

	plg := &commonpb.PlaceholderGroup{
		Placeholders: []*commonpb.PlaceholderValue{
			{
				Tag:    "$0",
				Type:   commonpb.PlaceholderType_FloatVector,
				Values: [][]byte{bs},
			},
		},
	}
	return proto.Marshal(plg)
}

// convertSearchResults converts milvuspb.SearchResults to our Result type.
func convertSearchResults(outputFields []string, resp *milvuspb.SearchResults) (*Result, error) {
	results := resp.GetResults()
	if results == nil {
		return &Result{CommandTag: "SELECT 0"}, nil
	}

	numResults := int(results.GetTopks()[0]) // results for the first (only) query vector
	scores := results.GetScores()

	// Build column definitions: _distance + requested output fields
	colDefs := make([]ColumnDef, 0, len(outputFields)+1)
	colDefs = append(colDefs, ColumnDef{Name: "_distance", Type: "float"})
	for _, col := range outputFields {
		colDefs = append(colDefs, ColumnDef{Name: col, Type: "text"})
	}

	// Build a lookup from field name to FieldData
	fieldMap := make(map[string]*schemapb.FieldData)
	for _, fd := range results.GetFieldsData() {
		fieldMap[fd.GetFieldName()] = fd
	}

	// Build rows
	rows := make([][]interface{}, numResults)
	for i := 0; i < numResults; i++ {
		row := make([]interface{}, 0, len(outputFields)+1)
		row = append(row, scores[i])

		for _, col := range outputFields {
			fd, ok := fieldMap[col]
			if !ok {
				row = append(row, nil)
				continue
			}
			row = append(row, extractFieldValue(fd, i))
		}
		rows[i] = row
	}

	return &Result{
		Columns:    colDefs,
		Rows:       rows,
		CommandTag: fmt.Sprintf("SELECT %d", numResults),
	}, nil
}

// extractFieldValue extracts the value at index i from a column-oriented FieldData.
func extractFieldValue(fd *schemapb.FieldData, i int) interface{} {
	switch fd.GetType() {
	case schemapb.DataType_Int64:
		data := fd.GetScalars().GetLongData().GetData()
		if i < len(data) {
			return data[i]
		}
	case schemapb.DataType_Int32:
		data := fd.GetScalars().GetIntData().GetData()
		if i < len(data) {
			return data[i]
		}
	case schemapb.DataType_VarChar, schemapb.DataType_String:
		data := fd.GetScalars().GetStringData().GetData()
		if i < len(data) {
			return data[i]
		}
	case schemapb.DataType_Float:
		data := fd.GetScalars().GetFloatData().GetData()
		if i < len(data) {
			return data[i]
		}
	case schemapb.DataType_Double:
		data := fd.GetScalars().GetDoubleData().GetData()
		if i < len(data) {
			return data[i]
		}
	case schemapb.DataType_Bool:
		data := fd.GetScalars().GetBoolData().GetData()
		if i < len(data) {
			return data[i]
		}
	case schemapb.DataType_JSON:
		data := fd.GetScalars().GetJsonData().GetData()
		if i < len(data) {
			return string(data[i])
		}
	case schemapb.DataType_FloatVector:
		data := fd.GetVectors().GetFloatVector().GetData()
		dim := int(fd.GetVectors().GetDim())
		if dim > 0 && i*dim+dim <= len(data) {
			return data[i*dim : (i+1)*dim]
		}
	}
	return nil
}
