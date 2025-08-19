// Package collection.events provides the event-emitting functionality that wraps around the
// core collection operations. This allows for a decoupled way to observe and react
// to data changes within the persistence layer.
package collection

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
)

// eventsCollection is a wrapper around a baseCollection that adds event-emitting capabilities.
// It intercepts method calls to the underlying collection, emits events for the start,
// success, and failure of each operation, and then calls the original method.
// This provides a mechanism for observability and for triggering side effects in a
// decoupled manner.
type eventsCollection struct {
	collection base.Collection
	bus        *events.TypedEventBus[base.PersistenceEvent]
	schema     *schema.SchemaDefinition
}

var _ base.Collection = (*eventsCollection)(nil)

// newEventEmittingCollection creates a new event-emitting collection wrapper.
// It takes a CollectionBase and returns a Collection that will emit events
// for all of its operations.
func newEventEmittingCollection(collection base.Collection, bus *events.TypedEventBus[base.PersistenceEvent], schema *schema.SchemaDefinition) *eventsCollection {
	return &eventsCollection{
		collection: collection,
		bus:        bus,
		schema:     schema,
	}
}

// emitEvent is a helper method to publish a persistence event to the event bus.
func (e *eventsCollection) emitEvent(event base.PersistenceEvent) {
	if e.bus != nil {
		e.bus.Emit(string(event.Type), event)
	}
}

// withEventEmission is a higher-order function that wraps a persistence operation
// with start, success, and failure events. It handles the timing of the operation
// and constructs the appropriate event for each stage.
func (e *eventsCollection) withEventEmission(
	operation string,
	startEventType base.PersistenceEventType,
	successEventType base.PersistenceEventType,
	failedEventType base.PersistenceEventType,
	input any,
	queryParam any,
	fn func() (any, error),
) (any, error) {
	startTime := time.Now()

	// Emit start event
	startEvent := utils.CreateEvent(
		startEventType,
		operation,
		e.schema.Name,
		input,
		nil, // No output yet
		queryParam,
		nil, // No error yet
		nil, // No issues yet
		startTime,
	)
	e.emitEvent(startEvent)

	// Execute the operation
	result, err := fn()

	if err != nil {
		// Emit failure event
		errStr := err.Error()
		failEvent := utils.CreateEvent(
			failedEventType,
			operation,
			e.schema.Name,
			input,
			nil, // No output on failure
			queryParam,
			&errStr,
			nil, // Issues can be added here if available
			startTime,
		)
		e.emitEvent(failEvent)
		return result, err
	}

	// Emit success event
	successEvent := utils.CreateEvent(
		successEventType,
		operation,
		e.schema.Name,
		input,
		result,
		queryParam,
		nil, // No error on success
		nil, // No issues on success
		startTime,
	)
	e.emitEvent(successEvent)

	return result, nil
}

// CreateOne wraps the underlying collection's CreateOne method, adding event emission.
func (e *eventsCollection) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	result, err := e.withEventEmission(
		"createOne",
		base.DocumentCreateStart,
		base.DocumentCreateSuccess,
		base.DocumentCreateFailed,
		doc,
		nil, // No query parameter for create
		func() (any, error) {
			return e.collection.CreateOne(ctx, doc)
		},
	)

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
func (e *eventsCollection) CreateMany(ctx context.Context, docs []data.Document) ([]base.CreateResult, error) {
	result, err := e.withEventEmission(
		"createMany",
		base.DocumentCreateStart,
		base.DocumentCreateSuccess,
		base.DocumentCreateFailed,
		docs,
		nil, // No query parameter for create
		func() (any, error) {
			return e.collection.CreateMany(ctx, docs)
		},
	)

	if err != nil {
		return result.([]base.CreateResult), err
	}

	return result.([]base.CreateResult), nil
}

// Read wraps the underlying collection's Read method, adding event emission
// for the start, success, and failure of the operation.
func (e *eventsCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	result, err := e.withEventEmission(
		"read",
		base.DocumentReadStart,
		base.DocumentReadSuccess,
		base.DocumentReadFailed,
		nil, // No input data for read
		q,
		func() (any, error) {
			return e.collection.Read(ctx, q)
		},
	)

	if err != nil {
		return nil, err
	}

	return result.(*base.ReadResult), nil
}

// Update wraps the underlying collection's Update method, adding event emission
// for the start, success, and failure of the operation.
func (e *eventsCollection) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	result, err := e.withEventEmission(
		"update",
		base.DocumentUpdateStart,
		base.DocumentUpdateSuccess,
		base.DocumentUpdateFailed,
		params.Data,
		params.Filter,
		func() (any, error) {
			return e.collection.Update(ctx, params)
		},
	)

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Delete wraps the underlying collection's Delete method, adding event emission
// for the start, success, and failure of the operation.
func (e *eventsCollection) Delete(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	result, err := e.withEventEmission(
		"delete",
		base.DocumentDeleteStart,
		base.DocumentDeleteSuccess,
		base.DocumentDeleteFailed,
		nil, // No input data for delete
		filter,
		func() (any, error) {
			return e.collection.Delete(ctx, filter, unsafe)
		},
	)

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Validate delegates the call to the underlying collection's Validate method.
// No events are emitted for validation as it is a read-only operation.
func (e *eventsCollection) Validate(ctx context.Context, data data.Document, loose bool) (*schema.ValidationResult, error) {
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

// RegisterSubscription wraps the underlying collection's RegisterSubscription method,
// emitting an event after a new subscription is successfully registered.
func (e *eventsCollection) RegisterSubscription(ctx context.Context, options base.RegisterSubscriptionOptions) string {
	id := e.collection.RegisterSubscription(ctx, options)
	return id
}

// UnregisterSubscription wraps the underlying collection's UnregisterSubscription method,
// emitting an event after a subscription is successfully unregistered.
func (e *eventsCollection) UnregisterSubscription(ctx context.Context, id string) {
	e.collection.UnregisterSubscription(ctx, id)
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
