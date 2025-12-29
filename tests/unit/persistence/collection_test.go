package persistence_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	cevents "github.com/asaidimu/go-anansi/v6/core/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	persistence "github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	pevents "github.com/asaidimu/go-anansi/v6/core/persistence/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	testutils.ConfigureDocumentFactory()
	os.Exit(m.Run())
}

// Helper to create a basic schema definition
func testSchema(name ...string) *schema.SchemaDefinition {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	return &schema.SchemaDefinition{
		Name:        sname,
		Version:     "1.0.0",
		Description: "test collection",
		Fields: map[string]*schema.FieldDefinition{
			"name":   {Name: "name", Type: "string", Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"status": {Name: "status", Type: "string"},
		},
	}
}

func resolveSchema(ctx context.Context, logicalName string) (string, *schema.SchemaDefinition, error) {
	return logicalName, nil, nil
}

// setupCollectionTest is a helper function to set up a new collection for testing.
func setupCollection(t *testing.T) (base.Collection, query.DatabaseInteractor, *zap.Logger, *schema.SchemaDefinition, *events.TypedEventBus[persistence.PersistenceEvent], context.Context) {
	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())
	ephemeralInteractor := ephemeral.NewEphemeral()
	manager := ephemeralInteractor.SchemaManager()
	logger := zap.NewNop()
	testSchemaDef := registry.EnrichSchema(testSchema())

	validator, err := schema.NewDocumentValidator(testSchemaDef, nil)
	assert.NoError(t, err)
	expected := data.MustNewDocument(map[string]any{"name": "Test1"})
	_, ok := validator.Validate(expected.AsMap(), false)
	assert.True(t, ok)

	err = manager.CreateCollection(context.Background(), *testSchemaDef)
	assert.NoError(t, err)

	engine := query.NewQueryEngine(ephemeralInteractor.Capabilities(), logger)
	factory := pevents.NewPersistenceEventFactory(testSchemaDef.Name, logger)
	eventEmitter := cevents.NewEventEmitter(pevents.NewGoEventsBusAdapter(bus), factory.CreateEvent, logger)
	c, err := collection.NewCollection(eventEmitter, testSchemaDef.Name, testSchemaDef, ephemeralInteractor, engine, logger, resolveSchema, nil)
	assert.NoError(t, err)

	ctx := context.Background()
	return c, ephemeralInteractor, logger, testSchemaDef, bus, ctx
}

// setupNonExistentCollection is a helper function to set up a collection with a non-existent schema for testing error cases.
func setupNonExistentCollection() (base.Collection, query.DatabaseInteractor, *zap.Logger, *schema.SchemaDefinition, *events.TypedEventBus[persistence.PersistenceEvent], context.Context) {
	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())
	nonExistentSchema := &schema.SchemaDefinition{Name: "non_existent"}
	ephemeralInteractor := ephemeral.NewEphemeral()
	logger := zap.NewNop()
	engine := query.NewQueryEngine(ephemeralInteractor.Capabilities(), logger)
	factory := pevents.NewPersistenceEventFactory(nonExistentSchema.Name, logger)
	eventEmitter := cevents.NewEventEmitter(pevents.NewGoEventsBusAdapter(bus), factory.CreateEvent, logger)
	c, _ := collection.NewCollection(eventEmitter, nonExistentSchema.Name, nonExistentSchema, ephemeralInteractor, engine, logger, resolveSchema, nil)
	ctx := context.Background()
	return c, ephemeralInteractor, logger, nonExistentSchema, bus, ctx
}

func TestNewCollection(t *testing.T) {
	collection, _, _, _, _, _ := setupCollection(t)
	assert.NotNil(t, collection)
}

