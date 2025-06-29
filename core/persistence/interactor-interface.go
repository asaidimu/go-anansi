// Package persistence defines the interfaces for direct database interactions,
// abstracting the underlying database technology.
package persistence

import (
	"context"

	"github.com/asaidimu/go-anansi/v2/core/query"
	"github.com/asaidimu/go-anansi/v2/core/schema"
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

	// TablePrefix is a string that will be prepended to all table names. This is
	// useful for avoiding naming conflicts in a shared database environment.
	TablePrefix string

	// SchemaName specifies the database schema (e.g., in PostgreSQL) where the
	// tables should be created. For databases like SQLite that do not use schema
	// names, this field is ignored.
	SchemaName string
}

// DatabaseInteractor defines the contract for low-level database operations.
// It abstracts the specific SQL dialect and database-dependent logic, providing a
// consistent interface for the persistence layer to interact with the database.
// Implementations of this interface are responsible for managing both non-transactional
// and transactional operations.
type DatabaseInteractor interface {
	// SelectDocuments retrieves documents from the database based on a QueryDSL query.
	SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.QueryDSL) ([]schema.Document, error)

	// UpdateDocuments modifies documents in the database that match the provided filters.
	UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error)

	// InsertDocuments adds new documents to the database.
	InsertDocuments(ctx context.Context, schema *schema.SchemaDefinition, records []map[string]any) ([]schema.Document, error)

	// DeleteDocuments removes documents from the database that match the provided filters.
	DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error)

	// CreateCollection generates and executes the necessary DDL statements to create a
	// table based on a schema definition.
	CreateCollection(schema schema.SchemaDefinition) error

	// GetColumnType maps a generic FieldType from the schema to a database-specific
	// column type (e.g., mapping FieldTypeString to VARCHAR(255) or TEXT).
	GetColumnType(fieldType schema.FieldType, field *schema.FieldDefinition) string

	// CreateIndex generates and executes the DDL statements to create an index on a table.
	CreateIndex(name string, index schema.IndexDefinition) error

	// DropCollection removes a table from the database.
	DropCollection(name string) error

	// CollectionExists checks if a table with the given name exists in the database.
	CollectionExists(name string) (bool, error)

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
