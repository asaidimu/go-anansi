package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

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

// CacheConfig holds configuration for the in-memory cache
type CacheConfig struct {
	// RefreshInterval defines how often to check for external changes
	RefreshInterval time.Duration
	// MaxCacheSize limits the number of entries kept in memory (0 = unlimited)
	MaxCacheSize int
	// EnableAutoRefresh determines if cache should auto-refresh in background
	EnableAutoRefresh bool
}

// DefaultCacheConfig provides sensible defaults for cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		RefreshInterval:   5 * time.Minute,
		MaxCacheSize:      0, // unlimited
		EnableAutoRefresh: true,
	}
}

// Keep the existing RegistryExecutor signature for compatibility
type RegistryExecutor func(ctx context.Context, transaction bool,
	fn func(ctx context.Context, collection base.Collection, manager query.SchemaManager) (any, error),
) (any, error)

// Simplified cache without background refresh complexity
type simpleCollectionCache struct {
	mu      sync.RWMutex
	entries map[string]*RegistryEntry
	maxSize int
}

func newSimpleCollectionCache(maxSize int) *simpleCollectionCache {
	return &simpleCollectionCache{
		entries: make(map[string]*RegistryEntry),
		maxSize: maxSize,
	}
}

func (c *simpleCollectionCache) get(name string) *RegistryEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if entry, exists := c.entries[name]; exists {
		// Return a copy to prevent external modification
		entryCopy := *entry
		return &entryCopy
	}
	return nil
}

func (c *simpleCollectionCache) set(name string, entry *RegistryEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store a copy to prevent external modification
	entryCopy := *entry
	c.entries[name] = &entryCopy
}

func (c *simpleCollectionCache) delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, name)
}

func (c *simpleCollectionCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*RegistryEntry)
}

// Simplified registry implementation that maintains the original interface
type collectionRegistry struct {
	executor  RegistryExecutor
	logger    *zap.Logger
	cache     *simpleCollectionCache
	validator *definition.DocumentValidator
}

var _ base.CollectionRegistry = (*collectionRegistry)(nil)

// Keep the original constructor signature
func NewCollectionRegistry(executor RegistryExecutor, logger *zap.Logger, config ...CacheConfig) (base.CollectionRegistry, error) {
	cacheConfig := DefaultCacheConfig()
	if len(config) > 0 {
		cacheConfig = config[0]
	}

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

	validator := schema.SchemaValidator()
	registry := &collectionRegistry{
		executor:  executor,
		logger:    logger,
		cache:     newSimpleCollectionCache(cacheConfig.MaxCacheSize),
		validator: validator,
	}

	// Warm up the cache
	if err := registry.warmCache(context.Background()); err != nil {
		logger.Warn("Failed to warm cache during initialization", zap.Error(err))
	}

	return registry, nil
}

// Close maintains compatibility but simplified
func (r *collectionRegistry) Close(ctx context.Context) error {
	r.cache.clear()
	return nil
}

// Simplified cache warming
func (r *collectionRegistry) warmCache(ctx context.Context) error {
	entries, err := r.loadAllFromDatabase(ctx)
	if err != nil {
		return common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_WARM_CACHE")
	}

	for _, entry := range entries {
		r.cache.set(entry.Name, entry)
	}

	return nil
}

func (r *collectionRegistry) CreateCollection(ctx context.Context, sc *definition.Schema) (*RegistryEntry, error) {
	results, err := r.CreateCollections(ctx, []*definition.Schema{sc})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// CreateCollections - streamlined without complex preparation phase
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

		// Basic validation
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
			// Generate physical name
			physicalName, err := generatePhysicalName(sc)
			if err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_GENERATE_PHYSICAL_NAME", fmt.Sprintf("for '%s' v%s", sc.Name, sc.Version.String()))
			}

			// Create physical collection
			tempSchema := sc.DeepCopy()
			tempSchema.Name = physicalName

			if err := manager.CreateCollection(tctx, *tempSchema); err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_COLLECTION_CREATION_FAILED", fmt.Sprintf("failed to create physical collection '%s'", physicalName))
			}

			// Build and persist registry entry
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
		r.cache.set(entry.Name, entry)
	}

	return results, nil
}

// GetRegistryEntry - simplified with straightforward caching
func (r *collectionRegistry) GetRegistryEntry(ctx context.Context, name string) (*RegistryEntry, error) {
	// Try cache first
	if cached := r.cache.get(name); cached != nil {
		return cached, nil
	}

	// Load from database
	entry, err := r.loadFromDatabase(ctx, name)
	if err != nil {
		return nil, err
	}

	// Update cache
	r.cache.set(name, entry)
	return entry, nil
}