func TestCollection_Create(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	t.Run("single document success", func(t *testing.T) {
		expected := data.Document{"name": "Test2"}

		result, err := collection.CreateOne(ctx, expected)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.IsType(t, persistence.CreateResult{}, result)
		actual := result.Data.StripMetadata()
		assert.Equal(t, actual["name"], expected["name"])

		// Verify the document was actually inserted by reading it back
		readQuery := query.NewQueryBuilder().Where("id").Eq(result.Data.Must().GetString("id")).Build()
		readResult, err := collection.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 1, readResult.Count)
		assert.Equal(t, expected, readResult.Data[0])
	})

	t.Run("multiple documents success", func(t *testing.T) {
		docs := []data.Document{{"name": "Test8"}, {"name": "Test3"}}

		result, err := collection.CreateMany(ctx, docs)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.IsType(t, []persistence.CreateResult{}, result)
		assert.Len(t, result, 2)

		// Verify the documents were actually inserted by reading them back
		readQuery := query.NewQueryBuilder().Where("id").In(
			result[0].Data.Must().GetString("id"),
			result[1].Data.Must().GetString("id")).Build()
		readResult, err := collection.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 2, readResult.Count)
		// Note: Order might not be guaranteed, so we'll just check for presence
		assert.Contains(t, readResult.Data, docs[0])
		assert.Contains(t, readResult.Data, docs[1])
	})

	t.Run("insert documents error - duplicate ID", func(t *testing.T) {
		// Name "Test2" already exists from previous test
		doc := data.Document{"name": "Test2"}
		_, err := collection.CreateOne(ctx, doc)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, persistence.ErrUniqueConstraintViolation))
	})
}

func TestCollection_Read(t *testing.T) {
	c, _, _, _, _, ctx := setupCollection(t)

	// Insert some data for reading
	_, err := c.CreateOne(ctx, data.Document{"name": "Test3"})
	assert.NoError(t, err)
	_, err = c.CreateOne(ctx, data.Document{"name": "Test4"})
	assert.NoError(t, err)

	q := query.NewQueryBuilder().Where("name").Eq("Test3").Build()

	t.Run("read documents success", func(t *testing.T) {
		expected := data.Document{"name": "Test3"}

		result, err := c.Read(ctx, &q)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.Count)

		final := result.Data[0]
		final.StripMetadata()
		assert.Equal(t, expected["name"], final["name"])
	})

	t.Run("read documents error - non-existent collection", func(t *testing.T) {
		// Create a new collection instance with a non-existent schema name
		_, ephemeralInteractor, logger, _, bus, ctx := setupCollection(t)
		nonExistentSchema := &schema.SchemaDefinition{Name: "non_existent"}
		engine := query.NewQueryEngine(ephemeralInteractor.Capabilities(), logger)
		factory := pevents.NewPersistenceEventFactory(nonExistentSchema.Name, logger)
		eventEmitter := cevents.NewEventEmitter(pevents.NewGoEventsBusAdapter(bus), factory.CreateEvent, logger)
		nonExistentCollection, _ := collection.NewCollection(eventEmitter, nonExistentSchema.Name, nonExistentSchema, ephemeralInteractor, engine, logger, resolveSchema, nil)

		result, err := nonExistentCollection.Read(ctx, &q)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "database query execution failed")
	})
}

