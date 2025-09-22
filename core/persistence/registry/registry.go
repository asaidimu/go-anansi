package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

type RegistryEntry = base.RegistryEntry
type SchemaVersionRecord = base.SchemaVersionRecord

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
	executor RegistryExecutor
	logger   *zap.Logger
	cache    *simpleCollectionCache
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
			return nil, &RegistryError{
				Operation: "NewCollectionRegistry",
				Message:   ErrFailedToCheckRegistryExistence.Error(),
				Cause:     errors.Join(ErrFailedToCheckRegistryExistence, err),
			}
		}

		if !exists {
			registrySchema := RegistrySchema()
			if err := manager.CreateCollection(ctx, *registrySchema); err != nil {
				return nil, &RegistryError{
					Operation: "NewCollectionRegistry",
					Message:   fmt.Sprintf("'_schemas_': %v", ErrFailedToCreateRegistryCollection),
					Cause:     errors.Join(ErrFailedToCreateRegistryCollection, err),
				}
			}
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}

	registry := &collectionRegistry{
		executor: executor,
		logger:   logger,
		cache:    newSimpleCollectionCache(cacheConfig.MaxCacheSize),
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
		return &RegistryError{
			Operation: "warmCache",
			Message:   ErrFailedToWarmCache.Error(),
			Cause:     errors.Join(ErrFailedToWarmCache, err),
		}
	}

	for _, entry := range entries {
		r.cache.set(entry.Name, entry)
	}

	return nil
}

