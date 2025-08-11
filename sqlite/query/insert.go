package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// SQLiteInsertValues handles the VALUES clause in an INSERT statement.
type SQLiteInsertValues struct {
	factory *sqliteFactory
	data    common.Document
	batch   []common.Document
	schema  *schema.SchemaDefinition
}

func (i *SQLiteInsertValues) Value() (string, []any, error) {
	if i.data == nil && i.batch == nil {
		return "", nil, fmt.Errorf("no data provided for insert")
	}

	if i.data != nil && i.batch != nil {
		return "", nil, fmt.Errorf("cannot specify both single document and batch")
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

func (i *SQLiteInsertValues) buildSingleInsert(doc common.Document) (string, []any, error) {
	if len(doc) == 0 {
		return "", nil, fmt.Errorf("empty document provided for insert")
	}

	// Get sorted fields for stable order
	sortedFields := make([]string, 0, len(doc))
	for field := range doc {
		sortedFields = append(sortedFields, field)
	}
	sort.Strings(sortedFields)

	var fields []string
	var placeholders []string
	var params []any

	for _, field := range sortedFields {
		fields = append(fields, field)
		placeholders = append(placeholders, i.factory.nextParam())

		value := doc[field]
		var fieldDef *schema.FieldDefinition
		if i.schema != nil {
			fieldDef = i.schema.FindField(field)
		}

		convertedValue, err := toSQLiteValue(fieldDef, value)
		if err != nil {
			return "", nil, fmt.Errorf("failed to convert field %s: %w", field, err)
		}
		params = append(params, convertedValue)
	}

	query := fmt.Sprintf("(%s) VALUES (%s) RETURNING *;",
		strings.Join(fields, ", "),
		strings.Join(placeholders, ", "))

	return query, params, nil
}

func (i *SQLiteInsertValues) buildBatchInsert(batch []common.Document) (string, []any, error) {
	if len(batch) == 0 {
		return "", nil, fmt.Errorf("empty batch provided for insert")
	}

	// Use first document to determine field order
	firstDoc := batch[0]
	if len(firstDoc) == 0 {
		return "", nil, fmt.Errorf("first document in batch is empty")
	}

	// Get sorted fields for stable order
	sortedFields := make([]string, 0, len(firstDoc))
	for field := range firstDoc {
		sortedFields = append(sortedFields, field)
	}
	sort.Strings(sortedFields)

	var fields []string
	var allPlaceholders []string
	var params []any

	// Build field names (only once)
	for _, field := range sortedFields {
		fields = append(fields, field)
	}

	// Build values for each document
	for docIdx, doc := range batch {
		var placeholders []string

		// Ensure all documents have the same fields
		if len(doc) != len(firstDoc) {
			return "", nil, fmt.Errorf("document %d has different number of fields than first document", docIdx)
		}

		for _, field := range sortedFields {
			value, exists := doc[field]
			if !exists {
				return "", nil, fmt.Errorf("document %d missing field %s", docIdx, field)
			}

			placeholders = append(placeholders, i.factory.nextParam())

			var fieldDef *schema.FieldDefinition
			if i.schema != nil {
				fieldDef = i.schema.FindField(field)
			}

			convertedValue, err := toSQLiteValue(fieldDef, value)
			if err != nil {
				return "", nil, fmt.Errorf("failed to convert field %s in document %d: %w", field, docIdx, err)
			}
			params = append(params, convertedValue)
		}

		allPlaceholders = append(allPlaceholders, fmt.Sprintf("(%s)", strings.Join(placeholders, ", ")))
	}

	query := fmt.Sprintf("(%s) VALUES %s RETURNING *;",
		strings.Join(fields, ", "),
		strings.Join(allPlaceholders, ", "))

	return query, params, nil
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
	case common.Document:
		values.data = v
	case []common.Document:
		values.batch = v
	default:
		return nil, fmt.Errorf("invalid data type for insert: %T", extra)
	}

	tree.values = values
	return &SQLiteInsertStatement{tree: tree}, nil
}
