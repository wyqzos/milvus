// Package translator converts PostgreSQL SQL queries to Milvus operations.
package translator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"
)

// MilvusProxy defines the minimal Milvus interface needed by the translator.
// The real Proxy satisfies this interface automatically.
type MilvusProxy interface {
	Search(ctx context.Context, req *milvuspb.SearchRequest) (*milvuspb.SearchResults, error)
	Insert(ctx context.Context, req *milvuspb.InsertRequest) (*milvuspb.MutationResult, error)
}

// Translator converts SQL queries to Milvus operations.
type Translator struct {
	proxy    MilvusProxy
	database string
}

// NewTranslator creates a new SQL translator.
func NewTranslator(proxy MilvusProxy, database string) *Translator {
	return &Translator{
		proxy:    proxy,
		database: database,
	}
}

// StatementType represents the type of SQL statement.
type StatementType int

const (
	StmtUnknown StatementType = iota
	StmtSelect
	StmtInsert
	StmtUpdate
	StmtDelete
	StmtCreateTable
	StmtDropTable
	StmtCreateIndex
	StmtDropIndex
	StmtShow
	StmtDescribe
)

// ParsedQuery represents a parsed SQL query.
type ParsedQuery struct {
	Type       StatementType
	Raw        string
	Collection string // Table name maps to collection

	// SELECT fields
	Columns    []string
	Where      string
	OrderBy    *OrderByClause
	Limit      int64
	Offset     int64

	// INSERT fields
	InsertCols []string
	InsertVals [][]interface{}

	// Vector search fields
	VectorField  string
	VectorQuery  []float32
	MetricType   string
	SearchParams map[string]interface{}
}

// OrderByClause represents an ORDER BY clause.
type OrderByClause struct {
	Field      string
	Descending bool

	// For vector similarity: ORDER BY embedding <-> '[...]'
	IsVectorDistance bool
	VectorField      string
	VectorQuery      []float32
}

// Execute translates and executes the SQL query.
func (t *Translator) Execute(ctx context.Context, sql string) (*Result, error) {
	// Step 1: Parse SQL
	parsed, err := t.Parse(sql)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Step 2: Execute based on statement type
	switch parsed.Type {
	case StmtSelect:
		return t.executeSelect(ctx, parsed)
	case StmtInsert:
		return t.executeInsert(ctx, parsed)
	case StmtDelete:
		return t.executeDelete(ctx, parsed)
	case StmtCreateTable:
		return t.executeCreateTable(ctx, parsed)
	case StmtDropTable:
		return t.executeDropTable(ctx, parsed)
	case StmtShow:
		return t.executeShow(ctx, parsed)
	case StmtDescribe:
		return t.executeDescribe(ctx, parsed)
	default:
		return nil, fmt.Errorf("unsupported statement type: %s", sql)
	}
}

// Parse parses a SQL query into a ParsedQuery.
func (t *Translator) Parse(sql string) (*ParsedQuery, error) {
	sql = strings.TrimSpace(sql)
	upper := strings.ToUpper(sql)

	// Determine statement type
	switch {
	case strings.HasPrefix(upper, "SELECT"):
		return t.parseSelect(sql)
	case strings.HasPrefix(upper, "INSERT"):
		return t.parseInsert(sql)
	case strings.HasPrefix(upper, "DELETE"):
		return t.parseDelete(sql)
	case strings.HasPrefix(upper, "CREATE TABLE"):
		return t.parseCreateTable(sql)
	case strings.HasPrefix(upper, "DROP TABLE"):
		return t.parseDropTable(sql)
	case strings.HasPrefix(upper, "SHOW"):
		return t.parseShow(sql)
	case strings.HasPrefix(upper, "DESCRIBE"), strings.HasPrefix(upper, "\\D"):
		return t.parseDescribe(sql)
	default:
		return nil, fmt.Errorf("unsupported SQL: %s", sql)
	}
}

// Result represents the result of query execution.
type Result struct {
	Columns      []ColumnDef
	Rows         [][]interface{}
	RowsAffected int64
	CommandTag   string
}

