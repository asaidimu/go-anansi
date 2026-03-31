package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// SQLiteInsertValues handles the VALUES clause in an INSERT statement.
type SQLiteInsertValues struct {
	factory *sqliteFactory
	data    map[string]any
	batch   []map[string]any
	schema  *definition.Schema
	fields  []string
}

func (i *SQLiteInsertValues) Value() (string, []any, error) {
	if i.data == nil && i.batch == nil {
		return "", nil, ErrInsertNoDataProvided
	}

	if i.data != nil && i.batch != nil {
		return "", nil, ErrInsertSingleAndBatchMutuallyExclusive
	}

	// Determine fields to use for insert
	if i.schema != nil {
		i.fields = i.schema.FieldNames()
	}

	if len(i.fields) == 0 {
		return "", nil, ErrInsertSchemaNoFields
	}

	// Handle single document case
	if i.data != nil {
		return i.buildSingleInsert(i.data)
	}

	// Handle batch case
	if len(i.batch) == 0 {
		return "", nil, ErrInsertEmptyBatch
	}

	return i.buildBatchInsert(i.batch)
}

func (i *SQLiteInsertValues) buildSingleInsert(doc map[string]any) (string, []any, error) {
	if len(doc) == 0 {
		return "", nil, ErrInsertEmptyDocument
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

func (i *SQLiteInsertValues) buildBatchInsert(batch []map[string]any) (string, []any, error) {
	if len(batch) == 0 {
		return "", nil, ErrInsertEmptyBatch
	}

	var allPlaceholders []string
	var allParams []any

	for docIdx, doc := range batch {
		placeholders, params, err := i.processDocumentFields(doc)
		if err != nil {
			return "", nil, ErrInsertDocumentError.WithCause(fmt.Errorf("document %d: %w", docIdx, err))
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
	doc map[string]any,
) ([]string, []any, error) {
	fields := i.fields
	placeholders := make([]string, 0, len(fields))
	params := make([]any, 0, len(fields))

	for _, fieldName := range fields {
		value, exists := doc[fieldName]
		if !exists {
			value = nil
		}

		placeholders = append(placeholders, i.factory.nextParam())

		var fieldDef *definition.Field
		if i.schema != nil {
			_, fieldDef = i.schema.FindField(fieldName)
		}

		convertedValue, err := toSQLiteValue(fieldDef, value)
		if err != nil {
			return nil, nil, ErrInsertFieldConversionFailed.WithCause(fmt.Errorf("failed to convert field %s: %w", fieldName, err))
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
		return "", nil, ErrInsertStatementNoTarget
	}
	targetSQL, targetParams, err := s.tree.target.Value()
	if err != nil {
		return "", nil, err
	}
	sqlParts = append(sqlParts, targetSQL)
	allParams = append(allParams, targetParams...)

	// VALUES clause
	if s.tree.values == nil {
		return "", nil, ErrInsertStatementNoValues
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
		return "", nil, ErrInsertNoTargetSpecified
	}
	return fmt.Sprintf("INSERT INTO %s", i.target.Name), nil, nil
}

// buildInsertTree builds a SQLNode for an INSERT statement.
func (f *sqliteFactory) buildInsertTree(q *query.Query, extra any) (SQLNode, error) {
	tree := &insertTree{}

	if q.Target == nil {
		return nil, ErrInsertQueryNoTarget
	}
	tree.target = &SQLiteInsertTargetClause{
		target: q.Target,
	}

	if extra == nil {
		return nil, ErrInsertQueryNoData
	}

	values := &SQLiteInsertValues{
		factory: f,
		schema:  q.Target.Schema,
	}

	switch v := extra.(type) {
	case map[string]any:
		values.data = v
	case []map[string]any:
		values.batch = v
	default:
		return nil, ErrInsertInvalidDataType.WithCause(fmt.Errorf("invalid data type for insert: %T", extra))
	}

	tree.values = values
	return &SQLiteInsertStatement{tree: tree}, nil
}
