package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	cevents "github.com/asaidimu/go-anansi/v7/core/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	persistence "github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/collection"
	pevents "github.com/asaidimu/go-anansi/v7/core/persistence/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v7/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/tests/testutils"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Test Setup Helper ---

func newTestSchema(name ...string) *definition.Schema {
	sname := "test_collection"
	if len(name) > 0 {
		sname = name[0]
	}
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name:        sname,
			Description: "test collection",
			Fields: map[definition.FieldId]definition.Field{
				"name": {
					Name: "name",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
					Required: true,
				},
			},
		},
	}
}

// setupTestEnv creates a fully functional, in-memory test environment, returning the
// registry to be tested and the schema manager for verifying physical state.
func setupTestEnv(t *testing.T) (base.CollectionRegistry, query.SchemaManager, persistence.Collection) {
	logger := zap.NewNop()

	// Configure the document factory
	testutils.ConfigureDocumentFactory()

	interactor := ephemeral.NewEphemeral()

	schemaManager := interactor.SchemaManager()

	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())

	engine := query.NewQueryEngine(interactor.Capabilities(), logger)

	registrySchemaDef := registry.RegistrySchema()

	factory := pevents.NewPersistenceEventFactory(registry.REGISTRY_COLLECTION_NAME, logger)
	eventEmitter := cevents.NewEventEmitter(pevents.NewGoEventsBusAdapter(bus), factory.CreateEvent, logger)
	registryCollection, err := collection.NewCollection(
		eventEmitter, registry.REGISTRY_COLLECTION_NAME, registrySchemaDef, interactor, engine, logger, nil, nil,
	)

	require.NoError(t, err)

	// Create the executor that provides transactional coordination
	executor := func(ctx context.Context, transact bool, fn func(ctx context.Context, collection base.Collection, manager query.SchemaManager) (any, error)) (any, error) {

		if transact {

			return transaction.Execute(ctx, interactor, logger, func(tctx context.Context, tx query.DatabaseInteractor) (any, error) {
				return fn(tctx, registryCollection, tx.SchemaManager())
			})
		}

		return fn(ctx, registryCollection, schemaManager)
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
		exists, err := schemaManager.CollectionExists(context.Background(), registry.REGISTRY_COLLECTION_NAME)
		require.NoError(t, err)
		assert.True(t, exists, "The physical '_schemas_' collection should have been created")
	})
}

