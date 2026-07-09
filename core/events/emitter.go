package events

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/utils"
	"go.uber.org/zap"
)

// EventEmitter provides common event emission functionality for any component.
// It wraps an EventBus and provides higher-level functionality including
// operation instrumentation and pattern-based event subscription.
type EventEmitter[T any] struct {
	bus     EventBus[T]
	Factory EventFactory[T]
	logger  *zap.Logger
}

// EventFactory defines a function that creates event objects of type T.
// It standardizes event creation with common metadata including timing,
// operation context, and error information.
type EventFactory[T any] func(
	ctx context.Context,
	eventType string,
	operation string,
	input any,
	output any,
	errorMsg *string,
	startTime time.Time,
	duration *int64,
	extra map[string]any,
) T

// NewEventEmitter creates a new generic event emitter with the specified
// event bus, factory function, and logger.
func NewEventEmitter[T any](bus EventBus[T], factory EventFactory[T], logger *zap.Logger) *EventEmitter[T] {
	return &EventEmitter[T]{
		bus:     bus,
		logger:  logger,
		Factory: factory,
	}
}

// EmitEvent publishes an event to the underlying event bus.
// Events are published to both the specific event type topic and the wildcard "*" topic
// to support both exact and pattern-based subscriptions.
func (e *EventEmitter[T]) EmitEvent(ctx context.Context, eventType string, event T) {
	if e.bus != nil {
		// Publish to specific topic for exact subscribers
		e.bus.Emit(ctx, eventType, event)
		// Publish to wildcard topic for pattern subscribers
		e.bus.Emit(ctx, "*", event)
	}
}

// isWildcardPattern determines if a pattern string represents a simple wildcard
// pattern rather than a complex regular expression. Simple wildcards contain
// only '*' characters and basic text, making them faster to process.
//
// Examples:
//   - "document:*" -> true (simple wildcard)
//   - "*:success" -> true (simple wildcard)
//   - "doc.*:create" -> false (regex pattern)
//   - "^document:" -> false (regex pattern)
func isWildcardPattern(pattern string) bool {
	// Must contain at least one wildcard
	if !strings.Contains(pattern, "*") {
		return false
	}

	// Check for regex special characters that indicate complex patterns
	regexChars := []string{".", "^", "$", "+", "?", "{", "}", "[", "]", "(", ")", "|", "\\"}
	for _, char := range regexChars {
		if strings.Contains(pattern, char) {
			return false
		}
	}

	return true
}

// matchesWildcard performs fast wildcard matching for simple patterns.
// Supports prefix wildcards (e.g., "document:*"), suffix wildcards (e.g., "*:success"),
// and exact matches. This is significantly faster than regex for simple patterns.
func matchesWildcard(eventType, pattern string) bool {
	// Handle exact match first
	if pattern == eventType {
		return true
	}

	// Handle global wildcard - matches everything
	if pattern == "*" {
		return true
	}

	// Handle prefix wildcard (e.g., "document:*")
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(eventType, prefix)
	}

	// Handle suffix wildcard (e.g., "*:success")
	if after, ok := strings.CutPrefix(pattern, "*"); ok {
		suffix := after
		return strings.HasSuffix(eventType, suffix)
	}

	return false
}

// createEventTypeFilter creates a filter function that matches events against
// the specified eventType pattern. Returns nil for exact matches (no filtering needed)
// to optimize performance for non-pattern subscriptions.
//
// The filter extracts the actual event type from the context (set during emission)
// and compares it against the subscription pattern using either wildcard matching
// or compiled regex, depending on the pattern complexity.
func (e *EventEmitter[T]) createEventTypeFilter(eventTypePattern string) func(ctx context.Context, event T) bool {
	// Optimization: exact matches don't need filtering
	if !strings.Contains(eventTypePattern, "*") && !isRegexPattern(eventTypePattern) {
		return nil
	}

	var compiledRegex *regexp.Regexp
	isWildcard := isWildcardPattern(eventTypePattern)

	// For complex patterns, compile as regex with error handling
	if !isWildcard {
		var err error
		compiledRegex, err = regexp.Compile(eventTypePattern)
		if err != nil {
			if e.logger != nil {
				e.logger.Warn("Invalid regex pattern, treating as literal match",
					zap.String("pattern", eventTypePattern),
					zap.Error(err))
			}
			// Graceful degradation: treat as exact match
			return nil
		}
	}

	return func(ctx context.Context, event T) bool {
		// Extract the actual event type from context (set during emission)
		eventType := ctx.Value("eventType")
		if eventType == nil {
			// No event type available - let event through to avoid silent failures
			return true
		}

		eventTypeStr, ok := eventType.(string)
		if !ok {
			// Invalid event type format - let through with warning
			return true
		}

		// Use appropriate matching strategy based on pattern type
		if isWildcard {
			return matchesWildcard(eventTypeStr, eventTypePattern)
		} else {
			return compiledRegex.MatchString(eventTypeStr)
		}
	}
}

