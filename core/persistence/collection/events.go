// Package collection.events provides the event-emitting functionality that wraps around the
// core collection operations. This allows for a decoupled way to observe and react
// to data changes within the persistence layer.
package collection

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/events"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/utils"
	"go.uber.org/zap"
)

// eventsCollection is a wrapper around a baseCollection that adds event-emitting capabilities.
// It intercepts method calls to the underlying collection, emits events for the start,
// success, and failure of each operation, and then calls the original method.
// This provides a mechanism for observability and for triggering side effects in a
// decoupled manner.
type eventsCollection struct {
	base.Collection // Embedded - provides all methods by default
	eventEmitter    *events.EventEmitter[base.PersistenceEvent]
	name            string
	logger          *zap.Logger
}

var _ base.Collection = (*eventsCollection)(nil)

// newEventEmittingCollection creates a new event-emitting collection wrapper.
// It takes a CollectionBase and returns a Collection that will emit events
// for all of its operations.
func newEventEmittingCollection(name string, collection base.Collection, eventEmitter *events.EventEmitter[base.PersistenceEvent], logger *zap.Logger) *eventsCollection {
	return &eventsCollection{
		Collection:   collection,
		eventEmitter: eventEmitter,
		name:         name,
		logger:       logger,
	}
}

// withEventEmission is a helper method that handles the transaction-aware event emission pattern.
// It checks if we're in a transaction and sets up the appropriate contexts for deferred emission.
func (e *eventsCollection) withEventEmission(
	ctx context.Context,
	config events.OperationConfig,
	op func() (any, error),
) (any, error) {
	tx, ok := transaction.GetCurrentTransaction(ctx)
	if !ok {
		// Non-transactional: emit immediately
		return e.eventEmitter.WithEventEmission(
			common.ContextWithCollectionName(ctx, e.name),
			config,
			op,
		)
	}
	actionQueue, hasQueue := utils.FromContext(ctx)

	if !hasQueue {
		// First write operation in this transaction - create queue and register hooks
		actionQueue = utils.NewDeferredActionQueue()
		ctx = utils.AttachToContext(ctx, actionQueue)

		// Register hooks with transaction
		tx.OnCommit(func() {
			actionQueue.ExecuteAll()
		})

		tx.OnRollback(func() {
			actionQueue.DiscardAll()
		})
	}


	return e.eventEmitter.WithEventEmission(
		common.ContextWithCollectionName(ctx, e.name),
		config,
		op,
		actionQueue,
	)

}

// CreateOne overrides the embedded Collection's CreateOne to add event emission.
func (e *eventsCollection) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	config := events.OperationConfig{
		Operation:         "createOne",
		StartEventTypes:   []string{string(base.DocumentCreateStart)},
		SuccessEventTypes: []string{string(base.DocumentCreateSuccess)},
		FailedEventTypes:  []string{string(base.DocumentCreateFailed)},
		Input:             doc,
	}

	result, err := e.withEventEmission(ctx, config, func() (any, error) {
		return e.Collection.CreateOne(ctx, doc)
	})

	r, ok := result.(base.CreateResult)
	if !ok {
		r = base.CreateResult{}
	}

	return r, err
}

// CreateMany overrides the embedded Collection's CreateMany to add event emission.
func (e *eventsCollection) CreateMany(ctx context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	config := events.OperationConfig{
		Operation:         "createMany",
		StartEventTypes:   []string{string(base.DocumentCreateStart)},
		SuccessEventTypes: []string{string(base.DocumentCreateSuccess)},
		FailedEventTypes:  []string{string(base.DocumentCreateFailed)},
		Input:             docs,
	}

	result, err := e.withEventEmission(ctx, config, func() (any, error) {
		return e.Collection.CreateMany(ctx, docs)
	})

	if err != nil {
		return nil, err
	}

	return result.([]base.CreateResult), nil
}

// Read overrides the embedded Collection's Read to add event emission.
func (e *eventsCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	config := events.OperationConfig{
		Operation:         "read",
		StartEventTypes:   []string{string(base.DocumentReadStart)},
		SuccessEventTypes: []string{string(base.DocumentReadSuccess)},
		FailedEventTypes:  []string{string(base.DocumentReadFailed)},
		Input:             q,
	}

	result, err := e.eventEmitter.WithEventEmission(ctx, config, func() (any, error) {
		return e.Collection.Read(ctx, q)
	})

	if err != nil {
		return nil, err
	}

	return result.(*base.ReadResult), nil
}

// Update overrides the embedded Collection's Update to add event emission.
func (e *eventsCollection) Update(ctx context.Context, params *base.CollectionUpdate) (*base.ReadResult, error) {
	config := events.OperationConfig{
		Operation:         "update",
		StartEventTypes:   []string{string(base.DocumentUpdateStart)},
		SuccessEventTypes: []string{string(base.DocumentUpdateSuccess)},
		FailedEventTypes:  []string{string(base.DocumentUpdateFailed)},
		Input:             params,
	}

	result, err := e.withEventEmission(ctx, config, func() (any, error) {
		return e.Collection.Update(ctx, params)
	})

	if err != nil {
		return nil, err
	}

	return result.(*base.ReadResult), nil
}

// Delete overrides the embedded Collection's Delete to add event emission.
func (e *eventsCollection) Delete(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	config := events.OperationConfig{
		Operation:         "delete",
		StartEventTypes:   []string{string(base.DocumentDeleteStart)},
		SuccessEventTypes: []string{string(base.DocumentDeleteSuccess)},
		FailedEventTypes:  []string{string(base.DocumentDeleteFailed)},
		Input:             filter,
	}

	result, err := e.withEventEmission(ctx, config, func() (any, error) {
		return e.Collection.Delete(ctx, filter, unsafe)
	})

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}
