package persistence_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	cevents "github.com/asaidimu/go-anansi/v7/core/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	persistence "github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/collection"
	pevents "github.com/asaidimu/go-anansi/v7/core/persistence/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/core/utils"
	"github.com/asaidimu/go-anansi/v7/tests/testutils"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	testutils.ConfigureDocumentFactory()
	os.Setenv("ANANSI_ENV", "development")
	os.Exit(m.Run())
}

// Helper to create a basic schema definition
func testSchema(name ...string) *definition.Schema {
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
					Unique:   true,
				},
				"status": {
					Name: "status",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
			},
		},
	}
}

func resolveSchema(ctx context.Context, logicalName string) (string, *definition.Schema, error) {
	return logicalName, nil, nil
}

// setupCollection is a helper function to set up a new collection for testing.
func setupCollection(t *testing.T) (base.Collection, query.DatabaseInteractor, *zap.Logger, *definition.Schema, *events.TypedEventBus[persistence.PersistenceEvent], context.Context) {
	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())
	ephemeralInteractor := ephemeral.NewEphemeral()
	manager := ephemeralInteractor.SchemaManager()
	logger := zap.NewNop()
	testSchemaDef := registry.MustEnrichSchema(testSchema())

	validator, err := definition.NewDocumentValidator(testSchemaDef, nil)
	assert.NoError(t, err)
	expected := data.MustNewDocument(map[string]any{"name": "Test1"})
	_, ok := validator.Validate(expected.ToMap())
	assert.True(t, ok)

	err = manager.CreateCollection(context.Background(), *testSchemaDef)
	assert.NoError(t, err)

	engine := query.NewQueryEngine(ephemeralInteractor.Capabilities(), logger)
	factory := pevents.NewPersistenceEventFactory(testSchemaDef.Name, logger)
	eventEmitter := cevents.NewEventEmitter(pevents.NewGoEventsBusAdapter(bus), factory.CreateEvent, logger)
	c, err := collection.NewCollection(eventEmitter, testSchemaDef.Name, collection.NewStaticSchemaProvider(testSchemaDef), ephemeralInteractor, engine, logger, resolveSchema, nil)
	assert.NoError(t, err)

	ctx := context.Background()
	return c, ephemeralInteractor, logger, testSchemaDef, bus, ctx
}

// setupNonExistentCollection is a helper function to set up a collection with a non-existent schema for testing error cases.
func setupNonExistentCollection() (base.Collection, query.DatabaseInteractor, *zap.Logger, *definition.Schema, *events.TypedEventBus[persistence.PersistenceEvent], context.Context) {
	bus, _ := events.NewTypedEventBus[persistence.PersistenceEvent](events.DefaultConfig())
	nonExistentSchema := &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name:   "non_existent",
			Fields: map[definition.FieldId]definition.Field{},
		},
	}
	ephemeralInteractor := ephemeral.NewEphemeral()
	logger := zap.NewNop()
	engine := query.NewQueryEngine(ephemeralInteractor.Capabilities(), logger)
	factory := pevents.NewPersistenceEventFactory(nonExistentSchema.Name, logger)
	eventEmitter := cevents.NewEventEmitter(pevents.NewGoEventsBusAdapter(bus), factory.CreateEvent, logger)
	c, _ := collection.NewCollection(eventEmitter, nonExistentSchema.Name, collection.NewStaticSchemaProvider(nonExistentSchema), ephemeralInteractor, engine, logger, resolveSchema, nil)
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
		expected := data.MustNewDocument(map[string]any{"name": "Test2"})

		result, err := collection.CreateOne(ctx, expected)
		if err != nil {
			t.Logf("Error creating single document: %v", err)
		}
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.IsType(t, persistence.CreateResult{}, result)
		actual := result.Data.StripMetadata()
		assert.Equal(t, actual.Must().GetString("name"), expected.Must().GetString("name"))

		// Verify the document was actually inserted by reading it back
		readQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(result.Data.ID()).Build()
		readResult, err := collection.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 1, readResult.Count)
		assert.True(t, expected.Equals(readResult.Data[0]))
	})

	t.Run("multiple documents success", func(t *testing.T) {
		docs := []*data.Document{data.MustNewDocument(map[string]any{"name": "Test8"}), data.MustNewDocument(map[string]any{"name": "Test3"})}

		result, err := collection.CreateMany(ctx, docs)
		if err != nil {
			t.Logf("Error creating multiple documents: %v", err)
		}

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.IsType(t, []persistence.CreateResult{}, result)
		assert.Len(t, result, 2)

		// Verify the documents were actually inserted by reading them back
		readQuery := query.NewQueryBuilder().Where(data.DocumentIDField).In(
			result[0].Data.ID(),
			result[1].Data.ID()).Build()
		readResult, err := collection.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 2, readResult.Count)
		// Note: Order might not be guaranteed, so we'll just check for presence
		for _, expectedDoc := range docs {
			found := slices.ContainsFunc(readResult.Data, expectedDoc.Equals)
			assert.True(t, found, "Expected document not found: %s", expectedDoc.String())
		}
	})

	t.Run("insert documents error - duplicate ID", func(t *testing.T) {
		// Name "Test2" already exists from previous test
		doc := data.MustNewDocument(map[string]any{"name": "Test2"})
		_, err := collection.CreateOne(ctx, doc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), persistence.ErrUniqueConstraintViolation.Error())
	})
}

