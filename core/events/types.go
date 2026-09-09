package events

import (
	"context"
)

// SubscriptionRequest describes how a subscription should be created.
type SubscriptionRequest[T any] struct {
	// EventType is the exact topic to subscribe to.
	EventType string
	// Handler is called for each delivered event.
	Handler func(ctx context.Context, event T) error
	// Filters further restrict which events are delivered to the handler.
	// All filters must return true for the event to be processed.
	Filters []func(ctx context.Context, event T) bool
	// Replay indicates the subscriber opts into replaying history. When false
	// (the default) only events published after subscription are delivered
	// (live-only). When true the bus replays past events before going live.
	Replay bool
	// Cursor optionally bounds replay from a specific point. Supported types:
	// time.Time, uuid.UUID, or string (UUID). Ignored when Replay is false.
	Cursor any
}

// EventBus defines the interface for an event bus that can emit and subscribe to events of type T.
type EventBus[T any] interface {
	Emit(ctx context.Context, eventType string, event T)
	// Subscribe registers a handler for an event of type T via a
	// SubscriptionRequest. Returns a function to unsubscribe.
	Subscribe(req SubscriptionRequest[T]) func()
}
