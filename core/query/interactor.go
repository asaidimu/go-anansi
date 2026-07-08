package query

import (
	"context"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

// InteractorOptions provides a set of configurations for the DatabaseInteractor.
// These options allow for customizing the behavior of database operations, such as
// table creation and naming conventions.
type InteractorOptions struct {
	// IfNotExists, when true, adds an "IF NOT EXISTS" clause to CREATE TABLE
	// statements. This prevents an error from being thrown if the table already
	// exists in the database.
	IfNotExists bool

	// DropIfExists, when true, will execute a "DROP TABLE IF EXISTS" statement
	// before attempting to create a new table. This is useful for ensuring a clean
	// slate but should be used with caution as it is a destructive operation.
	DropIfExists bool

	// CreateIndexes, when true, will create any indexes defined in the schema
	// immediately after the table is created. This is done within the same
	// transaction to ensure atomicity.
	CreateIndexes bool

	// CollectionPrefix is a string that will be prepended to all table names. This is
	// useful for avoiding naming conflicts in a shared database environment.
	CollectionPrefix string

	// SchemaName specifies the database schema (e.g., in PostgreSQL) where the
	// tables should be created. For databases like SQLite that do not use schema
	// names, this field is ignored.
	SchemaName string
}

// SchemaManager defines the contract for database schema operations.
// It abstracts the specific DDL (Data Definition Language) syntax for creating and
// managing collections (tables) and their indexes.
//
// Methods that the backend does not support (per Capabilities.SchemaEvolution) should
// return ErrDDLNotSupported so the migration layer can fall back to a full table copy.
type SchemaManager interface {
	// CreateCollection generates and executes the necessary DDL statements to create a
	// table based on a schema definition.
	CreateCollection(ctx context.Context, schema definition.Schema) error

	// DropCollection removes a table from the database.
	DropCollection(ctx context.Context, name string) error

	// CollectionExists checks if a table with the given name exists in the database.
	CollectionExists(ctx context.Context, name string) (bool, error)

	// --- Index DDL ---

	// CreateIndex generates and executes the DDL statements to create an index on a table.
	CreateIndex(ctx context.Context, collection string, index definition.Index) error

	// DropIndex removes an index from the database.
	DropIndex(ctx context.Context, collection string, index definition.Index) error

	// --- Column DDL — may return ErrDDLNotSupported ---

	// AddColumn adds a single column to an existing table/collection.
	AddColumn(ctx context.Context, collection string, field definition.Field) error

	// DropColumn removes a single column from an existing table/collection.
	DropColumn(ctx context.Context, collection string, fieldName string) error

	// RenameColumn changes the name of a column in-place.
	RenameColumn(ctx context.Context, collection string, oldName, newName string) error
}

// RawQueryResult represents the outcome of executing a raw database query.
// It provides a flexible container for various types of query results.
// Developers using raw queries are expected to understand their specific
// database and query type, and interpret results accordingly.
type RawQueryResult struct {
	// Data for SELECT queries. Can be a slice of map[string]any or a more generic type.
	// The specific type depends on the database implementation and query type.
	Data        any `json:"data,omitempty"`

	// Count of rows returned for SELECT queries.
	Count       int `json:"count,omitempty"`

	// AffectedRows for INSERT, UPDATE, DELETE queries.
	AffectedRows int64 `json:"affectedRows,omitempty"`

	// Message provides additional status or information for any query type.
	Message     string `json:"message,omitempty"`

	// Success indicates if the query executed without error.
	Success     bool   `json:"success"`
}

// DatabaseInteractor defines the contract for low-level database operations.
// It abstracts the specific SQL dialect and database-dependent logic, providing a
// consistent interface for the persistence layer to interact with the database.
// Implementations of this interface are responsible for managing both non-transactional
// and transactional operations.
type DatabaseInteractor interface {
	SchemaManager

	// SelectDocuments retrieves documents from the database based on a QueryDSL
	SelectDocuments(ctx context.Context, schema *definition.Schema, dsl *Query) ([]map[string]any, int64, error)

	// SelectStream executes a SELECT query and returns a channel of documents.
	SelectStream(ctx context.Context, sc *definition.Schema, dsl *Query) (<-chan map[string]any, <-chan error, error)

	// UpdateDocuments modifies documents in the database that match the provided filters.
	UpdateDocuments(ctx context.Context, schema *definition.Schema, updates map[string]any, computedUpdates map[string]Query, filters *QueryFilter, returning bool) ([]map[string]any,int64, error)

	// InsertDocuments adds new documents to the database.
	InsertDocuments(ctx context.Context, schema *definition.Schema, records []map[string]any) ([]map[string]any, error)

	// DeleteDocuments removes documents from the database that match the provided filters.
	DeleteDocuments(ctx context.Context, schema *definition.Schema, filters *QueryFilter, unsafeDelete bool) (int64, error)

	// Query executes a raw, templated query directly against the database.
	// This allows for operations that are not tied to a specific collection,
	// or for highly optimized, custom queries.
	Query(ctx context.Context, query *Query) (*RawQueryResult, error)

	HasTransaction(ctx context.Context) bool

	// Capabilities returns a list of capabilities provided by the underlying
	// database
	Capabilities() Capabilities

	// SchemaManager returns only the methods available to the schema manager
	SchemaManager() SchemaManager

	// StartTransaction begins a new database transaction and returns a new instance of
	// the DatabaseInteractor that is scoped to that transaction. All operations on the
	// returned interactor will be part of the transaction.
	StartTransaction(ctx context.Context) (DatabaseInteractor, error)

	// Commit finalizes the transaction, making all changes permanent. This should only
	// be called on a transactional DatabaseInteractor.
	Commit(ctx context.Context) error

	// Rollback aborts the transaction, discarding all changes made within it. This
	// should only be called on a transactional DatabaseInteractor.
	Rollback(ctx context.Context) error

}
