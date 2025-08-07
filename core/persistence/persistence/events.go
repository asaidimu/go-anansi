package persistence

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
	"go.uber.org/zap"
)

// eventsPersistence is a wrapper around a base.Persistence that adds event-emitting capabilities.
// It intercepts method calls to the underlying persistence, emits events for the start,
// success, and failure of each operation, and then calls the original method.
// This provides a mechanism for observability and for triggering side effects in a
// decoupled manner.
type eventsPersistence struct {
	persistence base.Persistence
	bus         *events.TypedEventBus[base.PersistenceEvent]
	logger      *zap.Logger
}


var _ base.Persistence = (*eventsPersistence)(nil)

// newEventEmittingPersistence creates a new event-emitting persistence wrapper.
func newEventEmittingPersistence(persistence base.Persistence, bus *events.TypedEventBus[base.PersistenceEvent], logger *zap.Logger) base.Persistence {
	return &eventsPersistence{
		persistence: persistence,
		bus:         bus,
		logger:      logger,
	}
}

// emitEvent is a helper method to publish a persistence event to the event bus.
func (e *eventsPersistence) emitEvent(event base.PersistenceEvent) {
	if e.bus != nil {
		e.bus.Emit(string(event.Type), event)
	}
}

// withEventEmission is a higher-order function that wraps a persistence operation
// with start, success, and failure events. It handles the timing of the operation
// and constructs the appropriate event for each stage.
func (e *eventsPersistence) withEventEmission(
	_ context.Context,
	operation string,
	startEventType base.PersistenceEventType,
	successEventType base.PersistenceEventType,
	failedEventType base.PersistenceEventType,
	input any,
	output any,
	fn func() (any, error),
) (any, error) {
	startTime := time.Now()

	// Emit start event
	startEvent := utils.CreateEvent(
		startEventType,
		operation,
		"", // Collection name is not applicable for top-level persistence events
		input,
		nil, // No output yet
		nil, // No query parameter for top-level events
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
			"", // Collection name is not applicable for top-level persistence events
			input,
			nil, // No output on failure
			nil, // No query parameter for top-level events
			&errStr,
			nil, // Issues can be added here if available
			startTime,
		)
		e.emitEvent(failEvent)
		return nil, err
	}

	// Emit success event
	successEvent := utils.CreateEvent(
		successEventType,
		operation,
		"", // Collection name is not applicable for top-level persistence events
		input,
		output, // Use the provided output for success event
		nil, // No query parameter for top-level events
		nil, // No error on success
		nil, // No issues on success
		startTime,
	)
	e.emitEvent(successEvent)

	return result, nil
}

// Create wraps the underlying persistence's Create method, adding event emission.
func (e *eventsPersistence) Create(sc schema.SchemaDefinition) (base.Collection, error) {
	result, err := e.withEventEmission(
		context.Background(),
		"createCollection",
		base.CollectionCreateStart,
		base.CollectionCreateSuccess,
		base.CollectionCreateFailed,
		sc, // Input is the schema definition
		nil, // Output will be set after the operation
		func() (any, error) {
			return e.persistence.Create(sc)
		},
	)

	if err != nil {
		return nil, err
	}

	return result.(base.Collection), nil
}

// Delete wraps the underlying persistence's Delete method, adding event emission.
func (e *eventsPersistence) Delete(id string) (bool, error) {
	result, err := e.withEventEmission(
		context.Background(),
		"deleteCollection",
		base.CollectionDeleteStart,
		base.CollectionDeleteSuccess,
		base.CollectionDeleteFailed,
		id, // Input is the collection ID
		nil, // Output will be set after the operation
		func() (any, error) {
			return e.persistence.Delete(id)
		},
	)

	if err != nil {
		return false, err
	}

	return result.(bool), nil
}

// All other methods simply delegate to the wrapped persistence.

func (e *eventsPersistence) Collection(name string) (base.Collection, error) {
	return e.persistence.Collection(name)
}

func (e *eventsPersistence) Collections() ([]string, error) {
	return e.persistence.Collections()
}

func (e *eventsPersistence) Metadata(filter *base.MetadataFilter) (base.Metadata, error) {
	return e.persistence.Metadata(filter)
}

func (e *eventsPersistence) RegisterSubscription(options base.RegisterSubscriptionOptions) string {
	return e.persistence.RegisterSubscription(options)
}

func (e *eventsPersistence) Schema(id string, version ...string) (*schema.SchemaDefinition, error) {
	return e.persistence.Schema(id, version...)
}

func (e *eventsPersistence) Subscriptions() ([]base.SubscriptionInfo, error) {
	return e.persistence.Subscriptions()
}

func (e *eventsPersistence) Transact(callback func(tx base.BasePersistence) (any, error)) (any, error) {
	result, err := e.withEventEmission(
		context.Background(),
		"transact",
		base.TransactionStart,
		base.TransactionSuccess,
		base.TransactionFailed,
		nil, // No specific input for transaction start
		nil, // Output will be set after the operation
		func() (any, error) {
			return e.persistence.Transact(callback)
		},
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (e *eventsPersistence) UnregisterSubscription(id string) {
	e.persistence.UnregisterSubscription(id)
}

func (e *eventsPersistence) Rollback(
	name string,
	version *string,
	dryRun *bool,
) (base.Collection, error) {
	return e.persistence.Rollback(name, version, dryRun)
}

func (e *eventsPersistence) Migrate(
	name string,
	migration schema.Migration,
	dryRun *bool,
) (base.Collection, error) {
	return e.persistence.Migrate(name, migration, dryRun)
}

func (e *eventsPersistence) Close() {
	e.persistence.Close()
}
