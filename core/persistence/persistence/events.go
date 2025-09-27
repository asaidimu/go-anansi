package persistence

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

// eventsPersistence is a wrapper around a base.Persistence that adds event-emitting capabilities.
// It intercepts method calls to the underlying persistence, emits events for the start,
// success, and failure of each operation, and then calls the original method.
// This provides a mechanism for observability and for triggering side effects in a
// decoupled manner.
type eventsPersistence struct {
	persistence  base.Persistence
	eventEmitter *events.EventEmitter[base.PersistenceEvent]
}

var _ base.Persistence = (*eventsPersistence)(nil)

// newEventEmittingPersistence creates a new event-emitting persistence wrapper.
func newEventEmittingPersistence(persistence base.Persistence,
eventEmitter *events.EventEmitter[base.PersistenceEvent],
_ *zap.Logger) base.Persistence {
	return &eventsPersistence{
		persistence:  persistence,
		eventEmitter: eventEmitter,
	}
}

// CreateCollection wraps the underlying persistence's CreateCollection method, adding event emission.
func (e *eventsPersistence) CreateCollection(ctx context.Context, sc schema.SchemaDefinition) (base.Collection, error) {
	config := events.OperationConfig{
		Operation:         "createCollection",
		StartEventTypes:   []string{string(base.CollectionCreateStart)},
		SuccessEventTypes: []string{string(base.CollectionCreateSuccess)},
		FailedEventTypes:  []string{string(base.CollectionCreateFailed)},
		Input:             sc,
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
		Operation:         "createManyCollections",
		StartEventTypes:   []string{string(base.CollectionCreateStart)},
		SuccessEventTypes: []string{string(base.CollectionCreateSuccess)},
		FailedEventTypes:  []string{string(base.CollectionCreateFailed)},
		Input:             schemas,
	}

	_, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return nil, e.persistence.CreateCollections(ctx, schemas)
	})

	return err
}

// Delete wraps the underlying persistence's Delete method, adding event emission.
func (e *eventsPersistence) Delete(ctx context.Context, name string) (bool, error) {
	config := events.OperationConfig{
		Operation:         "deleteCollection",
		StartEventTypes:   []string{string(base.CollectionDeleteStart)},
		SuccessEventTypes: []string{string(base.CollectionDeleteSuccess)},
		FailedEventTypes:  []string{string(base.CollectionDeleteFailed)},
		Input:             name,
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
		Operation:         "hasCollection",
		StartEventTypes:   []string{string(base.PersistenceReadStart)},
		SuccessEventTypes: []string{string(base.PersistenceReadSuccess)},
		FailedEventTypes:  []string{string(base.PersistenceReadFailed)},
		Input:             name,
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
	config := events.OperationConfig{
		Operation:         "collection",
		StartEventTypes:   []string{string(base.CollectionReadStart)},
		SuccessEventTypes: []string{string(base.CollectionReadSuccess)},
		FailedEventTypes:  []string{string(base.CollectionReadFailed)},
		Input:             name,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Collection(ctx, name)
	})

	if err != nil {
		return nil, err
	}

	return result.(base.Collection), nil
}

func (e *eventsPersistence) ListCollections(ctx context.Context) ([]string, error) {
	config := events.OperationConfig{
		Operation:         "listCollections",
		StartEventTypes:   []string{string(base.PersistenceReadStart)},
		SuccessEventTypes: []string{string(base.PersistenceReadSuccess)},
		FailedEventTypes:  []string{string(base.PersistenceReadFailed)},
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
		Operation:         "metadata",
		StartEventTypes:   []string{string(base.PersistenceReadStart)},
		SuccessEventTypes: []string{string(base.PersistenceReadSuccess)},
		FailedEventTypes:  []string{string(base.PersistenceReadFailed)},
		Input:             filter,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Metadata(ctx, filter)
	})

	if err != nil {
		return base.Metadata{}, err
	}

	return result.(base.Metadata), nil
}

func (e *eventsPersistence) Schema(ctx context.Context, id string, version ...string) (*schema.SchemaDefinition, error) {
	return e.persistence.Schema(ctx, id, version...)
}

func (e *eventsPersistence) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
	return e.persistence.Subscribe(ctx, options)
}

func (e *eventsPersistence) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	return e.persistence.Subscriptions(ctx)
}

func (e *eventsPersistence) Unsubscribe(ctx context.Context, id string) {
	e.persistence.Unsubscribe(ctx, id)
}

func (e *eventsPersistence) Transact(ctx context.Context, callback func(ctx context.Context, tx base.BasePersistence) (any, error)) (any, error) {
	config := events.OperationConfig{
		Operation:         "transact",
		StartEventTypes:   []string{string(base.TransactionStart)},
		SuccessEventTypes: []string{string(base.TransactionSuccess)},
		FailedEventTypes:  []string{string(base.TransactionFailed)},
	}

	return e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.persistence.Transact(ctx, callback)
	})
}

func (e *eventsPersistence) Rollback(
	ctx context.Context,
	name string,
	version *string,
	dryRun *bool,
) (base.Collection, error) {
	config := events.OperationConfig{
		Operation:         "rollback",
		StartEventTypes:   []string{string(base.RollbackStart), string(base.CollectionUpdateStart)},
		SuccessEventTypes: []string{string(base.RollbackSuccess), string(base.CollectionUpdateSuccess)},
		FailedEventTypes:  []string{string(base.RollbackFailed), string(base.CollectionUpdateFailed)},
		Input:             map[string]any{"name": name, "version": version, "dryRun": dryRun},
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
		Operation:         "migrate",
		StartEventTypes:   []string{string(base.MigrateStart), string(base.CollectionUpdateStart)},
		SuccessEventTypes: []string{string(base.MigrateSuccess), string(base.CollectionUpdateSuccess)},
		FailedEventTypes:  []string{string(base.MigrateFailed), string(base.CollectionUpdateFailed)},
		Input:             map[string]any{"name": name, "migration": migration, "dryRun": dryRun},
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
		Operation:         "close",
		StartEventTypes:   []string{string(base.PersistenceLifecycleStart)},
		SuccessEventTypes: []string{string(base.PersistenceLifecycleSuccess)},
		FailedEventTypes:  []string{string(base.PersistenceLifecycleFailed)},
	}

	e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		e.persistence.Close(ctx)
		return nil, nil
	})
}

func (e *eventsPersistence) Async(ctx context.Context, f func(ctx context.Context) (any, error)) base.Future {
	return e.persistence.Async(ctx, f)
}
