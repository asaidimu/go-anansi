package query

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/schema"
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
type SchemaManager interface {
	// CreateCollection generates and executes the necessary DDL statements to create a
	// table based on a schema definition.
	CreateCollection(ctx context.Context,schema schema.SchemaDefinition) error

	// CreateIndex generates and executes the DDL statements to create an index on a table.
	CreateIndex(ctx context.Context, collection string, index schema.IndexDefinition) error

	// DropCollection removes a table from the database.
	DropCollection(ctx context.Context, name string) error

	// DropIndex removes an index from the database.
	DropIndex(ctx context.Context, collection string, index schema.IndexDefinition) error

	// CollectionExists checks if a table with the given name exists in the database.
	CollectionExists(ctx context.Context,name string) (bool, error)
}

// DatabaseInteractor defines the contract for low-level database operations.
// It abstracts the specific SQL dialect and database-dependent logic, providing a
// consistent interface for the persistence layer to interact with the database.
// Implementations of this interface are responsible for managing both non-transactional
// and transactional operations.

type DatabaseInteractor interface {
	SchemaManager

	// SelectDocuments retrieves documents from the database based on a QueryDSL
	SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *Query) ([]data.Document, error)

	// SelectStream executes a SELECT query and returns a channel of documents.
	SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *Query) (<-chan data.Document, <-chan error, error)

	// UpdateDocuments modifies documents in the database that match the provided filters.
	UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, computedUpdates map[string]Query, filters *QueryFilter) (int64, error)

	// InsertDocuments adds new documents to the database.
	InsertDocuments(ctx context.Context, schema *schema.SchemaDefinition, records []data.Document) ([]data.Document, error)

	// DeleteDocuments removes documents from the database that match the provided filters.
	DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *QueryFilter, unsafeDelete bool) (int64, error)

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
