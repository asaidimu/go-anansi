package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	persistence "github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"

	"go.uber.org/zap"
)

// Helper to create a basic schema definition
func testSchema(name ...string) *schema.SchemaDefinition {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	return &schema.SchemaDefinition{
		Name:        sname,
		Version:     "1.0.0",
		Description: utils.StringPtr("test collection"),
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: "string", Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"name": {Name: "name", Type: "string", Required: utils.BoolPtr(true)},
		},
	}
}

// setupCollectionTest is a helper function to set up a new collection for testing.
func setupCollection(t *testing.T) (base.Collection, query.DatabaseInteractor, *zap.Logger, *schema.SchemaDefinition, *events.TypedEventBus[persistence.PersistenceEvent]) {
	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())
	ephemeralInteractor  := ephemeral.NewEphemeral()
	manager := ephemeralInteractor.SchemaManager()
	logger := zap.NewNop()
	testSchema := testSchema()

	err := manager.CreateCollection(*testSchema)
	assert.NoError(t, err)

	engine := query.NewQueryEngine(ephemeralInteractor, logger)
	opts := &persistence.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}
	c, err := collection.NewCollection(bus, testSchema.Name, testSchema, engine, logger, opts)
	assert.NoError(t, err)

	return c, ephemeralInteractor, logger, testSchema, bus
}

func TestNewCollection(t *testing.T) {
	collection, _, _, _, _ := setupCollection(t)
	assert.NotNil(t, collection)
}

func TestCollection_Create(t *testing.T) {
	collection, _, _, _, _ := setupCollection(t)

	t.Run("single document success", func(t *testing.T) {
		expected := common.Document{"id": "1", "name": "Test1"}

		result, err := collection.CreateOne(expected)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.IsType(t, &persistence.CreateResult{}, result)
		actual := result.Data.StripMetadata()
		assert.Equal(t, actual["name"], expected["name"])

		// Verify the document was actually inserted by reading it back
		readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
		readResult, err := collection.Read(&readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 1, readResult.Count)
		assert.Equal(t, expected, readResult.Data.([]common.Document)[0])
	})

	t.Run("multiple documents success", func(t *testing.T) {
		docs := []common.Document{{"id": "2", "name": "Test2"}, {"id": "3", "name": "Test3"}}

		result, err := collection.CreateMany(docs)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.IsType(t, []persistence.CreateResult{}, result)
		assert.Len(t, result, 2)

		// Verify the documents were actually inserted by reading them back
		readQuery := query.NewQueryBuilder().Where("id").In("2", "3").Build()
		readResult, err := collection.Read(&readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 2, readResult.Count)
		// Note: Order might not be guaranteed, so we'll just check for presence
		assert.Contains(t, readResult.Data.([]common.Document), docs[0])
		assert.Contains(t, readResult.Data.([]common.Document), docs[1])
	})

	t.Run("insert documents error - duplicate ID", func(t *testing.T) {
		// ID "1" already exists from previous test
		doc := common.Document{"id": "1", "name": "DuplicateTest"}
		results, err := collection.CreateOne(doc)
		t.Logf("Results %v", results)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unique constraint violation")
	})
}

func TestCollection_Read(t *testing.T) {
	c, _, _, _, _ := setupCollection(t)

	// Insert some data for reading
	_, err := c.CreateOne(common.Document{"id": "1", "name": "Test1"})
	assert.NoError(t, err)
	_, err = c.CreateOne(common.Document{"id": "2", "name": "Test2"})
	assert.NoError(t, err)

	q := query.NewQueryBuilder().Where("name").Eq("Test1").Build()

	t.Run("read documents success", func(t *testing.T) {
		expected := common.Document{"id": "1", "name": "Test1"}

		result, err := c.Read(&q)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.Count)

		results := result.Data.([]common.Document)

		final := results[0]
		final.StripMetadata()
		assert.Equal(t, expected["name"], results[0]["name"])
	})

	t.Run("read documents error - non-existent collection", func(t *testing.T) {
		// Create a new collection instance with a non-existent schema name
		_, ephemeralInteractor, logger, _, bus := setupCollection(t)
		nonExistentSchema := &schema.SchemaDefinition{Name: "non_existent"}
		engine := query.NewQueryEngine(ephemeralInteractor, logger)
		opts := &persistence.MetadataOptions{
			HmacSecretKey: []byte("test-secret"),
		}
		nonExistentCollection, _ := collection.NewCollection(bus, nonExistentSchema.Name, nonExistentSchema, engine, logger, opts)

		result, err := nonExistentCollection.Read(&q)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "collection not found")
	})
}

