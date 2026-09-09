package registry

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "time"

        "github.com/asaidimu/go-anansi/v8/core/cache"
        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/data"
        "github.com/asaidimu/go-anansi/v8/core/persistence/base"
        "github.com/asaidimu/go-anansi/v8/core/persistence/transaction"
        "github.com/asaidimu/go-anansi/v8/core/query"
        "github.com/asaidimu/go-anansi/v8/core/schema"
        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
        "go.uber.org/zap"
)

type RegistryEntry = base.RegistryEntry
type SchemaVersionRecord = base.SchemaVersionRecord

// RegistryExecutor defines a function that executes registry operations,
// optionally within a database transaction.
type RegistryExecutor func(ctx context.Context, transaction bool,
        fn func(ctx context.Context, collection base.Collection, manager query.SchemaManager) (any, error),
) (any, error)

// collectionRegistry implements base.CollectionRegistry using a transactional
// executor and a bounded, sharded, TTL-aware cache.
type collectionRegistry struct {
        executor  RegistryExecutor
        logger    *zap.Logger
        cache     cache.RepositoryCache[*RegistryEntry]
        validator *definition.DocumentValidator
}

var _ base.CollectionRegistry = (*collectionRegistry)(nil)

// NewCollectionRegistry creates a new registry, bootstrapping the _schemas_
// collection if needed. The cache is lazily populated via read-through on
// GetRegistryEntry; no full warm-up is performed.
func NewCollectionRegistry(executor RegistryExecutor, logger *zap.Logger, cacheConfig ...cache.CacheConfig) (base.CollectionRegistry, error) {
        // Bootstrap the registry collection
        _, err := executor(context.Background(), false, func(ctx context.Context, collection base.Collection, manager query.SchemaManager) (any, error) {
                exists, err := manager.CollectionExists(ctx, REGISTRY_COLLECTION_NAME)
                if err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_CHECK_REGISTRY_EXISTENCE")
                }

                if !exists {
                        registrySchema := RegistrySchema()
                        if err := manager.CreateCollection(ctx, *registrySchema); err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_COLLECTION", fmt.Sprintf("'_schemas_': %v", ErrFailedToCreateRegistryCollection))
                        }
                }

                return nil, nil
        })

        if err != nil {
                return nil, err
        }

        cfg := cache.DefaultCacheConfig()
        if len(cacheConfig) > 0 {
                cfg = cacheConfig[0]
        }
        // Registry entries do not expire by TTL — eviction is capacity-only.
        cfg.PositiveTTL = 0
        cfg.NegativeTTL = 30 * time.Second

        c := cache.NewManagedCache[*RegistryEntry](cfg, func(e *RegistryEntry) (*RegistryEntry, error) {
                return deepCopyEntry(e), nil
        })

        validator := schema.SchemaValidator()
        registry := &collectionRegistry{
                executor:  executor,
                logger:    logger,
                cache:     c,
                validator: validator,
        }

        return registry, nil
}

// Close shuts down the background cache goroutines and releases resources.
func (r *collectionRegistry) Close(ctx context.Context) error {
        return r.cache.Close()
}

func (r *collectionRegistry) CreateCollection(ctx context.Context, sc *definition.Schema) (*RegistryEntry, error) {
        results, err := r.CreateCollections(ctx, []*definition.Schema{sc})
        if err != nil {
                return nil, err
        }
        return results[0], nil
}