func TestCollection_Update(t *testing.T) {
	c, _, _, _, _, ctx := setupCollection(t)

	// Insert a document to be updated.
	r, err := c.CreateOne(ctx, data.Document{"name": "OriginalName"})
	require.NoError(t, err)

	q := query.NewQueryBuilder().Where("id").Eq(r.Data.Must().GetString("id")).Build()
	result, err := c.Read(ctx, &q)
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)

	d := result.Data[0]
	updates := data.Document{
		"name": "UpdatedName",
	}

	metadata, ok := d.Metadata()
	require.True(t, ok)
	updates.SetMetadata(metadata)


	t.Run("update document with wrong version should not update", func(t *testing.T) {
		// We need to read the document again to get the latest version
		readResult, err := c.Read(ctx, &q)
		require.NoError(t, err)
		latestVersion, _ := readResult.Data[0].GetInt("_metadata_.version")

		wrongVersion := latestVersion + 10
		updateParamsWrongVersion := &persistence.CollectionUpdate{Set: updates, Filter: q.Filters, Version: &wrongVersion}
		rowsAffected, err := c.Update(ctx, updateParamsWrongVersion)
		assert.NoError(t, err)
		assert.Equal(t, 0, rowsAffected)
	})

	t.Run("update multiple documents without optimistic locking", func(t *testing.T) {
		// Use a fresh collection for this test to avoid conflicts
		c, _, _, _, _, ctx := setupCollection(t)

		// Insert some data for updating
		_, err := c.CreateOne(ctx, data.Document{"name": "UpdateMulti1", "status": "pending"})
		assert.NoError(t, err)
		_, err = c.CreateOne(ctx, data.Document{"name": "UpdateMulti2", "status": "pending"})
		assert.NoError(t, err)

		// Filter for documents to update
		q := query.NewQueryBuilder().Where("status").Eq("pending").Build()

		// Define the update payload. No version is provided, so no optimistic locking.
		updates := data.Document{"status": "done"}
		updateParams := &persistence.CollectionUpdate{Set: updates, Filter: q.Filters, Version: nil} // Explicitly nil

		// Perform the update
		rowsAffected, err := c.Update(ctx, updateParams)
		assert.NoError(t, err)
		assert.Equal(t, 2, rowsAffected)

		// Verify the documents were actually updated
		readQuery := query.NewQueryBuilder().Where("status").Eq("done").Build()
		readResult, err := c.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 2, readResult.Count)

		docs := readResult.Data
		assert.Equal(t, "done", docs[0]["status"])
		assert.Equal(t, "done", docs[1]["status"])
	})

	t.Run("update documents error - non-existent collection", func(t *testing.T) {
		// Create a new collection instance with a non-existent schema name
		nonExistentCollection, _, _, _, _, ctx := setupNonExistentCollection()

		updates := data.Document{"name": "any"}
		updateParams := &persistence.CollectionUpdate{Set: updates, Filter: q.Filters, Version: nil}

		rowsAffected, err := nonExistentCollection.Update(ctx, updateParams)

		assert.Error(t, err)
		assert.Equal(t, 0, rowsAffected)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})
}

func TestCollection_Delete(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	// Insert some data for deleting
	r, err := collection.CreateOne(ctx, data.Document{"name": "ToDelete"})
	assert.NoError(t, err)
	_, err = collection.CreateOne(ctx, data.Document{"name": "ToKeep"})
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("id").Eq(r.Data.Must().GetString("id")).Build().Filters

	t.Run("delete documents success", func(t *testing.T) {
		rowsAffected, err := collection.Delete(ctx, filters, false)

		assert.NoError(t, err)
		assert.Equal(t, 1, rowsAffected)

		// Verify the document was actually deleted
		readQuery := query.NewQueryBuilder().Where("id").Eq(r.Data.Must().GetString("id")).Build()
		readResult, err := collection.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 0, readResult.Count)
	})

	t.Run("delete documents error - non-existent collection", func(t *testing.T) {
		// Create a new collection instance with a non-existent schema name
		nonExistentCollection, _, _, _, _, ctx := setupNonExistentCollection()

		rowsAffected, err := nonExistentCollection.Delete(ctx, filters, false)

		assert.Error(t, err)
		assert.Equal(t, 0, rowsAffected)
		assert.ErrorIs(t, err, base.ErrCollectionNotFound)
	})

	t.Run("delete all documents with unsafe flag", func(t *testing.T) {
		// Re-create collection and add data for this specific test case
		collection, _, _, _, _, ctx = setupCollection(t)
		collection.CreateOne(ctx, data.Document{"name": "Doc3"})
		collection.CreateOne(ctx, data.Document{"name": "Doc4"})

		// Debug: Read documents before deletion
		q := query.NewQueryBuilder().Build()
		_, err := collection.Read(ctx, &q)
		assert.NoError(t, err)

		// Delete all documents by passing nil filter and unsafe=true
		rowsAffected, err := collection.Delete(ctx, nil, true)

		assert.NoError(t, err)
		assert.Equal(t, 2, rowsAffected)

		// Verify all documents are deleted
		readQuery := query.NewQueryBuilder().Build()
		readResult, err := collection.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 0, readResult.Count)
	})

	t.Run("delete all documents without unsafe flag should fail", func(t *testing.T) {
		// Re-create collection and add data for this specific test case
		collection, _, _, _, _, ctx = setupCollection(t)
		collection.CreateOne(ctx, data.Document{"name": "Doc5"})

		// Attempt to delete all documents by passing nil filter and unsafe=false
		rowsAffected, err := collection.Delete(ctx, nil, false)

		assert.Error(t, err)
		assert.Equal(t, 0, rowsAffected)
		assert.Contains(t, err.Error(), "Delete operation requires a filter or unsafe flag")
	})
}

