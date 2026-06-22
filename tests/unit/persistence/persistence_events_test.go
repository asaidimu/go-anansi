package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	pevents "github.com/asaidimu/go-anansi/v7/core/persistence/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPersistence_DocumentEvents(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()
	goBus, _ := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	bus := pevents.NewGoEventsBusAdapter(goBus)
	p, err := persistence.NewPersistence(interactor, bus, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("test_collection")
	collection, err := p.CreateCollection(context.Background(), sc)
	require.NoError(t, err)

	var mu sync.Mutex
	receivedEvents := make(map[base.PersistenceEventType][]base.PersistenceEvent)

	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents[event.Type] = append(receivedEvents[event.Type], event)
		return nil
	}

	// Subscribe to all document events
	eventTypes := []base.PersistenceEventType{
		base.DocumentCreateStart, base.DocumentCreateSuccess, base.DocumentCreateFailed,
		base.DocumentReadStart, base.DocumentReadSuccess, base.DocumentReadFailed,
		base.DocumentUpdateStart, base.DocumentUpdateSuccess, base.DocumentUpdateFailed,
		base.DocumentDeleteStart, base.DocumentDeleteSuccess, base.DocumentDeleteFailed,
	}

	for _, eventType := range eventTypes {
		p.Subscribe(context.Background(), base.SubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}

	// Test Create
	docToCreate := data.MustNewDocument(map[string]any{"name": "value"})
	d, err := collection.CreateOne(context.Background(), docToCreate)
	if err != nil {
		t.Logf("Error creating docToCreate document: %v", err)
	}
	require.NoError(t, err)

	id := d.Data.ID()
	// Test Read
	readQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
	result, err := collection.Read(context.Background(), &readQuery)
	assert.Equal(t, 1, result.Count)
	require.NoError(t, err)

	// Test Update
	docToUpdate := result.Data[0]
	docToUpdate.Set("name", "new_value")
	updateFilter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters
	_, err = collection.Update(context.Background(), &base.CollectionUpdate{Set: docToUpdate, Filter: updateFilter})
	require.NoError(t, err)

	// Test Delete
	deleteFilter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters
	_, err = collection.Delete(context.Background(), deleteFilter, false)
	require.NoError(t, err)

	// Allow some time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedEvents[base.DocumentCreateStart], 1, "Expected 1 DocumentCreateStart event")
	assert.Len(t, receivedEvents[base.DocumentCreateSuccess], 1, "Expected 1 DocumentCreateSuccess event")
	assert.Len(t, receivedEvents[base.DocumentReadStart], 1, "Expected 1 DocumentReadStart event")
	assert.Len(t, receivedEvents[base.DocumentReadSuccess], 1, "Expected 1 DocumentReadSuccess event")
	assert.Len(t, receivedEvents[base.DocumentUpdateStart], 1, "Expected 1 DocumentUpdateStart event")
	assert.Len(t, receivedEvents[base.DocumentUpdateSuccess], 1, "Expected 1 DocumentUpdateSuccess event")
	assert.Len(t, receivedEvents[base.DocumentDeleteStart], 1, "Expected 1 DocumentDeleteStart event")
	assert.Len(t, receivedEvents[base.DocumentDeleteSuccess], 1, "Expected 1 DocumentDeleteSuccess event")

	// Ensure no failed events were triggered
	assert.Empty(t, receivedEvents[base.DocumentCreateFailed], "Expected 0 DocumentCreateFailed events")
	assert.Empty(t, receivedEvents[base.DocumentReadFailed], "Expected 0 DocumentReadFailed events")
	assert.Empty(t, receivedEvents[base.DocumentUpdateFailed], "Expected 0 DocumentUpdateFailed events")
	assert.Empty(t, receivedEvents[base.DocumentDeleteFailed], "Expected 0 DocumentDeleteFailed events")
}