// CreateView registers a read-only view collection backed by the given query.
//
// When materialized is false, the view is virtual: no physical table is
// created and reads compose the stored query with the user's query at
// runtime. This is the cheapest option but re-runs the view's query
// (including any joins) on every read.
//
// When materialized is true, the view is materialized: a physical table
// is created via CREATE TABLE <name> AS SELECT ... and populated by
// executing the view's SELECT. The view's indexes (declared on the
// derived schema) are created against the materialized table. Reads
// target the materialized table directly (no query composition);
// RefreshView re-populates it on demand.
//
// The view's result schema is derived from the query's projection via
// query.SchemaFromQuery and cached on the version record.
func (r *collectionRegistry) CreateView(ctx context.Context, name string, view *query.Query, materialized bool) (*RegistryEntry, error) {
        if name == "" {
                return nil, common.NewSystemError("ERR_REGISTRY_VIEW_NAME_REQUIRED", "view name is required")
        }
        if view == nil {
                return nil, common.NewSystemError("ERR_REGISTRY_VIEW_QUERY_REQUIRED", "view query is required")
        }
        if view.Target == nil || view.Target.Schema == nil {
                return nil, common.NewSystemError("ERR_REGISTRY_VIEW_TARGET_REQUIRED",
                        "view query must specify Target with a Schema so the result schema can be derived")
        }

        // Reject duplicate registration.
        if _, err := r.GetRegistryEntry(ctx, name); err == nil {
                return nil, base.ErrCollectionAlreadyExists.WithMessage(fmt.Sprintf("collection '%s' already exists", name))
        } else if !errors.Is(err, base.ErrCollectionNotFound) {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_CHECK_REGISTRY_EXISTENCE")
        }

        // Derive the view's result schema. SchemaFromQuery needs a target
        // schema; the view query must carry one (callers set view.Target.Schema
        // to the underlying collection's schema before calling CreateView).
        //
        // SchemaFromQuery has a fast path that returns q.Target.Schema directly
        // (no copy) when there are no joins/projection. We deep-copy the result
        // BEFORE mutating it so we don't corrupt the caller's original schema.
        derivedFromQuery, err := query.SchemaFromQuery(view, nil)
        if err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_SCHEMA_DERIVATION_FAILED",
                        fmt.Sprintf("failed to derive result schema for view '%s'", name))
        }
        derivedSchema := derivedFromQuery.DeepCopy()

        // Re-stamp the derived schema with the view's logical name so callers
        // see the view's name when introspecting CurrentSchema.
        derivedSchema.Name = name
        derivedSchema.Description = fmt.Sprintf("Derived schema for view '%s'", name)

        // Strip system fields (_id_, _metadata_) from the derived schema.
        // Materialized views don't have system columns (CTAS only produces
        // the columns from the SELECT's projection); virtual views don't
        // need them either (the composed query runs against the underlying
        // collection which has its own system fields). Carrying system fields
        // on the derived schema would cause the read path to reference
        // non-existent columns.
        if derivedSchema.Fields != nil {
                for fid, f := range derivedSchema.Fields {
                        // Strip system fields (_id_, _metadata_).
                        if string(f.Name) == data.DocumentIDField || string(f.Name) == data.MetadataField {
                                delete(derivedSchema.Fields, fid)
                                continue
                        }
                        // Strip join-added nested-schema fields (FieldTypeObject
                        // with a schema reference). These are artifacts of
                        // SchemaFromQuery's applyJoinsToSchema and don't
                        // correspond to real columns in the materialized table.
                        // Keeping them causes schema validation failures (duplicate
                        // field IDs across the root and nested schemas) and
                        // "no such column" errors at read time.
                        if f.Type == definition.FieldTypeObject && !f.Schema.IsZero() {
                                delete(derivedSchema.Fields, fid)
                        }
                }
        }
        // Clear nested schemas entirely — the materialized table is a flat
        // table with only scalar columns from the main target.
        derivedSchema.Schemas = nil

        // Drop any indexes the derived schema inherited from the target schema
        // that reference fields no longer present after projection. For virtual
        // views this avoids stale metadata; for materialized views it ensures
        // we only create physical indexes against columns that actually exist
        // in the materialized table.
        if derivedSchema.Indexes != nil {
                fieldNames := make(map[string]struct{}, len(derivedSchema.Fields))
                for _, f := range derivedSchema.Fields {
                        fieldNames[string(f.Name)] = struct{}{}
                }
                for idxID, idx := range derivedSchema.Indexes {
                        keep := true
                        for _, f := range idx.Fields {
                                if _, ok := fieldNames[string(f)]; !ok {
                                        keep = false
                                        break
                                }
                        }
                        if !keep {
                                delete(derivedSchema.Indexes, idxID)
                        }
                }
                if len(derivedSchema.Indexes) == 0 {
                        derivedSchema.Indexes = nil
                }
        }

        // Validate the derived schema — it must be self-consistent before we
        // store it. Compile() exercises the schema IR and is sufficient to
        // catch structurally invalid schemas.
        if _, err := definition.Compile(derivedSchema); err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_DERIVED_SCHEMA_INVALID",
                        fmt.Sprintf("derived schema for view '%s' is invalid: %v", name, err))
        }

        // Generate a physical name for the materialized view's table when
        // materialized. Virtual views leave Physical empty.
        var physicalName string
        if materialized {
                // Reuse the registry's physical-name generator so materialized
                // view tables follow the same naming convention as collections.
                tempSc := &definition.Schema{
                        BaseSchema: definition.BaseSchema{Name: name},
                        Version:    common.MustNewVersion("1.0.0"),
                }
                physicalName, err = generatePhysicalName(tempSc)
                if err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_GENERATE_PHYSICAL_NAME",
                                fmt.Sprintf("for materialized view '%s'", name))
                }
        }

        version := common.MustNewVersion("1.0.0")
        entry := &RegistryEntry{
                Name:          name,
                Description:   fmt.Sprintf("View collection backed by a stored query"),
                ActiveVersion: version,
                Versions: map[string]*SchemaVersionRecord{
                        version.String(): {
                                Physical:     physicalName,
                                Schema:       *derivedSchema,
                                View:         view,
                                Materialized: materialized,
                        },
                },
        }

        // Persist the registry entry AND issue DDL for materialized views.
        requiresTransaction := true
        if _, ok := transaction.GetCurrentTransaction(ctx); ok {
                requiresTransaction = false
        }
        persisted, err := execute(ctx, r.executor, requiresTransaction, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
                // For materialized views, issue CREATE TABLE AS SELECT and
                // create the view's indexes against the materialized table.
                // The CTAS needs the view's query to reference physical names
                // of underlying collections — we resolve them via the
                // registry before issuing the DDL.
                if materialized {
                        resolvedView, rerr := r.resolveViewQueryPhysicalNames(tctx, view, derivedSchema)
                        if rerr != nil {
                                return nil, rerr
                        }
                        if err := manager.CreateView(tctx, physicalName, resolvedView); err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_MATERIALIZATION_FAILED",
                                        fmt.Sprintf("failed to materialize view '%s'", name))
                        }
                        // Create the view's indexes against the materialized table.
                        for _, idx := range derivedSchema.Indexes {
                                if idx.Type == definition.IndexTypePrimary {
                                        continue
                                }
                                // FTS indexes on materialized views are created via
                                // CreateIndex (the createIndexTree builder emits the
                                // FTS5 virtual table + triggers against the target
                                // collection name). We don't skip them here because
                                // the materialized table doesn't go through
                                // createTableTree's inline FTS emission.
                                if err := manager.CreateIndex(tctx, physicalName, idx); err != nil {
                                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_INDEX_CREATION_FAILED",
                                                fmt.Sprintf("failed to create index '%s' on materialized view '%s'", idx.Name, name))
                                }
                        }
                }

                return r.persistRegistryEntry(tctx, collection, entry)
        })
        if err != nil {
                return nil, err
        }
        result := persisted
        r.cache.Set(name, result)
        return result, nil
}