func TestCollection_Update(t *testing.T) {
	c, _, _, _, _ := setupCollection(t)

	// Insert some data for updating
	_, err := c.CreateOne(common.Document{"id": "1", "name": "OriginalName"})
	assert.NoError(t, err)

	q := query.NewQueryBuilder().Where("id").Eq("1").Build()
	result, err := c.Read(&q)
	assert.NoError(t, err)
	assert.Equal(t, 1, result.Count)

	data := result.Data.([]common.Document)
	updates := common.Document{
		"name": "UpdatedName",
	}

	metadata, ok := data[0].Metadata()
	assert.Equal(t, ok, true)
	updates.SetMetadata(metadata)

	updateParams := &persistence.CollectionUpdate{Data: updates, Filter: q.Filters}

	t.Run("update documents success", func(t *testing.T) {
		rowsAffected, err := c.Update(updateParams)
		assert.NoError(t, err)
		assert.Equal(t, 1, rowsAffected)

		// Verify the document was actually updated
		readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
		readResult, err := c.Read(&readQuery)
		assert.NoError(t, err)
		assert.Equal(t, "UpdatedName", readResult.Data.([]common.Document)[0]["name"])
	})

	t.Run("update documents error - non-existent collection", func(t *testing.T) {
		// Create a new collection instance with a non-existent schema name
		nonExistentCollection, _, _, _, _ := setupNonExistentCollection()

		rowsAffected, err := nonExistentCollection.Update(updateParams)

		assert.Error(t, err)
		assert.Equal(t, 0, rowsAffected)
		assert.Contains(t, err.Error(), "collection not found")
	})
}

func TestCollection_Delete(t *testing.T) {
	collection, _, _, _, _ := setupCollection(t)

	// Insert some data for deleting
	_, err := collection.CreateOne(common.Document{"id": "1", "name": "ToDelete"})
	assert.NoError(t, err)
	_, err = collection.CreateOne(common.Document{"id": "2", "name": "ToKeep"})
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("id").Eq("1").Build().Filters

	t.Run("delete documents success", func(t *testing.T) {
		rowsAffected, err := collection.Delete(filters, false)

		assert.NoError(t, err)
		assert.Equal(t, 1, rowsAffected)

		// Verify the document was actually deleted
		readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
		readResult, err := collection.Read(&readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 0, readResult.Count)
	})

	t.Run("delete documents error - non-existent collection", func(t *testing.T) {
		// Create a new collection instance with a non-existent schema name
		nonExistentCollection, _, _, _, _ := setupNonExistentCollection()

		rowsAffected, err := nonExistentCollection.Delete(filters, false)

		assert.Error(t, err)
		assert.Equal(t, 0, rowsAffected)
		assert.Contains(t, err.Error(), "collection not found")
	})

	t.Run("delete all documents with unsafe flag", func(t *testing.T) {
		// Re-create collection and add data for this specific test case
		collection, _, _, _, _ = setupCollection(t)
		collection.CreateOne(common.Document{"id": "3", "name": "Doc3"})
		collection.CreateOne(common.Document{"id": "4", "name": "Doc4"})

		// Debug: Read documents before deletion
		q := query.NewQueryBuilder().Build()
		_, err := collection.Read(&q)
		assert.NoError(t, err)

		// Delete all documents by passing nil filter and unsafe=true
		rowsAffected, err := collection.Delete(nil, true)

		assert.NoError(t, err)
		assert.Equal(t, 2, rowsAffected)

		// Verify all documents are deleted
		readQuery := query.NewQueryBuilder().Build()
		readResult, err := collection.Read(&readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 0, readResult.Count)
	})

	t.Run("delete all documents without unsafe flag should fail", func(t *testing.T) {
		// Re-create collection and add data for this specific test case
		collection, _, _, _, _ = setupCollection(t)
		collection.CreateOne(common.Document{"id": "5", "name": "Doc5"})

		// Attempt to delete all documents by passing nil filter and unsafe=false
		rowsAffected, err := collection.Delete(nil, false)

		assert.Error(t, err)
		assert.Equal(t, 0, rowsAffected)
		assert.Contains(t, err.Error(), "delete operation requires a filter or the unsafe flag set to true")
	})
}

