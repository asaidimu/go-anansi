package events

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"go.uber.org/zap"
)

// EventContextKeysKey holds a slice of keys for values to include in events
const EventContextKeysKey common.ContextKey = "github.com/asaidimu/go-anansi/__event_contexts__"

// WithEventContext adds keys to the context that should be extracted for events
func WithEventContext(ctx context.Context, keys ...string) context.Context {
	// Get existing keys if any
	var existingKeys []string
	if existing, ok := ctx.Value(EventContextKeysKey).([]string); ok {
		existingKeys = existing
	}

	// Append new keys
	allKeys := append(existingKeys, keys...)
	return context.WithValue(ctx, EventContextKeysKey, allKeys)
}

// WithEventContextValue is a convenience function to add both a key-value pair and register the key for events
func WithEventContextValue(ctx context.Context, key string, value any) context.Context {
	ctx = context.WithValue(ctx, common.ContextKey(key), value)
	return WithEventContext(ctx, key)
}

// EventEmitter provides common event emission functionality for any component
type EventEmitter[T any] struct {
	bus            EventBus[T]
	logger         *zap.Logger
}

// NewEventEmitter creates a new generic event emitter
func NewEventEmitter[T any](bus EventBus[T], logger *zap.Logger) *EventEmitter[T] {
	return &EventEmitter[T]{
		bus:    bus,
		logger: logger,
	}
}

// EmitEvent publishes an event to the event bus
func (e *EventEmitter[T]) EmitEvent(eventType string, event T) {
	if e.bus != nil {
		e.bus.Emit(eventType, event)
	}
}

// OperationConfig contains the configuration for wrapping an operation with events
// This struct is currently persistence-specific due to PersistenceEventType.
// For a truly generic emitter, this would need to be generic or moved.
// For now, we'll keep it here and address its genericity later if needed.
type OperationConfig struct {
	Operation        string
	StartEventType   string
	SuccessEventType string
	FailedEventType  string
	Input            any
	QueryParam       any
}

// WithEventEmission wraps any operation with start, success, and failure events
// This function is highly coupled to the persistence layer's event structure (e.g., createEvent, extractTransactionID).
// For a truly generic emitter, this function would need significant refactoring or removal.
// For now, we'll move it and then address its genericity.
func (e *EventEmitter[T]) WithEventEmission(
	ctx context.Context,
	config OperationConfig,
	fn func() (any, error),
	eventFactory func(eventType string, operation string, input any, output any, query any, errorMsg *string, transactionID *string, startTime time.Time, duration *int64, contextMap map[string]any) T, // Pass factory
	extractTransactionID func(ctx context.Context) *string, // Pass extractor
) (any, error) {
	startTime := time.Now()

	// Extract transaction ID and context values
	transactionID := extractTransactionID(ctx) // Use passed extractor
	contextMap := e.extractContextValues(ctx)

	// Emit start event using the provided factory
	startEvent := eventFactory(
		config.StartEventType,
		config.Operation,
		config.Input,
		nil, // No output yet
		config.QueryParam,
		nil, // No error yet
		transactionID,
		startTime,
		nil, // No duration for start event
		contextMap,
	)
	e.EmitEvent(config.StartEventType, startEvent)

	// Execute the operation
	result, err := fn()

	// Calculate duration in milliseconds
	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		// Emit failure event using the provided factory
		errStr := err.Error()
		failEvent := eventFactory(
			config.FailedEventType,
			config.Operation,
			config.Input,
			nil, // No output on failure
			config.QueryParam,
			&errStr,
			transactionID,
			startTime,
			&duration,
			contextMap,
		)
		e.EmitEvent(config.FailedEventType, failEvent)
		return result, err
	}

	// Emit success event using the provided factory
	successEvent := eventFactory(
		config.SuccessEventType,
		config.Operation,
		config.Input,
		result,
		config.QueryParam,
		nil, // No error on success
		transactionID,
		startTime,
		&duration,
		contextMap,
	)
	e.EmitEvent(config.SuccessEventType, successEvent)

	return result, nil
}

// extractContextValues extracts context values based on keys stored in the context
// This function can remain generic as it operates on common.ContextKey and any values.
func (e *EventEmitter[T]) extractContextValues(ctx context.Context) map[string]any {
	contextMap := make(map[string]any)

	// Extract context keys from the special context value
	if contextKeys, ok := ctx.Value(EventContextKeysKey).([]string); ok {
		for _, key := range contextKeys {
			if value := ctx.Value(common.ContextKey(key)); value != nil {
				contextMap[key] = value
			}
		}
	}

	// Also check for context keys as regular strings
	if contextKeys, ok := ctx.Value(EventContextKeysKey).([]common.ContextKey); ok {
		for _, key := range contextKeys {
			if value := ctx.Value(key); value != nil {
				contextMap[string(key)] = value
			}
		}
	}

	// Return nil if no context values found
	if len(contextMap) == 0 {
		return nil
	}

	return contextMap
}
