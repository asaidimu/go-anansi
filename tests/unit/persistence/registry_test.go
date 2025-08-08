package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	persistence "github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v6/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Test Setup Helper ---

func newTestSchema(name ...string) *schema.SchemaDefinition {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	return &schema.SchemaDefinition{
		Name:        sname,
		Version:     "1.0.0",
		Description: "test collection",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: "string", Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"name": {Name: "name", Type: "string", Required: utils.BoolPtr(true)},
		},
	}
}

// setupTestEnv creates a fully functional, in-memory test environment, returning the
// registry to be tested and the schema manager for verifying physical state.
func setupTestEnv(t *testing.T) (base.CollectionRegistry, query.SchemaManager, persistence.Collection) {
	logger := zap.NewNop()
	interactor := ephemeral.NewEphemeral()
	schemaManager := interactor.SchemaManager()
	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())
	engine := query.NewQueryEngine(interactor, logger)

	registrySchemaDef := registry.RegistrySchema()

	registryCollection, err := collection.NewCollection(
		bus, registry.REGISTRY_COLLECTION_NAME, registrySchemaDef, engine, logger, &base.MetadataOptions{
			HmacSecretKey: []byte("test-secret"),
		}, nil,
	)

	require.NoError(t, err)

	// Create the executor that provides transactional coordination
	executor := func(ctx context.Context, transaction bool, fn func(collection base.Collection, manager query.SchemaManager) (any, error)) (any, error) {
		if transaction {
			tx, err := interactor.StartTransaction(ctx)
			if err != nil {
				return nil, err
			}

			ix := tx.(query.BaseDatabaseInteractor)
			engine := query.NewQueryEngine(ix, logger)

			collection, err := collection.NewCollection(
				bus, registry.REGISTRY_COLLECTION_NAME, registrySchemaDef, engine, logger, &base.MetadataOptions{
					HmacSecretKey: []byte("test-secret"),
				},
				nil,
			)

			if err != nil {
				return nil, err
			}

			result, err := fn(collection, tx.SchemaManager())
			if err != nil {
				tx.Rollback(ctx)
				return nil, err
			}
			tx.Commit(ctx)
			return result, err
		}
		return fn(registryCollection, schemaManager)
	}

	cr, err := registry.NewCollectionRegistry(executor, logger)
	require.NoError(t, err)

	return cr, schemaManager, registryCollection
}

// --- Tests ---

func TestNewCollectionRegistry(t *testing.T) {
	t.Run("Bootstraps successfully and creates the schema collection", func(t *testing.T) {
		_, schemaManager, _ := setupTestEnv(t)

		// Verify using the public API of the SchemaManager
		exists, err := schemaManager.CollectionExists(registry.REGISTRY_COLLECTION_NAME)
		require.NoError(t, err)
		assert.True(t, exists, "The physical '_schemas_' collection should have been created")
	})
}

