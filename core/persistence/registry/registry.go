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
		exists, err := manager.CollectionExists(context.Background(), REGISTRY_COLLECTION_NAME)
		if err != nil {
			return nil, &RegistryError{
				Operation: "NewCollectionRegistry",
				Message:   ErrFailedToCheckRegistryExistence.Error(),
				Cause:     errors.Join(ErrFailedToCheckRegistryExistence, err),
			}
		}

		if !exists {

			registrySchema := RegistrySchema()
			if err := manager.CreateCollection(context.Background(), *registrySchema); err != nil {
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

	cache := newCollectionCache(cacheConfig, logger)
	registry := &collectionRegistry{
		executor:    executor,
		logger:      logger,
		cache:       cache,
		stopRefresh: make(chan struct{}),
	}

	// Warm up the cache by loading all existing entries
	if err := registry.warmCache(context.Background()); err != nil {
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

// startBackgroundRefresh starts a goroutine to periodically refresh cache
func (r *collectionRegistry) startBackgroundRefresh(interval time.Duration) {
	r.refreshTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-r.refreshTicker.C:
			case <-r.stopRefresh:
				return
			}
		}
	}()
}

// refreshCache reloads data from database for potentially stale entries
func (r *collectionRegistry) refreshCache() error {
	// For now, we'll do a simple full refresh
	// In a more sophisticated implementation, we could track modification times
	return r.warmCache(context.Background())
}

