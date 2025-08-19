package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// SQLiteInsertValues handles the VALUES clause in an INSERT statement.
type SQLiteInsertValues struct {
	factory *sqliteFactory
	data    data.Document
	batch   []data.Document
	schema  *schema.SchemaDefinition
	fields  []string
}

func (i *SQLiteInsertValues) Value() (string, []any, error) {
	if i.data == nil && i.batch == nil {
		return "", nil, fmt.Errorf("no data provided for insert")
	}

	if i.data != nil && i.batch != nil {
		return "", nil, fmt.Errorf("cannot specify both single document and batch")
	}

	// Determine fields to use for insert
	if i.schema != nil {
		i.fields = i.schema.FieldNames()
	}

	if len(i.schema.FieldNames()) == 0 {
		return "", nil, fmt.Errorf("provided schema has no fields defined for insert")
	}

	// Handle single document case
	if i.data != nil {
		return i.buildSingleInsert(i.data)
	}

	// Handle batch case
	if len(i.batch) == 0 {
		return "", nil, fmt.Errorf("empty batch provided for insert")
	}

	return i.buildBatchInsert(i.batch)
}

func (i *SQLiteInsertValues) buildSingleInsert(doc data.Document) (string, []any, error) {
	if len(doc) == 0 {
		return "", nil, fmt.Errorf("empty document provided for insert")
	}

	placeholders, params, err := i.processDocumentFields(doc)
	if err != nil {
		return "", nil, err
	}

	query := fmt.Sprintf("(%s) VALUES (%s) RETURNING *;",
		strings.Join(i.fields, ", "),
		strings.Join(placeholders, ", "))

	return query, params, nil
}

func (i *SQLiteInsertValues) buildBatchInsert(batch []data.Document) (string, []any, error) {
	if len(batch) == 0 {
		return "", nil, fmt.Errorf("empty batch provided for insert")
	}

	var allPlaceholders []string
	var allParams []any

	for docIdx, doc := range batch {
		placeholders, params, err := i.processDocumentFields(doc)
		if err != nil {
			return "", nil, fmt.Errorf("document %d: %w", docIdx, err)
		}

		allPlaceholders = append(allPlaceholders, fmt.Sprintf("(%s)", strings.Join(placeholders, ", ")))
		allParams = append(allParams, params...)
	}

	query := fmt.Sprintf("(%s) VALUES %s RETURNING *;",
		strings.Join(i.fields, ", "),
		strings.Join(allPlaceholders, ", "))

	return query, allParams, nil
}

// processDocumentFields converts document values to SQLite placeholders and params
func (i *SQLiteInsertValues) processDocumentFields(
	doc data.Document,
) ([]string, []any, error) {
	fields := i.fields
	placeholders := make([]string, 0, len(fields))
	params := make([]any, 0, len(fields))

	for _, field := range fields {
		value, exists := doc[field]
		if !exists {
			value = nil
		}

		placeholders = append(placeholders, i.factory.nextParam())

		var fieldDef *schema.FieldDefinition
		if i.schema != nil {
			fieldDef = i.schema.FindField(field)
		}

		convertedValue, err := toSQLiteValue(fieldDef, value)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to convert field %s: %w", field, err)
		}

		params = append(params, convertedValue)
	}

	return placeholders, params, nil
}

// SQLiteInsertStatement represents a complete INSERT statement
type SQLiteInsertStatement struct {
	tree *insertTree
}

func (s *SQLiteInsertStatement) Value() (string, []any, error) {
	var sqlParts []string
	var allParams []any

	// INSERT INTO clause
	if s.tree.target == nil {
		return "", nil, fmt.Errorf("insert statement must have a target")
	}
	targetSQL, targetParams, err := s.tree.target.Value()
	if err != nil {
		return "", nil, err
	}
	sqlParts = append(sqlParts, targetSQL)
	allParams = append(allParams, targetParams...)

	// VALUES clause
	if s.tree.values == nil {
		return "", nil, fmt.Errorf("insert statement must have values")
	}
	valuesSQL, valuesParams, err := s.tree.values.Value()
	if err != nil {
		return "", nil, err
	}
	sqlParts = append(sqlParts, valuesSQL)
	allParams = append(allParams, valuesParams...)

	return strings.Join(sqlParts, " "), allParams, nil
}

func (s *SQLiteInsertStatement) StatementType() native.StatementType {
	return native.StmtInsert
}

// SQLiteInsertTargetClause handles the INSERT INTO clause
type SQLiteInsertTargetClause struct {
	target *query.QueryTarget
}

func (i *SQLiteInsertTargetClause) Value() (string, []any, error) {
	if i.target == nil {
		return "", nil, fmt.Errorf("no target specified for insert")
	}
	return fmt.Sprintf("INSERT INTO %s", i.target.Name), nil, nil
}

// buildInsertTree builds a SQLNode for an INSERT statement.
func (f *sqliteFactory) buildInsertTree(q *query.Query, extra any) (SQLNode, error) {
	tree := &insertTree{}

	if q.Target == nil {
		return nil, fmt.Errorf("insert query must have a target")
	}
	tree.target = &SQLiteInsertTargetClause{
		target: q.Target,
	}

	if extra == nil {
		return nil, fmt.Errorf("insert query must have data")
	}

	values := &SQLiteInsertValues{
		factory: f,
		schema:  q.Target.Schema,
	}

	switch v := extra.(type) {
	case data.Document:
		values.data = v
	case []data.Document:
		values.batch = v
	default:
		return nil, fmt.Errorf("invalid data type for insert: %T", extra)
	}

	tree.values = values
	return &SQLiteInsertStatement{tree: tree}, nil
}