func TestCollection_Read(t *testing.T) {
	c, _, _, _, _, ctx := setupCollection(t)

	// Insert some data for reading
	_, err := c.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "Test3"}))
	if err != nil {
		t.Logf("Error creating Test3 document: %v", err)
	}
	assert.NoError(t, err)
	_, err = c.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "Test4"}))
	if err != nil {
		t.Logf("Error creating Test4 document: %v", err)
	}
	assert.NoError(t, err)

	q := query.NewQueryBuilder().Where("name").Eq("Test3").Build()

	t.Run("read documents success", func(t *testing.T) {
		expected := data.MustNewDocument(map[string]any{"name": "Test3"})

		result, err := c.Read(ctx, &q)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.Count)

		final := result.Data[0]
		final.StripMetadata()
		assert.Equal(t, expected.Must().GetString("name"), final.Must().GetString("name"))
	})

	t.Run("read documents error - non-existent collection", func(t *testing.T) {
		// Create a new collection instance with a non-existent schema name
		_, ephemeralInteractor, logger, _, bus, ctx := setupCollection(t)
		nonExistentSchema := &definition.Schema{
			Version: common.MustNewVersion("1.0.0"),
			BaseSchema: definition.BaseSchema{
				Name:   "non_existent",
				Fields: map[definition.FieldId]definition.Field{},
			},
		}
		engine := query.NewQueryEngine(ephemeralInteractor.Capabilities(), logger)
		factory := pevents.NewPersistenceEventFactory(nonExistentSchema.Name, logger)
		eventEmitter := cevents.NewEventEmitter(pevents.NewGoEventsBusAdapter(bus), factory.CreateEvent, logger)
		nonExistentCollection, _ := collection.NewCollection(eventEmitter, nonExistentSchema.Name, collection.NewStaticSchemaProvider(nonExistentSchema), ephemeralInteractor, engine, logger, resolveSchema, nil)

		result, err := nonExistentCollection.Read(ctx, &q)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "database query execution failed")
	})
}

func TestCollection_Update(t *testing.T) {
	c, _, _, _, _, ctx := setupCollection(t)

	// Insert a document to be updated.
	r, err := c.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "OriginalName"}))
	if err != nil {
		t.Logf("Error creating OriginalName document: %v", err)
	}
	require.NoError(t, err)

	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(r.Data.ID()).Build()
	result, err := c.Read(ctx, &q)
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)

	d := result.Data[0]
	updates := data.MustNewDocument(map[string]any{
		"name": "UpdatedName",
	})

	metadata := d.Metadata()
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
		assert.Equal(t, 0, rowsAffected.Count)
	})

	t.Run("update multiple documents without optimistic locking", func(t *testing.T) {
		// Use a fresh collection for this test to avoid conflicts
		c, _, _, _, _, ctx := setupCollection(t)

		// Insert some data for updating
		_, err := c.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "UpdateMulti1", "status": "pending"}))
		if err != nil {
			t.Logf("Error creating UpdateMulti1 document: %v", err)
		}
		assert.NoError(t, err)
		_, err = c.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "UpdateMulti2", "status": "pending"}))
		if err != nil {
			t.Logf("Error creating UpdateMulti2 document: %v", err)
		}
		assert.NoError(t, err)

		// Filter for documents to update
		q := query.NewQueryBuilder().Where("status").Eq("pending").Build()

		// Define the update payload. No version is provided, so no optimistic locking.
		updates := data.MustNewDocument(map[string]any{"status": "done"})
		updateParams := &persistence.CollectionUpdate{Set: updates, Filter: q.Filters, Version: nil} // Explicitly nil

		// Perform the update
		rowsAffected, err := c.Update(ctx, updateParams)
		assert.NoError(t, err)
		require.NotNil(t, rowsAffected.Total)
		assert.Equal(t, 2, *rowsAffected.Total)

		// Verify the documents were actually updated
		readQuery := query.NewQueryBuilder().Where("status").Eq("done").Build()
		readResult, err := c.Read(ctx, &readQuery)
		assert.NoError(t, err)
		assert.Equal(t, 2, readResult.Count)

		docs := readResult.Data
		assert.Equal(t, "done", docs[0].Must().GetString("status"))
		assert.Equal(t, "done", docs[1].Must().GetString("status"))
	})

}

