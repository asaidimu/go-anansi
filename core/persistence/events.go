// Package persistence provides the event-emitting functionality that wraps around the
// core collection operations. This allows for a decoupled way to observe and react
// to data changes within the persistence layer.
package persistence

import (
	"time"

	"github.com/asaidimu/go-anansi/v5/core/query"
	"github.com/asaidimu/go-anansi/v5/core/schema"
	"github.com/asaidimu/go-events"
)

// Collection is a wrapper around a CollectionBase that adds event-emitting capabilities.
// It intercepts method calls to the underlying collection, emits events for the start,
// success, and failure of each operation, and then calls the original method.
// This provides a mechanism for observability and for triggering side effects in a
// decoupled manner.
type Collection struct {
	collection *CollectionBase
	bus        *events.TypedEventBus[PersistenceEvent]
	schema     *schema.SchemaDefinition
}

// NewEventEmittingCollection creates a new event-emitting collection wrapper.
// It takes a CollectionBase and returns a Collection that will emit events
// for all of its operations.
func NewEventEmittingCollection(collection *CollectionBase) *Collection {
	return &Collection{
		collection: collection,
		bus:        collection.bus,
		schema:     collection.schema,
	}
}

// emitEvent is a helper method to publish a persistence event to the event bus.
func (e *Collection) emitEvent(event PersistenceEvent) {
	if e.bus != nil {
		e.bus.Emit(string(event.Type), event)
	}
}

