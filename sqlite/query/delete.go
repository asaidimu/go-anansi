package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// SQLiteDeleteFromClause handles the DELETE FROM clause
type SQLiteDeleteFromClause struct {
	target *query.QueryTarget
}

func (c *SQLiteDeleteFromClause) Value() (string, []any, error) {
	if c.target == nil {
		return "", nil, ErrDeleteNoTarget
	}
	return fmt.Sprintf("DELETE FROM %s", c.target.Name), nil, nil
}

// SQLiteDeleteStatement represents a complete DELETE statement
type SQLiteDeleteStatement struct {
	tree *deleteTree
}

func (s *SQLiteDeleteStatement) Value() (string, []any, error) {
	var sqlParts []string
	var allParams []any

	// DELETE FROM clause
	if s.tree.target == nil {
		return "", nil, ErrDeleteStatementNoTarget
	}
	targetSQL, targetParams, err := s.tree.target.Value()
	if err != nil {
		return "", nil, err
	}
	sqlParts = append(sqlParts, targetSQL)
	allParams = append(allParams, targetParams...)

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

func (s *SQLiteDeleteStatement) StatementType() string {
	return "DELETE"
}

// buildDeleteTree builds a SQLNode for a DELETE statement.
func (f *sqliteFactory) buildDeleteTree(q *query.Query) (SQLNode, error) {
	tree := &deleteTree{}

	// Build target (FROM clause)
	if q.Target == nil {
		return nil, ErrDeleteQueryNoTarget
	}
	tree.target = &SQLiteDeleteFromClause{
		target: q.Target,
	}

	// Build filters (WHERE clause)
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

	return &SQLiteDeleteStatement{tree: tree}, nil
}
