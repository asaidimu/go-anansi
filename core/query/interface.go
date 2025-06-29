// Package query defines the interfaces for generating database-specific queries
// from the abstract QueryDSL.
package query

import (
	"github.com/asaidimu/go-anansi/v2/core/schema"
)

// QueryGeneratorFactory defines the interface for a factory that creates QueryGenerator instances.
// This allows for the creation of query generators that are specific to a given database schema.
type QueryGeneratorFactory interface {
	// CreateGenerator creates a new QueryGenerator for a specific schema.
	CreateGenerator(schema *schema.SchemaDefinition) (QueryGenerator, error)
}

// QueryGenerator defines the interface for generating database-specific query strings
// from a generic QueryDSL object. Each implementation of this interface will be
// responsible for translating the abstract query representation into a concrete
// SQL dialect (e.g., PostgreSQL, SQLite, MySQL).
type QueryGenerator interface {
	// GenerateSelectSQL creates a SQL SELECT query string and its corresponding parameters
	// from a QueryDSL object. This includes translating filters, sorting, pagination,
	// and projections into the target SQL dialect.
	GenerateSelectSQL(dsl *QueryDSL) (string, []any, error)

	// GenerateUpdateSQL creates a SQL UPDATE query string and its parameters from a map
	// of updates and a QueryFilter. It is responsible for constructing the SET and
	// WHERE clauses of the update statement.
	GenerateUpdateSQL(updates map[string]any, filters *QueryFilter) (string, []any, error)

	// GenerateInsertSQL creates a SQL INSERT query string and its parameters from a slice
	// of records. It supports both single and batch inserts, generating the appropriate
	// syntax for the target database.
	GenerateInsertSQL(records []map[string]any) (string, []any, error)

	// GenerateDeleteSQL creates a SQL DELETE query string and its parameters from a
	// QueryFilter. For safety, it requires a WHERE clause unless the `unsafeDelete`
	// flag is explicitly set to true.
	GenerateDeleteSQL(filters *QueryFilter, unsafeDelete bool) (string, []any, error)
}
