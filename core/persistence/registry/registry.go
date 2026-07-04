package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/v7/core/cache"
	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
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
				Versions: map[string]SchemaVersionRecord{
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

// ResolvePhysicalName returns the physical name for a collection, optionally for a specific version.
func (r *collectionRegistry) ResolvePhysicalName(ctx context.Context, name string, version ...string) (string, error) {
	sc, err := r.GetSchema(ctx, name, version...)
	if err != nil {
		return "", err
	}
	return sc.Name, nil
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
		if err := manager.CreateCollection(tctx, *tempSchema); err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_COLLECTION_CREATION_FAILED", fmt.Sprintf("failed to create physical collection '%s'", actualPhysicalName))
		}

		entry.Versions[version] = SchemaVersionRecord{
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

	versions := make(map[string]SchemaVersionRecord, len(src.Versions))
	for k, v := range src.Versions {
		versions[k] = SchemaVersionRecord{
			Physical: v.Physical,
			Schema:   *v.Schema.DeepCopy(),
		}
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