// isRegexPattern detects if a pattern contains regex special characters,
// indicating it should be treated as a regular expression rather than
// a simple wildcard pattern.
func isRegexPattern(pattern string) bool {
	regexChars := []string{".", "^", "$", "+", "?", "{", "}", "[", "]", "(", ")", "|", "\\"}
	for _, char := range regexChars {
		if strings.Contains(pattern, char) {
			return true
		}
	}
	return false
}

// Subscribe registers an event handler for the specified event type pattern.
// This method automatically handles pattern matching, allowing subscribers to use:
//
//   - Exact event types: "document:create:success"
//   - Wildcard patterns: "document:*", "*:success", "*"
//   - Regex patterns: "^(document|collection):(create|update):.*"
//
// For exact matches, the subscription is made directly to the event bus for optimal
// performance. For patterns, the subscription is made to "*" with automatic filtering.
//
// Additional filters can be provided to further restrict which events are delivered
// to the handler. All filters must return true for the event to be processed.
//
// Returns an unsubscribe function that removes the subscription when called.
//
// Examples:
//
//	// Listen to all document events
//	unsubscribe := emitter.Subscribe("document:*", handler)
//
//	// Listen to all success events with custom filtering
//	unsubscribe := emitter.Subscribe("*:success", handler, customFilter)
//
//	// Listen to specific event (no filtering overhead)
//	unsubscribe := emitter.Subscribe("document:create:success", handler)
func (e *EventEmitter[T]) Subscribe(eventType string, handler func(ctx context.Context, event T) error, filters ...func(ctx context.Context, event T) bool) func() {
	if e.bus == nil {
		return func() {} // Return no-op unsubscribe for nil bus
	}

	// Create automatic event type filter for patterns
	eventTypeFilter := e.createEventTypeFilter(eventType)

	// Combine automatic pattern filter with user-provided filters
	var allFilters []func(ctx context.Context, event T) bool
	if eventTypeFilter != nil {
		allFilters = append(allFilters, eventTypeFilter)
	}
	allFilters = append(allFilters, filters...)

	// Determine subscription strategy based on pattern presence
	subscriptionEventType := eventType
	if eventTypeFilter != nil {
		// Pattern detected: subscribe to wildcard and filter
		subscriptionEventType = "*"
	}

	// Exact match: subscribe directly (no filtering overhead)

	return e.bus.Subscribe(subscriptionEventType, handler, allFilters...)
}

// OperationConfig defines the configuration for instrumenting operations with
// automatic event emission. This includes the operation name, event types to emit
// at different stages, and optional input/output data to include in events.
//
// Note: This struct is currently persistence-specific due to the string event types.
// For a truly generic emitter, this would need to be parameterized or moved to
// a domain-specific layer. This design choice prioritizes current usability over
// complete genericity and can be refactored later if needed.
type OperationConfig struct {
	// Operation is a human-readable name for the operation being instrumented
	Operation string

	// StartEventTypes are emitted before the operation begins
	StartEventTypes []string

	// SuccessEventTypes are emitted after successful operation completion
	SuccessEventTypes []string

	// FailedEventTypes are emitted when the operation fails
	FailedEventTypes []string

	// Input data to include in emitted events (optional)
	Input any

	// Output data to include in success events (optional, overridden by actual result)
	Output any

	//
	Extra map[string]any
}