// GetSchema - direct implementation without higher-order functions
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

// ResolvePhysicalName - simplified
func (r *collectionRegistry) ResolvePhysicalName(ctx context.Context, name string, version ...string) (string, error) {
	sc, err := r.GetSchema(ctx, name, version...)
	if err != nil {
		return "", err
	}
	return sc.Name, nil
}

// AddSchemaVersion - direct implementation
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

	// Determine physical name
	actualPhysicalName := ""
	if len(physicalName) > 0 {
		actualPhysicalName = physicalName[0]
	} else {
		actualPhysicalName, err = generatePhysicalName(sc)
		if err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_GENERATE_PHYSICAL_NAME", fmt.Sprintf("for '%s v%s'", sc.Name, sc.Version.String()))
		}
	}

	// Execute in transaction
	updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
		// Create physical collection
		tempSchema := enrichedSchema.DeepCopy()
		tempSchema.Name = actualPhysicalName
		if err := manager.CreateCollection(tctx, *tempSchema); err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_COLLECTION_CREATION_FAILED", fmt.Sprintf("failed to create physical collection '%s'", actualPhysicalName))
		}

		// Update registry entry
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

	// Update cache
	r.cache.set(name, updatedEntry)
	return updatedEntry, nil
}

// SetActiveVersion - simplified
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

	// Execute in transaction
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

	// Update cache
	r.cache.set(name, updatedEntry)
	return updatedEntry, nil
}

// DropCollection - simplified
func (r *collectionRegistry) DropCollection(ctx context.Context, name string, opts base.DropCollectionOptions) error {
	entry, err := r.GetRegistryEntry(ctx, name)
	if err != nil {
		return err
	}

	_, err = execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (bool, error) {
		// Drop physical collections if requested
		if opts.DeletePhysicalData {
			for _, versionRecord := range entry.Versions {
				if err := manager.DropCollection(ctx, versionRecord.Physical); err != nil {
					return false, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_DROP_PHYSICAL_COLLECTION", fmt.Sprintf("failed to drop physical collection '%s'", versionRecord.Physical))
				}
			}
		}

		// Remove registry entry
		if err := r.deleteRegistryEntry(tctx, collection, name); err != nil {
			return false, err
		}

		return true, nil
	})

	if err != nil {
		return err
	}

	// Remove from cache
	r.cache.delete(name)
	return nil
}

// PruneVersion - simplified
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

	// Execute in transaction
	updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
		// Drop the physical collection
		if err := manager.DropCollection(ctx, versionRecord.Physical); err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_REGISTRY_FAILED_TO_DROP_PHYSICAL_COLLECTION", fmt.Sprintf("failed to drop physical collection '%s'", versionRecord.Physical))
		}

		// Remove version from entry
		delete(entry.Versions, version)

		if err := r.updateRegistryEntry(tctx, collection, name, entry); err != nil {
			return nil, err
		}

		return entry, nil
	})

	if err != nil {
		return nil, err
	}

	// Update cache
	r.cache.set(name, updatedEntry)
	return updatedEntry, nil
}

// List - simplified
func (r *collectionRegistry) List(ctx context.Context) ([]*RegistryEntry, error) {
	// Load all from database (could optimize by checking cache first)
	allEntries, err := r.loadAllFromDatabase(ctx)
	if err != nil {
		return nil, base.ErrFailedToListCollections.WithCause(err)
	}

	// Update cache with fresh data
	for _, entry := range allEntries {
		r.cache.set(entry.Name, entry)
	}

	return allEntries, nil
}

// Simplified helper methods - using the original execute pattern

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

// Keep existing helper methods but simplified

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

// Maintain compatibility methods but simplified internals

func (r *collectionRegistry) CacheStats() map[string]any {
	r.cache.mu.RLock()
	defer r.cache.mu.RUnlock()

	return map[string]any{
		"entries":          len(r.cache.entries),
		"max_size":         r.cache.maxSize,
		"refresh_enabled":  false, // No longer supported
		"refresh_interval": "disabled",
	}
}

func (r *collectionRegistry) InvalidateCache(name string) {
	if name == "" {
		r.cache.clear()
	} else {
		r.cache.delete(name)
	}
}

func (r *collectionRegistry) RefreshCache() error {
	return r.warmCache(context.Background())
}
