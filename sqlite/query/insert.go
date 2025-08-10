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
	schema  *schema.SchemaDefinition
}

func (i *SQLiteInsertValues) Value() (string, []any, error) {
	if len(i.data) == 0 {
		return "", nil, fmt.Errorf("no data provided for insert")
	}

	var fields []string
	var placeholders []string
	var params []any

	// To ensure stable order for testing
	sortedFields := make([]string, 0, len(i.data))
	for field := range i.data {
		sortedFields = append(sortedFields, field)
	}
	sort.Strings(sortedFields)

	for _, field := range sortedFields {
		fields = append(fields, field)
		placeholders = append(placeholders, i.factory.nextParam())
		value := i.data[field]

		var fieldDef *schema.FieldDefinition
		if i.schema != nil {
			fieldDef = i.schema.FindField(field)
		}

		convertedValue, err := toSQLiteValue(fieldDef, value)
		if err != nil {
			return "", nil, err
		}
		params = append(params, convertedValue)
	}

	return fmt.Sprintf("(%s) VALUES (%s)", strings.Join(fields, ", "), strings.Join(placeholders, ", ")), params, nil
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
	data, ok := extra.(common.Document)
	if !ok {
		return nil, fmt.Errorf("invalid data type for insert: %T", extra)
	}
	tree.values = &SQLiteInsertValues{
		factory: f,
		data:    data,
		schema:  q.Target.Schema,
	}

	return &SQLiteInsertStatement{tree: tree}, nil
}