func TestCreateCollection(t *testing.T) {
	ctx := context.Background()
	sampleSchema := newTestSchema("test_coll")

	t.Run("Success case", func(t *testing.T) {
		cr, schemaManager, _ := setupTestEnv(t)

		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		assert.NotNil(t, entry)

		activeVersion, ok := entry.Versions[entry.ActiveVersion]
		assert.True(t, ok)

		physicalName := activeVersion.Physical

		// Verify physical collection was created via public API
		entry, err = cr.GetRegistryEntry(context.Background(), sampleSchema.Name)
		exists, err := schemaManager.CollectionExists(physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "The physical collection should have been created")

		// Verify registry entry was created via public API
		regEntry, err := cr.GetRegistryEntry(ctx, entry.Name)
		require.NoError(t, err)
		assert.Equal(t, sampleSchema.Name, regEntry.Name)
		assert.Equal(t, "1.0.0", regEntry.ActiveVersion)
		assert.Equal(t, physicalName, regEntry.Versions["1.0.0"].Physical)
	})

	t.Run("Fails if collection already exists", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.CreateCollection(ctx, sampleSchema) // First time succeeds
		require.NoError(t, err)

		_, err = cr.CreateCollection(ctx, sampleSchema) // Second time fails
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestDropCollection(t *testing.T) {
	ctx := context.Background()
	sampleSchema := newTestSchema("test_coll_to_drop")

	t.Run("Successfully drops collection and its physical counterpart", func(t *testing.T) {
		cr, schemaManager, _ := setupTestEnv(t)

		// Create the collection first
		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, entry)

		physicalName := entry.Versions[entry.ActiveVersion].Physical

		// Verify physical collection exists before dropping
		exists, err := schemaManager.CollectionExists(physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection should exist before drop")

		// Drop the collection
		err = cr.DropCollection(ctx, sampleSchema.Name, base.DropCollectionOptions{DeletePhysicalData: true})
		require.NoError(t, err)

		// Verify registry entry is gone
		_, err = cr.GetRegistryEntry(ctx, sampleSchema.Name)
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound, "Registry entry should be gone")

		// Verify physical collection is gone
		exists, err = schemaManager.CollectionExists(physicalName)
		require.NoError(t, err)
		assert.False(t, exists, "Physical collection should be dropped")
	})

	t.Run("Successfully drops collection but keeps physical counterpart", func(t *testing.T) {
		cr, schemaManager, _ := setupTestEnv(t)

		// Create the collection first
		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, entry)

		physicalName := entry.Versions[entry.ActiveVersion].Physical

		// Verify physical collection exists before dropping
		exists, err := schemaManager.CollectionExists(physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection should exist before drop")

		// Drop the collection without dropping physical
		err = cr.DropCollection(ctx, sampleSchema.Name, base.DropCollectionOptions{DeletePhysicalData: false})
		require.NoError(t, err)

		// Verify registry entry is gone
		_, err = cr.GetRegistryEntry(ctx, sampleSchema.Name)
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound, "Registry entry should be gone")

		// Verify physical collection still exists
		exists, err = schemaManager.CollectionExists(physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection should NOT be dropped")
	})

	t.Run("Returns ErrCollectionNotFound if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		err := cr.DropCollection(ctx, "non_existent_collection", base.DropCollectionOptions{DeletePhysicalData: true})
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound)
	})
}

func TestPruneVersion(t *testing.T) {
	ctx := context.Background()
	sampleSchema := newTestSchema("test_coll_prune")

	t.Run("Successfully prunes a non-active version", func(t *testing.T) {
		cr, schemaManager, _ := setupTestEnv(t)

		// Create the initial collection
		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, entry)

		// Add a new version
		newSchema := *sampleSchema
		newSchema.Version = "1.1.0"
		newSchema.Fields["new_field"] = &schema.FieldDefinition{Name: "new_field", Type: "string"}
		updatedEntry, err := cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", &newSchema)
		require.NoError(t, err)
		require.NotNil(t, updatedEntry)

		// Set the new version as active
		finalEntry, err := cr.SetActiveVersion(ctx, sampleSchema.Name, "1.1.0")
		require.NoError(t, err)
		require.NotNil(t, finalEntry)

		// Verify both physical collections exist
		oldPhysicalName := entry.Versions["1.0.0"].Physical
		newPhysicalName := finalEntry.Versions["1.1.0"].Physical

		oldPhysicalExists, err := schemaManager.CollectionExists(oldPhysicalName)
		require.NoError(t, err)
		assert.True(t, oldPhysicalExists, "Old physical collection should exist")

		newPhysicalExists, err := schemaManager.CollectionExists(newPhysicalName)
		require.NoError(t, err)
		assert.True(t, newPhysicalExists, "New physical collection should exist")

		// Prune the old version
		prunedEntry, err := cr.PruneVersion(ctx, sampleSchema.Name, "1.0.0")
		require.NoError(t, err)
		require.NotNil(t, prunedEntry)

		// Verify old version is removed from registry entry
		_, ok := prunedEntry.Versions["1.0.0"]
		assert.False(t, ok, "Old version should be removed from registry entry")

		// Verify old physical collection is gone
		exists, err := schemaManager.CollectionExists(oldPhysicalName)
		require.NoError(t, err)
		assert.False(t, exists, "Old physical collection should be pruned")

		// Verify new physical collection still exists
		exists, err = schemaManager.CollectionExists(newPhysicalName)
		require.NoError(t, err)
		assert.True(t, exists, "New physical collection should still exist")
	})

	t.Run("Fails to prune active version", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, entry)

		// Attempt to prune the active version
		_, err = cr.PruneVersion(ctx, sampleSchema.Name, entry.ActiveVersion)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot prune active version")
	})

	t.Run("Fails to prune non-existent version", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to prune a non-existent version
		_, err = cr.PruneVersion(ctx, sampleSchema.Name, "9.9.9")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version '9.9.9' not found")
	})

	t.Run("Returns ErrCollectionNotFound if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.PruneVersion(ctx, "non_existent_collection", "1.0.0")
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound)
	})
}

