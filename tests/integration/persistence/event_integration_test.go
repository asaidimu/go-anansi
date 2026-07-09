package persistence_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v8"
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	persistenceUtils "github.com/asaidimu/go-anansi/v8/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	sqliteExecutor "github.com/asaidimu/go-anansi/v8/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v8/sqlite/query"
	testEvents "github.com/asaidimu/go-anansi/v8/utils" // Import from the correct events package
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// setupEventTest creates a new persistence instance with an event bus for testing.
func setupEventTest(t *testing.T) (base.Persistence, *testEvents.WatermillEventBus[base.PersistenceEvent], base.Collection, func()) {
	// Setup in-memory SQLite DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory(nil)

	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	require.NoError(t, err)

	// Setup Event Bus
	eventBus := testEvents.NewWatermillEventBus[base.PersistenceEvent](logger)

	// Setup persistence
	cfg := anansi.SetupConfig{
		Interactor: interactor,
		Logger:     logger,
		DocumentFactoryConfig: data.DocumentFactoryConfig{
			GlobalSanitizer: &data.FieldMaskConfig{
				DefaultPolicy: data.MaskPreserve,
			},
		},
		Decorators: &persistenceUtils.Decorators{},
		EventBus:   eventBus,
	}
	p, err := anansi.Setup(cfg)
	require.NoError(t, err)

	// Create a test collection
	v1, _ := common.NewVersion("1.0.0")
	testSchema := &definition.Schema{
		Version: v1,
		BaseSchema: definition.BaseSchema{
			Name: "test_collection",
			Fields: map[definition.FieldId]definition.Field{
				"name":  {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}, Required: true},
				"value": {Name: "value", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
		},
	}
	collection, err := p.CreateCollection(context.Background(), testSchema)
	require.NoError(t, err)

	cleanup := func() {
		p.Close(context.Background())
		eventBus.Close()
		db.Close()
	}

	return p, eventBus, collection, cleanup
}

func TestPersistenceEvents(t *testing.T) {
	p, eventBus, collection, cleanup := setupEventTest(t)
	defer cleanup()

	t.Run("TestDocumentCreateEvent", func(t *testing.T) {
		receivedEventChan := make(chan base.PersistenceEvent, 1)

		var val int
		// Subscribe to DocumentCreateSuccess event
		subId := collection.Subscribe(context.Background(), base.SubscriptionOptions{
			Event: base.DocumentCreateSuccess,
			Callback: func(ctx context.Context, event base.PersistenceEvent) error {
				val = ctx.Value("CONTEXT_KEY").(int)
				receivedEventChan <- event
				return nil
			},
		})
		unsubscribe := func() {
			collection.Unsubscribe(context.Background(), subId)
		}
		defer unsubscribe()

		// Create a document
		ctx := context.WithValue(context.Background(), "CONTEXT_KEY", 1)
		docToCreate := data.MustNewDocument(map[string]any{"name": "test_document", "value": 123})
		createResult, err := collection.CreateOne(ctx, docToCreate)
		require.NoError(t, err)

		// Wait for the event
		select {
		case receivedEvent := <-receivedEventChan:
			assert.Equal(t, base.DocumentCreateSuccess, receivedEvent.Type)
			assert.Equal(t, collection.Metadata(context.Background(), nil, false).Name, *receivedEvent.Collection)

			outputMap, ok := receivedEvent.Output.(map[string]any)
			require.True(t, ok)

			outputData, ok := outputMap["data"]
			require.True(t, ok, "CreateResult in event output should have a 'data' field")

			outputDoc, ok := data.DocumentFrom(outputData)
			require.True(t, ok, "The 'data' field in the CreateResult should be convertible to a Document")

			assert.Equal(t, "test_document", outputDoc.Must().GetString("name"))
			assert.Equal(t, float64(123), outputDoc.Must().GetFloat64("value"))
			assert.NotEmpty(t, outputDoc.ID())
			assert.Equal(t, createResult.Data.ID(), outputDoc.ID())

			assert.Equal(t, val, 1)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for DocumentCreateSuccess event")
		}
	})

	t.Run("TestDocumentUpdateEvent", func(t *testing.T) {
		// Create a document first
		docToUpdate := data.MustNewDocument(map[string]any{"name": "original_name", "value": 100})
		createResult, err := collection.CreateOne(context.Background(), docToUpdate)
		require.NoError(t, err)
		initialDocID := createResult.Data.ID()

		receivedEventChan := make(chan base.PersistenceEvent, 1)

		// Subscribe to DocumentUpdateSuccess event
		unsubscribe := eventBus.Subscribe(
			string(base.DocumentUpdateSuccess),
			func(ctx context.Context, event base.PersistenceEvent) error {
				receivedEventChan <- event
				return nil
			},
		)
		defer unsubscribe()

		// Update the document
		updatePatch := data.Patch{"name": "updated_name", "value": 200}.Document()
		filter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(initialDocID).Build().Filters
		_, err = collection.Update(context.Background(), &base.CollectionUpdate{Set: updatePatch, Filter: filter, ReturnDocument: true})
		require.NoError(t, err)

		// Wait for the event
		select {
		case receivedEvent := <-receivedEventChan:
			assert.Equal(t, base.DocumentUpdateSuccess, receivedEvent.Type)
			assert.Equal(t, collection.Metadata(context.Background(), nil, false).Name, *receivedEvent.Collection)

			outputMap, ok := receivedEvent.Output.(map[string]any)
			require.True(t, ok)

			outputData, ok := outputMap["data"].([]any)
			require.True(t, ok, "ReadResult in event output should have a 'data' field which is a slice")
			require.Len(t, outputData, 1, "Should have one document in the ReadResult")

			outputDoc, ok := data.DocumentFrom(outputData[0])
			require.True(t, ok, "The 'data' field in the ReadResult should contain a document")

			assert.Equal(t, initialDocID, outputDoc.ID())
			assert.Equal(t, "updated_name", outputDoc.Must().GetString("name"))
			assert.Equal(t, float64(200), outputDoc.Must().GetFloat64("value"))

		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for DocumentUpdateSuccess event")
		}
	})

	t.Run("TestDocumentDeleteEvent", func(t *testing.T) {
		// Create a document first
		docToDelete := data.MustNewDocument(map[string]any{"name": "document_to_delete", "value": 300})
		createResult, err := collection.CreateOne(context.Background(), docToDelete)
		require.NoError(t, err)
		deletedDocID := createResult.Data.ID()

		receivedEventChan := make(chan base.PersistenceEvent, 1)

		// Subscribe to DocumentDeleteSuccess event
		unsubscribe := eventBus.Subscribe(
			string(base.DocumentDeleteSuccess),
			func(ctx context.Context, event base.PersistenceEvent) error {
				receivedEventChan <- event
				return nil
			},
		)
		defer unsubscribe()

		// Delete the document
		filter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(deletedDocID).Build().Filters
		_, err = collection.Delete(context.Background(), filter, false) // false for not deleting physical data
		require.NoError(t, err)

		// Wait for the event
		select {
		case receivedEvent := <-receivedEventChan:
			assert.Equal(t, base.DocumentDeleteSuccess, receivedEvent.Type)
			assert.Equal(t, collection.Metadata(context.Background(), nil, false).Name, *receivedEvent.Collection)


			inputMap, ok := receivedEvent.Input.(map[string]any)
			require.True(t, ok)

			// The input of a delete event is a query.FilterCondition after JSON (un)marshaling
			// So directly access the value of the "value" field.
			id, ok := inputMap["value"].(string)
			require.True(t, ok)
			assert.Equal(t, deletedDocID, id)

		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for DocumentDeleteSuccess event")
		}
	})

	t.Run("TestTransactionalEvents_Commit", func(t *testing.T) {
		// Create an initial document that will be updated in the transaction
		existingDoc := data.MustNewDocument(map[string]any{"name": "existing_document", "value": 10})
		_, err := collection.CreateOne(context.Background(), existingDoc)
		require.NoError(t, err)
		existingDocID := existingDoc.ID()

		createEventChan := make(chan base.PersistenceEvent, 1)
		updateEventChan := make(chan base.PersistenceEvent, 1)

		createId := collection.Subscribe(context.Background(), base.SubscriptionOptions{
			Event: base.DocumentCreateSuccess,
			Callback: func(ctx context.Context, event base.PersistenceEvent) error {
				createEventChan <- event
				return nil
			},
		})
		defer collection.Unsubscribe(context.Background(), createId)

		updateId := collection.Subscribe(context.Background(), base.SubscriptionOptions{
			Event: base.DocumentUpdateSuccess,
			Callback: func(ctx context.Context, event base.PersistenceEvent) error {
				updateEventChan <- event
				return nil
			},
		})
		defer collection.Unsubscribe(context.Background(), updateId)

		// Execute a transaction that creates and updates documents
		_, err = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
			txCollection, err := tx.Collection(tctx, collection.Metadata(tctx, nil, false).Name)
			require.NoError(t, err)

			// Create a new document within the transaction
			newDoc := data.MustNewDocument(map[string]any{"name": "new_document_in_tx", "value": 50})
			_, err = txCollection.CreateOne(tctx, newDoc)
			require.NoError(t, err)

			// Update the existing document within the transaction
			updatePatch := data.Patch{"value": 25}.Document()
			filter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(existingDocID).Build().Filters
			_, err = txCollection.Update(tctx, &base.CollectionUpdate{Set: updatePatch, Filter: filter, ReturnDocument: true})
			require.NoError(t, err)

			// Crucial: Check that events have NOT been emitted yet
			select {
			case <-createEventChan:
				t.Fatal("DocumentCreateSuccess event received prematurely before transaction commit")
			case <-updateEventChan:
				t.Fatal("DocumentUpdateSuccess event received prematurely before transaction commit")
			default:
				// Expected: no events yet
			}

			return nil, nil // Commit the transaction
		})
		require.NoError(t, err)

		// Wait for both events after transaction commit with shorter timeout
		var receivedCreateEvent, receivedUpdateEvent base.PersistenceEvent

		timeout := time.After(100 * time.Millisecond) // Much shorter timeout
		eventsReceived := 0

		for eventsReceived < 2 {
			select {
			case receivedCreateEvent = <-createEventChan:
				eventsReceived++
			case receivedUpdateEvent = <-updateEventChan:
				eventsReceived++
			case <-timeout:
				t.Fatalf("Timeout waiting for events after commit. Received %d/2 events", eventsReceived)
			}
		}

		// Assertions for the create event
		assert.Equal(t, base.DocumentCreateSuccess, receivedCreateEvent.Type)
		assert.Equal(t, collection.Metadata(context.Background(), nil, false).Name, *receivedCreateEvent.Collection)
		createOutputMap, ok := receivedCreateEvent.Output.(map[string]any)
		require.True(t, ok)
		createOutputData, ok := createOutputMap["data"]
		require.True(t, ok)
		createInputDoc, ok := data.DocumentFrom(createOutputData)
		require.True(t, ok)
		assert.Equal(t, "new_document_in_tx", createInputDoc.Must().GetString("name"))

		// Assertions for the update event
		assert.Equal(t, base.DocumentUpdateSuccess, receivedUpdateEvent.Type)
		assert.Equal(t, collection.Metadata(context.Background(), nil, false).Name, *receivedUpdateEvent.Collection)
		updateOutputMap, ok := receivedUpdateEvent.Output.(map[string]any)
		require.True(t, ok)
		updateOutputData, ok := updateOutputMap["data"].([]any)
		require.True(t, ok)
		require.Len(t, updateOutputData, 1)
		updateInputDoc, ok := data.DocumentFrom(updateOutputData[0])
		require.True(t, ok)
		assert.Equal(t, existingDocID, updateInputDoc.ID())
		assert.Equal(t, float64(25), updateInputDoc.Must().GetFloat64("value"))

		// Verify the final state of the existing document in the DB
		readQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(existingDocID).Build()
		readResult, err := collection.Read(context.Background(), &readQuery)
		require.NoError(t, err)
		require.Equal(t, 1, readResult.Count)
		assert.Equal(t, float64(25), readResult.Data[0].Must().GetFloat64("value"))
	})
	t.Run("TestTransactionalEvents_Rollback", func(t *testing.T) {
		// Create an initial document that will be updated in the transaction and then rolled back
		existingDoc := data.MustNewDocument(map[string]any{"name": "original_document", "value": 100})
		_, err := collection.CreateOne(context.Background(), existingDoc)
		require.NoError(t, err)
		existingDocID := existingDoc.ID()

		createEventChan := make(chan base.PersistenceEvent, 1)
		updateEventChan := make(chan base.PersistenceEvent, 1)

		// Subscribe to create and update events
		unsubscribeCreate := eventBus.Subscribe(
			string(base.DocumentCreateSuccess),
			func(ctx context.Context, event base.PersistenceEvent) error {
				createEventChan <- event
				return nil
			},
		)
		defer unsubscribeCreate()

		unsubscribeUpdate := eventBus.Subscribe(
			string(base.DocumentUpdateSuccess),
			func(ctx context.Context, event base.PersistenceEvent) error {
				updateEventChan <- event
				return nil
			},
		)
		defer unsubscribeUpdate()


		// Execute a transaction that attempts to create and update, but then fails
		txErr := fmt.Errorf("simulated transaction failure")
		_, err = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
			txCollection, err := tx.Collection(tctx, collection.Metadata(tctx, nil, false).Name)
			require.NoError(t, err)

			// Create a new document within the transaction
			newDoc := data.MustNewDocument(map[string]any{"name": "new_document_in_tx2", "value": 50})
			_, err = txCollection.CreateOne(tctx, newDoc)
			require.NoError(t, err)

			// Update the existing document within the transaction
			updatePatch := data.Patch{"value": 25}.Document()
			filter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(existingDocID).Build().Filters
			_, err = txCollection.Update(tctx, &base.CollectionUpdate{Set: updatePatch, Filter: filter})
			require.NoError(t, err)

			return nil, txErr // Force a rollback
		})
		assert.ErrorIs(t, err, txErr)

		// Assert that no events were received after the rollback
		select {
		case <-createEventChan:
			t.Fatal("DocumentCreateSuccess event received despite transaction rollback")
		case <-updateEventChan:
			t.Fatal("DocumentUpdateSuccess event received despite transaction rollback")
		case <-time.After(100 * time.Millisecond): // Give a short moment to ensure no events arrive
			// Expected: no events
		}

		// Verify the original document's state in the DB is unchanged
		readQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(existingDocID).Build()
		readResult, err := collection.Read(context.Background(), &readQuery)
		require.NoError(t, err)
		require.Equal(t, 1, readResult.Count)
		assert.Equal(t, float64(100), readResult.Data[0].Must().GetFloat64("value")) // Should be original value

		// Verify the new document created in the transaction does not exist
		readNewDocQuery := query.NewQueryBuilder().Where("name").Eq("new_document_in_tx2").Build()
		readNewDocResult, err := collection.Read(context.Background(), &readNewDocQuery)
		require.NoError(t, err)
		assert.Equal(t, 0, readNewDocResult.Count) // Should not exist
	})
}