// withEventEmission is a higher-order function that wraps a persistence operation
// with start, success, and failure events. It handles the timing of the operation
// and constructs the appropriate event for each stage.
func (e *Collection) withEventEmission(
	operation string,
	startEventType PersistenceEventType,
	successEventType PersistenceEventType,
	failedEventType PersistenceEventType,
	input any,
	queryParam any,
	fn func() (any, error),
) (any, error) {
	startTime := time.Now()

	// Emit start event
	startEvent := createEvent(
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
		failEvent := createEvent(
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
		return nil, err
	}

	// Emit success event
	successEvent := createEvent(
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

// Create wraps the underlying collection's Create method, adding event emission
// for the start, success, and failure of the operation.
func (e *Collection) Create(data any) (any, error) {
	result, err := e.withEventEmission(
		"create",
		DocumentCreateStart,
		DocumentCreateSuccess,
		DocumentCreateFailed,
		data,
		nil, // No query parameter for create
		func() (any, error) {
			return e.collection.Create(data)
		},
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}

// Read wraps the underlying collection's Read method, adding event emission
// for the start, success, and failure of the operation.
func (e *Collection) Read(q *query.QueryDSL) (*query.QueryResult, error) {
	result, err := e.withEventEmission(
		"read",
		DocumentReadStart,
		DocumentReadSuccess,
		DocumentReadFailed,
		nil, // No input data for read
		q,
		func() (any, error) {
			return e.collection.Read(q)
		},
	)

	if err != nil {
		return nil, err
	}

	return result.(*query.QueryResult), nil
}

// Update wraps the underlying collection's Update method, adding event emission
// for the start, success, and failure of the operation.
func (e *Collection) Update(params *CollectionUpdate) (int, error) {
	result, err := e.withEventEmission(
		"update",
		DocumentUpdateStart,
		DocumentUpdateSuccess,
		DocumentUpdateFailed,
		params.Data,
		params.Filter,
		func() (any, error) {
			return e.collection.Update(params)
		},
	)

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Delete wraps the underlying collection's Delete method, adding event emission
// for the start, success, and failure of the operation.
func (e *Collection) Delete(filter *query.QueryFilter, unsafe bool) (int, error) {
	result, err := e.withEventEmission(
		"delete",
		DocumentDeleteStart,
		DocumentDeleteSuccess,
		DocumentDeleteFailed,
		nil, // No input data for delete
		filter,
		func() (any, error) {
			return e.collection.Delete(filter, unsafe)
		},
	)

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Validate delegates the call to the underlying collection's Validate method.
// No events are emitted for validation as it is a read-only operation.
func (e *Collection) Validate(data any, loose bool) (*schema.ValidationResult, error) {
	return e.collection.Validate(data, loose)
}

// Rollback wraps the underlying collection's Rollback method, adding event emission
// for the start, success, and failure of the operation.
func (e *Collection) Rollback(version *string, dryRun *bool) (struct {
	Schema  schema.SchemaDefinition `json:"schema"`
	Preview any                   `json:"preview"`
}, error) {
	input := map[string]any{
		"version": version,
		"dryRun":  dryRun,
	}

	result, err := e.withEventEmission(
		"rollback",
		RollbackStart,
		RollbackSuccess,
		RollbackFailed,
		input,
		nil, // No query parameter for rollback
		func() (any, error) {
			return e.collection.Rollback(version, dryRun)
		},
	)

	if err != nil {
		return struct {
			Schema  schema.SchemaDefinition `json:"schema"`
			Preview any                   `json:"preview"`
		}{}, err
	}

	return result.(struct {
		Schema  schema.SchemaDefinition `json:"schema"`
		Preview any                   `json:"preview"`
	}), nil
}

// Migrate wraps the underlying collection's Migrate method, adding event emission
// for the start, success, and failure of the operation.
func (e *Collection) Migrate(
	description string,
	cb func(h schema.SchemaMigrationHelper) (schema.DataTransform[any, any], error),
	dryRun *bool,
) (struct {
	Schema  schema.SchemaDefinition `json:"schema"`
	Preview any                   `json:"preview"`
}, error) {
	input := map[string]any{
		"description": description,
		"dryRun":      dryRun,
	}

	result, err := e.withEventEmission(
		"migrate",
		MigrateStart,
		MigrateSuccess,
		MigrateFailed,
		input,
		nil, // No query parameter for migrate
		func() (any, error) {
			return e.collection.Migrate(description, cb, dryRun)
		},
	)

	if err != nil {
		return struct {
			Schema  schema.SchemaDefinition `json:"schema"`
			Preview any                   `json:"preview"`
		}{}, err
	}

	return result.(struct {
		Schema  schema.SchemaDefinition `json:"schema"`
		Preview any                   `json:"preview"`
	}), nil
}

// Metadata delegates the call to the underlying collection's Metadata method,
// but also emits a telemetry event to record that metadata was requested.
func (e *Collection) Metadata(
	filter *MetadataFilter,
	forceRefresh bool,
) (Metadata, error) {
	startTime := time.Now()

	// Emit telemetry event for metadata calls
	telemetryEvent := createEvent(
		MetadataCalled,
		"metadata",
		e.schema.Name,
		map[string]any{
			"filter":       filter,
			"forceRefresh": forceRefresh,
		},
		nil, // No output in the event itself
		nil, // No query parameter
		nil, // No error
		nil, // No issues
		startTime,
	)
	e.emitEvent(telemetryEvent)

	return e.collection.Metadata(filter, forceRefresh)
}

// RegisterSubscription wraps the underlying collection's RegisterSubscription method,
// emitting an event after a new subscription is successfully registered.
func (e *Collection) RegisterSubscription(options RegisterSubscriptionOptions) string {
	id := e.collection.RegisterSubscription(options)

	// Emit subscription register event
	event := createEvent(
		SubscriptionRegister,
		"register_subscription",
		e.schema.Name,
		map[string]any{
			"event":       options.Event,
			"label":       options.Label,
			"description": options.Description,
		},
		map[string]any{
			"subscriptionId": id,
		},
		nil, // No query parameter
		nil, // No error
		nil, // No issues
		time.Now(),
	)
	e.emitEvent(event)

	return id
}

// UnregisterSubscription wraps the underlying collection's UnregisterSubscription method,
// emitting an event after a subscription is successfully unregistered.
func (e *Collection) UnregisterSubscription(id string) {
	e.collection.UnregisterSubscription(id)

	// Emit subscription unregister event
	event := createEvent(
		SubscriptionUnregister,
		"unregister_subscription",
		e.schema.Name,
		map[string]any{
			"subscriptionId": id,
		},
		nil, // No output
		nil, // No query parameter
		nil, // No error
		nil, // No issues
		time.Now(),
	)
	e.emitEvent(event)
}

// Subscriptions delegates the call to the underlying collection's Subscriptions method.
// No events are emitted for this operation.
func (e *Collection) Subscriptions() ([]SubscriptionInfo, error) {
	return e.collection.Subscriptions()
}