func TestGetRegistryEntry(t *testing.T) {
	ctx := context.Background()
	sampleSchema := newTestSchema("test_get_registry_entry")

	t.Run("Successfully retrieves registry entry", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the collection
		createdEntry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, createdEntry)

		// Get the registry entry
		retrievedEntry, err := cr.GetRegistryEntry(ctx, sampleSchema.Name)
		require.NoError(t, err)
		assert.NotNil(t, retrievedEntry)
		assert.Equal(t, createdEntry.Name, retrievedEntry.Name)
		assert.Equal(t, createdEntry.ActiveVersion, retrievedEntry.ActiveVersion)
		assert.Equal(t, createdEntry.Versions[createdEntry.ActiveVersion].Physical, retrievedEntry.Versions[retrievedEntry.ActiveVersion].Physical)
	})

	t.Run("Returns ErrCollectionNotFound if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.GetRegistryEntry(ctx, "non_existent_collection")
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound)
	})
}

func TestAddSchemaVersion(t *testing.T) {
	ctx := context.Background()
	sampleSchema := newTestSchema("test_add_version")

	t.Run("Successfully adds a new schema version", func(t *testing.T) {
		cr, schemaManager, _ := setupTestEnv(t)

		// Create the initial collection
		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, entry)

		// Add a new version
		newSchema := *sampleSchema
		newSchema.Version = "1.1.0"
		newSchema.Fields["new_field"] = &schema.FieldDefinition{Name: "new_field", Type: "string"}
		updatedEntry, err := cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", &newSchema)
		require.NoError(t, err)
		require.NotNil(t, updatedEntry)

		// Verify the new version exists in the registry entry
		_, ok := updatedEntry.Versions["1.1.0"]
		assert.True(t, ok, "New version should be present in registry entry")
		assert.Equal(t, 2, len(updatedEntry.Versions), "Registry entry should have two versions")

		// Verify the physical collection for the new version was created
		newPhysicalName := updatedEntry.Versions["1.1.0"].Physical
		exists, err := schemaManager.CollectionExists(newPhysicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection for new version should be created")
	})

	t.Run("Fails if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		newSchema := *sampleSchema
		newSchema.Version = "1.1.0"
		_, err := cr.AddSchemaVersion(ctx, "non_existent_collection", "1.1.0", &newSchema)
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound)
	})

	t.Run("Fails if version already exists", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to add the same version again
		_, err = cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.0.0", sampleSchema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("Fails if new schema is invalid", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Create an invalid new schema (e.g., missing required fields)
		invalidSchema := &schema.SchemaDefinition{
			Name:   sampleSchema.Name,
			Fields: map[string]*schema.FieldDefinition{},
		}
		_, err = cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", invalidSchema)
		assert.Error(t, err)
	})
}