// RefreshView re-populates a materialized view's physical table by dropping
// and re-creating it from the stored SELECT. Returns an error if the named
// collection is not a materialized view.
func (r *collectionRegistry) RefreshView(ctx context.Context, name string) (*RegistryEntry, error) {
        entry, err := r.GetRegistryEntry(ctx, name)
        if err != nil {
                return nil, err
        }
        if !entry.IsMaterialized() {
                return nil, base.ErrNotMaterialized.WithMessage(fmt.Sprintf("collection '%s' is not a materialized view", name))
        }

        activeStr := entry.ActiveVersion.String()
        versionRecord, ok := entry.Versions[activeStr]
        if !ok {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION",
                        fmt.Sprintf("active version '%s' not found for collection '%s'", activeStr, name))
        }

        // Resolve the view's query to physical names of underlying collections
        // before issuing the refresh DDL.
        resolvedView, err := r.resolveViewQueryPhysicalNames(ctx, versionRecord.View, &versionRecord.Schema)
        if err != nil {
                return nil, err
        }

        requiresTransaction := true
        if _, ok := transaction.GetCurrentTransaction(ctx); ok {
                requiresTransaction = false
        }
        _, err = execute(ctx, r.executor, requiresTransaction, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (any, error) {
                if err := manager.RefreshView(tctx, versionRecord.Physical, resolvedView); err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_REFRESH_FAILED",
                                fmt.Sprintf("failed to refresh materialized view '%s'", name))
                }
                // Re-create the view's indexes against the freshly-populated table.
                for _, idx := range versionRecord.Schema.Indexes {
                        if idx.Type == definition.IndexTypePrimary {
                                continue
                        }
                        if err := manager.CreateIndex(tctx, versionRecord.Physical, idx); err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_INDEX_RECREATION_FAILED",
                                        fmt.Sprintf("failed to recreate index '%s' on materialized view '%s'", idx.Name, name))
                        }
                }
                return &RegistryEntry{}, nil // non-nil placeholder so execute's type assertion succeeds
        })
        if err != nil {
                return nil, err
        }

        return entry, nil
}