// ensureWildcard adds the "*" wildcard event type to a slice if it doesn't
// already exist. This ensures that wildcard subscribers receive all events
// without requiring explicit configuration.
//
// This is an internal optimization that automatically adds wildcard emission
// to make the event system more discoverable and useful for monitoring tools.
func ensureWildcard(eventTypes []string) []string {
	if slices.Contains(eventTypes, "*") {
		return eventTypes
	}
	// Append wildcard to ensure global listeners receive events
	return append(eventTypes, "*")
}

// WithEventEmission wraps any operation with automatic event emission at key
// lifecycle points: start, success, and failure. This provides standardized
// instrumentation for operations without requiring manual event emission code.
//
// The wrapper:
//   - Emits start events before execution with timing information
//   - Executes the provided operation function
//   - Emits success events with results and duration on success
//   - Emits failure events with error details and duration on failure
//   - Automatically adds "*" to all event type lists for wildcard subscribers
//   - Enriches context with event type information for pattern-based filtering
//
// Timing is captured in milliseconds and included in success/failure events.
// The original operation result and error are returned unchanged.
//
// Example:
//
//	config := OperationConfig{
//	    Operation: "user:create",
//	    StartEventTypes: []string{"user:create:start"},
//	    SuccessEventTypes: []string{"user:create:success"},
//	    FailedEventTypes: []string{"user:create:failed"},
//	    Input: createUserRequest,
//	}
//
//	result, err := emitter.WithEventEmission(ctx, config, func() (any, error) {
//	    return userService.CreateUser(createUserRequest)
//	})
func (e *EventEmitter[T]) WithEventEmission(
	ctx context.Context,
	config OperationConfig,
	fn func() (any, error),
	actionQueue ...*utils.DeferredActionQueue,
) (any, error) {
	if e.bus == nil {
		return fn()
	}

	var deferredQueue *utils.DeferredActionQueue
	shouldDefer := len(actionQueue) > 0
	if shouldDefer {
		deferredQueue = actionQueue[0]
	}

	startEventTypes := ensureWildcard(config.StartEventTypes)
	successEventTypes := ensureWildcard(config.SuccessEventTypes)
	failedEventTypes := ensureWildcard(config.FailedEventTypes)

	startTime := time.Now()

	// Immediate event emission
	emitImmediateEvent := func(ictx context.Context,eventType string, event T) {
		e.EmitEvent(ictx, eventType, event)
	}

	// Deferred event emission - queue action
	emitDeferredEvent := func(eventType string, event T) {
		capturedCtx := ctx
		capturedEventType := eventType
		capturedEvent := event
		deferredQueue.Enqueue(func() {
			e.EmitEvent(capturedCtx, capturedEventType, capturedEvent)
		})
	}

	// ALWAYS emit start events immediately
	for _, eventType := range startEventTypes {
		eventCtx := context.WithValue(ctx, "eventType", eventType)
		startEvent := e.Factory(
			eventCtx,
			eventType,
			config.Operation,
			config.Input,
			nil,
			nil,
			startTime,
			nil,
			config.Extra,
		)
		emitImmediateEvent(eventCtx, eventType, startEvent)
	}

	// Execute the instrumented operation
	result, err := fn()

	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		// ALWAYS emit failure events immediately
		errStr := err.Error()
		for _, eventType := range failedEventTypes {
			eventCtx := context.WithValue(ctx, "eventType", eventType)
			failEvent := e.Factory(
				eventCtx,
				eventType,
				config.Operation,
				config.Input,
				nil,
				&errStr,
				startTime,
				&duration,
				config.Extra,
			)
			emitImmediateEvent(eventCtx, eventType, failEvent)
		}
		return result, err
	}

	// Emit success events - deferred if queue provided, immediate otherwise
	for _, eventType := range successEventTypes {
		eventCtx := context.WithValue(ctx, "eventType", eventType)
		successEvent := e.Factory(
			eventCtx,
			eventType,
			config.Operation,
			config.Input,
			result,
			nil,
			startTime,
			&duration,
			config.Extra,
		)
		if shouldDefer {
			emitDeferredEvent(eventType, successEvent)
		} else {
			emitImmediateEvent(eventCtx, eventType, successEvent)
		}
	}

	return result, nil
}