// ColumnDef defines a result column.
type ColumnDef struct {
	Name string
	Type string
}

// Placeholder implementations - to be filled in select.go, insert.go, etc.

// parseSelect parses SELECT statements with manual string matching.
// Supports: SELECT cols FROM table [WHERE expr] [ORDER BY vec <-> '[...]'] [LIMIT n]
func (t *Translator) parseSelect(sql string) (*ParsedQuery, error) {
	q := &ParsedQuery{Type: StmtSelect, Raw: sql}
	sql = strings.TrimSuffix(strings.TrimSpace(sql), ";")

	upper := strings.ToUpper(sql)

	// Find FROM position to split columns from the rest
	fromIdx := strings.Index(upper, " FROM ")
	if fromIdx == -1 {
		return q, nil // bare SELECT like "SELECT 1", leave unparsed
	}

	// Extract column list (between SELECT and FROM)
	colsPart := strings.TrimSpace(sql[len("SELECT"):fromIdx])
	if colsPart == "*" {
		q.Columns = []string{"*"}
	} else {
		for _, col := range strings.Split(colsPart, ",") {
			q.Columns = append(q.Columns, strings.TrimSpace(col))
		}
	}

	// Everything after FROM
	rest := strings.TrimSpace(sql[fromIdx+len(" FROM "):])

	// Extract table name (next word)
	spaceIdx := strings.IndexAny(rest, " \t\n")
	if spaceIdx == -1 {
		q.Collection = rest
		return q, nil
	}
	q.Collection = rest[:spaceIdx]
	rest = strings.TrimSpace(rest[spaceIdx:])
	upperRest := strings.ToUpper(rest)

	// Extract WHERE clause
	if strings.HasPrefix(upperRest, "WHERE ") {
		rest = rest[len("WHERE "):]
		upperRest = strings.ToUpper(rest)

		// WHERE ends at ORDER BY or LIMIT or end of string
		endIdx := len(rest)
		for _, kw := range []string{" ORDER BY ", " LIMIT "} {
			if idx := strings.Index(upperRest, kw); idx != -1 && idx < endIdx {
				endIdx = idx
			}
		}
		q.Where = strings.TrimSpace(rest[:endIdx])
		rest = strings.TrimSpace(rest[endIdx:])
		upperRest = strings.ToUpper(rest)
	}

	// Extract ORDER BY clause — look for vector distance operators
	// Supported: <-> (L2), <=> (COSINE), <#> (IP)
	// Metric type is optional — if not mapped, Milvus auto-detects from the index.
	if strings.HasPrefix(upperRest, "ORDER BY ") {
		rest = rest[len("ORDER BY "):]

		// Try each vector distance operator
		type vecOp struct {
			op     string
			metric string // empty = let Milvus auto-detect
		}
		ops := []vecOp{{"<->", ""}, {"<=>", ""}, {"<#>", ""}}

		for _, vop := range ops {
			arrowIdx := strings.Index(rest, vop.op)
			if arrowIdx == -1 {
				continue
			}
			vectorField := strings.TrimSpace(rest[:arrowIdx])

			afterArrow := strings.TrimSpace(rest[arrowIdx+len(vop.op):])
			vecLiteral, endPos := extractQuotedLiteral(afterArrow)

			parser := &VectorLiteralParser{}
			vecData, err := parser.Parse(vecLiteral)
			if err != nil {
				return nil, fmt.Errorf("invalid vector literal in ORDER BY: %w", err)
			}

			q.OrderBy = &OrderByClause{
				IsVectorDistance: true,
				VectorField:     vectorField,
				VectorQuery:     vecData,
			}
			q.VectorField = vectorField
			q.VectorQuery = vecData
			q.MetricType = vop.metric

			rest = strings.TrimSpace(afterArrow[endPos:])
			upperRest = strings.ToUpper(rest)
			break
		}
	}

	// Extract LIMIT
	if strings.HasPrefix(upperRest, "LIMIT ") {
		limitStr := strings.TrimSpace(rest[len("LIMIT "):])
		// Take the next word as the limit value
		limitEnd := strings.IndexAny(limitStr, " \t\n;")
		if limitEnd == -1 {
			limitEnd = len(limitStr)
		}
		n, err := strconv.ParseInt(limitStr[:limitEnd], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid LIMIT value: %w", err)
		}
		q.Limit = n
	}

	return q, nil
}