// resolveViewQueryPhysicalNames returns a deep copy of the view's stored query
// with all logical collection names (the main target and any join targets)
// rewritten to their physical names. This is required before issuing CTAS
// DDL because the emitted SQL references physical table names directly.
//
// The view's stored query carries the underlying collection's logical name
// in view.Target.Name; joins carry each join target's logical name in
// join.Target.Name. We look each up via the registry and rewrite.
//
// The derivedSchema parameter is set as the resolved query's Target.Schema
// so the CTAS builder can derive the correct column alias list (the view's
// derived schema has system fields stripped and carries only the fields
// the materialized table should have).
func (r *collectionRegistry) resolveViewQueryPhysicalNames(ctx context.Context, view *query.Query, derivedSchema *definition.Schema) (*query.Query, error) {
        if view == nil {
                return nil, nil
        }

        // Deep-copy the view via Clone (not JSON roundtrip — JSON roundtrip
        // corrupts FilterValue's tagged union because FieldRefVal serializes
        // to {"field":"...","type":""} which unmarshals into ObjectVal instead
        // of FieldRefVal).
        resolved, err := view.Clone()
        if err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_CLONE_FAILED")
        }

        // Resolve the main target's logical → physical name.
        if resolved.Target != nil && resolved.Target.Name != "" {
                originalLogical := resolved.Target.Name
                physName, _, err := r.resolveUnderlyingPhysicalName(ctx, originalLogical)
                if err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_TARGET_RESOLUTION_FAILED",
                                fmt.Sprintf("failed to resolve underlying collection '%s' for view", originalLogical))
                }
                resolved.Target.Name = physName
                // Set the alias to the original logical name so SQL
                // field references can use "LogicalName.field" instead
                // of the physical name.
                alias := originalLogical
                resolved.Target.Alias = &alias
                // Override the schema with the view's derived schema so the
                // CTAS builder can derive the correct column alias list.
                if derivedSchema != nil {
                        resolved.Target.Schema = derivedSchema
                }
        }

        // Add a projection with explicit column aliases so the CTAS
        // produces a materialized table with plain (un-qualified)
        // column names. Without this, CREATE TABLE foo AS SELECT * FROM
        // bar produces columns named "bar.col1", "bar.col2".
        //
        // For join queries, each field must be qualified with the main
        // target's alias to avoid "ambiguous column name" errors (both
        // the main table and join targets may have columns with the same
        // name, e.g. "id").
        if derivedSchema != nil && len(derivedSchema.Fields) > 0 {
                // Determine the alias to qualify fields with.
                mainAlias := ""
                if resolved.Target != nil {
                        if resolved.Target.Alias != nil {
                                mainAlias = *resolved.Target.Alias
                        } else {
                                mainAlias = resolved.Target.Name
                        }
                }
                fieldNames := derivedSchema.FieldNames()
                projFields := make([]query.ProjectionField, 0, len(fieldNames))
                for _, fn := range fieldNames {
                        // Skip fields that are nested-schema references
                        // (added by SchemaFromQuery's applyJoinsToSchema).
                        // These are FieldTypeObject fields pointing to
                        // nested schemas — they don't correspond to real
                        // columns in the underlying table and would cause
                        // "no such column" errors if included in the
                        // CTAS projection.
                        _, field := derivedSchema.FindField(fn)
                        if field != nil && field.Type == definition.FieldTypeObject && !field.Schema.IsZero() {
                                continue
                        }
                        // For join queries, qualify the field with the
                        // main target's alias to avoid "ambiguous column
                        // name" errors.
                        qualifiedName := fn
                        if mainAlias != "" && len(resolved.Joins) > 0 {
                                qualifiedName = mainAlias + "." + fn
                        }
                        projFields = append(projFields, query.ProjectionField{
                                Name:  qualifiedName,
                        })
                }
                resolved.Projection = &query.ProjectionConfiguration{
                        Include: projFields,
                }
        }

        // Resolve each join target's logical → physical name.
        for i := range resolved.Joins {
                if resolved.Joins[i].Target.Name != "" {
                        originalJoinLogical := resolved.Joins[i].Target.Name
                        physName, joinSchema, err := r.resolveUnderlyingPhysicalName(ctx, originalJoinLogical)
                        if err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_VIEW_JOIN_RESOLUTION_FAILED",
                                        fmt.Sprintf("failed to resolve join target '%s' for view", originalJoinLogical))
                        }
                        resolved.Joins[i].Target.Name = physName
                        // Set the alias to the original logical name so
                        // field references like "Profiles.user_id" in the
                        // ON clause resolve correctly (buildSelectTree
                        // registers the schema under the alias).
                        alias := originalJoinLogical
                        resolved.Joins[i].Target.Alias = &alias
                        if joinSchema != nil {
                                resolved.Joins[i].Target.Schema = joinSchema
                        }
                }
        }

        return resolved, nil
}

// resolveUnderlyingPhysicalName looks up a collection by its logical name
// and returns (physicalName, schema, nil). Used by resolveViewQueryPhysicalNames
// to rewrite the view's stored query before issuing CTAS DDL.
func (r *collectionRegistry) resolveUnderlyingPhysicalName(ctx context.Context, logicalName string) (string, *definition.Schema, error) {
        sc, err := r.GetSchema(ctx, logicalName)
        if err != nil {
                return "", nil, err
        }
        return sc.Name, sc, nil
}

// CurrentView returns the stored view query for the active version of a
// view-backed collection. Returns (nil, nil) for schema-backed collections.
func (r *collectionRegistry) CurrentView(ctx context.Context, name string) (*query.Query, error) {
        entry, status := r.cache.GetStatus(name)
        switch status {
        case cache.CacheHitNegative:
                return nil, base.ErrCollectionNotFound
        case cache.CacheMiss:
                var err error
                entry, err = r.loadFromDatabase(ctx, name)
                if err != nil {
                        if errors.Is(err, base.ErrCollectionNotFound) {
                                r.cache.Nullify(name)
                        }
                        return nil, err
                }
                r.cache.Set(name, entry)
        }

        activeStr := entry.ActiveVersion.String()
        versionRecord, ok := entry.Versions[activeStr]
        if !ok {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION",
                        fmt.Sprintf("active version '%s' not found for collection '%s'", activeStr, name))
        }
        if !versionRecord.IsView() {
                return nil, nil
        }
        return versionRecord.View, nil
}