func TestCreateCollection(t *testing.T) {
	sampleSchema := newTestSchema("test_coll")
	ctx := context.Background()

	t.Run("Success case", func(t *testing.T) {
		cr, schemaManager, _ := setupTestEnv(t)

		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		assert.NotNil(t, entry)

		activeVersion, ok := entry.Versions[entry.ActiveVersion.String()]
		assert.True(t, ok)

		physicalName := activeVersion.Physical

		// Verify physical collection was created via public API
		_, err = cr.GetRegistryEntry(context.Background(), sampleSchema.Name)
		require.NoError(t, err)
		exists, err := schemaManager.CollectionExists(context.Background(), physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "The physical collection should have been created")

		// Verify registry entry was created via public API
		regEntry, err := cr.GetRegistryEntry(ctx, entry.Name)
		require.NoError(t, err)
		assert.Equal(t, sampleSchema.Name, regEntry.Name)
		assert.Equal(t, "1.0.0", regEntry.ActiveVersion.String())
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

		physicalName := entry.Versions[entry.ActiveVersion.String()].Physical

		// Verify physical collection exists before dropping
		exists, err := schemaManager.CollectionExists(context.Background(), physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection should exist before drop")

		// Drop the collection
		err = cr.DropCollection(ctx, sampleSchema.Name, base.DropCollectionOptions{DeletePhysicalData: true})
		require.NoError(t, err)

		// Verify registry entry is gone
		_, err = cr.GetRegistryEntry(ctx, sampleSchema.Name)
		assert.ErrorIs(t, err, base.ErrCollectionNotFound, "Registry entry should be gone")

		// Verify physical collection is gone
		exists, err = schemaManager.CollectionExists(context.Background(), physicalName)
		require.NoError(t, err)
		assert.False(t, exists, "Physical collection should be dropped")
	})

	t.Run("Successfully drops collection but keeps physical counterpart", func(t *testing.T) {
		cr, schemaManager, _ := setupTestEnv(t)

		// Create the collection first
		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, entry)

		physicalName := entry.Versions[entry.ActiveVersion.String()].Physical

		// Verify physical collection exists before dropping
		exists, err := schemaManager.CollectionExists(context.Background(), physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection should exist before drop")

		// Drop the collection without dropping physical
		err = cr.DropCollection(ctx, sampleSchema.Name, base.DropCollectionOptions{DeletePhysicalData: false})
		require.NoError(t, err)

		// Verify registry entry is gone
		_, err = cr.GetRegistryEntry(ctx, sampleSchema.Name)
		assert.ErrorIs(t, err, base.ErrCollectionNotFound, "Registry entry should be gone")

		// Verify physical collection still exists
		exists, err = schemaManager.CollectionExists(context.Background(), physicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection should NOT be dropped")
	})

	t.Run("Returns ErrCollectionNotFound if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		err := cr.DropCollection(ctx, "non_existent_collection", base.DropCollectionOptions{DeletePhysicalData: true})
		assert.ErrorIs(t, err, base.ErrCollectionNotFound)
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
		newSchema.Version = common.MustNewVersion("1.1.0")
		newSchema.BaseSchema.Fields["new_field"] = definition.Field{
			Name: "new_field",
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
		}
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

		oldPhysicalExists, err := schemaManager.CollectionExists(context.Background(), oldPhysicalName)
		require.NoError(t, err)
		assert.True(t, oldPhysicalExists, "Old physical collection should exist")

		newPhysicalExists, err := schemaManager.CollectionExists(context.Background(), newPhysicalName)
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
		exists, err := schemaManager.CollectionExists(context.Background(), oldPhysicalName)
		require.NoError(t, err)
		assert.False(t, exists, "Old physical collection should be pruned")

		// Verify new physical collection still exists
		exists, err = schemaManager.CollectionExists(context.Background(), newPhysicalName)
		require.NoError(t, err)
		assert.True(t, exists, "New physical collection should still exist")
	})

	t.Run("Fails to prune active version", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		entry, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)
		require.NotNil(t, entry)

		_, err = cr.PruneVersion(ctx, sampleSchema.Name, entry.ActiveVersion.String())
		assert.Error(t, err)
		var sysErr *common.SystemError
		assert.True(t, errors.As(err, &sysErr))
		assert.Equal(t, "ERR_REGISTRY_CANNOT_PRUNE_ACTIVE_VERSION", sysErr.Code)
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
		assert.ErrorIs(t, err, base.ErrCollectionNotFound)
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
		assert.Equal(t, createdEntry.ActiveVersion.String(), retrievedEntry.ActiveVersion.String())
		assert.Equal(t, createdEntry.Versions[createdEntry.ActiveVersion.String()].Physical, retrievedEntry.Versions[retrievedEntry.ActiveVersion.String()].Physical)
	})

	t.Run("Returns ErrCollectionNotFound if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.GetRegistryEntry(ctx, "non_existent_collection")
		assert.ErrorIs(t, err, base.ErrCollectionNotFound)
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
		newSchema.Version = common.MustNewVersion("1.1.0")
		newSchema.BaseSchema.Fields["new_field"] = definition.Field{
			Name: "new_field",
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
		}
		updatedEntry, err := cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", &newSchema)
		require.NoError(t, err)
		require.NotNil(t, updatedEntry)

		// Verify the new version exists in the registry entry
		_, ok := updatedEntry.Versions["1.1.0"]
		assert.True(t, ok, "New version should be present in registry entry")
		assert.Equal(t, 2, len(updatedEntry.Versions), "Registry entry should have two versions")

		// Verify the physical collection for the new version was created
		newPhysicalName := updatedEntry.Versions["1.1.0"].Physical
		exists, err := schemaManager.CollectionExists(context.Background(), newPhysicalName)
		require.NoError(t, err)
		assert.True(t, exists, "Physical collection for new version should be created")
	})

	t.Run("Fails if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		newSchema := *sampleSchema
		newSchema.Version = common.MustNewVersion("1.1.0")
		_, err := cr.AddSchemaVersion(ctx, "non_existent_collection", "1.1.0", &newSchema)
		assert.ErrorIs(t, err, base.ErrCollectionNotFound)
	})

	t.Run("Fails if version already exists", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to add the same version again
		_, err = cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.0.0", sampleSchema)
		assert.Error(t, err)
		var sysErr *common.SystemError
		assert.True(t, errors.As(err, &sysErr))
		assert.Equal(t, "ERR_PERSISTENCE_VERSION_ALREADY_EXISTS", sysErr.Code)
	})

	t.Run("Fails if new schema is invalid", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Create an invalid new schema (e.g., missing required fields)
		invalidSchema := &definition.Schema{
			BaseSchema: definition.BaseSchema{
				Name:   sampleSchema.Name,
				Fields: map[definition.FieldId]definition.Field{},
			},
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
		newSchema.Version = common.MustNewVersion("1.1.0")
		_, err = cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", &newSchema)
		require.NoError(t, err)

		// Set the new version as active
		updatedEntry, err := cr.SetActiveVersion(ctx, sampleSchema.Name, "1.1.0")
		require.NoError(t, err)
		assert.NotNil(t, updatedEntry)
		assert.Equal(t, "1.1.0", updatedEntry.ActiveVersion.String(), "Active version should be updated")

		// Verify the change is persisted by retrieving the entry again
		retrievedEntry, err := cr.GetRegistryEntry(ctx, sampleSchema.Name)
		require.NoError(t, err)
		assert.Equal(t, "1.1.0", retrievedEntry.ActiveVersion.String(), "Persisted active version should be updated")
	})

	t.Run("Fails if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.SetActiveVersion(ctx, "non_existent_collection", "1.0.0")
		assert.ErrorIs(t, err, base.ErrCollectionNotFound)
	})

	t.Run("Fails if version does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to set a non-existent version as active
		_, err = cr.SetActiveVersion(ctx, sampleSchema.Name, "9.9.9")
		assert.Error(t, err)
		var sysErr *common.SystemError
		assert.True(t, errors.As(err, &sysErr))
		assert.Equal(t, "ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", sysErr.Code)
	})

	t.Run("Fails if version is already active", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to set the current active version again
		_, err = cr.SetActiveVersion(ctx, sampleSchema.Name, "1.0.0")
		assert.Error(t, err)
		var sysErr *common.SystemError
		assert.True(t, errors.As(err, &sysErr))
		assert.Equal(t, "ERR_REGISTRY_VERSION_ALREADY_ACTIVE", sysErr.Code)
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
		assert.Equal(t, sampleSchema.Version.String(), retrievedSchema.Version.String())
	})

	t.Run("Successfully retrieves specific version schema", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the initial collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Add a new version
		newSchema := *sampleSchema
		newSchema.Version = common.MustNewVersion("1.1.0")
		newSchema.BaseSchema.Fields["new_field"] = definition.Field{
			Name: "new_field",
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
		}
		_, err = cr.AddSchemaVersion(ctx, sampleSchema.Name, "1.1.0", &newSchema)
		require.NoError(t, err)

		// Get the specific version schema
		retrievedSchema, err := cr.GetSchema(ctx, sampleSchema.Name, "1.1.0")
		require.NoError(t, err)
		assert.NotNil(t, retrievedSchema)
		assert.Equal(t, "1.1.0", retrievedSchema.Version.String())
		_, exists := retrievedSchema.BaseSchema.Fields["new_field"]
		assert.True(t, exists)
	})

	t.Run("Returns ErrCollectionNotFound if collection does not exist", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)
		_, err := cr.GetSchema(ctx, "non_existent_collection")
		assert.ErrorIs(t, err, base.ErrCollectionNotFound)
	})

	t.Run("Returns error if specific version not found", func(t *testing.T) {
		cr, _, _ := setupTestEnv(t)

		// Create the collection
		_, err := cr.CreateCollection(ctx, sampleSchema)
		require.NoError(t, err)

		// Attempt to get a non-existent version
		_, err = cr.GetSchema(ctx, sampleSchema.Name, "9.9.9")
		assert.Error(t, err)
		var sysErr *common.SystemError
		assert.True(t, errors.As(err, &sysErr))
		assert.Equal(t, "ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", sysErr.Code)
	})
}
