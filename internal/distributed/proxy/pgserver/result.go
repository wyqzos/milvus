package pgserver

import (
	"fmt"
	"strconv"
)

// QueryResult represents the result of a query execution.
type QueryResult struct {
	// Type of result
	Type ResultType

	// For SELECT/Search queries
	Columns []ColumnInfo
	Rows    [][]interface{}

	// For INSERT/UPDATE/DELETE
	RowsAffected int64

	// For DDL commands
	CommandTag string

	// Error if query failed
	Error error
}

// ResultType indicates the type of query result.
type ResultType int

const (
	ResultTypeSelect ResultType = iota
	ResultTypeInsert
	ResultTypeUpdate
	ResultTypeDelete
	ResultTypeDDL
	ResultTypeEmpty
	ResultTypeError
)

// ColumnInfo describes a result column.
type ColumnInfo struct {
	Name     string
	Type     string
	TypeOID  int32
	Nullable bool
}

// NewSelectResult creates a result for SELECT queries.
func NewSelectResult(columns []ColumnInfo, rows [][]interface{}) *QueryResult {
	return &QueryResult{
		Type:    ResultTypeSelect,
		Columns: columns,
		Rows:    rows,
	}
}

// NewInsertResult creates a result for INSERT queries.
func NewInsertResult(rowsAffected int64) *QueryResult {
	return &QueryResult{
		Type:         ResultTypeInsert,
		RowsAffected: rowsAffected,
		CommandTag:   fmt.Sprintf("INSERT 0 %d", rowsAffected),
	}
}

// NewDeleteResult creates a result for DELETE queries.
func NewDeleteResult(rowsAffected int64) *QueryResult {
	return &QueryResult{
		Type:         ResultTypeDelete,
		RowsAffected: rowsAffected,
		CommandTag:   fmt.Sprintf("DELETE %d", rowsAffected),
	}
}

// NewDDLResult creates a result for DDL commands.
func NewDDLResult(commandTag string) *QueryResult {
	return &QueryResult{
		Type:       ResultTypeDDL,
		CommandTag: commandTag,
	}
}

// NewErrorResult creates an error result.
func NewErrorResult(err error) *QueryResult {
	return &QueryResult{
		Type:  ResultTypeError,
		Error: err,
	}
}

// ToRowDescription converts column info to PostgreSQL RowDescription.
func (r *QueryResult) ToRowDescription() *RowDescription {
	if r.Type != ResultTypeSelect || len(r.Columns) == 0 {
		return nil
	}

	fields := make([]FieldDescription, len(r.Columns))
	for i, col := range r.Columns {
		fields[i] = FieldDescription{
			Name:         col.Name,
			TableOID:     0,
			ColumnAttrNo: int16(i + 1),
			TypeOID:      col.TypeOID,
			TypeSize:     -1, // Variable length
			TypeModifier: -1,
			Format:       0, // Text format
		}
	}

	return &RowDescription{Fields: fields}
}

// ToDataRows converts result rows to PostgreSQL DataRow format.
func (r *QueryResult) ToDataRows() []*DataRow {
	if r.Type != ResultTypeSelect {
		return nil
	}

	dataRows := make([]*DataRow, len(r.Rows))
	for i, row := range r.Rows {
		values := make([][]byte, len(row))
		for j, val := range row {
			values[j] = formatValue(val)
		}
		dataRows[i] = &DataRow{Values: values}
	}

	return dataRows
}

// formatValue converts a Go value to PostgreSQL text format.
func formatValue(val interface{}) []byte {
	if val == nil {
		return nil // NULL
	}

	switch v := val.(type) {
	case string:
		return []byte(v)
	case int:
		return []byte(strconv.Itoa(v))
	case int32:
		return []byte(strconv.FormatInt(int64(v), 10))
	case int64:
		return []byte(strconv.FormatInt(v, 10))
	case float32:
		return []byte(strconv.FormatFloat(float64(v), 'f', -1, 32))
	case float64:
		return []byte(strconv.FormatFloat(v, 'f', -1, 64))
	case bool:
		if v {
			return []byte("t")
		}
		return []byte("f")
	case []byte:
		return v
	case []float32:
		// Format vector as PostgreSQL array
		return formatFloatArray(v)
	default:
		return []byte(fmt.Sprintf("%v", v))
	}
}

// formatFloatArray formats a float32 slice as a PostgreSQL array.
func formatFloatArray(arr []float32) []byte {
	if len(arr) == 0 {
		return []byte("{}")
	}

	result := []byte("{")
	for i, v := range arr {
		if i > 0 {
			result = append(result, ',')
		}
		result = append(result, []byte(strconv.FormatFloat(float64(v), 'f', -1, 32))...)
	}
	result = append(result, '}')

	return result
}

// PostgreSQL type OIDs for common types
const (
	OIDInt4        = 23
	OIDInt8        = 20
	OIDFloat4      = 700
	OIDFloat8      = 701
	OIDText        = 25
	OIDVarchar     = 1043
	OIDBool        = 16
	OIDTimestamp   = 1114
	OIDFloat4Array = 1021
	OIDJSON        = 114
	OIDJSONB       = 3802
)

// MilvusTypeToOID maps Milvus types to PostgreSQL OIDs.
func MilvusTypeToOID(milvusType string) int32 {
	switch milvusType {
	case "Int8", "Int16", "Int32":
		return OIDInt4
	case "Int64":
		return OIDInt8
	case "Float":
		return OIDFloat4
	case "Double":
		return OIDFloat8
	case "VarChar", "String":
		return OIDText
	case "Bool":
		return OIDBool
	case "FloatVector", "BinaryVector":
		return OIDFloat4Array
	case "JSON":
		return OIDJSONB
	default:
		return OIDText
	}
}
