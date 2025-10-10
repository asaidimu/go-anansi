// Package native provides abstractions for database-specific query execution.
// This package defines interfaces and types that allow the Anansi framework
// to work with different database dialects (SQL, MongoDB, etc.) through a
// common abstraction layer while maintaining type safety for native query formats.
package native

import (
	"context"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// StatementType represents the type of database operation.
// This enumeration provides a database-agnostic way to categorize
// different types of database operations across various dialects.
type StatementType string

const (
	// StmtRaw represents a raw native query
	StmtRaw StatementType = "RAW"

	// StmtSelect represents a data retrieval operation (SELECT in SQL, find in MongoDB)
	StmtSelect StatementType = "SELECT"

	// StmtUpdate represents a data modification operation (UPDATE in SQL, updateMany in MongoDB)
	StmtUpdate StatementType = "UPDATE"

	// StmtDelete represents a data removal operation (DELETE in SQL, deleteMany in MongoDB)
	StmtDelete StatementType = "DELETE"

	// StmtInsert represents a data insertion operation (INSERT in SQL, insertMany in MongoDB)
	StmtInsert StatementType = "INSERT"

	// StmtCreateCollection represents collection/table creation (CREATE TABLE in SQL, createCollection in MongoDB)
	StmtCreateCollection StatementType = "CREATE_COLLECTION"

	// StmtCheckCollection represents as check for a collection/table
	StmtCheckCollection StatementType = "CHECK_COLLECTION"

	// StmtDropCollection represents collection/table deletion (DROP TABLE in SQL, drop in MongoDB)
	StmtDropCollection StatementType = "DROP_COLLECTION"

	// StmtCreateIndex represents index creation (CREATE INDEX in SQL, createIndex in MongoDB)
	StmtCreateIndex StatementType = "CREATE_INDEX"

	// StmtDropIndex represents index deletion (DROP INDEX in SQL, dropIndex in MongoDB)
	StmtDropIndex StatementType = "DROP_INDEX"
)

// Query is a generic, type-safe representation of a database-native query.
// This interface wraps database-specific query objects (like *sql.Stmt for SQL
// or primitive.M for MongoDB) while providing common metadata.
//
// Type parameter T represents the database's native query payload type:
//   - For SQL dialects: typically a prepared statement or query string
//   - For MongoDB: typically primitive.M or bson.D
//   - For other databases: their respective native query format
type Query[T any] interface {
	// Raw returns the database's native query payload.
	// This provides direct access to the underlying database-specific query object
	// for cases where dialect-specific functionality is needed.
	Raw() T

	// StatementType returns the high-level type of the statement.
	// This allows the framework to handle different operation types appropriately
	// without needing to inspect the native query format.
	StatementType() StatementType
}

// NativeQuery represents a complete query package ready for execution.
// It combines the native query with its associated schema definition,
// providing all the information needed for query execution and result mapping.
//
// Type parameter T represents the database's native query format.
type NativeQuery[T any] struct {

	// Query contains the database-specific query representation
	Query Query[T]

	// Schema defines the structure and constraints for the data being queried.
	// This is used for result mapping, validation, and type conversion.
	Schema *schema.SchemaDefinition
}

// QueryFactory is implemented by each database dialect (SQL, MongoDB, etc.).
// It provides the core functionality to convert framework-agnostic DSL queries
// into database-specific native queries while maintaining type safety.
//
// Type parameter T represents the target database's native query format.
type QueryFactory[T any] interface {
	// Build converts a DSL Query into a dialect-specific NativeQuery[T].
	//
	// Parameters:
	//   - q: The framework's DSL query representation
	//   - stmtType: The type of operation being performed
	//   - extra: Additional dialect-specific parameters or options
	//
	// Returns the native query or an error if conversion fails.
	Build(q *query.Query, stmtType StatementType, extra any) (Query[T], error)

	// Capabilities returns the query capabilities supported by this dialect.
	// This allows the framework to understand what operations and features
	// are available in the target database system.
	Capabilities() query.Capabilities
}

// QueryExecutor handles the actual execution of native queries against a database.
// It provides both synchronous and streaming query execution, along with
// transaction management capabilities.
//
// Type parameter T represents the database's native query format.
type QueryExecutor[T any] interface {
	// Query executes a query and returns all results synchronously.
	// Suitable for queries expected to return a bounded result set.
	//
	// Returns a slice of documents or an error if execution fails.
	Query(ctx context.Context, query NativeQuery[T]) ([]data.Document, error)

	// ExecuteRawQuery executes a raw, templated query directly against the database.
	// This allows for operations that are not tied to a specific collection,
	// or for highly optimized, custom queries.
	ExecuteQuery(ctx context.Context, query NativeQuery[T]) (*query.RawQueryResult, error)

	// Exec executes a non-query statement (INSERT, UPDATE, DELETE).
	// Returns the number of affected rows/documents, or an error if execution fails.
	Exec(ctx context.Context, query NativeQuery[T]) (int64, error)

	// QueryStream executes a query and returns results as a stream.
	// Suitable for large result sets or when memory usage needs to be controlled.
	//
	// Returns:
	//   - A channel for receiving documents
	//   - A channel for receiving errors during streaming
	//   - An immediate error if the query cannot be started
	QueryStream(ctx context.Context, query NativeQuery[T]) (<-chan data.Document, <-chan error, error)

	// BeginTransaction starts a new database transaction.
	// Returns a new QueryExecutor instance that operates within the transaction context.
	BeginTransaction(ctx context.Context) (QueryExecutor[T], error)

	// Commit commits the current transaction.
	// Should only be called on QueryExecutor instances returned by BeginTransaction.
	Commit(ctx context.Context) error

	// Rollback rolls back the current transaction.
	// Should only be called on QueryExecutor instances returned by BeginTransaction.
	Rollback(ctx context.Context) error

	// Close releases any resources held by the executor.
	// This should be called when the executor is no longer needed.
	Close() error
}

// NativeQueryBuilder is the public entry point for converting DSL queries to native queries.
// This interface provides a simplified API for query building, hiding the complexity
// of the underlying QueryFactory implementation.
//
// Type parameter T represents the database's native query format.
type NativeQueryBuilder[T any] interface {
	// Build converts a DSL Query into a database-specific NativeQuery[T].
	// This is the primary method used by application code to prepare queries for execution.
	//
	// Parameters:
	//   - q: The framework's DSL query representation
	//   - stmtType: The type of operation being performed
	//   - extra: Additional dialect-specific parameters or options
	//
	// Returns the native query or an error if conversion fails.
	Build(q *query.Query, stmtType StatementType, extra any) (Query[T], error)
}
