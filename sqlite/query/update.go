package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// SQLiteUpdateAssignments handles the SET clause in an UPDATE statement.
type SQLiteUpdateAssignments struct {
	factory *sqliteFactory
	data    common.Document
	schema  *schema.SchemaDefinition
}

func (u *SQLiteUpdateAssignments) Value() (string, []any, error) {
	if len(u.data) == 0 {
		return "", nil, fmt.Errorf("no data provided for update")
	}

	var parts []string
	var params []any

	// To ensure stable order for testing
	fields := make([]string, 0, len(u.data))
	for field := range u.data {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		value := u.data[field]
		param := u.factory.nextParam()
		parts = append(parts, fmt.Sprintf("%s = %s", field, param))

		var fieldDef *schema.FieldDefinition
		if u.schema != nil {
			fieldDef = u.schema.FindField(field)
		}

		convertedValue, err := toSQLiteValue(fieldDef, value)
		if err != nil {
			return "", nil, err
		}
		params = append(params, convertedValue)
	}

	return fmt.Sprintf("SET %s", strings.Join(parts, ", ")), params, nil
}

// SQLiteUpdateStatement represents a complete UPDATE statement
type SQLiteUpdateStatement struct {
	tree *updateTree
}

func (s *SQLiteUpdateStatement) Value() (string, []any, error) {
	var sqlParts []string
	var allParams []any

	// UPDATE clause (target)
	if s.tree.target == nil {
		return "", nil, fmt.Errorf("update statement must have a target")
	}
	targetSQL, targetParams, err := s.tree.target.Value()
	if err != nil {
		return "", nil, err
	}
	sqlParts = append(sqlParts, targetSQL)
	allParams = append(allParams, targetParams...)

	// SET clause
	if s.tree.assignments == nil {
		return "", nil, fmt.Errorf("update statement must have assignments")
	}
	assignmentsSQL, assignmentsParams, err := s.tree.assignments.Value()
	if err != nil {
		return "", nil, err
	}
	sqlParts = append(sqlParts, assignmentsSQL)
	allParams = append(allParams, assignmentsParams...)

	// WHERE clause
	if s.tree.filters != nil {
		filterSQL, filterParams, err := s.tree.filters.Value()
		if err != nil {
			return "", nil, err
		}
		if filterSQL != "" {
			sqlParts = append(sqlParts, filterSQL)
			allParams = append(allParams, filterParams...)
		}
	}

	return strings.Join(sqlParts, " "), allParams, nil
}

func (s *SQLiteUpdateStatement) StatementType() string {
	return "UPDATE"
}

// SQLiteUpdateTargetClause handles the UPDATE clause
type SQLiteUpdateTargetClause struct {
	target *query.QueryTarget
}

func (u *SQLiteUpdateTargetClause) Value() (string, []any, error) {
	if u.target == nil {
		return "", nil, fmt.Errorf("no target specified for update")
	}
	return fmt.Sprintf("UPDATE %s", u.target.Name), nil, nil
}

// buildUpdateTree builds a SQLNode for an UPDATE statement.
func (f *sqliteFactory) buildUpdateTree(q *query.Query, extra any) (SQLNode, error) {
	tree := &updateTree{}

	if q.Target == nil {
		return nil, fmt.Errorf("update query must have a target")
	}
	tree.target = &SQLiteUpdateTargetClause{
		target: q.Target,
	}

	if extra == nil {
		return nil, fmt.Errorf("update query must have data")
	}
	data, ok := extra.(common.Document)
	if !ok {
		return nil, fmt.Errorf("invalid data type for update: %T", extra)
	}
	tree.assignments = &SQLiteUpdateAssignments{
		factory: f,
		data:    data,
		schema:  q.Target.Schema,
	}

	if q.Filters != nil {
		// Create a projection for the WHERE clause
		schemas := make(map[string]*schema.SchemaDefinition)
		if q.Target.Schema != nil {
			name := q.Target.Name
			if q.Target.Alias != nil {
				name = *q.Target.Alias
			}
			schemas[name] = q.Target.Schema
		}

		projection := &SQLiteSelectProjection{
			factory: f,
			schemas: schemas,
		}

		tree.filters = &SQLiteWhereClause{
			factory:    f,
			filter:     q.Filters,
			projection: projection,
		}
	}

	return &SQLiteUpdateStatement{tree: tree}, nil
}
