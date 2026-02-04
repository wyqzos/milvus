package translator

import (
	"context"
	"fmt"
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
// Milvus: Insert(collection, fields_data=[...])
func (e *InsertExecutor) Execute(ctx context.Context, q *ParsedQuery) (*Result, error) {
	// TODO: Implement when integrating with Milvus
	//
	// Steps:
	// 1. Get collection schema: proxy.DescribeCollection()
	// 2. Convert SQL values to Milvus FieldData
	// 3. Build InsertRequest
	// 4. Execute: proxy.Insert()
	// 5. Return result with rows affected

	_ = ctx
	_ = q

	return nil, fmt.Errorf("INSERT not implemented")
}

// getCollectionSchema retrieves the collection schema.
func (e *InsertExecutor) getCollectionSchema(ctx context.Context, collection string) error {
	// TODO: Call proxy.DescribeCollection()
	_ = ctx
	_ = collection
	return fmt.Errorf("not implemented")
}

// convertToFieldsData converts SQL values to Milvus FieldData.
func (e *InsertExecutor) convertToFieldsData(columns []string, values [][]interface{}) error {
	// TODO: Implement type conversion
	_ = columns
	_ = values
	return fmt.Errorf("not implemented")
}
