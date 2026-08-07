package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/events"
	goevents "github.com/asaidimu/go-events/v2"
	"github.com/gofrs/uuid/v5"
)

// goEventsBusAdapter adapts a go-events v2 raw EventBus to the anansi
// EventBus interface. Payloads are JSON-encoded on emit and decoded on
// delivery, mirroring go-events' SimpleEventBus. Subscriber IDs are
// auto-generated so callers don't need to manage checkpoint identity.
type goEventsBusAdapter[T any] struct {
	bus     *goevents.EventBus
	counter atomic.Int64
}

// NewGoEventsBusAdapter creates a new adapter for a go-events v2 EventBus.
func NewGoEventsBusAdapter[T any](bus *goevents.EventBus) events.EventBus[T] {
	return &goEventsBusAdapter[T]{bus: bus}
}

// Emit serialises event as JSON and publishes it to the given topic.
func (a *goEventsBusAdapter[T]) Emit(_ context.Context, eventType string, event T) {
	if a.bus == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("goEventsBusAdapter: marshal failed: eventType=%s err=%v", eventType, err)
		return
	}
	if err := a.bus.Publish(eventType, data); err != nil {
		log.Printf("goEventsBusAdapter: publish failed: eventType=%s err=%v", eventType, err)
	}
}

// Subscribe registers a handler for the given event type via a request.
// Live delivery is the default (Replay false); opting into Replay replays
// history from Cursor (or the beginning when Cursor is nil).
func (a *goEventsBusAdapter[T]) Subscribe(req events.SubscriptionRequest[T]) func() {
	if a.bus == nil {
		return func() {}
	}
	id := fmt.Sprintf("simple:%s:%d", req.EventType, a.counter.Add(1))

	opts := goevents.SubscribeOptions{
		LiveOnly:       !req.Replay,
		LiveBufferSize: 1024,
	}
	if req.Replay {
		opts.StartAt = a.cursorToUUID(req.Cursor)
	}

	filters := req.Filters
	return a.bus.SubscribeWithOptions(id, req.EventType, func(ctx context.Context, raw goevents.Event) error {
		var typed T
		if err := json.Unmarshal(raw.Payload, &typed); err != nil {
			return fmt.Errorf("goEventsBusAdapter: unmarshal: %w", err)
		}
		for _, f := range filters {
			if f != nil && !f(ctx, typed) {
				return nil
			}
		}
		return req.Handler(ctx, typed)
	}, opts)
}

// cursorToUUID maps an anansi replay cursor to a go-events sequence UUID.
// Supported cursor types: time.Time, uuid.UUID, and string (UUID). A nil or
// unrecognised cursor yields the zero UUID (start from the beginning).
func (a *goEventsBusAdapter[T]) cursorToUUID(cursor any) uuid.UUID {
	switch c := cursor.(type) {
	case nil:
		return uuid.Nil
	case time.Time:
		return goevents.UUIDForTime(c)
	case uuid.UUID:
		return c
	case string:
		parsed, err := uuid.FromString(c)
		if err != nil {
			return uuid.Nil
		}
		return parsed
	default:
		return uuid.Nil
	}
}