package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

var ErrCollectionNotFound = errors.New("collection not found in registry")

type RegistryEntry = base.RegistryEntry
type SchemaVersionRecord = base.SchemaVersionRecord

type RegistryExecutor func(ctx context.Context, transaction bool,
	fn func(collection base.Collection, manager query.SchemaManager) (any, error),
) (any, error)

// collectionRegistry implements the CollectionRegistry interface.
// It uses a dedicated persistence collection to manage schema metadata and a
// schema manager to perform physical DDL operations.
type collectionRegistry struct {
	executor      RegistryExecutor
	logger        *zap.Logger
	cache         *collectionCache
	refreshTicker *time.Ticker
	stopRefresh   chan struct{}
}

var _ base.CollectionRegistry = (*collectionRegistry)(nil)

// NewCollectionRegistry creates a new implementation of the CollectionRegistry.
// It also handles the crucial bootstrapping logic of ensuring that the underlying
// "_schemas_" collection exists, creating it if it doesn't.
func NewCollectionRegistry(executor RegistryExecutor, logger *zap.Logger, config ...CacheConfig) (base.CollectionRegistry, error) {
	cacheConfig := DefaultCacheConfig()
	if len(config) > 0 {
		cacheConfig = config[0]
	}
	_, err := executor(context.Background(), false, func(collection base.Collection, manager query.SchemaManager) (any, error) {
		exists, err := manager.CollectionExists(REGISTRY_COLLECTION_NAME)
		if err != nil {
			return nil, fmt.Errorf("failed to check for existence of registry collection: %w", err)
		}

		if !exists {
			logger.Info("registry collection '_schemas_' not found, creating it now")

			registrySchema := RegistrySchema()
			if err := manager.CreateCollection(*registrySchema); err != nil {
				return nil, fmt.Errorf("failed to create registry collection '_schemas_': %w", err)
			}
			logger.Info("successfully created registry collection '_schemas_'")
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}

	cache := newCollectionCache(cacheConfig, logger)
	registry := &collectionRegistry{
		executor:    executor,
		logger:      logger,
		cache:       cache,
		stopRefresh: make(chan struct{}),
	}

	// Warm up the cache by loading all existing entries
	if err := registry.warmCache(context.Background()); err != nil {
		logger.Warn("failed to warm cache on startup", zap.Error(err))
	}

	// Start background refresh if enabled
	if cacheConfig.EnableAutoRefresh && cacheConfig.RefreshInterval > 0 {
		registry.startBackgroundRefresh(cacheConfig.RefreshInterval)
	}

	return registry, nil

}

// Close stops background processes and cleans up resources
func (r *collectionRegistry) Close(ctx context.Context) error {
	if r.refreshTicker != nil {
		r.refreshTicker.Stop()
		close(r.stopRefresh)
	}
	r.cache.clear()
	return nil
}

// warmCache loads all registry entries into memory
func (r *collectionRegistry) warmCache(ctx context.Context) error {
	r.logger.Info("warming registry cache")

	entries, err := r.loadAllFromDatabase(ctx)
	if err != nil {
		return fmt.Errorf("failed to load entries for cache warming: %w", err)
	}

	for _, entry := range entries {
		r.cache.set(entry.Name, entry)
	}

	r.logger.Info("cache warmed", zap.Int("entries", len(entries)))
	return nil
}

// startBackgroundRefresh starts a goroutine to periodically refresh cache
func (r *collectionRegistry) startBackgroundRefresh(interval time.Duration) {
	r.refreshTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-r.refreshTicker.C:
				if err := r.refreshCache(); err != nil {
					r.logger.Warn("cache refresh failed", zap.Error(err))
				}
			case <-r.stopRefresh:
				return
			}
		}
	}()
	r.logger.Info("started background cache refresh", zap.Duration("interval", interval))
}

// refreshCache reloads data from database for potentially stale entries
func (r *collectionRegistry) refreshCache() error {
	// For now, we'll do a simple full refresh
	// In a more sophisticated implementation, we could track modification times
	return r.warmCache(context.Background())
}

