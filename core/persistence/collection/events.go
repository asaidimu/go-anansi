// Package collection.events provides the event-emitting functionality that wraps around the
// core collection operations. This allows for a decoupled way to observe and react
// to data changes within the persistence layer.
package collection

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

// eventsCollection is a wrapper around a baseCollection that adds event-emitting capabilities.
// It intercepts method calls to the underlying collection, emits events for the start,
// success, and failure of each operation, and then calls the original method.
// This provides a mechanism for observability and for triggering side effects in a
// decoupled manner.
type eventsCollection struct {
	collection   base.Collection
	eventEmitter *events.EventEmitter[base.PersistenceEvent]
	schema       *schema.SchemaDefinition
	name         string
}

var _ base.Collection = (*eventsCollection)(nil)

// newEventEmittingCollection creates a new event-emitting collection wrapper.
// It takes a CollectionBase and returns a Collection that will emit events
// for all of its operations.
func newEventEmittingCollection(name string, collection base.Collection, eventEmitter *events.EventEmitter[base.PersistenceEvent], schema *schema.SchemaDefinition, _ *zap.Logger) *eventsCollection {
	return &eventsCollection{
		collection:   collection,
		schema:       schema,
		eventEmitter: eventEmitter,
		name:         name,
	}
}

func (e *eventsCollection) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	config := events.OperationConfig{
		Operation:         "createOne",
		StartEventTypes:   []string{string(base.DocumentCreateStart)},
		SuccessEventTypes: []string{string(base.DocumentCreateSuccess)},
		FailedEventTypes:  []string{string(base.DocumentCreateFailed)},
		Input:             doc,
	}

	result, err := e.eventEmitter.WithEventEmission(
		utils.ContextWithCollectionName(ctx, e.name),
		config, func() (any, error) {
			return e.collection.CreateOne(ctx, doc)
		})

	r, ok := result.(base.CreateResult)
	if !ok {
		r = base.CreateResult{}
	}

	if err != nil {
		return r, err
	}

	return r, nil
}

// CreateMany wraps the underlying collection's CreateMany method, adding event emission.
func (e *eventsCollection) CreateMany(ctx context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	config := events.OperationConfig{
		Operation:         "createMany",
		StartEventTypes:   []string{string(base.DocumentCreateStart)},
		SuccessEventTypes: []string{string(base.DocumentCreateSuccess)},
		FailedEventTypes:  []string{string(base.DocumentCreateFailed)},
		Input:             docs,
	}

	result, err := e.eventEmitter.WithEventEmission(utils.ContextWithCollectionName(ctx, e.name), config, func() (any, error) {
		return e.collection.CreateMany(ctx, docs)
	})

	if err != nil {
		return nil, err
	}

	return result.([]base.CreateResult), nil
}

// Read wraps the underlying collection's Read method, adding event emission
// for the start, success, and failure of the operation.
func (e *eventsCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	config := events.OperationConfig{
		Operation:         "read",
		StartEventTypes:   []string{string(base.DocumentReadStart)},
		SuccessEventTypes: []string{string(base.DocumentReadSuccess)},
		FailedEventTypes:  []string{string(base.DocumentReadFailed)},
		Input:             q,
	}

	result, err := e.eventEmitter.WithEventEmission(utils.ContextWithCollectionName(ctx, e.name), config, func() (any, error) {
		return e.collection.Read(ctx, q)
	})

	if err != nil {
		return nil, err
	}

	return result.(*base.ReadResult), nil
}

// Update wraps the underlying collection's Update method, adding event emission
// for the start, success, and failure of the operation.
func (e *eventsCollection) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	config := events.OperationConfig{
		Operation:         "update",
		StartEventTypes:   []string{string(base.DocumentUpdateStart)},
		SuccessEventTypes: []string{string(base.DocumentUpdateSuccess)},
		FailedEventTypes:  []string{string(base.DocumentUpdateFailed)},
		Input:             params,
	}

	result, err := e.eventEmitter.WithEventEmission(utils.ContextWithCollectionName(ctx, e.name), config, func() (any, error) {
		return e.collection.Update(ctx, params)
	})

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Delete wraps the underlying collection's Delete method, adding event emission
// for the start, success, and failure of the operation.
func (e *eventsCollection) Delete(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	config := events.OperationConfig{
		Operation:         "delete",
		StartEventTypes:   []string{string(base.DocumentDeleteStart)},
		SuccessEventTypes: []string{string(base.DocumentDeleteSuccess)},
		FailedEventTypes:  []string{string(base.DocumentDeleteFailed)},
		Input:             filter,
	}

	result, err := e.eventEmitter.WithEventEmission(utils.ContextWithCollectionName(ctx, e.name), config, func() (any, error) {
		return e.collection.Delete(ctx, filter, unsafe)
	})

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Validate delegates the call to the underlying collection's Validate method.
// No events are emitted for validation as it is a read-only operation.
func (e *eventsCollection) Validate(ctx context.Context, data *data.Document, loose bool) (*schema.ValidationResult, error) {
	return e.collection.Validate(ctx, data, loose)
}

// Metadata delegates the call to the underlying collection's Metadata method,
// but also emits a telemetry event to record that metadata was requested.
func (e *eventsCollection) Metadata(
	ctx context.Context,
	filter *base.MetadataFilter,
	forceRefresh bool,
) (*base.CollectionMetadata, error) {
	return e.collection.Metadata(ctx, filter, forceRefresh)
}

// Subscribe wraps the underlying collection's Subscribe method,
// emitting an event after a new subscription is successfully registered.
func (e *eventsCollection) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
	id := e.collection.Subscribe(ctx, options)
	return id
}

// Unsubscribe wraps the underlying collection's Unsubscribe method,
// emitting an event after a subscription is successfully unregistered.
func (e *eventsCollection) Unsubscribe(ctx context.Context, id string) {
	e.collection.Unsubscribe(ctx, id)
}

// Subscriptions delegates the call to the underlying collection's Subscriptions method.
// No events are emitted for this operation.
func (e *eventsCollection) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	return e.collection.Subscriptions(ctx)
}

// Capabilities delegates the call to the underlying collection's Capabilities method.
func (e *eventsCollection) Capabilities(ctx context.Context) *query.Capabilities {
	return e.collection.Capabilities(ctx)
}
