package events

import (
	"context"
)

// EventBus defines the interface for an event bus that can emit and subscribe to events of type T.
type EventBus[T any] interface {
	Emit(eventType string, event T)
	// Subscribe registers a handler for an event of type T, with an optional filter function.
	// If a filter is provided, the handler will only be called if the filter returns true.
	// Returns a function to unsubscribe.
	Subscribe(eventType string, handler func(ctx context.Context, event T) error, filter ...func(ctx context.Context, event T) bool) func()
}