func TestCollection_Validate(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	t.Run("valid document", func(t *testing.T) {
		doc := data.Document{"id": "1", "name": "Valid Name"}
		result, err := collection.Validate(ctx, doc, false)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Issues)
	})

	t.Run("invalid document - missing required field", func(t *testing.T) {
		doc := data.Document{"id": "1"} // Missing 'name'
		result, err := collection.Validate(ctx, doc, false)
		assert.NoError(t, err) // Validation itself should not return an error, but a result with issues
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Issues)
		assert.Contains(t, result.Issues[0].Message, "Required")
	})

	t.Run("invalid document - wrong type", func(t *testing.T) {
		doc := data.Document{"id": "1", "name": 123} // Name should be string
		result, err := collection.Validate(ctx, doc, false)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Issues)
		assert.Contains(t, result.Issues[0].Message, "Expected")
	})

	t.Run("loose validation - missing required field allowed", func(t *testing.T) {
		doc := data.Document{"id": "1"}                    // Missing 'name'
		result, err := collection.Validate(ctx, doc, true) // Loose validation
		assert.NoError(t, err)
		assert.True(t, result.Valid) // Should be valid in loose mode for missing required
		assert.Empty(t, result.Issues)
	})
}

func TestCollection_Metadata(t *testing.T) {
	collection, _, _, testSchema, _, ctx := setupCollection(t)

	t.Run("metadata success", func(t *testing.T) {
		metadata, err := collection.Metadata(ctx, nil, false) // No filter, no force refresh
		assert.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.Equal(t, testSchema.Name, metadata.Name)
	})
}

func TestCollection_RegisterSubscription(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	options := persistence.SubscriptionOptions{
		Event: persistence.DocumentCreateSuccess,
		Label: utils.StringPtr("test_sub"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}

	id := collection.Subscribe(ctx, options)
	assert.NotEmpty(t, id)

	subs, _ := collection.Subscriptions(ctx)
	assert.Len(t, subs, 1)
	assert.Equal(t, *subs[0].Id, id)
}

func TestCollection_UnregisterSubscription(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	options := persistence.SubscriptionOptions{
		Event: persistence.DocumentCreateSuccess,
		Label: utils.StringPtr("test_sub"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}

	id := collection.Subscribe(ctx, options)
	assert.NotEmpty(t, id)

	collection.Unsubscribe(ctx, id)
	subs, _ := collection.Subscriptions(ctx)
	assert.Empty(t, subs)
}

func TestCollection_Subscriptions(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	options1 := persistence.SubscriptionOptions{
		Event: persistence.DocumentCreateSuccess,
		Label: utils.StringPtr("sub1"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}
	options2 := persistence.SubscriptionOptions{
		Event: persistence.DocumentUpdateSuccess,
		Label: utils.StringPtr("sub2"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}

	collection.Subscribe(ctx, options1)
	collection.Subscribe(ctx, options2)

	subs, _ := collection.Subscriptions(ctx)
	assert.Len(t, subs, 2)
	// Check if both subscriptions are present (order might vary)
	foundSub1 := false
	foundSub2 := false
	for _, sub := range subs {
		if *sub.Label == "sub1" {
			foundSub1 = true
		}
		if *sub.Label == "sub2" {
			foundSub2 = true
		}
	}
	assert.True(t, foundSub1)
	assert.True(t, foundSub2)
}

func TestCollection_Capabilities(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	capabilities := collection.Capabilities(ctx)
	assert.NotNil(t, capabilities)
	// Assert some known capabilities from the ephemeral interactor
	assert.True(t, capabilities.SupportsGroupBy)
	assert.True(t, capabilities.SupportsDistinct)
	assert.True(t, capabilities.SupportsNestedFields)
	assert.Contains(t, capabilities.SupportedPaginationTypes, query.PaginationTypeOffset)
}