func TestCollection_Delete(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	// Insert some data for deleting
	r, err := collection.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "ToDelete"}))
	if err != nil {
		t.Logf("Error creating ToDelete document: %v", err)
	}
	assert.NoError(t, err)
	_, err = collection.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "ToKeep"}))
	if err != nil {
		t.Logf("Error creating ToKeep document: %v", err)
	}
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(r.Data.ID()).Build().Filters

	t.Run("delete documents success", func(t *testing.T) {
		rowsAffected, err := collection.Delete(ctx, filters, false)

		assert.NoError(t, err)
		assert.Equal(t, 1, rowsAffected)

		// Verify the document was actually deleted
		readQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(r.Data.ID()).Build()
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
		assert.Contains(t, err.Error(), base.ErrCollectionNotFound.Error())
	})

	t.Run("delete all documents with unsafe flag", func(t *testing.T) {
		// Re-create collection and add data for this specific test case
		collection, _, _, _, _, ctx = setupCollection(t)
		collection.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "Doc3"}))
		if err != nil {
			t.Logf("Error creating Doc3 document: %v", err)
		}
		collection.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "Doc4"}))
		if err != nil {
			t.Logf("Error creating Doc4 document: %v", err)
		}

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
		collection.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "Doc5"}))
		if err != nil {
			t.Logf("Error creating Doc5 document: %v", err)
		}

		// Attempt to delete all documents by passing nil filter and unsafe=false
		rowsAffected, err := collection.Delete(ctx, nil, false)

		assert.Error(t, err)
		assert.Equal(t, 0, rowsAffected)
		assert.Contains(t, err.Error(), base.ErrDangerousDelete.Error())
	})
}

func TestCollection_Validate(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	t.Run("valid document", func(t *testing.T) {
		doc := data.MustNewDocument(map[string]any{data.DocumentIDField: "1", "name": "Valid Name"})
		issues, ok := collection.Validate(ctx, doc, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("invalid document - missing required field", func(t *testing.T) {
		doc := data.MustNewDocument(map[string]any{data.DocumentIDField: "1"}) // Missing 'name'
		issues, ok := collection.Validate(ctx, doc, false)
		assert.False(t, ok)
		assert.NotEmpty(t, issues)
		assert.Contains(t, issues[0].Message, "Required")
	})

	t.Run("invalid document - wrong type", func(t *testing.T) {
		doc := data.MustNewDocument(map[string]any{data.DocumentIDField: "1", "name": 123}) // Name should be string
		issues, ok := collection.Validate(ctx, doc, false)
		assert.False(t, ok)
		assert.NotEmpty(t, issues)
		assert.Contains(t, issues[0].Message, "Expected")
	})

	t.Run("loose validation - missing required field allowed", func(t *testing.T) {
		doc := data.MustNewDocument(map[string]any{data.DocumentIDField: "1"}) // Missing 'name'
		issues, ok := collection.Validate(ctx, doc, true)                     // Loose validation
		assert.True(t, ok)                                                   // Should be valid in loose mode for missing required
		assert.Empty(t, issues)
	})
}

func TestCollection_Metadata(t *testing.T) {
	collection, _, _, testSchema, _, ctx := setupCollection(t)

	t.Run("metadata success", func(t *testing.T) {
		metadata := collection.Metadata(ctx, nil, false) // No filter, no force refresh
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

func TestHashConsistencyOnRead(t *testing.T) {
	collection, _, _, _, _, ctx := setupCollection(t)

	// 1. Create a document to be read
	docToCreate := data.MustNewDocument(map[string]any{
		"name":   "consistent_hash_test",
		"status": "active",
	})
	createResult, err := collection.CreateOne(ctx, docToCreate)
	require.NoError(t, err)
	docID := createResult.Data.ID()

	// 2. Read the document for the first time
	query := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(docID).Build()
	readResult1, err := collection.Read(ctx, &query)
	require.NoError(t, err)
	require.Equal(t, 1, readResult1.Count)
	doc1 := readResult1.Data[0]

	// 3. Read the document for the second time
	readResult2, err := collection.Read(ctx, &query)
	require.NoError(t, err)
	require.Equal(t, 1, readResult2.Count)
	doc2 := readResult2.Data[0]

	// 4. Assertions
	checksum1, err := doc1.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, checksum1, "Document 1 should have a checksum")

	checksum2, err := doc2.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, checksum2, "Document 2 should have a checksum")

	// The checksums should be identical, proving the hashing process is deterministic on read.
	assert.Equal(t, checksum1, checksum2, "Hashes from two separate reads should be consistent")

	// Verify that the hash is indeed valid for both documents.
	ok1, err := doc1.VerifyHash()
	require.NoError(t, err)
	assert.True(t, ok1, "Hash for doc1 should be valid")

	ok2, err := doc2.VerifyHash()
	require.NoError(t, err)
	assert.True(t, ok2, "Hash for doc2 should be valid")
}