// extractQuotedLiteral extracts a single-quoted string from input.
// Returns the content (without quotes) and the position after the closing quote.
func extractQuotedLiteral(s string) (string, int) {
	if len(s) == 0 || s[0] != '\'' {
		// No quote — take until whitespace
		end := strings.IndexAny(s, " \t\n")
		if end == -1 {
			return s, len(s)
		}
		return s[:end], end
	}
	// Find closing quote
	closeIdx := strings.Index(s[1:], "'")
	if closeIdx == -1 {
		return s[1:], len(s)
	}
	return s[1 : closeIdx+1], closeIdx + 2
}

// parseInsert parses INSERT statements with manual string matching.
// Supports: INSERT INTO table (col1, col2, ...) VALUES (val1, val2, ...)
func (t *Translator) parseInsert(sql string) (*ParsedQuery, error) {
	q := &ParsedQuery{Type: StmtInsert, Raw: sql}
	sql = strings.TrimSuffix(strings.TrimSpace(sql), ";")
	upper := strings.ToUpper(sql)

	// Must start with INSERT INTO
	if !strings.HasPrefix(upper, "INSERT INTO ") {
		return nil, fmt.Errorf("expected INSERT INTO, got: %s", sql)
	}
	rest := strings.TrimSpace(sql[len("INSERT INTO "):])

	// Extract table name (next word before '(' or space)
	nameEnd := strings.IndexAny(rest, " (\t\n")
	if nameEnd == -1 {
		return nil, fmt.Errorf("missing column list in INSERT")
	}
	q.Collection = rest[:nameEnd]
	rest = strings.TrimSpace(rest[nameEnd:])

	// Extract column list: (col1, col2, ...)
	if rest[0] != '(' {
		return nil, fmt.Errorf("expected '(' for column list, got: %c", rest[0])
	}
	closeParen := strings.Index(rest, ")")
	if closeParen == -1 {
		return nil, fmt.Errorf("unclosed '(' in column list")
	}
	colList := rest[1:closeParen]
	for _, col := range strings.Split(colList, ",") {
		q.InsertCols = append(q.InsertCols, strings.TrimSpace(col))
	}
	rest = strings.TrimSpace(rest[closeParen+1:])

	// Must have VALUES
	upper = strings.ToUpper(rest)
	if !strings.HasPrefix(upper, "VALUES") {
		return nil, fmt.Errorf("expected VALUES, got: %s", rest)
	}
	rest = strings.TrimSpace(rest[len("VALUES"):])

	// Extract value rows: (val1, val2), (val3, val4), ...
	for len(rest) > 0 {
		if rest[0] != '(' {
			break
		}
		// Find matching close paren, respecting quoted strings
		closeParen := findMatchingParen(rest)
		if closeParen == -1 {
			return nil, fmt.Errorf("unclosed '(' in VALUES")
		}
		valStr := rest[1:closeParen]
		vals, err := parseValueList(valStr, len(q.InsertCols))
		if err != nil {
			return nil, fmt.Errorf("error parsing VALUES: %w", err)
		}
		q.InsertVals = append(q.InsertVals, vals)
		rest = strings.TrimSpace(rest[closeParen+1:])
		rest = strings.TrimPrefix(rest, ",")
		rest = strings.TrimSpace(rest)
	}

	if len(q.InsertVals) == 0 {
		return nil, fmt.Errorf("no values in INSERT")
	}

	return q, nil
}

// findMatchingParen finds the closing ')' for an opening '(' at position 0,
// skipping over single-quoted strings.
func findMatchingParen(s string) int {
	inQuote := false
	for i := 1; i < len(s); i++ {
		switch {
		case s[i] == '\'' && !inQuote:
			inQuote = true
		case s[i] == '\'' && inQuote:
			inQuote = false
		case s[i] == ')' && !inQuote:
			return i
		}
	}
	return -1
}

