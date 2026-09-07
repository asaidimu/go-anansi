package base

import (
        "context"
        "sync"

        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/query"
        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// SchemaVersionRecord represents a historical version of a collection's schema and its physical name.
type SchemaVersionRecord struct {
        Physical string            `json:"physical"` // The physical name of the collection in the database for this version.
        Schema   definition.Schema `json:"schema"`   // The full schema definition for this version.

        // View, when non-nil, marks this version as a view-backed collection.
        // The stored Query is the read-only definition of the view; the Schema
        // field carries a derived (best-effort) result schema produced from
        // the query's projection. View-backed collections reject all write
        // operations (Create/Update/Delete/Validate).
        View *query.Query `json:"view,omitempty"`

        validatorOnce sync.Once
        validator     *definition.DocumentValidator
}

// IsView reports whether this version is a view-backed (read-only) collection.
func (r *SchemaVersionRecord) IsView() bool {
        return r != nil && r.View != nil
}

// Validator returns the lazily-built DocumentValidator for this schema version.
// It is built at most once per SchemaVersionRecord and cached thereafter.
//
// For view-backed versions the validator is a no-op (always succeeds)
// because views are read-only and validation only runs on the write path.
func (r *SchemaVersionRecord) Validator() (*definition.DocumentValidator, error) {
        r.validatorOnce.Do(func() {
                if r.IsView() {
                        // Views are read-only — never reached on the write path,
                        // but we synthesize a trivially-passing validator so
                        // CurrentValidator callers don't panic.
                        r.validator, _ = definition.NewDocumentValidator(&r.Schema, nil)
                        return
                }
                r.validator, _ = definition.NewDocumentValidator(&r.Schema, nil)
        })
        return r.validator, nil
}

type RegistryEntry struct {
        Name          string                      `json:"name"`                  // The name of the collection this schema defines.
        Description   string                      `json:"description,omitempty"` // A human-readable description of the schema.
        ActiveVersion *common.Version             `json:"version"`               // The current active version of the schema, pointing to an entry in 'Versions'.
        Versions      map[string]*SchemaVersionRecord `json:"versions,omitempty"`    // A map of all schema versions, keyed by version string.
        // TODO: Watch for this should you ever decide to change the name of the
        // metadata field
        Metadata      map[string]any              `json:"_metadata_,omitempty"`    // A map of all schema versions, keyed by version string.
}

// IsView reports whether the active version of this entry is a view-backed
// (read-only) collection.
func (e *RegistryEntry) IsView() bool {
        if e == nil || e.ActiveVersion == nil {
                return false
        }
        if v, ok := e.Versions[e.ActiveVersion.String()]; ok {
                return v.IsView()
        }
        return false
}

// SchemaProvider is the interface collection layers use to resolve the active schema
// and its compiled validator without holding a direct schema pointer. Two
// implementations exist:
//
//   - RegistrySchemaProvider — delegates to CollectionRegistry for live resolution.
//   - StaticSchemaProvider  — returns a fixed schema (used for bootstrapping the
//     registry collection before the registry itself exists).
//
// The interface also exposes IsView/CurrentView so collection decorators
// can route reads to the stored view query and reject writes on views.
type SchemaProvider interface {
        CurrentSchema(ctx context.Context) (*definition.Schema, error)
        CurrentValidator(ctx context.Context) (*definition.DocumentValidator, error)
        PhysicalName(ctx context.Context) (string, error)

        // IsView reports whether this provider backs a read-only view
        // collection. When true, CurrentView returns the stored view query
        // and write operations must be rejected by the collection layer.
        IsView() bool

        // CurrentView returns the stored view query for a view-backed
        // collection. Returns (nil, nil) for schema-backed collections.
        CurrentView(ctx context.Context) (*query.Query, error)
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
        CreateCollection(ctx context.Context, schema *definition.Schema) (*RegistryEntry, error)

        // CreateCollections creates multiple collections atomically in a single transaction.
        // All collections are validated upfront - if any collection fails validation,
        // the entire operation fails without creating any collections. Each collection
        // gets its first version ("1.0.0"), is marked as active, and has its underlying
        // physical collection provisioned in the database.
        CreateCollections(ctx context.Context, schemas []*definition.Schema) ([]*RegistryEntry, error)

        // CreateView registers a read-only view collection backed by the given query
        // rather than a physical table. The view's result schema is derived from the
        // query's projection via query.SchemaFromQuery and cached on the version
        // record. No DDL is issued against the database; the view's data is fetched
        // by composing the stored query with the user's read query at runtime.
        CreateView(ctx context.Context, name string, view *query.Query) (*RegistryEntry, error)

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
        // Callers should treat the returned schema as immutable.
        GetSchema(ctx context.Context, name string, version ...string) (*definition.Schema, error)

        // CurrentSchema returns the active schema for the named collection.
        // Unlike GetSchema, this returns a shared reference (not a deep copy) and is
        // suitable for high-frequency reads where the caller does not mutate the result.
        CurrentSchema(ctx context.Context, name string) (*definition.Schema, error)

        // CurrentValidator returns the lazily-built DocumentValidator for the active
        // schema version. The validator is cached on the version record and rebuilt
        // only when a new version is added.
        CurrentValidator(ctx context.Context, name string) (*definition.DocumentValidator, error)

        // CurrentView returns the stored view query for the active version of a
        // view-backed collection. Returns (nil, nil) for schema-backed collections.
        CurrentView(ctx context.Context, name string) (*query.Query, error)

        // GetRegistryEntry retrieves the complete management record for a collection.
        GetRegistryEntry(ctx context.Context, name string) (*RegistryEntry, error)

        // AddSchemaVersion introduces a new version of a schema for an existing collection.
        // If a new physicalName is provided, this method will provision the new physical
        // collection in the database.
        AddSchemaVersion(ctx context.Context, name, version string, schema *definition.Schema, physicalName ...string) (*RegistryEntry, error)

        // SetActiveVersion changes the active schema version for a collection.
        SetActiveVersion(ctx context.Context, name, version string) (*RegistryEntry, error)

        // List retrieves the registry entries for all registered collections.
        List(ctx context.Context) ([]*RegistryEntry, error)

        // ResolveName returns the physical name of a schema
        // If no version is provided, it returns the currently active schema version.
        ResolvePhysicalName(ctx context.Context, name string, version ...string) (string, error)

        // Close stops background goroutines (e.g. cache janitor/evictor) and
        // releases resources held by the registry.
        Close(ctx context.Context) error
}
