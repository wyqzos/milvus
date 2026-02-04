package translator

import (
	"fmt"
	"strconv"
	"strings"
)

// Type mapping between PostgreSQL and Milvus

// MilvusDataType represents Milvus data types.
// TODO: Replace with schemapb.DataType when integrating with Milvus
type MilvusDataType int32

const (
	DataTypeNone        MilvusDataType = 0
	DataTypeBool        MilvusDataType = 1
	DataTypeInt8        MilvusDataType = 2
	DataTypeInt16       MilvusDataType = 3
	DataTypeInt32       MilvusDataType = 4
	DataTypeInt64       MilvusDataType = 5
	DataTypeFloat       MilvusDataType = 10
	DataTypeDouble      MilvusDataType = 11
	DataTypeString      MilvusDataType = 20
	DataTypeVarChar     MilvusDataType = 21
	DataTypeJSON        MilvusDataType = 23
	DataTypeFloatVector MilvusDataType = 101
	DataTypeBinaryVector MilvusDataType = 100
)

// SQLType represents a PostgreSQL SQL type.
type SQLType struct {
	Name      string
	Length    int  // For VARCHAR(n)
	Precision int  // For NUMERIC(p,s)
	Scale     int
	IsArray   bool
}

// ParseSQLType parses a SQL type string.
func ParseSQLType(s string) (*SQLType, error) {
	s = strings.TrimSpace(strings.ToUpper(s))

	// Check for array type
	isArray := strings.HasSuffix(s, "[]")
	if isArray {
		s = strings.TrimSuffix(s, "[]")
	}

	// Check for parameterized types like VARCHAR(256) or VECTOR(128)
	if idx := strings.Index(s, "("); idx != -1 {
		name := s[:idx]
		params := strings.TrimSuffix(s[idx+1:], ")")

		sqlType := &SQLType{Name: name, IsArray: isArray}

		// Parse parameters
		parts := strings.Split(params, ",")
		if len(parts) >= 1 {
			if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				sqlType.Length = n
				sqlType.Precision = n
			}
		}
		if len(parts) >= 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				sqlType.Scale = n
			}
		}

		return sqlType, nil
	}

	return &SQLType{Name: s, IsArray: isArray}, nil
}

// ToMilvusType converts a SQL type to Milvus DataType.
func (t *SQLType) ToMilvusType() (MilvusDataType, error) {
	switch t.Name {
	// Integer types
	case "BIGINT", "INT8":
		return DataTypeInt64, nil
	case "INTEGER", "INT", "INT4":
		return DataTypeInt32, nil
	case "SMALLINT", "INT2":
		return DataTypeInt16, nil
	case "TINYINT":
		return DataTypeInt8, nil

	// Floating point types
	case "REAL", "FLOAT4":
		return DataTypeFloat, nil
	case "DOUBLE PRECISION", "FLOAT8", "DOUBLE":
		return DataTypeDouble, nil

	// Boolean
	case "BOOLEAN", "BOOL":
		return DataTypeBool, nil

	// String types
	case "VARCHAR", "CHAR", "TEXT", "STRING":
		return DataTypeVarChar, nil

	// Vector type (custom extension)
	case "VECTOR":
		return DataTypeFloatVector, nil

	// Binary vector (custom extension)
	case "BITVECTOR", "BVECTOR":
		return DataTypeBinaryVector, nil

	// JSON types
	case "JSON", "JSONB":
		return DataTypeJSON, nil

	default:
		return DataTypeNone, fmt.Errorf("unsupported SQL type: %s", t.Name)
	}
}

// MilvusTypeToSQL converts Milvus DataType to SQL type string.
func MilvusTypeToSQL(dt MilvusDataType, params map[string]string) string {
	switch dt {
	case DataTypeBool:
		return "BOOLEAN"
	case DataTypeInt8:
		return "TINYINT"
	case DataTypeInt16:
		return "SMALLINT"
	case DataTypeInt32:
		return "INTEGER"
	case DataTypeInt64:
		return "BIGINT"
	case DataTypeFloat:
		return "REAL"
	case DataTypeDouble:
		return "DOUBLE PRECISION"
	case DataTypeVarChar, DataTypeString:
		if maxLen, ok := params["max_length"]; ok {
			return fmt.Sprintf("VARCHAR(%s)", maxLen)
		}
		return "TEXT"
	case DataTypeFloatVector:
		if dim, ok := params["dim"]; ok {
			return fmt.Sprintf("VECTOR(%s)", dim)
		}
		return "VECTOR"
	case DataTypeBinaryVector:
		if dim, ok := params["dim"]; ok {
			return fmt.Sprintf("BITVECTOR(%s)", dim)
		}
		return "BITVECTOR"
	case DataTypeJSON:
		return "JSONB"
	default:
		return "UNKNOWN"
	}
}

// VectorLiteralParser parses vector literals in SQL.
// Supports formats:
//   - '[0.1, 0.2, 0.3]' (JSON array)
//   - '{0.1, 0.2, 0.3}' (PostgreSQL array)
//   - 'ARRAY[0.1, 0.2, 0.3]' (PostgreSQL ARRAY constructor)
type VectorLiteralParser struct{}

// Parse parses a vector literal string into a float32 slice.
func (p *VectorLiteralParser) Parse(s string) ([]float32, error) {
	s = strings.TrimSpace(s)

	// Remove quotes if present
	s = strings.Trim(s, "'\"")

	// Handle different formats
	switch {
	case strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
		// JSON array format: [0.1, 0.2, 0.3]
		return p.parseJSONArray(s)

	case strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}"):
		// PostgreSQL array format: {0.1, 0.2, 0.3}
		return p.parsePGArray(s)

	case strings.HasPrefix(strings.ToUpper(s), "ARRAY["):
		// ARRAY constructor: ARRAY[0.1, 0.2, 0.3]
		return p.parseArrayConstructor(s)

	default:
		return nil, fmt.Errorf("unrecognized vector format: %s", s)
	}
}

func (p *VectorLiteralParser) parseJSONArray(s string) ([]float32, error) {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	return p.parseFloatList(s)
}

func (p *VectorLiteralParser) parsePGArray(s string) ([]float32, error) {
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	return p.parseFloatList(s)
}

func (p *VectorLiteralParser) parseArrayConstructor(s string) ([]float32, error) {
	// Remove ARRAY[ prefix and ] suffix
	upper := strings.ToUpper(s)
	idx := strings.Index(upper, "ARRAY[")
	if idx == -1 {
		return nil, fmt.Errorf("invalid ARRAY constructor")
	}
	s = s[idx+6:]
	s = strings.TrimSuffix(s, "]")
	return p.parseFloatList(s)
}

func (p *VectorLiteralParser) parseFloatList(s string) ([]float32, error) {
	if s == "" {
		return []float32{}, nil
	}

	parts := strings.Split(s, ",")
	result := make([]float32, len(parts))

	for i, part := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, fmt.Errorf("invalid float at position %d: %w", i, err)
		}
		result[i] = float32(f)
	}

	return result, nil
}
