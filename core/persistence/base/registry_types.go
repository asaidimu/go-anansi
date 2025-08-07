package base

import (
	"context"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// SchemaVersionRecord represents a historical version of a collection's schema and its physical name.
type SchemaVersionRecord struct {
	Physical string                  `json:"physical"` // The physical name of the collection in the database for this version.
	Schema   schema.SchemaDefinition `json:"schema"`   // The full schema definition for this version.
}

type RegistryEntry struct {
	Name          string                         `json:"name"`                  // The name of the collection this schema defines.
	Description   string                         `json:"description,omitempty"` // A human-readable description of the schema.
	ActiveVersion string                         `json:"version"`               // The current active version of the schema, pointing to an entry in 'Versions'.
	Versions      map[string]SchemaVersionRecord `json:"versions,omitempty"`    // A map of all schema versions, keyed by version string.
	// TODO: Watch for this should you ever decide to change the name of the
	// metadata field
	Metadata      map[string]any                 `json:"_metadata_,omitempty"`    // A map of all schema versions, keyed by version string.
}

// DropCollectionOptions provides flags to control the behavior of the DropCollection method,
// ensuring that destructive operations are explicit and intentional.
type DropCollectionOptions struct {
	// If true, all physical collections associated with all versions of the schema
	// will be permanently deleted from the database.
	// If false, only the schema's entry in the registry is removed, leaving the
	// physical data intact (though unmanaged).
	DeletePhysicalData bool
}

// CollectionRegistry defines the interface for managing the lifecycle of collections.
// It provides a centralized mechanism for creating, retrieving, evolving, and retiring
// collections and their schemas in a versioned and non-disruptive manner.
type CollectionRegistry interface {
	// CreateCollection creates the initial entry for a new collection in the registry.
	// It sets up the first version ("1.0.0"), marks it as active, and provisions
	// the underlying physical collection in the database.
	CreateCollection(ctx context.Context, schema *schema.SchemaDefinition) (*RegistryEntry, error)

	// DropCollection removes a collection's entire schema history from the registry.
	// The options force the caller to be explicit about deleting the underlying physical data.
	DropCollection(ctx context.Context, name string, opts DropCollectionOptions) error

	// PruneVersion permanently deletes the physical database collection (e.g., table)
	// associated with a specific, non-active schema version. This is a destructive
	// cleanup operation intended for use after a successful data migration.
	//
	// This method is atomic: it both drops the physical collection and updates the
	// registry to remove the reference to it, preventing data inconsistency.
	//
	// It will return an error if the specified version is the currently active version.
	PruneVersion(ctx context.Context, name, version string) (*RegistryEntry, error)

	// GetSchema retrieves a specific schema definition for a collection.
	// If no version is provided, it returns the currently active schema version.
	GetSchema(ctx context.Context, name string, version ...string) (*schema.SchemaDefinition, error)

	// GetRegistryEntry retrieves the complete management record for a collection.
	GetRegistryEntry(ctx context.Context, name string) (*RegistryEntry, error)

	// AddSchemaVersion introduces a new version of a schema for an existing collection.
	// If a new physicalName is provided, this method will provision the new physical
	// collection in the database.
	AddSchemaVersion(ctx context.Context, name, version string, schema *schema.SchemaDefinition, physicalName ...string) (*RegistryEntry, error)

	// SetActiveVersion changes the active schema version for a collection.
	SetActiveVersion(ctx context.Context, name, version string) (*RegistryEntry, error)

	// List retrieves the registry entries for all registered collections.
	List(ctx context.Context) ([]*RegistryEntry, error)


	// ResolveName returns the physical name of a schema
	// If no version is provided, it returns the currently active schema version.
	ResolvePhysicalName(ctx context.Context, name string, version ...string) (string, error)
}
