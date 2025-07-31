package query

import "github.com/asaidimu/go-anansi/v6/core/schema"

// SchemaManager defines the contract for database schema operations.
// It abstracts the specific DDL (Data Definition Language) syntax for creating and
// managing collections (tables) and their indexes.
type SchemaManager interface {
	// CreateCollection generates and executes the necessary DDL statements to create a
	// table based on a schema definition.
	CreateCollection(schema schema.SchemaDefinition) error

	// CreateIndex generates and executes the DDL statements to create an index on a table.
	CreateIndex(name string, index schema.IndexDefinition) error

	// DropCollection removes a table from the database.
	DropCollection(name string) error

	// CollectionExists checks if a table with the given name exists in the database.
	CollectionExists(name string) (bool, error)
}
