package events

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/events"
	goevents "github.com/asaidimu/go-events"
)


// goEventsBusAdapter adapts go-events.TypedEventBus to the EventBus interface.
type goEventsBusAdapter[T any] struct {
	bus *goevents.TypedEventBus[T]
}

// NewGoEventsBusAdapter creates a new adapter for go-events.TypedEventBus.
func NewGoEventsBusAdapter[T any](bus *goevents.TypedEventBus[T]) events.EventBus[T] {
	return &goEventsBusAdapter[T]{bus: bus}
}

// Emit publishes a persistence event to the underlying go-events.TypedEventBus.
func (a *goEventsBusAdapter[T]) Emit(eventType string, event T) {
	if a.bus != nil {
		a.bus.Emit(eventType, event)
	}
}

// Subscribe registers a handler for a persistence event with the underlying go-events.TypedEventBus.
func (a *goEventsBusAdapter[T]) Subscribe(eventType string, handler func(ctx context.Context, event T) error, filters ...func(ctx context.Context, event T) bool) func() {
	if len(filters) > 0 && filters[0] != nil {
		filter := filters[0]
		options := goevents.SubscribeOptions{
			Filter: func(event goevents.Event) bool {
				payload, ok := event.Payload.(T)
				if !ok {
					return false
				}
				// Call our contextual filter with context.Background() as go-events.EventFilter does not provide context
				return filter(context.Background(), payload)
			},
		}
		return a.bus.SubscribeWithOptions(eventType, handler, options)
	}
	// Otherwise, use the basic Subscribe
	return a.bus.Subscribe(eventType, handler)
}
