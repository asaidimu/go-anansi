package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPersistence_DocumentEvents(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("test_collection")
	collection, err := p.Create(context.Background(), *sc)
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
		p.RegisterSubscription(context.Background(), base.RegisterSubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}

	// Test Create
	docToCreate := common.Document{"id": "1", "name": "value"}
	_, err = collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	// Test Read
	readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
	result, err := collection.Read(context.Background(), &readQuery)
	assert.Equal(t, 1, result.Count)
	require.NoError(t, err)

	// Test Update
	docToUpdate := result.Data.(common.Document)
	docToUpdate["name"] = "new_value"
	updateFilter := query.NewQueryBuilder().Where("id").Eq("1").Build().Filters
	_, err = collection.Update(context.Background(), &base.CollectionUpdate{Data: docToUpdate, Filter: updateFilter})
	require.NoError(t, err)

	// Test Delete
	deleteFilter := query.NewQueryBuilder().Where("id").Eq("1").Build().Filters
	_, err = collection.Delete(context.Background(), deleteFilter, false)
	require.NoError(t, err)

	// Allow some time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedEvents[base.DocumentCreateStart], 2, "Expected 2 DocumentCreateStart event")
	assert.Len(t, receivedEvents[base.DocumentCreateSuccess], 2, "Expected 2 DocumentCreateSuccess event")
	assert.Len(t, receivedEvents[base.DocumentReadStart], 3, "Expected 3 DocumentReadStart event")
	assert.Len(t, receivedEvents[base.DocumentReadSuccess], 3, "Expected 1 DocumentReadSuccess event")
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
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
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
	// These events are emitted to provide hooks for external systems
	// to react to collection lifecycle changes (creation, deletion).
	// 'Start' events allow for pre-processing or validation,
	// 'Success' events for post-processing or notifications,
	// and 'Failed' events for error handling and rollback.
	collectionEventTypes := []base.PersistenceEventType{
		base.CollectionCreateStart, base.CollectionCreateSuccess, base.CollectionCreateFailed,
		base.CollectionDeleteStart, base.CollectionDeleteSuccess, base.CollectionDeleteFailed,
	}

	for _, eventType := range collectionEventTypes {
		p.RegisterSubscription(context.Background(), base.RegisterSubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}

	// Test Collection Create
	sc := newTestSchema("new_test_collection")
	_, err = p.Create(context.Background(), *sc)
	require.NoError(t, err)

	// Test Collection Delete
	_, err = p.Delete(context.Background(), sc.Name)
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
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
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
	// These events provide observability into the lifecycle of database transactions,
	// allowing for monitoring, auditing, and integration with distributed tracing systems.
	transactionEventTypes := []base.PersistenceEventType{
		base.TransactionStart, base.TransactionSuccess, base.TransactionFailed,
	}

	for _, eventType := range transactionEventTypes {
		p.RegisterSubscription(context.Background(), base.RegisterSubscriptionOptions{
			Event:    eventType,
			Callback: callback,
		})
	}

	// Test successful transaction
	_, err = p.Transact(context.Background(), func(tx base.BasePersistence) (any, error) {
		// Perform some operation within the transaction
		return "success", nil
	})
	require.NoError(t, err)

	// Test failed transaction
	_, err = p.Transact(context.Background(), func(tx base.BasePersistence) (any, error) {
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