// CreateCollections creates collections atomically in a single transaction.
func (r *collectionRegistry) CreateCollections(ctx context.Context, schemas []*definition.Schema) ([]*RegistryEntry, error) {
        if len(schemas) == 0 {
                return []*RegistryEntry{}, nil
        }

        validSchemas := make(map[string]*definition.Schema)
        for _, sc := range schemas {
                schemaKey := fmt.Sprintf("%s@%s", sc.Name, sc.Version.String())

                if _, ok := validSchemas[schemaKey]; ok {
                        return nil, common.NewSystemError("ERR_REGISTRY_DUPLICATE_SCHEMA_IN_BATCH", fmt.Sprintf("duplicate schema in batch: '%s' version '%s'", sc.Name, sc.Version.String()))
                }

                if issues, err := schema.ValidateSchema(sc); err != nil {
                        return nil, ErrInvalidSchema.WithIssues(issues)
                }

                enrichedSchema, err := EnrichSchema(sc)
                if err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_INVALID_SCHEMA", fmt.Sprintf("invalid schema '%s' v%s", sc.Name, sc.Version.String()))
                }

                // Check if collection already exists
                if _, err := r.GetRegistryEntry(ctx, sc.Name); err == nil {
                        return nil, base.ErrCollectionAlreadyExists.WithMessage(fmt.Sprintf("collection '%s' already exists", sc.Name))
                } else if !errors.Is(err, base.ErrCollectionNotFound) {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_CHECK_REGISTRY_EXISTENCE")
                }
                validSchemas[schemaKey] = enrichedSchema
        }

        // Execute in transaction
        requiresTransaction := true
        if _, ok := transaction.GetCurrentTransaction(ctx); ok {
                requiresTransaction = false
        }

        results, err := execute(ctx, r.executor, requiresTransaction, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) ([]*RegistryEntry, error) {
                var createdEntries []*RegistryEntry

                for _, sc := range validSchemas {
                        physicalName, err := generatePhysicalName(sc)
                        if err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_GENERATE_PHYSICAL_NAME", fmt.Sprintf("for '%s' v%s", sc.Name, sc.Version.String()))
                        }

                        tempSchema := sc.DeepCopy()
                        tempSchema.Name = physicalName

                        if err := manager.CreateCollection(tctx, *tempSchema); err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_COLLECTION_CREATION_FAILED", fmt.Sprintf("failed to create physical collection '%s'", physicalName))
                        }

                        entry := &RegistryEntry{
                                Name:          sc.Name,
                                Description:   sc.Description,
                                ActiveVersion: sc.Version,
                                Versions: map[string]*SchemaVersionRecord{
                                        sc.Version.String(): {
                                                Physical: physicalName,
                                                Schema:   *tempSchema,
                                        },
                                },
                        }

                        result, err := r.persistRegistryEntry(tctx, collection, entry)
                        if err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_PERSIST_REGISTRY_ENTRY", fmt.Sprintf("for '%s'", entry.Name))
                        }
                        createdEntries = append(createdEntries, result)
                }

                return createdEntries, nil
        })
        if err != nil {
                return nil, err
        }

        // Update cache after successful completion
        for _, entry := range results {
                r.cache.Set(entry.Name, entry)
        }

        return results, nil
}

// GetRegistryEntry retrieves a registry entry with read-through caching.
// On cache miss, it queries _schemas_ from the database, populates the cache,
// and returns a deep copy. Non-existent collections are negative-cached.
func (r *collectionRegistry) GetRegistryEntry(ctx context.Context, name string) (*RegistryEntry, error) {
        val, status := r.cache.GetStatus(name)
        switch status {
        case cache.CacheHitPositive:
                return deepCopyEntry(val), nil
        case cache.CacheHitNegative:
                return nil, base.ErrCollectionNotFound
        }

        // Read-through from database
        entry, err := r.loadFromDatabase(ctx, name)
        if err != nil {
                if errors.Is(err, base.ErrCollectionNotFound) {
                        r.cache.Nullify(name)
                }
                return nil, err
        }

        r.cache.Set(name, entry)
        return deepCopyEntry(entry), nil
}

// GetSchema resolves a schema by name and optional version.
func (r *collectionRegistry) GetSchema(ctx context.Context, name string, version ...string) (*definition.Schema, error) {
        entry, err := r.GetRegistryEntry(ctx, name)
        if err != nil {
                return nil, err
        }

        resolvedVersion := entry.ActiveVersion.String()
        if len(version) > 0 {
                resolvedVersion = version[0]
        }

        versionRecord, ok := entry.Versions[resolvedVersion]
        if !ok {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", fmt.Sprintf("version '%s' not found for collection '%s'", resolvedVersion, name))
        }

        clone := versionRecord.Schema.DeepCopy()
        return clone, nil
}

// CurrentSchema returns a shared reference to the active schema from the
// cached registry entry (not a deep copy). The caller must not mutate the
// returned schema.
func (r *collectionRegistry) CurrentSchema(ctx context.Context, name string) (*definition.Schema, error) {
        entry, status := r.cache.GetStatus(name)
        switch status {
        case cache.CacheHitNegative:
                return nil, base.ErrCollectionNotFound
        case cache.CacheMiss:
                // Read-through: fetch from database, caches on success
                var err error
                entry, err = r.loadFromDatabase(ctx, name)
                if err != nil {
                        if errors.Is(err, base.ErrCollectionNotFound) {
                                r.cache.Nullify(name)
                        }
                        return nil, err
                }
                r.cache.Set(name, entry)
        }

        activeStr := entry.ActiveVersion.String()
        versionRecord, ok := entry.Versions[activeStr]
        if !ok {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", fmt.Sprintf("active version '%s' not found for collection '%s'", activeStr, name))
        }

        return &versionRecord.Schema, nil
}

