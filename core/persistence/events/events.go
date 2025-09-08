// / Package events provides generic event-emitting wrappers for persistence layer components.
package events

import (
	"context"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
	"github.com/asaidimu/go-events"
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

// EventEmitter provides common event emission functionality for any persistence component
type EventEmitter struct {
	bus            *events.TypedEventBus[base.PersistenceEvent]
	collectionName string // Empty for persistence-level events
	logger         *zap.Logger
}

// NewEventEmitter creates a new event emitter
func NewEventEmitter(bus *events.TypedEventBus[base.PersistenceEvent], collectionName string, logger *zap.Logger) *EventEmitter {
	return &EventEmitter{
		bus:            bus,
		collectionName: collectionName,
		logger:         logger,
	}
}

// EmitEvent publishes a persistence event to the event bus
func (e *EventEmitter) EmitEvent(event base.PersistenceEvent) {
	if e.bus != nil {
		e.bus.Emit(string(event.Type), event)
	}
}

// OperationConfig contains the configuration for wrapping an operation with events
type OperationConfig struct {
	Operation        string
	StartEventType   base.PersistenceEventType
	SuccessEventType base.PersistenceEventType
	FailedEventType  base.PersistenceEventType
	Input            any
	QueryParam       any
}

// WithEventEmission wraps any operation with start, success, and failure events
func (e *EventEmitter) WithEventEmission(
	ctx context.Context,
	config OperationConfig,
	fn func() (any, error),
) (any, error) {
	startTime := time.Now()

	// Extract transaction ID and context values
	transactionID := e.extractTransactionID(ctx)
	contextMap := e.extractContextValues(ctx)

	// Emit start event using our own createEvent method
	startEvent := e.createEvent(
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
	e.EmitEvent(startEvent)

	// Execute the operation
	result, err := fn()

	// Calculate duration in milliseconds
	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		// Emit failure event using our own createEvent method
		errStr := err.Error()
		failEvent := e.createEvent(
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
		e.EmitEvent(failEvent)
		return result, err
	}

	// Emit success event using our own createEvent method
	successEvent := e.createEvent(
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
	e.EmitEvent(successEvent)

	return result, nil
}

// createEvent constructs a complete PersistenceEvent with all fields properly populated
func (e *EventEmitter) createEvent(
	eventType base.PersistenceEventType,
	operation string,
	input any,
	output any,
	query any,
	errorMsg *string,
	transactionID *string,
	startTime time.Time,
	duration *int64,
	contextMap map[string]any,
) base.PersistenceEvent {
	// Handle collection name (could be empty for persistence-level events)
	var collectionName *string
	if e.collectionName != "" {
		collectionName = &e.collectionName
	}

	// Extract issues if provided
	var issues []common.Issue

	// Create the complete event
	event := base.PersistenceEvent{
		Type:          eventType,
		Timestamp:     startTime.UnixMilli(),
		Operation:     operation,
		Collection:    collectionName,
		Input:         input,
		Output:        output,
		Error:         errorMsg,
		Issues:        issues,
		Query:         query,
		TransactionID: transactionID,
		Duration:      duration,
		Context:       contextMap,
	}

	// Log event creation if logger is available
	if e.logger != nil {
		etype := string(eventType)

		logLevel := zap.DebugLevel
		if errorMsg != nil {
			logLevel = zap.ErrorLevel
		} else if etype != "" && strings.Contains(strings.ToLower(etype), "success") {
			logLevel = zap.InfoLevel
		}

		e.logger.Log(logLevel, "Persistence event created",
			zap.String("type", string(eventType)),
			zap.String("operation", operation),
			zap.Any("collection", collectionName),
			zap.Any("transactionID", transactionID),
			zap.Any("duration", duration),
			zap.Bool("hasError", errorMsg != nil),
			zap.Int("issuesCount", len(issues)),
		)
	}

	return event
}

// extractTransactionID tries to get transaction ID from operation context or Go context
func (e *EventEmitter) extractTransactionID(ctx context.Context) *string {
	if tx, ok := transaction.GetCurrentTransaction(ctx); ok {
		id := tx.ID()
		return &id
	}

	return nil
}

// extractContextValues extracts context values based on keys stored in the context
func (e *EventEmitter) extractContextValues(ctx context.Context) map[string]any {
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

/*
Usage Examples:

1. Basic usage (existing behavior):
   collection.CreateOne(ctx, document)

2. Manual context setup:
   ctx = context.WithValue(ctx, TransactionIDKey, "tx_123")
   ctx = context.WithValue(ctx, ContextKey("userID"), "user_456")
   ctx = WithEventContext(ctx, "userID") // Register userID for event extraction
   collection.CreateOne(ctx, document)

3. Helper functions for easier context setup:
   ctx = WithEventContextValue(ctx, "userID", "user_456")
   ctx = WithEventContextValue(ctx, "requestID", "req_789")
   ctx = WithEventContextValue(ctx, "source", "mobile_app")
   collection.CreateOne(ctx, document)

The resulting PersistenceEvent will include:
- Type, Timestamp, Operation, Collection: Always populated
- Input, Output, Query: Based on operation
- TransactionID: From context or operation context
- Duration: Calculated in milliseconds from start to completion
- Context: Merged from Go context (via event keys) and operation context
- Issues: From operation context if provided
- Error: Only on failure events
*/
