package persistence

import (
	"time"

	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
	"github.com/asaidimu/go-events"
)

// Collection wraps a PersistenceCollection and adds event emission
type Collection struct {
	collection *CollectionBase
	bus        *events.TypedEventBus[PersistenceEvent]
	schema     *schema.SchemaDefinition
}

// NewEventEmittingCollection creates a new event-emitting collection wrapper
func NewEventEmittingCollection(collection *CollectionBase) *Collection {
	return &Collection{
		collection: collection,
		bus:        collection.bus,
		schema:     collection.schema,
	}
}

// emitEvent is a helper method to emit events
func (e *Collection) emitEvent(event PersistenceEvent) {
	if e.bus != nil {
		e.bus.Emit(string(event.Type), event)
	}
}

// withEventEmission wraps an operation with start, success, and failure events
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
		nil,
		queryParam,
		nil,
		nil,
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
			nil,
			queryParam,
			&errStr,
			nil,
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
		nil,
		nil,
		startTime,
	)
	e.emitEvent(successEvent)

	return result, nil
}

// Create wraps the collection's Create method with event emission
func (e *Collection) Create(data any) (any, error) {
	result, err := e.withEventEmission(
		"create",
		DocumentCreateStart,
		DocumentCreateSuccess,
		DocumentCreateFailed,
		data,
		nil,
		func() (any, error) {
			return e.collection.Create(data)
		},
	)

	if err != nil {
		return nil, err
	}

	return result.(*query.QueryResult), nil
}

// Read wraps the collection's Read method with event emission
func (e *Collection) Read(q *query.QueryDSL) (*query.QueryResult, error) {
	result, err := e.withEventEmission(
		"read",
		DocumentReadStart,
		DocumentReadSuccess,
		DocumentReadFailed,
		nil,
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

// Update wraps the collection's Update method with event emission
func (e *Collection) Update(params *CollectionUpdate) (int, error) {
	result, err := e.withEventEmission(
		"update",
		DocumentUpdateStart,
		DocumentUpdateSuccess,
		DocumentUpdateFailed,
		params,
		params.Filter,
		func() (any, error) {
			count, err := e.collection.Update(params)
			return count, err
		},
	)

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Delete wraps the collection's Delete method with event emission
func (e *Collection) Delete(params *query.QueryFilter, unsafe bool) (int, error) {
	result, err := e.withEventEmission(
		"delete",
		DocumentDeleteStart,
		DocumentDeleteSuccess,
		DocumentDeleteFailed,
		params,
		params,
		func() (any, error) {
			count, err := e.collection.Delete(params, unsafe)
			return count, err
		},
	)

	if err != nil {
		return 0, err
	}

	return result.(int), nil
}

// Validate delegates to the underlying collection (no events needed for validation)
func (e *Collection) Validate(data any, loose bool) (*schema.ValidationResult, error) {
	return e.collection.Validate(data, loose)
}

// Rollback wraps the collection's Rollback method with event emission
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
		nil,
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

// Migrate wraps the collection's Migrate method with event emission
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
		nil,
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

// Metadata delegates to the underlying collection with telemetry event
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
		nil,
		nil,
		nil,
		nil,
		startTime,
	)
	e.emitEvent(telemetryEvent)

	return e.collection.Metadata(filter, forceRefresh)
}

// RegisterSubscription wraps subscription registration with event emission
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
		nil,
		nil,
		nil,
		time.Now(),
	)
	e.emitEvent(event)

	return id
}

// UnregisterSubscription wraps subscription unregistration with event emission
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
		nil,
		nil,
		nil,
		nil,
		time.Now(),
	)
	e.emitEvent(event)
}

// Subscriptions delegates to the underlying collection
func (e *Collection) Subscriptions() ([]SubscriptionInfo, error) {
	return e.collection.Subscriptions()
}

