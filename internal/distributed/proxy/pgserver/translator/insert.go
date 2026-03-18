package translator

import (
	"context"
	"fmt"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"
	"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"
)

// InsertExecutor handles INSERT query execution.
type InsertExecutor struct {
	translator *Translator
}

// NewInsertExecutor creates a new INSERT executor.
func NewInsertExecutor(t *Translator) *InsertExecutor {
	return &InsertExecutor{translator: t}
}

// Execute runs an INSERT query.
//
// SQL: INSERT INTO my_collection (id, name, embedding) VALUES (1, 'foo', '[0.1, 0.2]')
// Milvus: Insert(collection, FieldsData=[column-oriented data])
func (e *InsertExecutor) Execute(ctx context.Context, q *ParsedQuery) (*Result, error) {
	if e.translator.proxy == nil {
		return nil, fmt.Errorf("no Milvus proxy connected")
	}

	if q.Collection == "" {
		return nil, fmt.Errorf("missing table name in INSERT")
	}
	if len(q.InsertCols) == 0 {
		return nil, fmt.Errorf("missing column list in INSERT")
	}
	if len(q.InsertVals) == 0 {
		return nil, fmt.Errorf("missing values in INSERT")
	}

	// Transpose row-oriented data to column-oriented FieldData
	fieldsData, err := buildFieldsData(q.InsertCols, q.InsertVals)
	if err != nil {
		return nil, fmt.Errorf("build fields data: %w", err)
	}

	req := &milvuspb.InsertRequest{
		DbName:         e.translator.database,
		CollectionName: q.Collection,
		FieldsData:     fieldsData,
		NumRows:        uint32(len(q.InsertVals)),
	}

	resp, err := e.translator.proxy.Insert(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("insert failed: %w", err)
	}

	if resp.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
		return nil, fmt.Errorf("insert error: %s", resp.GetStatus().GetReason())
	}

	cnt := resp.GetInsertCnt()
	return &Result{
		RowsAffected: cnt,
		CommandTag:   fmt.Sprintf("INSERT 0 %d", cnt),
	}, nil
}

// buildFieldsData transposes row-oriented SQL values into column-oriented schemapb.FieldData.
// Type is inferred from the first non-nil Go value in each column:
//
//	int64     → DataType_Int64 (LongData)
//	float64   → DataType_Double (DoubleData)
//	string    → DataType_VarChar (StringData)
//	bool      → DataType_Bool (BoolData)
//	[]float32 → DataType_FloatVector (FloatVector)
func buildFieldsData(cols []string, rows [][]interface{}) ([]*schemapb.FieldData, error) {
	numCols := len(cols)
	numRows := len(rows)

	fields := make([]*schemapb.FieldData, numCols)

	for c := 0; c < numCols; c++ {
		// Collect column values across all rows
		colVals := make([]interface{}, numRows)
		for r := 0; r < numRows; r++ {
			if c < len(rows[r]) {
				colVals[r] = rows[r][c]
			}
		}

		// Find first non-nil value to infer type
		var sample interface{}
		for _, v := range colVals {
			if v != nil {
				sample = v
				break
			}
		}
		if sample == nil {
			return nil, fmt.Errorf("column %q has all NULL values, cannot infer type", cols[c])
		}

		fd, err := buildColumnFieldData(cols[c], sample, colVals)
		if err != nil {
			return nil, fmt.Errorf("column %q: %w", cols[c], err)
		}
		fields[c] = fd
	}

	return fields, nil
}

// buildColumnFieldData builds a single column's FieldData from the inferred type and values.
func buildColumnFieldData(name string, sample interface{}, vals []interface{}) (*schemapb.FieldData, error) {
	switch sample.(type) {
	case int64:
		data := make([]int64, len(vals))
		for i, v := range vals {
			if v == nil {
				data[i] = 0
			} else if n, ok := v.(int64); ok {
				data[i] = n
			} else {
				return nil, fmt.Errorf("row %d: expected int64, got %T", i, v)
			}
		}
		return &schemapb.FieldData{
			Type:      schemapb.DataType_Int64,
			FieldName: name,
			Field: &schemapb.FieldData_Scalars{
				Scalars: &schemapb.ScalarField{
					Data: &schemapb.ScalarField_LongData{
						LongData: &schemapb.LongArray{Data: data},
					},
				},
			},
		}, nil

	case float64:
		data := make([]float64, len(vals))
		for i, v := range vals {
			if v == nil {
				data[i] = 0
			} else if f, ok := v.(float64); ok {
				data[i] = f
			} else if n, ok := v.(int64); ok {
				data[i] = float64(n) // int literal in a float column
			} else {
				return nil, fmt.Errorf("row %d: expected float64, got %T", i, v)
			}
		}
		return &schemapb.FieldData{
			Type:      schemapb.DataType_Double,
			FieldName: name,
			Field: &schemapb.FieldData_Scalars{
				Scalars: &schemapb.ScalarField{
					Data: &schemapb.ScalarField_DoubleData{
						DoubleData: &schemapb.DoubleArray{Data: data},
					},
				},
			},
		}, nil

	case string:
		data := make([]string, len(vals))
		for i, v := range vals {
			if v == nil {
				data[i] = ""
			} else if s, ok := v.(string); ok {
				data[i] = s
			} else {
				return nil, fmt.Errorf("row %d: expected string, got %T", i, v)
			}
		}
		return &schemapb.FieldData{
			Type:      schemapb.DataType_VarChar,
			FieldName: name,
			Field: &schemapb.FieldData_Scalars{
				Scalars: &schemapb.ScalarField{
					Data: &schemapb.ScalarField_StringData{
						StringData: &schemapb.StringArray{Data: data},
					},
				},
			},
		}, nil

	case bool:
		data := make([]bool, len(vals))
		for i, v := range vals {
			if v == nil {
				data[i] = false
			} else if b, ok := v.(bool); ok {
				data[i] = b
			} else {
				return nil, fmt.Errorf("row %d: expected bool, got %T", i, v)
			}
		}
		return &schemapb.FieldData{
			Type:      schemapb.DataType_Bool,
			FieldName: name,
			Field: &schemapb.FieldData_Scalars{
				Scalars: &schemapb.ScalarField{
					Data: &schemapb.ScalarField_BoolData{
						BoolData: &schemapb.BoolArray{Data: data},
					},
				},
			},
		}, nil

	case []float32:
		// FloatVector: flatten all vectors into a single float array
		dim := len(sample.([]float32))
		allFloats := make([]float32, 0, len(vals)*dim)
		for i, v := range vals {
			if v == nil {
				allFloats = append(allFloats, make([]float32, dim)...)
			} else if vec, ok := v.([]float32); ok {
				if len(vec) != dim {
					return nil, fmt.Errorf("row %d: vector dimension mismatch (expected %d, got %d)", i, dim, len(vec))
				}
				allFloats = append(allFloats, vec...)
			} else {
				return nil, fmt.Errorf("row %d: expected []float32, got %T", i, v)
			}
		}
		return &schemapb.FieldData{
			Type:      schemapb.DataType_FloatVector,
			FieldName: name,
			Field: &schemapb.FieldData_Vectors{
				Vectors: &schemapb.VectorField{
					Dim: int64(dim),
					Data: &schemapb.VectorField_FloatVector{
						FloatVector: &schemapb.FloatArray{Data: allFloats},
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported value type %T", sample)
	}
}
