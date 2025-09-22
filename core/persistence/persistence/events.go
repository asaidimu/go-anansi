package persistence

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/events"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	goevents "github.com/asaidimu/go-events"
	"go.uber.org/zap"
)

// eventsPersistence is a wrapper around a base.Persistence that adds event-emitting capabilities.
// It intercepts method calls to the underlying persistence, emits events for the start,
// success, and failure of each operation, and then calls the original method.
// This provides a mechanism for observability and for triggering side effects in a
// decoupled manner.
type eventsPersistence struct {
	persistence  base.Persistence
	eventEmitter *events.EventEmitter
}

var _ base.Persistence = (*eventsPersistence)(nil)

// newEventEmittingPersistence creates a new event-emitting persistence wrapper.
func newEventEmittingPersistence(persistence base.Persistence, bus *goevents.TypedEventBus[base.PersistenceEvent], logger *zap.Logger) base.Persistence {
	return &eventsPersistence{
		persistence:  persistence,
		eventEmitter: events.NewEventEmitter(bus, "", logger), // Empty collection name for persistence-level events
	}
}

// CreateCollection wraps the underlying persistence's CreateCollection method, adding event emission.
func (e *eventsPersistence) CreateCollection(ctx context.Context, sc schema.SchemaDefinition) (base.Collection, error) {
	config := events.OperationConfig{
		Operation:        "createCollection",
		StartEventType:   base.CollectionCreateStart,
		SuccessEventType: base.CollectionCreateSuccess,
		FailedEventType:  base.CollectionCreateFailed,
		Input:            sc,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.CreateCollection(ctx, sc)
	})

	if err != nil {
		return nil, err
	}

	return result.(base.Collection), nil
}

func (e *eventsPersistence) CreateCollections(ctx context.Context, schemas []schema.SchemaDefinition) error {
	config := events.OperationConfig{
		Operation:        "createManyCollections",
		StartEventType:   base.CollectionCreateStart,
		SuccessEventType: base.CollectionCreateSuccess,
		FailedEventType:  base.CollectionCreateFailed,
		Input:            schemas,
	}

	_, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return nil, e.persistence.CreateCollections(ctx, schemas)
	})

	return err
}

// Delete wraps the underlying persistence's Delete method, adding event emission.
func (e *eventsPersistence) Delete(ctx context.Context, name string) (bool, error) {
	config := events.OperationConfig{
		Operation:        "deleteCollection",
		StartEventType:   base.CollectionDeleteStart,
		SuccessEventType: base.CollectionDeleteSuccess,
		FailedEventType:  base.CollectionDeleteFailed,
		Input:            name,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Delete(ctx, name)
	})

	if err != nil {
		return false, err
	}

	return result.(bool), nil
}

func (e *eventsPersistence) HasCollection(ctx context.Context, name string) (bool, error) {
	config := events.OperationConfig{
		Operation:        "hasCollection",
		StartEventType:   base.PersistenceReadStart,
		SuccessEventType: base.PersistenceReadSuccess,
		FailedEventType:  base.PersistenceReadFailed,
		Input:            name,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.HasCollection(ctx, name)
	})

	if err != nil {
		return false, err
	}

	return result.(bool), nil
}

func (e *eventsPersistence) Collection(ctx context.Context, name string) (base.Collection, error) {
	// This operation retrieves a collection, which is then used for other operations.
	// The subsequent operations on the collection will have their own events.
	// Therefore, we don't emit a separate event here.
	return e.persistence.Collection(ctx, name)
}

func (e *eventsPersistence) ListCollections(ctx context.Context) ([]string, error) {
	config := events.OperationConfig{
		Operation:        "listCollections",
		StartEventType:   base.PersistenceReadStart,
		SuccessEventType: base.PersistenceReadSuccess,
		FailedEventType:  base.PersistenceReadFailed,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.ListCollections(ctx)
	})

	if err != nil {
		return nil, err
	}

	return result.([]string), nil
}

func (e *eventsPersistence) Metadata(ctx context.Context, filter *base.MetadataFilter) (base.Metadata, error) {
	config := events.OperationConfig{
		Operation:        "metadata",
		StartEventType:   base.PersistenceReadStart,
		SuccessEventType: base.PersistenceReadSuccess,
		FailedEventType:  base.PersistenceReadFailed,
		Input:            filter,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Metadata(ctx, filter)
	})

	if err != nil {
		return base.Metadata{}, err
	}

	return result.(base.Metadata), nil
}

func (e *eventsPersistence) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
	// Subscriptions are handled by the underlying persistence and may have their own eventing mechanism.
	// For now, we delegate directly.
	return e.persistence.Subscribe(ctx, options)
}

func (e *eventsPersistence) Schema(ctx context.Context, id string, version ...string) (*schema.SchemaDefinition, error) {
	return e.persistence.Schema(ctx, id, version...)
}

func (e *eventsPersistence) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	return e.persistence.Subscriptions(ctx)
}

func (e *eventsPersistence) Transact(ctx context.Context, callback func(ctx context.Context, tx base.BasePersistence) (any, error)) (any, error) {
	config := events.OperationConfig{
		Operation:        "transact",
		StartEventType:   base.TransactionStart,
		SuccessEventType: base.TransactionSuccess,
		FailedEventType:  base.TransactionFailed,
	}

	return e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Transact(ctx, callback)
	})
}

func (e *eventsPersistence) Unsubscribe(ctx context.Context, id string) {
	e.persistence.Unsubscribe(ctx, id)
}

func (e *eventsPersistence) Rollback(
	ctx context.Context,
	name string,
	version *string,
	dryRun *bool,
) (base.Collection, error) {
	config := events.OperationConfig{
		Operation:        "rollback",
		StartEventType:   base.CollectionUpdateStart,
		SuccessEventType: base.CollectionUpdateSuccess,
		FailedEventType:  base.CollectionUpdateFailed,
		Input:            map[string]any{"name": name, "version": version, "dryRun": dryRun},
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Rollback(ctx, name, version, dryRun)
	})

	if err != nil {
		return nil, err
	}

	return result.(base.Collection), nil
}

func (e *eventsPersistence) Migrate(
	ctx context.Context,
	name string,
	migration schema.Migration,
	dryRun *bool,
) (base.Collection, error) {
	config := events.OperationConfig{
		Operation:        "migrate",
		StartEventType:   base.CollectionUpdateStart,
		SuccessEventType: base.CollectionUpdateSuccess,
		FailedEventType:  base.CollectionUpdateFailed,
		Input:            map[string]any{"name": name, "migration": migration, "dryRun": dryRun},
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Migrate(ctx, name, migration, dryRun)
	})

	if err != nil {
		return nil, err
	}

	return result.(base.Collection), nil
}

func (e *eventsPersistence) Close(ctx context.Context) {
	config := events.OperationConfig{
		Operation:        "close",
		StartEventType:   base.PersistenceLifecycleStart,
		SuccessEventType: base.PersistenceLifecycleSuccess,
		FailedEventType:  base.PersistenceLifecycleFailed,
	}

	e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		e.persistence.Close(ctx)
		return nil, nil
	})
}

func (e *eventsPersistence) Async(ctx context.Context, f func(ctx context.Context) (any, error)) base.Future {
	// This is for running background tasks, not directly a persistence operation to be audited.
	return e.persistence.Async(ctx, f)
}