// CreateCollection creates the initial entry for a new collection in the registry.
func (r *collectionRegistry) CreateCollection(ctx context.Context, schema *schema.SchemaDefinition) (*RegistryEntry, error) {
	enrichedSchema := enrichSchema(schema)
	entry, err := r.withSchemaValidationAndNotExists(ctx, enrichedSchema, func() (*RegistryEntry, error) {
		// Define initial version and physical name
		initialVersion := schema.Version
		physicalName, err := generatePhysicalName(schema)
		if err != nil {
			return nil, fmt.Errorf("could not generate physical name for '%s v%s': %w", schema.Name, schema.Version, err)
		}

		// Create the registry entry
		entry, sc := r.buildRegistryEntry(enrichedSchema, initialVersion, physicalName)

		// Provision the physical collection and persist the registry entry
		return execute(ctx, r.executor, true, func(collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
			// Create physical collection
			if err := r.createPhysicalCollection(manager, sc, physicalName); err != nil {
				return nil, err
			}

			// Persist registry entry
			result, err := r.persistRegistryEntry(ctx, collection, entry)
			if err != nil {
				return nil, err
			}

			return result, nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Update cache after successful write
	r.cache.set(entry.Name, entry)
	return entry, nil
}

// DropCollection removes a collection's entire schema history from the registry.
func (r *collectionRegistry) DropCollection(ctx context.Context, name string, opts base.DropCollectionOptions) error {
	entry, err := r.GetRegistryEntry(ctx, name)
	if err != nil {
		return err
	}

	_, err = execute(ctx, r.executor, true, func(collection base.Collection, manager query.SchemaManager) (bool, error) {
		// Drop physical collections if requested
		if opts.DeletePhysicalData {
			for _, versionRecord := range entry.Versions {
				if err := manager.DropCollection(versionRecord.Physical); err != nil {
					return false, fmt.Errorf("failed to drop physical collection %s: %w", versionRecord.Physical, err)
				}
			}
		}

		// Remove registry entry
		if err := r.deleteRegistryEntry(ctx, collection, name); err != nil {
			return false, err
		}

		return true, nil
	})

	if err != nil {
		return err
	}

	// Remove from cache after successful deletion
	r.cache.delete(name)
	return nil

}

// PruneVersion permanently deletes the physical database collection associated with a non-active schema version.
func (r *collectionRegistry) PruneVersion(ctx context.Context, name, version string) (*RegistryEntry, error) {
	entry, err := r.withEntryAndVersionValidation(ctx, name, version, func(entry *RegistryEntry, versionRecord SchemaVersionRecord) (*RegistryEntry, error) {
		if entry.ActiveVersion == version {
			return nil, fmt.Errorf("cannot prune active version '%s' for collection '%s'", version, name)
		}

		return r.executeWithEntryUpdate(ctx, name, entry, func(collection base.Collection, manager query.SchemaManager, entry *RegistryEntry) error {
			// Drop the physical collection
			if err := manager.DropCollection(versionRecord.Physical); err != nil {
				return fmt.Errorf("failed to drop physical collection %s: %w", versionRecord.Physical, err)
			}

			// Remove version from entry
			delete(entry.Versions, version)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Update cache with modified entry
	r.cache.set(name, entry)
	return entry, nil
}

// GetSchema retrieves a specific schema definition for a collection.
func (r *collectionRegistry) GetSchema(ctx context.Context, name string, version ...string) (*schema.SchemaDefinition, error) {
	return r.withEntryAndOptionalVersion(ctx, name, version, func(entry *RegistryEntry, resolvedVersion string, versionRecord SchemaVersionRecord) (*schema.SchemaDefinition, error) {
		return &versionRecord.Schema, nil
	})
}

// GetSchema retrieves a specific schema definition for a collection.
func (r *collectionRegistry) ResolvePhysicalName(ctx context.Context, name string, version ...string) (string, error) {
	schema, err := r.GetSchema(ctx, name, version...)
	if err != nil {
		return "", err
	}
	return schema.Name, nil
}

// GetRegistryEntry retrieves the complete management record for a collection.
func (r *collectionRegistry) GetRegistryEntry(ctx context.Context, name string) (*RegistryEntry, error) {
	// Try cache first
	if cached := r.cache.get(name); cached != nil {
		r.logger.Debug("cache hit for registry entry", zap.String("collection", name))
		return cached, nil
	}

	r.logger.Debug("cache miss for registry entry", zap.String("collection", name))

	// Cache miss - load from database
	entry, err := r.loadFromDatabase(ctx, name)
	if err != nil {
		return nil, err
	}

	// Store in cache for future requests
	r.cache.set(name, entry)
	return entry, nil
}

// loadFromDatabase loads a single registry entry from the database
func (r *collectionRegistry) loadFromDatabase(ctx context.Context, name string) (*RegistryEntry, error) {
	q := r.buildNameQuery(name)

	result, err := execute(ctx, r.executor, false, func(collection base.Collection, manager query.SchemaManager) (*base.ReadResult, error) {
		return collection.Read(ctx, &q)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query registry for collection '%s': %w", name, err)
	}

	if result.Count == 0 {
	    fmt.Printf("Collection %s \n", name)
		return nil, ErrCollectionNotFound
	}

	if result.Count > 1 {
		return nil, fmt.Errorf("internal error: found multiple entries for collection '%s'", name)
	}

	row := result.Data.(data.Document)
	return unmarshalEntry(row)
}

// AddSchemaVersion introduces a new version of a schema for an existing collection.
func (r *collectionRegistry) AddSchemaVersion(ctx context.Context, name, version string, schema *schema.SchemaDefinition, physicalName ...string) (*RegistryEntry, error) {
	enrichedSchema := enrichSchema(schema)
	entry, err := r.withSchemaValidationAndEntryExists(ctx, name, enrichedSchema, func(entry *RegistryEntry) (*RegistryEntry, error) {
		// Check if the version already exists
		if _, exists := entry.Versions[version]; exists {
			return nil, fmt.Errorf("version '%s' already exists for collection '%s'", version, name)
		}

		// Determine physical name
		actualPhysicalName, err := r.resolvePhysicalName(schema, physicalName...)
		if err != nil {
			return nil, err
		}

		return r.executeWithEntryUpdate(ctx, name, entry, func(collection base.Collection, manager query.SchemaManager, entry *RegistryEntry) error {
			// Create physical collection
			if err := r.createPhysicalCollection(manager, enrichedSchema, actualPhysicalName); err != nil {
				return err
			}

			enrichedSchema.Name = actualPhysicalName
			// Add version to entry
			entry.Versions[version] = SchemaVersionRecord{
				Physical: actualPhysicalName,
				Schema:   *enrichedSchema,
			}
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Update cache with modified entry
	r.cache.set(name, entry)
	return entry, nil
}

// SetActiveVersion changes the active schema version for a collection.
func (r *collectionRegistry) SetActiveVersion(ctx context.Context, name, version string) (*RegistryEntry, error) {

	entry, err := r.withEntryAndVersionValidation(ctx, name, version, func(entry *RegistryEntry, versionRecord SchemaVersionRecord) (*RegistryEntry, error) {
		if entry.ActiveVersion == version {
			return nil, fmt.Errorf("requested version is already the active version for collection '%s'", name)
		}

		return r.executeWithEntryUpdate(ctx, name, entry, func(collection base.Collection, manager query.SchemaManager, entry *RegistryEntry) error {
			entry.ActiveVersion = version
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Update cache with modified entry
	r.cache.set(name, entry)
	return entry, nil
}

// List retrieves the registry entries for all registered collections.
func (r *collectionRegistry) List(ctx context.Context) ([]*RegistryEntry, error) {
	// Get cached entries
	cachedNames := r.cache.list()
	entries := make([]*RegistryEntry, 0)

	// Collect cached entries
	for _, name := range cachedNames {
		if cached := r.cache.get(name); cached != nil {
			entries = append(entries, cached)
		}
	}

	// If we have some cached entries but might be missing some,
	// we need to check the database for a complete list
	allEntries, err := r.loadAllFromDatabase(ctx)
	if err != nil {
		// If database query fails but we have cached entries, return those
		if len(entries) > 0 {
			r.logger.Warn("database query failed, returning cached entries only", zap.Error(err))
			return entries, nil
		}
		return nil, fmt.Errorf("failed to query registry for collections: %w", err)
	}

	// Update cache with any missing entries
	for _, entry := range allEntries {
		r.cache.set(entry.Name, entry)
	}

	return allEntries, nil
}

// loadAllFromDatabase loads all registry entries from the database
func (r *collectionRegistry) loadAllFromDatabase(ctx context.Context) ([]*RegistryEntry, error) {
	q := query.NewQueryBuilder().Build()

	result, err := execute(ctx, r.executor, false, func(collection base.Collection, manager query.SchemaManager) (*base.ReadResult, error) {
		return collection.Read(ctx, &q)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query registry for collections: %w", err)
	}

	if result.Count == 0 {
		return []*RegistryEntry{}, nil
	}

	entries := make([]*RegistryEntry, 0, result.Count)

	rows := make([]data.Document, 0, result.Count)

	if result.Count == 1 {
		row := result.Data.(data.Document)
		rows = append(rows, row)
	} else {
		rows = result.Data.([]data.Document)
	}

	for _, row := range rows {
		entry, err := unmarshalEntry(row)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal registry entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Higher-order functions for common patterns

func (r *collectionRegistry) withSchemaValidationAndNotExists(ctx context.Context, schema *schema.SchemaDefinition, fn func() (*RegistryEntry, error)) (*RegistryEntry, error) {
	// Validate the schema
	if err := schema.Validate(); err != nil {
		return nil, fmt.Errorf("invalid schema %s: %w, %v", schema.Name, err, schema)
	}

	// Check if a collection with the same name already exists
	exists, err := r.GetRegistryEntry(ctx, schema.Name)
	if exists != nil {
		return nil, fmt.Errorf("collection with name '%s' already exists", schema.Name)
	}
	if !errors.Is(err, ErrCollectionNotFound) {
		return nil, fmt.Errorf("failed to check for existing collection: %w", err)
	}

	return fn()
}

func (r *collectionRegistry) withSchemaValidationAndEntryExists(ctx context.Context, name string, schema *schema.SchemaDefinition, fn func(entry *RegistryEntry) (*RegistryEntry, error)) (*RegistryEntry, error) {
	// Validate the schema
	if err := schema.Validate(); err != nil {
		return nil, fmt.Errorf("invalid schema %s: %w", schema.Name, err)
	}

	// Get existing entry
	entry, err := r.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, err
	}

	return fn(entry)
}

func (r *collectionRegistry) withEntryAndVersionValidation(ctx context.Context, name, version string, fn func(entry *RegistryEntry, versionRecord SchemaVersionRecord) (*RegistryEntry, error)) (*RegistryEntry, error) {
	entry, err := r.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, err
	}

	versionRecord, ok := entry.Versions[version]
	if !ok {
		return nil, fmt.Errorf("version '%s' not found for collection '%s'", version, name)
	}

	return fn(entry, versionRecord)
}

func (r *collectionRegistry) withEntryAndOptionalVersion(ctx context.Context, name string, version []string, fn func(entry *RegistryEntry, resolvedVersion string, versionRecord SchemaVersionRecord) (*schema.SchemaDefinition, error)) (*schema.SchemaDefinition, error) {
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
		return nil, fmt.Errorf("version '%s' not found for collection '%s'", resolvedVersion, name)
	}

	return fn(entry, resolvedVersion, versionRecord)
}

func (r *collectionRegistry) executeWithEntryUpdate(ctx context.Context, name string, entry *RegistryEntry, fn func(collection base.Collection, manager query.SchemaManager, entry *RegistryEntry) error) (*RegistryEntry, error) {
	return execute(ctx, r.executor, true, func(collection base.Collection, manager query.SchemaManager) (*RegistryEntry, error) {
		if err := fn(collection, manager, entry); err != nil {
			return nil, err
		}

		if err := r.updateRegistryEntry(ctx, collection, name, entry); err != nil {
			return nil, err
		}

		return entry, nil
	})
}

func (r *collectionRegistry) buildRegistryEntry(sc *schema.SchemaDefinition, version, physicalName string) (*RegistryEntry, *schema.SchemaDefinition) {
	tempSchema := *sc
	tempSchema.Name = physicalName
	return &RegistryEntry{
		Name:          sc.Name,
		Description:   sc.Description,
		ActiveVersion: version,
		Versions: map[string]SchemaVersionRecord{
			version: {
				Physical: physicalName,
				Schema:   tempSchema,
			},
		},
	}, &tempSchema

}

func (r *collectionRegistry) createPhysicalCollection(manager query.SchemaManager, schema *schema.SchemaDefinition, physicalName string) error {
	tempSchema := *schema
	tempSchema.Name = physicalName

	if err := manager.CreateCollection(tempSchema); err != nil {
		return fmt.Errorf("failed to create physical collection: %w", err)
	}
	return nil
}

func (r *collectionRegistry) entryToDocument(entry *RegistryEntry) (data.Document, error) {
	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return data.Document{}, fmt.Errorf("failed to marshal registry entry: %w", err)
	}

	var docData map[string]any
	if err := json.Unmarshal(entryBytes, &docData); err != nil {
		return data.Document{}, fmt.Errorf("failed to unmarshal registry entry to document: %w", err)
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
		return nil, fmt.Errorf("failed to create registry entry: %w", err)
	}

	if len(result.Issues) > 0 {
		return nil, fmt.Errorf("failed to create registry entry with issues: %v", result.Issues)
	}

	rentry, err := unmarshalEntry(result.Data)

	if err != nil {
		return nil, fmt.Errorf("failed to create registry entry: %w", err)
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
		return fmt.Errorf("failed to update registry entry for collection %s: %w", name, err)
	}

	return nil
}

func (r *collectionRegistry) deleteRegistryEntry(ctx context.Context, collection base.Collection, name string) error {
	q := r.buildNameQuery(name)
	_, err := collection.Delete(ctx, q.Filters, false)
	if err != nil {
		return fmt.Errorf("failed to delete registry entry for collection %s: %w", name, err)
	}
	return nil
}

func (r *collectionRegistry) buildNameQuery(name string) query.Query {
	return query.NewQueryBuilder().Where("name").Eq(name).Build()
}

func (r *collectionRegistry) resolvePhysicalName(schema *schema.SchemaDefinition, physicalName ...string) (string, error) {
	if len(physicalName) > 0 {
		return physicalName[0], nil
	}

	actualPhysicalName, err := generatePhysicalName(schema)
	if err != nil {
		return "", fmt.Errorf("could not generate physical name for '%s v%s': %w", schema.Name, schema.Version, err)
	}

	return actualPhysicalName, nil
}

func execute[T any](
	ctx context.Context,
	executor RegistryExecutor,
	requiresTransaction bool,
	fn func(collection base.Collection, manager query.SchemaManager) (T, error),
) (T, error) {
	result, err := executor(ctx, requiresTransaction, func(collection base.Collection, manager query.SchemaManager) (any, error) {
		return fn(collection, manager)
	})

	if err != nil {
		var zero T
		return zero, err
	}

	return result.(T), nil
}

// CacheStats returns statistics about cache performance
func (r *collectionRegistry) CacheStats() map[string]any {
	r.cache.mu.RLock()
	defer r.cache.mu.RUnlock()

	return map[string]any{
		"entries":          len(r.cache.entries),
		"max_size":         r.cache.maxSize,
		"refresh_enabled":  r.cache.config.EnableAutoRefresh,
		"refresh_interval": r.cache.config.RefreshInterval.String(),
	}
}

// InvalidateCache removes a specific entry from cache or all entries if name is empty
func (r *collectionRegistry) InvalidateCache(name string) {
	if name == "" {
		r.cache.clear()
		r.logger.Info("cleared entire registry cache")
	} else {
		r.cache.delete(name)
		r.logger.Info("invalidated cache entry", zap.String("collection", name))
	}
}

// RefreshCache manually triggers a cache refresh
func (r *collectionRegistry) RefreshCache() error {
	return r.refreshCache()
}