// CurrentValidator returns the lazily-built DocumentValidator for the active
// schema version. The validator is cached on the cached *SchemaVersionRecord
// via sync.Once and reused across callers.
func (r *collectionRegistry) CurrentValidator(ctx context.Context, name string) (*definition.DocumentValidator, error) {
        entry, status := r.cache.GetStatus(name)
        switch status {
        case cache.CacheHitNegative:
                return nil, base.ErrCollectionNotFound
        case cache.CacheMiss:
                var err error
                entry, err = r.loadFromDatabase(ctx, name)
                if err != nil {
                        if errors.Is(err, base.ErrCollectionNotFound) {
                                r.cache.Nullify(name)
                        }
                        return nil, err
                }
                r.cache.Set(name, entry)
        }

        activeStr := entry.ActiveVersion.String()
        versionRecord, ok := entry.Versions[activeStr]
        if !ok {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", fmt.Sprintf("active version '%s' not found for collection '%s'", activeStr, name))
        }

        return versionRecord.Validator()
}

// ResolvePhysicalName returns the physical name for a collection, optionally for a specific version.
func (r *collectionRegistry) ResolvePhysicalName(ctx context.Context, name string, version ...string) (string, error) {
        // For materialized views, the physical name is stored on the version
        // record's Physical field — the schema's Name field carries the view's
        // logical name (set by CreateView), not the physical table name.
        entry, err := r.GetRegistryEntry(ctx, name)
        if err != nil {
                return "", err
        }
        resolvedVersion := entry.ActiveVersion.String()
        if len(version) > 0 {
                resolvedVersion = version[0]
        }
        vr, ok := entry.Versions[resolvedVersion]
        if !ok {
                return "", common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION",
                        fmt.Sprintf("version '%s' not found for collection '%s'", resolvedVersion, name))
        }
        if vr.IsMaterialized() && vr.Physical != "" {
                return vr.Physical, nil
        }
        // Non-materialized: fall back to the schema's Name field (which the
        // registry rewrote to the physical name during CreateCollection).
        return vr.Schema.Name, nil
}

// AddSchemaVersion adds a new schema version to an existing collection.
func (r *collectionRegistry) AddSchemaVersion(ctx context.Context, name, version string, sc *definition.Schema, physicalName ...string) (*RegistryEntry, error) {
        issues, ok := r.validator.Validate(sc.AsMap())
        if !ok {
                return nil, ErrInvalidSchema.WithIssues(issues)
        }
        enrichedSchema, err := EnrichSchema(sc)
        if err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_INVALID_SCHEMA", fmt.Sprintf("Invalid schema : %v", err))
        }

        entry, err := r.GetRegistryEntry(ctx, name)
        if err != nil {
                return nil, err
        }

        if _, exists := entry.Versions[version]; exists {
                return nil, base.ErrVersionAlreadyExists.WithMessage(fmt.Sprintf("version '%s' for collection '%s'", version, name))
        }

        actualPhysicalName := ""
        if len(physicalName) > 0 {
                actualPhysicalName = physicalName[0]
        } else {
                actualPhysicalName, err = generatePhysicalName(sc)
                if err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_GENERATE_PHYSICAL_NAME", fmt.Sprintf("for '%s v%s'", sc.Name, sc.Version.String()))
                }
        }

        updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
                tempSchema := enrichedSchema.DeepCopy()
                tempSchema.Name = actualPhysicalName

                // Only create the physical collection when the caller did not provide
                // an explicit physicalName (i.e., the collection was generated fresh).
                // When physicalName IS provided, the physical collection already exists
                // because the caller applied DDL in-place.
                if len(physicalName) == 0 {
                        if err := manager.CreateCollection(tctx, *tempSchema); err != nil {
                                return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_COLLECTION_CREATION_FAILED", fmt.Sprintf("failed to create physical collection '%s'", actualPhysicalName))
                        }
                }

                entry.Versions[version] = &SchemaVersionRecord{
                        Physical: actualPhysicalName,
                        Schema:   *tempSchema,
                }

                if err := r.updateRegistryEntry(tctx, collection, name, entry); err != nil {
                        return nil, err
                }

                return entry, nil
        })

        if err != nil {
                return nil, err
        }

        r.cache.Set(name, updatedEntry)
        return updatedEntry, nil
}

