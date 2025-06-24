package query

import "github.com/asaidimu/go-anansi/core"

type QueryGeneratorFactory interface {
	CreateGenerator(schema *core.SchemaDefinition) (QueryGenerator, error)
}

// QueryGenerator defines the interface for generating database-specific queries
// from a generic QueryDSL object.
type QueryGenerator interface {
    // GenerateSelectSQL creates a SQL query string and a slice of query parameters
    // for a given table name and QueryDSL object.
    GenerateSelectSQL(dsl *QueryDSL) (string, []any, error)

    // GenerateUpdateSQL creates a complete SQL UPDATE query string and its corresponding
    // parameters from a map of updates and a QueryFilter for the WHERE clause.
    GenerateUpdateSQL(updates map[string]any, filters *QueryFilter) (string, []any, error)

    // GenerateInsertSQL creates a SQL INSERT query string and its corresponding
    // parameters from a slice of records (maps of field names to values).
    // Supports both single and batch inserts.
    GenerateInsertSQL(records []map[string]any) (string, []any, error)

    // GenerateDeleteSQL creates a SQL DELETE query string and its corresponding
    // parameters from a QueryFilter for the WHERE clause.
    // By default, requires a WHERE clause for safety. Set unsafeDelete to true
    // to allow deletion without WHERE clause (deletes all records).
    GenerateDeleteSQL(filters *QueryFilter, unsafeDelete bool) (string, []any, error)
}
