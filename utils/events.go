package utils

import (
	"context"
	"encoding/json"
	"sync"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"go.uber.org/zap"
)

// contextStore manages contexts with reference counting for fanout support
type contextStore struct {
	contexts map[string]*contextEntry
	mu       sync.RWMutex
}

type contextEntry struct {
	ctx      context.Context
	refCount int
}

func newContextStore() *contextStore {
	return &contextStore{
		contexts: make(map[string]*contextEntry),
	}
}

func (cs *contextStore) store(uuid string, ctx context.Context, subscriberCount int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.contexts[uuid] = &contextEntry{
		ctx:      ctx,
		refCount: subscriberCount,
	}
}

func (cs *contextStore) retrieve(uuid string) (context.Context, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	entry, exists := cs.contexts[uuid]
	if !exists {
		return nil, false
	}

	ctx := entry.ctx
	entry.refCount--

	// Delete when all subscribers have retrieved it
	if entry.refCount <= 0 {
		delete(cs.contexts, uuid)
	}

	return ctx, true
}

func (cs *contextStore) cleanup(uuid string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.contexts, uuid)
}

// WatermillEventBus implements EventBus using Watermill's in-memory pub/sub
type WatermillEventBus[T any] struct {
	pubSub          *gochannel.GoChannel
	logger          *zap.Logger
	contextStore    *contextStore
	subscriberCount map[string]int // Track subscribers per event type
	subscriberMu    sync.RWMutex
}

// NewWatermillEventBus creates a new event bus instance
func NewWatermillEventBus[T any](logger *zap.Logger) *WatermillEventBus[T] {
	return &WatermillEventBus[T]{
		pubSub:          gochannel.NewGoChannel(gochannel.Config{}, nil),
		logger:          logger,
		contextStore:    newContextStore(),
		subscriberCount: make(map[string]int),
	}
}

// Emit publishes an event to the specified event type topic with context
func (eb *WatermillEventBus[T]) Emit(ctx context.Context, eventType string, event T) {
	payload, err := json.Marshal(event)
	if err != nil {
		eb.logger.Error("Failed to marshal event",
			zap.Error(err),
			zap.String("eventType", eventType))
		return
	}

	msg := message.NewMessage(watermill.NewUUID(), payload)

	// Get subscriber count for this event type
	eb.subscriberMu.RLock()
	count := eb.subscriberCount[eventType]
	eb.subscriberMu.RUnlock()

	// Only store context if there are subscribers
	if count > 0 {
		eb.contextStore.store(msg.UUID, ctx, count)
	}

	if err := eb.pubSub.Publish(eventType, msg); err != nil {
		eb.logger.Error("Failed to publish event",
			zap.Error(err),
			zap.String("eventType", eventType))
		// Clean up context on failure
		eb.contextStore.cleanup(msg.UUID)
	}
}

// Subscribe registers a handler for events of the specified type
func (eb *WatermillEventBus[T]) Subscribe(
	eventType string,
	handler func(ctx context.Context, event T) error,
	filter ...func(ctx context.Context, event T) bool,
) func() {
	// Increment subscriber count
	eb.subscriberMu.Lock()
	eb.subscriberCount[eventType]++
	eb.subscriberMu.Unlock()

	messages, err := eb.pubSub.Subscribe(context.Background(), eventType)
	if err != nil {
		eb.logger.Error("Failed to subscribe",
			zap.Error(err),
			zap.String("eventType", eventType))
		// Decrement on failure
		eb.subscriberMu.Lock()
		eb.subscriberCount[eventType]--
		eb.subscriberMu.Unlock()
		return func() {}
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-cancelCtx.Done():
				return
			case msg, ok := <-messages:
				if !ok {
					return
				}

				// Retrieve the original context
				ctx, hasContext := eb.contextStore.retrieve(msg.UUID)
				if !hasContext {
					ctx = context.Background()
				}

				var event T
				if err := json.Unmarshal(msg.Payload, &event); err != nil {
					eb.logger.Error("Failed to unmarshal event",
						zap.Error(err),
						zap.String("eventType", eventType))
					msg.Nack()
					continue
				}

				// Apply all filters if provided
				shouldProcess := true
				for _, f := range filter {
					if f != nil && !f(ctx, event) {
						shouldProcess = false
						break
					}
				}
				if !shouldProcess {
					msg.Ack()
					continue
				}

				// Call handler with original context
				if err := handler(ctx, event); err != nil {
					eb.logger.Error("Handler error",
						zap.Error(err),
						zap.String("eventType", eventType))
					msg.Nack()
					continue
				}
				msg.Ack()
			}
		}
	}()

	// Return unsubscribe function
	return func() {
		cancel()
		// Decrement subscriber count
		eb.subscriberMu.Lock()
		eb.subscriberCount[eventType]--
		if eb.subscriberCount[eventType] < 0 {
			eb.subscriberCount[eventType] = 0
		}
		eb.subscriberMu.Unlock()
	}
}

// Close shuts down the event bus
func (eb *WatermillEventBus[T]) Close() error {
	return eb.pubSub.Close()
}