// parseValueList splits a comma-separated value list, respecting quotes.
// Returns parsed Go values (string, int64, float64, []float32).
func parseValueList(s string, expectedCols int) ([]interface{}, error) {
	parts := splitRespectingQuotes(s, ',')
	if len(parts) != expectedCols {
		return nil, fmt.Errorf("expected %d values, got %d", expectedCols, len(parts))
	}

	vals := make([]interface{}, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		vals[i] = parseSQLValue(part)
	}
	return vals, nil
}

// splitRespectingQuotes splits a string by delimiter, ignoring delimiters
// inside single-quoted strings.
func splitRespectingQuotes(s string, delim byte) []string {
	var parts []string
	inQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'':
			inQuote = !inQuote
		case s[i] == delim && !inQuote:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// parseSQLValue converts a SQL literal string to a Go value.
func parseSQLValue(s string) interface{} {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)

	// NULL
	if upper == "NULL" {
		return nil
	}

	// Boolean
	if upper == "TRUE" {
		return true
	}
	if upper == "FALSE" {
		return false
	}

	// Quoted string — could be a plain string or a vector literal like '[0.1, 0.2]'
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		inner := s[1 : len(s)-1]
		// Check if it looks like a vector literal
		if len(inner) > 0 && (inner[0] == '[' || inner[0] == '{') {
			parser := &VectorLiteralParser{}
			if vec, err := parser.Parse(inner); err == nil {
				return vec
			}
		}
		return inner
	}

	// Integer
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}

	// Float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Fallback: return as string
	return s
}

func (t *Translator) parseDelete(sql string) (*ParsedQuery, error) {
	// TODO: implement
	return &ParsedQuery{Type: StmtDelete, Raw: sql}, nil
}

func (t *Translator) parseCreateTable(sql string) (*ParsedQuery, error) {
	// TODO: implement
	return &ParsedQuery{Type: StmtCreateTable, Raw: sql}, nil
}

func (t *Translator) parseDropTable(sql string) (*ParsedQuery, error) {
	// TODO: implement
	return &ParsedQuery{Type: StmtDropTable, Raw: sql}, nil
}

func (t *Translator) parseShow(sql string) (*ParsedQuery, error) {
	// TODO: implement
	return &ParsedQuery{Type: StmtShow, Raw: sql}, nil
}

func (t *Translator) parseDescribe(sql string) (*ParsedQuery, error) {
	// TODO: implement
	return &ParsedQuery{Type: StmtDescribe, Raw: sql}, nil
}

func (t *Translator) executeSelect(ctx context.Context, q *ParsedQuery) (*Result, error) {
	return NewSelectExecutor(t).Execute(ctx, q)
}

func (t *Translator) executeInsert(ctx context.Context, q *ParsedQuery) (*Result, error) {
	return NewInsertExecutor(t).Execute(ctx, q)
}

func (t *Translator) executeDelete(ctx context.Context, q *ParsedQuery) (*Result, error) {
	// TODO: implement - see delete.go
	return nil, fmt.Errorf("DELETE not implemented")
}

func (t *Translator) executeCreateTable(ctx context.Context, q *ParsedQuery) (*Result, error) {
	// TODO: implement - see ddl.go
	return nil, fmt.Errorf("CREATE TABLE not implemented")
}

func (t *Translator) executeDropTable(ctx context.Context, q *ParsedQuery) (*Result, error) {
	// TODO: implement - see ddl.go
	return nil, fmt.Errorf("DROP TABLE not implemented")
}

func (t *Translator) executeShow(ctx context.Context, q *ParsedQuery) (*Result, error) {
	// TODO: implement
	return nil, fmt.Errorf("SHOW not implemented")
}

func (t *Translator) executeDescribe(ctx context.Context, q *ParsedQuery) (*Result, error) {
	// TODO: implement
	return nil, fmt.Errorf("DESCRIBE not implemented")
}