func TestSetActiveVersion(t *testing.T) {
	ctx := context.Background()
	sampleSchema := newTestSchema("test_set_active_version")

	t.Run("Successfully sets a new active version", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Add a new version
		newSchema := *sampleSchema
		newSchema.Version = "1.1.0"
		_, err = cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", &newSchema)
		require.NoError(t, err)

		// Set the new version as active
		updatedEntry, err := cr.SetActiveVersion(ctx, sampleSchema.Name, "1.1.0")
		require.NoError(t, err)
		assert.NotNil(t, updatedEntry)
		assert.Equal(t, "1.1.0", updatedEntry.ActiveVersion, "Active version should be updated")

		// Verify the change is persisted by retrieving the entry again
		retrievedEntry, err := cr.GetRegistryEntry(ctx, sampleSchema.Name)
		require.NoError(t, err)
		assert.Equal(t, "1.1.0", retrievedEntry.ActiveVersion, "Persisted active version should be updated")
	})

	t.Run("Fails if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.SetActiveVersion(ctx, "non_existent_collection", "1.0.0")
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound)
	})

	t.Run("Fails if version does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to set a non-existent version as active
		_, err = cr.SetActiveVersion(ctx, sampleSchema.Name, "9.9.9")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version '9.9.9' not found")
	})

	t.Run("Fails if version is already active", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to set the current active version again
		_, err = cr.SetActiveVersion(ctx, sampleSchema.Name, "1.0.0")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already the active version")
	})
}

func TestList(t *testing.T) {
	ctx := context.Background()

	t.Run("Successfully lists all registry entries", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create multiple collections
		_, err := cr.CreateCollection(ctx, newTestSchema("collection_a"))
		require.NoError(t, err)
		_, err = cr.CreateCollection(ctx, newTestSchema("collection_b"))
		require.NoError(t, err)
		_, err = cr.CreateCollection(ctx, newTestSchema("collection_c"))
		require.NoError(t, err)

		// List all entries
		entries, err := cr.List(ctx)
		require.NoError(t, err)
		assert.Len(t, entries, 3, "Should retrieve all three collections")

		// Verify names (order is not guaranteed, so check for presence)
		foundNames := make(map[string]bool)
		for _, entry := range entries {
			foundNames[entry.Name] = true
		}
		assert.True(t, foundNames["collection_a"])
		assert.True(t, foundNames["collection_b"])
		assert.True(t, foundNames["collection_c"])
	})

	ctx = context.Background()
	sampleSchema := newTestSchema("test_get_schema")

	t.Run("Successfully retrieves active schema", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Get the active schema
		retrievedSchema, err := cr.GetSchema(ctx, sampleSchema.Name)
		require.NoError(t, err)
		assert.NotNil(t, retrievedSchema)
		assert.Equal(t, sampleSchema.Version, retrievedSchema.Version)
	})

	t.Run("Successfully retrieves specific version schema", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Add a new version
		newSchema := *sampleSchema
		newSchema.Version = "1.1.0"
		newSchema.Fields["new_field"] = &schema.FieldDefinition{Name: "new_field", Type: "string"}
		_, err = cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", &newSchema)
		require.NoError(t, err)

		// Get the specific version schema
		retrievedSchema, err := cr.GetSchema(ctx, sampleSchema.Name, "1.1.0")
		require.NoError(t, err)
		assert.NotNil(t, retrievedSchema)
		assert.Equal(t, "1.1.0", retrievedSchema.Version)
		assert.NotNil(t, retrievedSchema.Fields["new_field"])
	})

	t.Run("Returns ErrCollectionNotFound if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.GetSchema(ctx, "non_existent_collection")
		assert.ErrorIs(t, err, registry.ErrCollectionNotFound)
	})

	t.Run("Returns error if specific version not found", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to get a non-existent version
		_, err = cr.GetSchema(ctx, sampleSchema.Name, "9.9.9")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version '9.9.9' not found")
	})
}