// CreateCollection - simplified implementation
func (r *collectionRegistry) CreateCollection(ctx context.Context, sc *schema.SchemaDefinition) (*RegistryEntry, error) {
	results, err := r.CreateCollections(ctx, []schema.SchemaDefinition{*sc})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// CreateCollections - streamlined without complex preparation phase
func (r *collectionRegistry) CreateCollections(ctx context.Context, schemas []schema.SchemaDefinition) ([]*RegistryEntry, error) {
	if len(schemas) == 0 {
		return []*RegistryEntry{}, nil
	}

	// Simple upfront validation
	schemaNames := make(map[string]bool)
	for _, schema := range schemas {
		schemaKey := fmt.Sprintf("%s@%s", schema.Name, schema.Version)
		if schemaNames[schemaKey] {
			return nil, &RegistryError{
				Operation: "CreateCollections",
				Message:   fmt.Sprintf("'%s' version '%s'", schema.Name, schema.Version),
				Cause:     ErrDuplicateSchemaInBatch,
			}
		}
		schemaNames[schemaKey] = true

		// Basic validation
		enrichedSchema := EnrichSchema(&schema)
		if err := enrichedSchema.Validate(); err != nil {
			return nil, &RegistryError{
				Operation: "CreateCollections",
				Message:   fmt.Sprintf("invalid schema '%s' v%s: %v", schema.Name, schema.Version, err),
				Cause:     errors.Join(data.ErrSchemaViolation, err),
			}
		}

		// Check if collection already exists
		if _, err := r.GetRegistryEntry(ctx, schema.Name); err == nil {
			return nil, &RegistryError{
				Operation: "CreateCollections",
				Message:   fmt.Sprintf("'%s'", schema.Name),
				Cause:     ErrCollectionAlreadyExists,
			}
		} else if !errors.Is(err, ErrCollectionNotFound) {
			return nil, &RegistryError{
				Operation: "CreateCollections",
				Message:   ErrFailedToCheckRegistryExistence.Error(),
				Cause:     errors.Join(ErrFailedToCheckRegistryExistence, err),
			}
		}
	}

	// Execute in transaction
	results, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) ([]*RegistryEntry, error) {
		var createdEntries []*RegistryEntry

		for _, schema := range schemas {
			enrichedSchema := EnrichSchema(&schema)

			// Generate physical name
			physicalName, err := generatePhysicalName(&schema)
			if err != nil {
				return nil, &RegistryError{
					Operation: "CreateCollections",
					Message:   fmt.Sprintf("for '%s' v%s", schema.Name, schema.Version),
					Cause:     errors.Join(ErrFailedToGeneratePhysicalName, err),
				}
			}

			// Create physical collection
			tempSchema := *enrichedSchema
			tempSchema.Name = physicalName
			if err := manager.CreateCollection(tctx, tempSchema); err != nil {
				return nil, &RegistryError{
					Operation: "CreateCollections",
					Message:   fmt.Sprintf("'%s'", physicalName),
					Cause:     errors.Join(base.ErrCollectionCreation, err),
				}
			}

			// Build and persist registry entry
			entry := &RegistryEntry{
				Name:          schema.Name,
				Description:   schema.Description,
				ActiveVersion: schema.Version,
				Versions: map[string]SchemaVersionRecord{
					schema.Version: {
						Physical: physicalName,
						Schema:   tempSchema,
					},
				},
			}

			result, err := r.persistRegistryEntry(tctx, collection, entry)
			if err != nil {
				return nil, &RegistryError{
					Operation: "CreateCollections",
					Message:   fmt.Sprintf("for '%s'", entry.Name),
					Cause:     errors.Join(ErrFailedToPersistRegistryEntry, err),
				}
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
func (r *collectionRegistry) GetSchema(ctx context.Context, name string, version ...string) (*schema.SchemaDefinition, error) {
	entry, err := r.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, err
	}

	resolvedVersion := entry.ActiveVersion
	if len(version) > 0 {
		resolvedVersion = version[0]
	}

	versionRecord, ok := entry.Versions[resolvedVersion]
	if !ok {
		return nil, &RegistryError{
			Operation: "GetSchema",
			Message:   fmt.Sprintf("version '%s' not found for collection '%s'", resolvedVersion, name),
			Cause:     ErrVersionNotFoundForCollection,
		}
	}

	return &versionRecord.Schema, nil
}

// ResolvePhysicalName - simplified
func (r *collectionRegistry) ResolvePhysicalName(ctx context.Context, name string, version ...string) (string, error) {
	schema, err := r.GetSchema(ctx, name, version...)
	if err != nil {
		return "", err
	}
	return schema.Name, nil
}

// AddSchemaVersion - direct implementation
func (r *collectionRegistry) AddSchemaVersion(ctx context.Context, name, version string, schema *schema.SchemaDefinition, physicalName ...string) (*RegistryEntry, error) {
	enrichedSchema := EnrichSchema(schema)
	if err := enrichedSchema.Validate(); err != nil {
		return nil, &RegistryError{
			Operation: "AddSchemaVersion",
			Message:   fmt.Sprintf("invalid schema: %v", err),
			Cause:     errors.Join(base.ErrInvalidSchema, err),
		}
	}

	entry, err := r.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, err
	}

	if _, exists := entry.Versions[version]; exists {
		return nil, &RegistryError{
			Operation: "AddSchemaVersion",
			Message:   fmt.Sprintf("version '%s' for collection '%s'", version, name),
			Cause:     ErrVersionAlreadyExists,
		}
	}

	// Determine physical name
	actualPhysicalName := ""
	if len(physicalName) > 0 {
		actualPhysicalName = physicalName[0]
	} else {
		actualPhysicalName, err = generatePhysicalName(schema)
		if err != nil {
			return nil, &RegistryError{
				Operation: "AddSchemaVersion",
				Message:   fmt.Sprintf("for '%s v%s'", schema.Name, schema.Version),
				Cause:     errors.Join(ErrFailedToGeneratePhysicalName, err),
			}
		}
	}

	// Execute in transaction
	updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
		// Create physical collection
		tempSchema := *enrichedSchema
		tempSchema.Name = actualPhysicalName
		if err := manager.CreateCollection(tctx, tempSchema); err != nil {
			return nil, &RegistryError{
				Operation: "AddSchemaVersion",
				Message:   fmt.Sprintf("failed to create physical collection '%s'", actualPhysicalName),
				Cause:     errors.Join(base.ErrCollectionCreation, err),
			}
		}

		// Update registry entry
		entry.Versions[version] = SchemaVersionRecord{
			Physical: actualPhysicalName,
			Schema:   tempSchema,
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

	if entry.ActiveVersion == version {
		return nil, &RegistryError{
			Operation: "SetActiveVersion",
			Message:   fmt.Sprintf("'%s'", name),
			Cause:     ErrVersionAlreadyActive,
		}
	}

	if _, ok := entry.Versions[version]; !ok {
		return nil, &RegistryError{
			Operation: "SetActiveVersion",
			Message:   fmt.Sprintf("version '%s' not found for collection '%s'", version, name),
			Cause:     ErrVersionNotFoundForCollection,
		}
	}

	// Execute in transaction
	updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
		entry.ActiveVersion = version
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
					return false, &RegistryError{
						Operation: "DropCollection",
						Message:   fmt.Sprintf("%s", versionRecord.Physical),
						Cause:     errors.Join(base.ErrDropCollection, err),
					}
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
		return nil, &RegistryError{
			Operation: "PruneVersion",
			Message:   fmt.Sprintf("version '%s' not found for collection '%s'", version, name),
			Cause:     ErrVersionNotFoundForCollection,
		}
	}

	if entry.ActiveVersion == version {
		return nil, &RegistryError{
			Operation: "PruneVersion",
			Message:   fmt.Sprintf("version '%s' for collection '%s'", version, name),
			Cause:     ErrCannotPruneActiveVersion,
		}
	}

	// Execute in transaction
	updatedEntry, err := execute(ctx, r.executor, true, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
		// Drop the physical collection
		if err := manager.DropCollection(ctx, versionRecord.Physical); err != nil {
			return nil, &RegistryError{
				Operation: "PruneVersion",
				Message:   fmt.Sprintf("%s", versionRecord.Physical),
				Cause:     errors.Join(ErrFailedToDropPhysicalCollection, err),
			}
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
		return nil, &RegistryError{
			Operation: "List",
			Message:   ErrFailedToListCollections.Error(),
			Cause:     errors.Join(ErrFailedToListCollections, err),
		}
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
		return nil, &RegistryError{
			Operation: "loadFromDatabase",
			Message:   fmt.Sprintf("'%s'", name),
			Cause:     errors.Join(ErrFailedToQueryRegistryCollection, err),
		}
	}

	readResult := result
	if readResult.Count == 0 {
		return nil, ErrCollectionNotFound
	}

	if readResult.Count > 1 {
		return nil, &RegistryError{
			Operation: "loadFromDatabase",
			Message:   fmt.Sprintf("'%s'", name),
			Cause:     ErrMultipleRegistryEntriesFound,
		}
	}

	row := readResult.Data.(data.Document)
	return unmarshalEntry(row)
}

func (r *collectionRegistry) loadAllFromDatabase(ctx context.Context) ([]*RegistryEntry, error) {
	q := query.NewQueryBuilder().Build()

	result, err := execute(ctx, r.executor, false, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (*base.ReadResult, error) {
		return collection.Read(tctx, &q)
	})

	if err != nil {
		return nil, &RegistryError{
			Operation: "loadAllFromDatabase",
			Message:   ErrFailedToListCollections.Error(),
			Cause:     errors.Join(ErrFailedToListCollections, err),
		}
	}

	readResult := result
	if readResult.Count == 0 {
		return []*RegistryEntry{}, nil
	}

	entries := make([]*RegistryEntry, 0, readResult.Count)

	if readResult.Count == 1 {
		row := readResult.Data.(data.Document)
		entry, err := unmarshalEntry(row)
		if err != nil {
			return nil, &RegistryError{
				Operation: "loadAllFromDatabase",
				Message:   ErrFailedToUnmarshalRegistryEntry.Error(),
				Cause:     errors.Join(ErrFailedToUnmarshalRegistryEntry, err),
			}
		}
		entries = append(entries, entry)
	} else {
		rows := readResult.Data.([]data.Document)
		for _, row := range rows {
			entry, err := unmarshalEntry(row)
			if err != nil {
				return nil, &RegistryError{
					Operation: "loadAllFromDatabase",
					Message:   ErrFailedToUnmarshalRegistryEntry.Error(),
					Cause:     errors.Join(ErrFailedToUnmarshalRegistryEntry, err),
				}
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// Keep existing helper methods but simplified

func (r *collectionRegistry) entryToDocument(entry *RegistryEntry) (data.Document, error) {
	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return data.MustNewDocument(nil), &RegistryError{
			Operation: "entryToDocument",
			Message:   ErrFailedToMarshalRegistryEntry.Error(),
			Cause:     errors.Join(ErrFailedToMarshalRegistryEntry, err),
		}
	}

	var docData map[string]any
	if err := json.Unmarshal(entryBytes, &docData); err != nil {
		return data.MustNewDocument(nil), &RegistryError{
			Operation: "entryToDocument",
			Message:   ErrFailedToUnmarshalRegistryEntry.Error(),
			Cause:     errors.Join(ErrFailedToUnmarshalRegistryEntry, err),
		}
	}

	return data.MustNewDocument(docData), nil
}

func (r *collectionRegistry) persistRegistryEntry(ctx context.Context, collection base.Collection, entry *RegistryEntry) (*RegistryEntry, error) {
	doc, err := r.entryToDocument(entry)
	if err != nil {
		return nil, err
	}

	result, err := collection.CreateOne(ctx, doc)
	if err != nil {
		return nil, &RegistryError{
			Operation: "persistRegistryEntry",
			Message:   ErrFailedToCreateRegistryEntry.Error(),
			Cause:     errors.Join(ErrFailedToCreateRegistryEntry, err),
		}
	}

	if len(result.Issues) > 0 {
		return nil, &RegistryError{
			Operation: "persistRegistryEntry",
			Message:   fmt.Sprintf("%v", result.Issues),
			Cause:     ErrFailedToCreateRegistryEntryWithIssues,
		}
	}

	rentry, err := unmarshalEntry(result.Data)
	if err != nil {
		return nil, &RegistryError{
			Operation: "persistRegistryEntry",
			Message:   ErrFailedToCreateRegistryEntry.Error(),
			Cause:     errors.Join(ErrFailedToCreateRegistryEntry, err),
		}
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
		Data:   doc,
	})

	if err != nil {
		return &RegistryError{
			Operation: "updateRegistryEntry",
			Message:   fmt.Sprintf("for collection %s", name),
			Cause:     errors.Join(ErrFailedToUpdateRegistryEntry, err),
		}
	}

	return nil
}

func (r *collectionRegistry) deleteRegistryEntry(ctx context.Context, collection base.Collection, name string) error {
	q := r.buildNameQuery(name)
	_, err := collection.Delete(ctx, q.Filters, false)
	if err != nil {
		return &RegistryError{
			Operation: "deleteRegistryEntry",
			Message:   fmt.Sprintf("for collection %s", name),
			Cause:     errors.Join(ErrFailedToDeleteRegistryEntry, err),
		}
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