// SetActiveVersion changes the active schema version for a collection.
func (r *collectionRegistry) SetActiveVersion(ctx context.Context, name, version string) (*RegistryEntry, error) {
        entry, err := r.GetRegistryEntry(ctx, name)
        if err != nil {
                return nil, err
        }

        if entry.ActiveVersion.String() == version {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_ALREADY_ACTIVE", fmt.Sprintf("version '%s' for collection '%s' is already active", version, name))
        }

        if _, ok := entry.Versions[version]; !ok {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", fmt.Sprintf("version '%s' not found for collection '%s'", version, name))
        }

        parsedVersion, err := common.NewVersion(version)
        if err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_INVALID_VERSION_FORMAT")
        }

        updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
                entry.ActiveVersion = parsedVersion
                if err := r.updateRegistryEntry(tctx, collection, name, entry); err != nil {
                        return nil, err
                }
                return entry, nil
        })

        if err != nil {
                return nil, err
        }

        r.cache.Set(name, updatedEntry)
        return updatedEntry, nil
}

// DropCollection removes a collection from the registry, optionally dropping physical data.
func (r *collectionRegistry) DropCollection(ctx context.Context, name string, opts base.DropCollectionOptions) error {
        entry, err := r.GetRegistryEntry(ctx, name)
        if err != nil {
                return err
        }

        _, err = execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (bool, error) {
                if opts.DeletePhysicalData {
                        for _, versionRecord := range entry.Versions {
                                if err := manager.DropCollection(ctx, versionRecord.Physical); err != nil {
                                        return false, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_DROP_PHYSICAL_COLLECTION", fmt.Sprintf("failed to drop physical collection '%s'", versionRecord.Physical))
                                }
                        }
                }

                if err := r.deleteRegistryEntry(tctx, collection, name); err != nil {
                        return false, err
                }

                return true, nil
        })

        if err != nil {
                return err
        }

        r.cache.Evict(name)
        return nil
}

// PruneVersion removes a specific non-active version from a collection.
func (r *collectionRegistry) PruneVersion(ctx context.Context, name, version string) (*RegistryEntry, error) {
        entry, err := r.GetRegistryEntry(ctx, name)
        if err != nil {
                return nil, err
        }

        versionRecord, ok := entry.Versions[version]
        if !ok {
                return nil, common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", fmt.Sprintf("version '%s' not found for collection '%s'", version, name))
        }

        if entry.ActiveVersion.String() == version {
                return nil, common.NewSystemError("ERR_REGISTRY_CANNOT_PRUNE_ACTIVE_VERSION", fmt.Sprintf("version '%s' for collection '%s' is active", version, name))
        }

        updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
                if err := manager.DropCollection(ctx, versionRecord.Physical); err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_DROP_PHYSICAL_COLLECTION", fmt.Sprintf("failed to drop physical collection '%s'", versionRecord.Physical))
                }

                delete(entry.Versions, version)

                if err := r.updateRegistryEntry(tctx, collection, name, entry); err != nil {
                        return nil, err
                }

                return entry, nil
        })

        if err != nil {
                return nil, err
        }

        r.cache.Set(name, updatedEntry)
        return updatedEntry, nil
}

// List returns all registry entries by scanning the _schemas_ collection.
func (r *collectionRegistry) List(ctx context.Context) ([]*RegistryEntry, error) {
        // Full database scan — acceptable for administrative operations.
        allEntries, err := r.loadAllFromDatabase(ctx)
        if err != nil {
                return nil, base.ErrFailedToListCollections.WithCause(err)
        }

        // Warm cache with fresh data from the scan.
        for _, entry := range allEntries {
                r.cache.Set(entry.Name, entry)
        }

        return allEntries, nil
}

// ---------------------------------------------------------------------------
// Cache maintenance helpers
// ---------------------------------------------------------------------------

func (r *collectionRegistry) InvalidateCache(name string) {
        if name == "" {
                r.cache.Clear()
        } else {
                r.cache.Evict(name)
        }
}

// CacheStats returns current cache statistics for observability.
func (r *collectionRegistry) CacheStats() map[string]any {
        stats := r.cache.Stats()
        return map[string]any{
                "entries":          stats.Size,
                "positive_count":   stats.PositiveCount,
                "negative_count":   stats.NegativeCount,
                "hits":             stats.Hits,
                "misses":           stats.Misses,
                "negative_hits":    stats.NegativeHits,
                "evictions":        stats.Evictions,
                "expirations":      stats.Expirations,
                "evictor_active":   stats.EvictorActive,
        }
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func execute[T any](
        ctx context.Context,
        executor RegistryExecutor,
        requiresTransaction bool,
        fn func(ctx context.Context, collection base.Collection, manager query.SchemaManager) (T, error),
) (T, error) {
        result, err := executor(ctx, requiresTransaction, func(ctx context.Context, collection base.Collection, manager query.SchemaManager) (any, error) {
                return fn(ctx, collection, manager)
        })

        if err != nil {
                var zero T
                return zero, err
        }

        return result.(T), nil
}

func (r *collectionRegistry) loadFromDatabase(ctx context.Context, name string) (*RegistryEntry, error) {
        q := r.buildNameQuery(name)

        result, err := execute(ctx, r.executor, false, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*base.ReadResult, error) {
                return collection.Read(tctx, &q)
        })

        if err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_QUERY_REGISTRY_COLLECTION", fmt.Sprintf("failed to query registry for collection '%s'", name))
        }

        readResult := result
        if readResult.Count == 0 {
                return nil, base.ErrCollectionNotFound
        }

        if readResult.Count > 1 {
                return nil, base.ErrMultipleEntriesFound.WithMessage(fmt.Sprintf("multiple entries found for collection '%s'", name))
        }

        row := readResult.Data[0]
        return unmarshalEntry(row)
}

