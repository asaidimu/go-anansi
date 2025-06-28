package persistence

import (
	"context"

	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
)

// InteractorOptions provides configuration for the interactor.
type InteractorOptions struct {
	// IfNotExists adds IF NOT EXISTS clause to CREATE TABLE statements.
	// When true, the CREATE TABLE statement will include "IF NOT EXISTS",
	// preventing an error if the table already exists.
	IfNotExists bool

	// DropIfExists drops the table before creating it.
	// When true, a DROP TABLE IF EXISTS statement will be executed
	// prior to attempting to create the new table. This operation is outside
	// the main transaction.
	DropIfExists bool

	// CreateIndexes determines whether to create indexes along with the table.
	// When true, any indexes defined in the schema will be created after
	// the table itself within the same transaction.
	CreateIndexes bool

	// TablePrefix adds a prefix to all table names.
	// The prefix is prepended to the base table name from the schema definition.
	TablePrefix string

	// SchemaName for databases that support it (e.g., PostgreSQL).
	// For SQLite, this field is typically not used as SQLite databases
	// do not have explicit schema names in the same way.
	SchemaName string
}

// DatabaseInteractor defines the interface for interacting with the database.
// It can operate in either a non-transactional (default) or transactional mode.
// Note: This interface includes the transactional methods, but they only
// become truly active/meaningful on an instance returned by StartTransaction.
type DatabaseInteractor interface {
	SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.QueryDSL) ([]schema.Document, error)
	UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error)
		InsertDocuments(ctx context.Context, schema *schema.SchemaDefinition, records []map[string]any) ([]schema.Document, error)
	DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error)

	// CreateCollection generates and executes DDL statements to create a table from a schema definition.
	CreateCollection(schema schema.SchemaDefinition) error

	// GetColumnType maps a FieldType to the database-specific column type.
	GetColumnType(fieldType schema.FieldType, field *schema.FieldDefinition) string

	// CreateIndex generates and executes DDL statements to create an index.
	CreateIndex(name string, index schema.IndexDefinition) error

	// DropCollection drops a table if it exists.
	DropCollection(name string) error

	// CollectionExists checks if a table exists in the database.
	CollectionExists(name string) (bool, error)

	// StartTransaction initiates a new database transaction.
	// It returns a *new* instance of DatabaseInteractor that operates
	// within the scope of that transaction.
	// Operations on the returned interactor are part of the transaction.
	// The original interactor instance remains non-transactional.
	StartTransaction(ctx context.Context) (DatabaseInteractor, error)

	// Commit commits the transaction.
	// This method should only be called on a DatabaseInteractor instance
	// that was returned by StartTransaction. Calling it on the original
	// (non-transactional) interactor should result in an error or no-op.
	Commit(ctx context.Context) error

	// Rollback rolls back the transaction.
	// This method should only be called on a DatabaseInteractor instance
	// that was returned by StartTransaction. Calling it on the original
	// (non-transactional) interactor should result in an error or no-op.
	Rollback(ctx context.Context) error
}