// setupNonExistentCollection is a helper function to set up a collection with a non-existent schema for testing error cases.
func setupNonExistentCollection() (base.Collection, query.DatabaseInteractor, *zap.Logger, *schema.SchemaDefinition, *events.TypedEventBus[persistence.PersistenceEvent]) {
	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())
	nonExistentSchema := &schema.SchemaDefinition{Name: "non_existent"}
	ephemeralInteractor  := ephemeral.NewEphemeral()
	logger := zap.NewNop()
	engine := query.NewQueryEngine(ephemeralInteractor, logger)
	opts := &persistence.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}
	c, _ := collection.NewCollection(bus, nonExistentSchema.Name, nonExistentSchema, engine, logger, opts)
	return c, ephemeralInteractor, logger, nonExistentSchema, bus
}

func TestCollection_Validate(t *testing.T) {
	collection, _, _, _, _ := setupCollection(t)

	t.Run("valid document", func(t *testing.T) {
		doc := common.Document{"id": "1", "name": "Valid Name"}
		result, err := collection.Validate(doc, false)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Issues)
	})

	t.Run("invalid document - missing required field", func(t *testing.T) {
		doc := common.Document{"id": "1"} // Missing 'name'
		result, err := collection.Validate(doc, false)
		assert.NoError(t, err) // Validation itself should not return an error, but a result with issues
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Issues)
		assert.Contains(t, result.Issues[0].Message, "Required")
	})

	t.Run("invalid document - wrong type", func(t *testing.T) {
		doc := common.Document{"id": "1", "name": 123} // Name should be string
		result, err := collection.Validate(doc, false)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Issues)
		assert.Contains(t, result.Issues[0].Message, "Expected")
	})

	t.Run("loose validation - missing required field allowed", func(t *testing.T) {
		doc := common.Document{"id": "1"}             // Missing 'name'
		result, err := collection.Validate(doc, true) // Loose validation
		assert.NoError(t, err)
		assert.True(t, result.Valid) // Should be valid in loose mode for missing required
		assert.Empty(t, result.Issues)
	})
}

func TestCollection_Metadata(t *testing.T) {
	collection, _, _, testSchema, _ := setupCollection(t)

	t.Run("metadata success", func(t *testing.T) {
		metadata, err := collection.Metadata(nil, false) // No filter, no force refresh
		assert.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.Equal(t, testSchema.Name, metadata.Name)
	})
}

func TestCollection_RegisterSubscription(t *testing.T) {
	collection, _, _, _, _ := setupCollection(t)

	options := persistence.RegisterSubscriptionOptions{
		Event: persistence.DocumentCreateSuccess,
		Label: utils.StringPtr("test_sub"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}

	id := collection.RegisterSubscription(options)
	assert.NotEmpty(t, id)

	subs, _ := collection.Subscriptions()
	assert.Len(t, subs, 1)
	assert.Equal(t, *subs[0].Id, id)
}

func TestCollection_UnregisterSubscription(t *testing.T) {
	collection, _, _, _, _ := setupCollection(t)

	options := persistence.RegisterSubscriptionOptions{
		Event: persistence.DocumentCreateSuccess,
		Label: utils.StringPtr("test_sub"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}

	id := collection.RegisterSubscription(options)
	assert.NotEmpty(t, id)

	collection.UnregisterSubscription(id)
	subs, _ := collection.Subscriptions()
	assert.Empty(t, subs)
}

func TestCollection_Subscriptions(t *testing.T) {
	collection, _, _, _, _ := setupCollection(t)

	options1 := persistence.RegisterSubscriptionOptions{
		Event: persistence.DocumentCreateSuccess,
		Label: utils.StringPtr("sub1"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}
	options2 := persistence.RegisterSubscriptionOptions{
		Event: persistence.DocumentUpdateSuccess,
		Label: utils.StringPtr("sub2"),
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			return nil
		},
	}

	collection.RegisterSubscription(options1)
	collection.RegisterSubscription(options2)

	subs, _ := collection.Subscriptions()
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
	collection, _, _, _, _ := setupCollection(t)

	capabilities := collection.Capabilities()
	assert.NotNil(t, capabilities)
	// Assert some known capabilities from the ephemeral interactor
	assert.True(t, capabilities.SupportsGroupBy)
	assert.True(t, capabilities.SupportsDistinct)
	assert.True(t, capabilities.SupportsNestedFields)
	assert.Contains(t, capabilities.SupportedPaginationTypes, query.PaginationTypeOffset)
}