func (r *collectionRegistry) loadAllFromDatabase(ctx context.Context) ([]*RegistryEntry, error) {
        q := query.NewQueryBuilder().Build()

        result, err := execute(ctx, r.executor, false, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*base.ReadResult, error) {
                return collection.Read(tctx, &q)
        })

        if err != nil {
                return nil, base.ErrFailedToListCollections.WithCause(err)
        }

        readResult := result
        if readResult.Count == 0 {
                return []*RegistryEntry{}, nil
        }

        entries := make([]*RegistryEntry, 0, readResult.Count)

        rows := readResult.Data
        for _, row := range rows {
                entry, err := unmarshalEntry(row)
                if err != nil {
                        return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_UNMARSHAL_REGISTRY_ENTRY")
                }
                entries = append(entries, entry)
        }

        return entries, nil
}

func (r *collectionRegistry) entryToDocument(entry *RegistryEntry) (*data.Document, error) {
        entryBytes, err := json.Marshal(entry)
        if err != nil {
                return data.MustNewDocument(nil), common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_MARSHAL_REGISTRY_ENTRY")
        }

        var docData map[string]any
        if err := json.Unmarshal(entryBytes, &docData); err != nil {
                return data.MustNewDocument(nil), common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_UNMARSHAL_REGISTRY_ENTRY")
        }

        return data.MustNewDocument(docData), nil
}

func (r *collectionRegistry) persistRegistryEntry(ctx context.Context, collection base.Collection, entry *RegistryEntry) (*RegistryEntry, error) {
        doc, err := r.entryToDocument(entry)
        if err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_DOCUMENT")
        }

        result, err := collection.CreateOne(ctx, doc)
        if err != nil {
                if len(result.Issues) > 0 {
                        return nil, common.NewSystemError("ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_ENTRY_WITH_ISSUES", fmt.Sprintf("%v", result.Issues))
                }
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_ENTRY")
        }

        rentry, err := unmarshalEntry(result.Data)
        if err != nil {
                return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_ENTRY")
        }

        return rentry, nil
}

func (r *collectionRegistry) updateRegistryEntry(ctx context.Context, collection base.Collection, name string, entry *RegistryEntry) error {
        doc, err := r.entryToDocument(entry)
        if err != nil {
                return err
        }

        q := r.buildNameQuery(name)
        _, err = collection.Update(ctx, &base.CollectionUpdate{
                Filter: q.Filters,
                Set:    doc,
        })

        if err != nil {
                return common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_UPDATE_REGISTRY_ENTRY", fmt.Sprintf("for collection %s", name))
        }

        return nil
}

func (r *collectionRegistry) deleteRegistryEntry(ctx context.Context, collection base.Collection, name string) error {
        q := r.buildNameQuery(name)
        _, err := collection.Delete(ctx, q.Filters, false)
        if err != nil {
                return common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_DELETE_REGISTRY_ENTRY", fmt.Sprintf("for collection %s", name))
        }
        return nil
}

func (r *collectionRegistry) buildNameQuery(name string) query.Query {
        return query.NewQueryBuilder().From(REGISTRY_COLLECTION_NAME).Alias(REGISTRY_COLLECTION_NAME).
                Schema(RegistrySchema()).
                Where("name").Eq(name).Build()
}

// deepCopyEntry creates a complete, independent copy of a RegistryEntry to
// prevent callers from mutating the cached copy.
func deepCopyEntry(src *RegistryEntry) *RegistryEntry {
        if src == nil {
                return nil
        }

        versions := make(map[string]*SchemaVersionRecord, len(src.Versions))
        for k, v := range src.Versions {
                copied := &SchemaVersionRecord{
                        Physical:     v.Physical,
                        Schema:       *v.Schema.DeepCopy(),
                        Materialized: v.Materialized,
                }
                // Deep-copy the View query (if present) via JSON roundtrip so
                // the cached copy doesn't share pointers with the source.
                if v.View != nil {
                        viewBytes, err := json.Marshal(v.View)
                        if err == nil {
                                var viewCopy query.Query
                                if err := json.Unmarshal(viewBytes, &viewCopy); err == nil {
                                        copied.View = &viewCopy
                                }
                        }
                }
                versions[k] = copied
        }

        var activeVer *common.Version
        if src.ActiveVersion != nil {
                v := *src.ActiveVersion
                activeVer = &v
        }

        var meta map[string]any
        if src.Metadata != nil {
                meta = make(map[string]any, len(src.Metadata))
                for k, v := range src.Metadata {
                        meta[k] = v
                }
        }

        return &RegistryEntry{
                Name:          src.Name,
                Description:   src.Description,
                ActiveVersion: activeVer,
                Versions:      versions,
                Metadata:      meta,
        }
}