// CreateCollection creates the initial entry for a new collection in the registry.
func (r *collectionRegistry) CreateCollection(ctx context.Context, sc *schema.SchemaDefinition) (*RegistryEntry, error) {
	results, err := r.CreateCollections(ctx, []schema.SchemaDefinition{*sc})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// CreateCollections creates multiple collections atomically in a single transaction.
// All validations are performed upfront - if any collection fails validation,
// the entire operation fails without creating any collections.
func (r *collectionRegistry) CreateCollections(ctx context.Context, schemas []schema.SchemaDefinition) ([]*RegistryEntry, error) {
	if len(schemas) == 0 {
		return []*RegistryEntry{}, nil
	}

	// Phase 1: Validate all schemas and prepare data upfront
	collectionsToCreate, err := r.prepareCollectionData(ctx, schemas)
	if err != nil {
		return nil, err
	}

	// Phase 2: Execute all operations in a single transaction
	results, err := execute(ctx, r.executor, true, func(collection base.Collection, manager query.SchemaManager) ([]*RegistryEntry, error) {
		return r.createCollectionsInTransaction(ctx, collection, manager, collectionsToCreate)
	})

	if err != nil {
		return nil, err
	}

	// Phase 3: Update cache after successful completion
	for _, entry := range results {
		r.cache.set(entry.Name, entry)
	}

	return results, nil
}

// collectionData holds all the prepared data needed to create a collection
type collectionData struct {
	enrichedSchema *schema.SchemaDefinition
	physicalName   string
	entry          *RegistryEntry
	schemaConfig   *schema.SchemaDefinition
}

// prepareCollectionData validates all schemas and prepares the data needed for creation
func (r *collectionRegistry) prepareCollectionData(ctx context.Context, schemas []schema.SchemaDefinition) ([]collectionData, error) {
	collectionsToCreate := make([]collectionData, 0, len(schemas))
	physicalNames := make(map[string]bool)
	schemaNames := make(map[string]bool)

	for _, schema := range schemas {
		// Check for duplicates within the batch
		schemaKey := fmt.Sprintf("%s@%s", schema.Name, schema.Version)
		if schemaNames[schemaKey] {
			return nil, &RegistryError{
			Operation: "prepareCollectionData",
			Message:   fmt.Sprintf("'%s' version '%s'", schema.Name, schema.Version),
			Cause:     ErrDuplicateSchemaInBatch,
		}
		}
		schemaNames[schemaKey] = true

		// Enrich and validate schema, check it doesn't already exist
		enrichedSchema := EnrichSchema(&schema)
		_, err := r.withSchemaValidationAndNotExists(ctx, enrichedSchema, func() (*RegistryEntry, error) {
			return nil, nil // Just validate, don't create anything yet
		})
		if err != nil {
			return nil, &RegistryError{
			Operation: "prepareCollectionData",
			Message:   fmt.Sprintf("for schema '%s' v%s", schema.Name, schema.Version),
			Cause:     errors.Join(data.ErrSchemaViolation, err),
		}
		}

		// Generate physical name
		physicalName, err := generatePhysicalName(&schema)
		if err != nil {
			return nil, &RegistryError{
			Operation: "prepareCollectionData",
			Message:   fmt.Sprintf("for '%s' v%s", schema.Name, schema.Version),
			Cause:     errors.Join(ErrFailedToGeneratePhysicalName, err),
		}
		}

		// Check for physical name conflicts within the batch
		if physicalNames[physicalName] {
			return nil, &RegistryError{
			Operation: "prepareCollectionData",
			Message:   fmt.Sprintf("'%s' (generated for '%s' v%s)", physicalName, schema.Name, schema.Version),
			Cause:     ErrPhysicalNameConflictInBatch,
		}
		}

		physicalNames[physicalName] = true

		// Build registry entry
		entry, sc := r.buildRegistryEntry(enrichedSchema, schema.Version, physicalName)

		collectionsToCreate = append(collectionsToCreate, collectionData{
			enrichedSchema: enrichedSchema,
			physicalName:   physicalName,
			entry:          entry,
			schemaConfig:   sc,
		})
	}

	return collectionsToCreate, nil
}

// createCollectionsInTransaction creates all physical collections and persists registry entries within a transaction
func (r *collectionRegistry) createCollectionsInTransaction(ctx context.Context, collection base.Collection, manager query.SchemaManager, collectionsToCreate []collectionData) ([]*RegistryEntry, error) {
	createdEntries := make([]*RegistryEntry, 0, len(collectionsToCreate))

	// Create all physical collections
	for _, data := range collectionsToCreate {
		if err := r.createPhysicalCollection(ctx, manager, data.schemaConfig, data.physicalName); err != nil {
			return nil, &RegistryError{
				Operation: "createCollectionsInTransaction",
				Message:   fmt.Sprintf("'%s'", data.physicalName),
				Cause:     errors.Join(base.ErrCollectionCreation, err),
			}
		}
	}

	// Persist all registry entries
	for _, data := range collectionsToCreate {
		result, err := r.persistRegistryEntry(ctx, collection, data.entry)
		if err != nil {
			return nil, &RegistryError{
				Operation: "createCollectionsInTransaction",
				Message:   fmt.Sprintf("for '%s'", data.entry.Name),
				Cause:     errors.Join(ErrFailedToPersistRegistryEntry, err),
			}
		}
		createdEntries = append(createdEntries, result)
	}

	return createdEntries, nil
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
			return nil, &RegistryError{
			Operation: "PruneVersion",
			Message:   fmt.Sprintf("version '%s' for collection '%s'", version, name),
			Cause:     ErrCannotPruneActiveVersion,
		}
		}

		return r.executeWithEntryUpdate(ctx, name, entry, func(collection base.Collection, manager query.SchemaManager, entry *RegistryEntry) error {
			// Drop the physical collection
			if err := manager.DropCollection(ctx, versionRecord.Physical); err != nil {
				return &RegistryError{
					Operation: "PruneVersion",
					Message:   fmt.Sprintf("%s", versionRecord.Physical),
					Cause:     errors.Join(ErrFailedToDropPhysicalCollection, err),
				}
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
		return cached, nil
	}

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
		return nil, &RegistryError{
			Operation: "loadFromDatabase",
			Message:   fmt.Sprintf("'%s'", name),
			Cause:     errors.Join(ErrFailedToQueryRegistryCollection, err),
		}
	}

	if result.Count == 0 {
		return nil, ErrCollectionNotFound
	}

	if result.Count > 1 {
		return nil, &RegistryError{
			Operation: "loadFromDatabase",
			Message:   fmt.Sprintf("'%s'", name),
			Cause:     ErrMultipleRegistryEntriesFound,
		}
	}

	row := result.Data.(data.Document)
	return unmarshalEntry(row)
}

// AddSchemaVersion introduces a new version of a schema for an existing collection.
func (r *collectionRegistry) AddSchemaVersion(ctx context.Context, name, version string, schema *schema.SchemaDefinition, physicalName ...string) (*RegistryEntry, error) {
	enrichedSchema := EnrichSchema(schema)
	entry, err := r.withSchemaValidationAndEntryExists(ctx, name, enrichedSchema, func(entry *RegistryEntry) (*RegistryEntry, error) {
		// Check if the version already exists
		if _, exists := entry.Versions[version]; exists {
			return nil, &RegistryError{
			Operation: "withSchemaValidationAndEntryExists",
			Message:   fmt.Sprintf("version '%s' for collection '%s'", version, name),
			Cause:     ErrVersionAlreadyExists,
		}
		}

		// Determine physical name
		actualPhysicalName, err := r.resolvePhysicalName(schema, physicalName...)
		if err != nil {
			return nil, err
		}

		return r.executeWithEntryUpdate(ctx, name, entry, func(collection base.Collection, manager query.SchemaManager, entry *RegistryEntry) error {
			// Create physical collection
			if err := r.createPhysicalCollection(ctx, manager, enrichedSchema, actualPhysicalName); err != nil {
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
			return nil, &RegistryError{
			Operation: "SetActiveVersion",
			Message:   fmt.Sprintf("'%s'", name),
			Cause:     ErrVersionAlreadyActive,
		}
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
			return entries, nil
		}
		return nil, &RegistryError{
			Operation: "List",
			Message:   ErrFailedToListCollections.Error(),
			Cause:     errors.Join(ErrFailedToListCollections, err),
		}
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
		return nil, &RegistryError{
			Operation: "loadAllFromDatabase",
			Message:   ErrFailedToListCollections.Error(),
			Cause:     errors.Join(ErrFailedToListCollections, err),
		}
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
			return nil, &RegistryError{
			Operation: "loadAllFromDatabase",
			Message:   ErrFailedToUnmarshalRegistryEntry.Error(),
			Cause:     errors.Join(ErrFailedToUnmarshalRegistryEntry, err),
		}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Higher-order functions for common patterns

func (r *collectionRegistry) withSchemaValidationAndNotExists(ctx context.Context, schema *schema.SchemaDefinition, fn func() (*RegistryEntry, error)) (*RegistryEntry, error) {
	// Validate the schema
	if err := schema.Validate(); err != nil {
		return nil, &RegistryError{
			Operation: "withSchemaValidationAndNotExists",
			Message:   fmt.Sprintf("invalid schema '%s', details: %v", schema.Name, err),
			Cause:     errors.Join(base.ErrInvalidSchema, err),
		}
	}

	// Check if a collection with the same name already exists
	exists, err := r.GetRegistryEntry(ctx, schema.Name)
	if exists != nil {
		return nil, &RegistryError{
			Operation: "withSchemaValidationAndNotExists",
			Message:   fmt.Sprintf("'%s'", schema.Name),
			Cause:     ErrCollectionAlreadyExists,
		}
	}
	if !errors.Is(err, ErrCollectionNotFound) {
		return nil, &RegistryError{
			Operation: "withSchemaValidationAndNotExists",
			Message:   ErrFailedToCheckRegistryExistence.Error(),
			Cause:     errors.Join(ErrFailedToCheckRegistryExistence, err),
		}
	}

	return fn()
}

func (r *collectionRegistry) withSchemaValidationAndEntryExists(ctx context.Context, name string, schema *schema.SchemaDefinition, fn func(entry *RegistryEntry) (*RegistryEntry, error)) (*RegistryEntry, error) {
	// Validate the schema
	if err := schema.Validate(); err != nil {
		return nil, &RegistryError{
			Operation: "withSchemaValidationAndEntryExists",
			Message:   fmt.Sprintf("%s: %s", base.ErrInvalidSchema.Error(), schema.Name),
			Cause:     errors.Join(base.ErrInvalidSchema, err),
		}
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
		return nil, &RegistryError{
			Operation: "withEntryAndVersionValidation",
			Message:   fmt.Sprintf("version '%s' not found for collection '%s'", version, name),
			Cause:     ErrVersionNotFoundForCollection,
		}
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
		return nil, &RegistryError{
			Operation: "withEntryAndOptionalVersion",
			Message:   fmt.Sprintf("version '%s' not found for collection '%s'", resolvedVersion, name),
			Cause:     ErrVersionNotFoundForCollection,
		}
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

func (r *collectionRegistry) createPhysicalCollection(ctx context.Context, manager query.SchemaManager, schema *schema.SchemaDefinition, physicalName string) error {
	tempSchema := *schema
	tempSchema.Name = physicalName

	if err := manager.CreateCollection(ctx, tempSchema); err != nil {
		return &RegistryError{
			Operation: "createPhysicalCollection",
			Message:   base.ErrCollectionCreation.Error(),
			Cause:     errors.Join(base.ErrCollectionCreation, err),
		}
	}
	return nil
}

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

func (r *collectionRegistry) resolvePhysicalName(schema *schema.SchemaDefinition, physicalName ...string) (string, error) {
	if len(physicalName) > 0 {
		return physicalName[0], nil
	}

	actualPhysicalName, err := generatePhysicalName(schema)
	if err != nil {
		return "", &RegistryError{
			Operation: "resolvePhysicalName",
			Message:   fmt.Sprintf("for '%s v%s'", schema.Name, schema.Version),
			Cause:     errors.Join(ErrFailedToGeneratePhysicalName, err),
		}
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
	} else {
		r.cache.delete(name)
	}
}

// RefreshCache manually triggers a cache refresh
func (r *collectionRegistry) RefreshCache() error {
	return r.refreshCache()
}