func TestPersistence_CollectionEvents(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	goBus, _ := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	bus := pevents.NewGoEventsBusAdapter(goBus)
	p, err := persistence.NewPersistence(interactor, bus, logger, nil)
	require.NoError(t, err)

	var mu sync.Mutex
	receivedEvents := make(map[base.PersistenceEventType][]base.PersistenceEvent)

	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents[event.Type] = append(receivedEvents[event.Type], event)
		return nil
	}

	// Subscribe to all collection events
	collectionEventTypes := []base.PersistenceEventType{
		base.CollectionCreateStart, base.CollectionCreateSuccess, base.CollectionCreateFailed,
		base.CollectionDeleteStart, base.CollectionDeleteSuccess, base.CollectionDeleteFailed,
	}

	for _, eventType := range collectionEventTypes {
		p.Subscribe(context.Background(), base.SubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}
	time.Sleep(10 * time.Millisecond)

	// Test Collection Create
	sc := newTestSchema("new_test_collection")
	_, err = p.CreateCollection(context.Background(), sc)
	require.NoError(t, err)

	// Test Collection Delete
	_, err = p.Delete(context.Background(), sc.BaseSchema.Name)
	require.NoError(t, err)

	// Allow some time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Assertions for successful collection creation and deletion events
	assert.Len(t, receivedEvents[base.CollectionCreateStart], 1, "Expected 1 CollectionCreateStart event")
	assert.Len(t, receivedEvents[base.CollectionCreateSuccess], 1, "Expected 1 CollectionCreateSuccess event")
	assert.Len(t, receivedEvents[base.CollectionDeleteStart], 1, "Expected 1 CollectionDeleteStart event")
	assert.Len(t, receivedEvents[base.CollectionDeleteSuccess], 1, "Expected 1 CollectionDeleteSuccess event")

	// Ensure no failed events were triggered for successful operations
	assert.Empty(t, receivedEvents[base.CollectionCreateFailed], "Expected 0 CollectionCreateFailed events")
	assert.Empty(t, receivedEvents[base.CollectionDeleteFailed], "Expected 0 CollectionDeleteFailed events")
}

func TestPersistence_TransactionEvents(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	goBus, _ := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	bus := pevents.NewGoEventsBusAdapter(goBus)
	p, err := persistence.NewPersistence(interactor, bus, logger, nil)
	require.NoError(t, err)

	var mu sync.Mutex
	receivedEvents := make(map[base.PersistenceEventType][]base.PersistenceEvent)

	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents[event.Type] = append(receivedEvents[event.Type], event)
		return nil
	}

	// Subscribe to transaction events
	transactionEventTypes := []base.PersistenceEventType{
		base.TransactionStart, base.TransactionSuccess, base.TransactionFailed,
	}

	for _, eventType := range transactionEventTypes {
		p.Subscribe(context.Background(), base.SubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}
	time.Sleep(10 * time.Millisecond)

	// Test successful transaction
	_, err = p.Transact(context.Background(), func(ctx context.Context, tx base.BasePersistence) (any, error) {
		// Perform some operation within the transaction
		return "success", nil
	})
	require.NoError(t, err)

	// Test failed transaction
	_, err = p.Transact(context.Background(), func(ctx context.Context, tx base.BasePersistence) (any, error) {
		// Simulate an error within the transaction
		return nil, assert.AnError
	})
	require.Error(t, err)

	// Allow some time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Assertions for successful transaction events
	assert.Len(t, receivedEvents[base.TransactionStart], 2, "Expected 2 TransactionStart events (one for success, one for failure)")
	assert.Len(t, receivedEvents[base.TransactionSuccess], 1, "Expected 1 TransactionSuccess event")
	assert.Len(t, receivedEvents[base.TransactionFailed], 1, "Expected 1 TransactionFailed event")
}

