package persistence

import (
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/core"
	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-events"
)

// Collection wraps a PersistenceCollection and adds event emission
type Collection struct {
	collection *CollectionBase
	bus        *events.TypedEventBus[core.PersistenceEvent]
	schema     *core.SchemaDefinition
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
func (e *Collection) emitEvent(event core.PersistenceEvent) {
	if e.bus != nil {
		e.bus.Emit(string(event.Type), event)
	}
}

// withEventEmission wraps an operation with start, success, and failure events
func (e *Collection) withEventEmission(
	operation string,
	startEventType core.PersistenceEventType,
	successEventType core.PersistenceEventType,
	failedEventType core.PersistenceEventType,
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
		core.DocumentCreateStart,
		core.DocumentCreateSuccess,
		core.DocumentCreateFailed,
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
func (e *Collection) Read(input any) (any, error) {
	query, ok := input.(query.QueryDSL)
	if ! ok {
		return nil, fmt.Errorf("Input to read is not a valid QueryDSL")
	}
	result, err := e.withEventEmission(
		"read",
		core.DocumentReadStart,
		core.DocumentReadSuccess,
		core.DocumentReadFailed,
		nil,
		query,
		func() (any, error) {
			return e.collection.Read(query)
		},
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}

// Update wraps the collection's Update method with event emission
func (e *Collection) Update(params *core.CollectionUpdate) (int, error) {
	result, err := e.withEventEmission(
		"update",
		core.DocumentUpdateStart,
		core.DocumentUpdateSuccess,
		core.DocumentUpdateFailed,
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
func (e *Collection) Delete(params any, unsafe bool) (int, error) {
	result, err := e.withEventEmission(
		"delete",
		core.DocumentDeleteStart,
		core.DocumentDeleteSuccess,
		core.DocumentDeleteFailed,
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
func (e *Collection) Validate(data any, loose bool) (*core.ValidationResult, error) {
	return e.collection.Validate(data, loose)
}

// Rollback wraps the collection's Rollback method with event emission
func (e *Collection) Rollback(version *string, dryRun *bool) (struct {
	Schema  core.SchemaDefinition `json:"schema"`
	Preview any                   `json:"preview"`
}, error) {
	input := map[string]any{
		"version": version,
		"dryRun":  dryRun,
	}

	result, err := e.withEventEmission(
		"rollback",
		core.RollbackStart,
		core.RollbackSuccess,
		core.RollbackFailed,
		input,
		nil,
		func() (any, error) {
			return e.collection.Rollback(version, dryRun)
		},
	)

	if err != nil {
		return struct {
			Schema  core.SchemaDefinition `json:"schema"`
			Preview any                   `json:"preview"`
		}{}, err
	}

	return result.(struct {
		Schema  core.SchemaDefinition `json:"schema"`
		Preview any                   `json:"preview"`
	}), nil
}

// Migrate wraps the collection's Migrate method with event emission
func (e *Collection) Migrate(
	description string,
	cb func(h core.SchemaMigrationHelper) (core.DataTransform[any, any], error),
	dryRun *bool,
) (struct {
	Schema  core.SchemaDefinition `json:"schema"`
	Preview any                   `json:"preview"`
}, error) {
	input := map[string]any{
		"description": description,
		"dryRun":      dryRun,
	}

	result, err := e.withEventEmission(
		"migrate",
		core.MigrateStart,
		core.MigrateSuccess,
		core.MigrateFailed,
		input,
		nil,
		func() (any, error) {
			return e.collection.Migrate(description, cb, dryRun)
		},
	)

	if err != nil {
		return struct {
			Schema  core.SchemaDefinition `json:"schema"`
			Preview any                   `json:"preview"`
		}{}, err
	}

	return result.(struct {
		Schema  core.SchemaDefinition `json:"schema"`
		Preview any                   `json:"preview"`
	}), nil
}

// Metadata delegates to the underlying collection with telemetry event
func (e *Collection) Metadata(
	filter *core.MetadataFilter,
	forceRefresh bool,
) (core.Metadata, error) {
	startTime := time.Now()

	// Emit telemetry event for metadata calls
	telemetryEvent := createEvent(
		core.MetadataCalled,
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
func (e *Collection) RegisterSubscription(options core.RegisterSubscriptionOptions) string {
	id := e.collection.RegisterSubscription(options)

	// Emit subscription register event
	event := createEvent(
		core.SubscriptionRegister,
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
		core.SubscriptionUnregister,
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

// RegisterTrigger wraps trigger registration with event emission
func (e *Collection) RegisterTrigger(options core.RegisterTriggerOptions) (core.TriggerInfo, error) {
	result, err := e.collection.RegisterTrigger(options)

	var errStr *string
	if err != nil {
		s := err.Error()
		errStr = &s
	}

	// Emit trigger register event
	event := createEvent(
		core.TriggerRegister,
		"register_trigger",
		e.schema.Name,
		options,
		result,
		nil,
		errStr,
		nil,
		time.Now(),
	)
	e.emitEvent(event)

	return result, err
}

// UnregisterTrigger wraps trigger unregistration with event emission
func (e *Collection) UnregisterTrigger(options core.UnregisterTriggerOptions) error {
	err := e.collection.UnregisterTrigger(options)

	var errStr *string
	if err != nil {
		s := err.Error()
		errStr = &s
	}

	// Emit trigger unregister event
	event := createEvent(
		core.TriggerUnregister,
		"unregister_trigger",
		e.schema.Name,
		options,
		nil,
		nil,
		errStr,
		nil,
		time.Now(),
	)
	e.emitEvent(event)

	return err
}

// RegisterTask wraps task registration with event emission
func (e *Collection) RegisterTask(options core.RegisterTaskOptions) (core.TaskInfo, error) {
	result, err := e.collection.RegisterTask(options)

	var errStr *string
	if err != nil {
		s := err.Error()
		errStr = &s
	}

	// Emit task register event
	event := createEvent(
		core.TaskRegister,
		"register_task",
		e.schema.Name,
		options,
		result,
		nil,
		errStr,
		nil,
		time.Now(),
	)
	e.emitEvent(event)

	return result, err
}

// UnregisterTask wraps task unregistration with event emission
func (e *Collection) UnregisterTask(options core.UnregisterTaskOptions) error {
	err := e.collection.UnregisterTask(options)

	var errStr *string
	if err != nil {
		s := err.Error()
		errStr = &s
	}

	// Emit task unregister event
	event := createEvent(
		core.TaskUnregister,
		"unregister_task",
		e.schema.Name,
		options,
		nil,
		nil,
		errStr,
		nil,
		time.Now(),
	)
	e.emitEvent(event)

	return err
}

// Subscriptions delegates to the underlying collection
func (e *Collection) Subscriptions() ([]core.SubscriptionInfo, error) {
	return e.collection.Subscriptions()
}

// Triggers delegates to the underlying collection
func (e *Collection) Triggers() ([]core.TriggerInfo, error) {
	return e.collection.Triggers()
}

// Tasks delegates to the underlying collection
func (e *Collection) Tasks() ([]core.TaskInfo, error) {
	return e.collection.Tasks()
}