func TestPersistence_DocumentUpdateEvents(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	goBus, _ := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	bus := pevents.NewGoEventsBusAdapter(goBus)
	p, err := persistence.NewPersistence(interactor, bus, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("update_collection")
	collection, err := p.CreateCollection(context.Background(), sc)
	require.NoError(t, err)

	// Create a document to update
	docToCreate := data.MustNewDocument(map[string]any{"name": "initial"})
	d, err := collection.CreateOne(context.Background(), docToCreate)
	if err != nil {
		t.Logf("Error creating initial document for update: %v", err)
	}
	require.NoError(t, err)

	id := d.Data.ID()
	var mu sync.Mutex
	receivedEvents := make(map[base.PersistenceEventType][]base.PersistenceEvent)

	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents[event.Type] = append(receivedEvents[event.Type], event)
		return nil
	}

	// Subscribe to document update events
	updateEventTypes := []base.PersistenceEventType{
		base.DocumentUpdateStart, base.DocumentUpdateSuccess, base.DocumentUpdateFailed,
	}

	for _, eventType := range updateEventTypes {
		p.Subscribe(context.Background(), base.SubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}
	time.Sleep(10 * time.Millisecond)

	// Test successful update
	readQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	originalDoc := readResult.Data[0]

	// Create a new document for update, copying original metadata
	updateDoc := originalDoc.Clone()
	updateDoc.Set("name", "updated")

	updateFilter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters
	_, err = collection.Update(context.Background(), &base.CollectionUpdate{Set: updateDoc, Filter: updateFilter})
	require.NoError(t, err)

	// Test failed update
	failedDocUpdate := data.MustNewDocument(map[string]any{data.DocumentIDField: "2", "name": "failed_update"})
	failedUpdateFilter := query.NewQueryBuilder().Where(data.DocumentIDField).Eq("2").Build().Filters
	rows, err := collection.Update(context.Background(), &base.CollectionUpdate{Set: failedDocUpdate, Filter: failedUpdateFilter})
	assert.Equal(t, rows.Count, 0)

	// Allow some time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedEvents[base.DocumentUpdateStart], 2, "Expected 2 DocumentUpdateStart events")
	assert.Len(t, receivedEvents[base.DocumentUpdateSuccess], 2, "Expected 2 DocumentUpdateSuccess event")
}

func TestPersistence_PersistenceLifecycleAndReadEvents(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	goBus, _ := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	bus := pevents.NewGoEventsBusAdapter(goBus)
	p, err := persistence.NewPersistence(interactor, bus, logger, nil)
	require.NoError(t, err)

	var mu sync.Mutex
	receivedEvents := make(map[base.PersistenceEventType][]base.PersistenceEvent)

	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents[event.Type] = append(receivedEvents[event.Type], event)
		return nil
	}

	// Subscribe to persistence lifecycle and read events
	lifecycleEventTypes := []base.PersistenceEventType{
		base.PersistenceLifecycleStart, base.PersistenceLifecycleSuccess, base.PersistenceLifecycleFailed,
		base.PersistenceReadStart, base.PersistenceReadSuccess, base.PersistenceReadFailed,
	}

	for _, eventType := range lifecycleEventTypes {
		p.Subscribe(context.Background(), base.SubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}
	time.Sleep(10 * time.Millisecond)

	// Test successful Persistence Read (e.g., Metadata call)
	_, err = p.Metadata(context.Background(), nil)
	require.NoError(t, err)

	// Test successful Persistence Lifecycle (Close)
	p.Close(context.Background())

	// Test failed Persistence Lifecycle (attempting operation on closed persistence)
	_, err = p.CreateCollection(context.Background(), newTestSchema("closed_collection"))
	require.Error(t, err) // Expecting an error because persistence is closed

	// Allow some time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedEvents[base.PersistenceReadStart], 1, "Expected 1 PersistenceReadStart event")
	assert.Len(t, receivedEvents[base.PersistenceReadSuccess], 1, "Expected 1 PersistenceReadSuccess event")
	assert.Empty(t, receivedEvents[base.PersistenceReadFailed], "Expected 0 PersistenceReadFailed events for successful read")

	assert.Len(t, receivedEvents[base.PersistenceLifecycleStart], 1, "Expected 1 PersistenceLifecycleStart event (for Close)")
	assert.Len(t, receivedEvents[base.PersistenceLifecycleSuccess], 1, "Expected 1 PersistenceLifecycleSuccess event (for Close)")
	assert.Empty(t, receivedEvents[base.PersistenceLifecycleFailed], "Expected 0 PersistenceLifecycleFailed event for successful Close")
}
