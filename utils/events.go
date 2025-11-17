package utils

import (
	"context"
	"encoding/json"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"go.uber.org/zap"
)

// WatermillEventBus implements EventBus using Watermill's in-memory pub/sub
type WatermillEventBus[T any] struct {
	pubSub *gochannel.GoChannel
	logger *zap.Logger
}

// NewWatermillEventBus creates a new event bus instance
func NewWatermillEventBus[T any](logger *zap.Logger) *WatermillEventBus[T] {
	return &WatermillEventBus[T]{
		pubSub: gochannel.NewGoChannel(gochannel.Config{}, nil),
		logger: logger,
	}
}

// Emit publishes an event to the specified event type topic
func (eb *WatermillEventBus[T]) Emit(eventType string, event T) {
	payload, err := json.Marshal(event)
	if err != nil {
		eb.logger.Error("Failed to marshal event",
			zap.Error(err),
			zap.String("eventType", eventType))
		return
	}

	msg := message.NewMessage(watermill.NewUUID(), payload)

	if err := eb.pubSub.Publish(eventType, msg); err != nil {
		eb.logger.Error("Failed to publish event",
			zap.Error(err),
			zap.String("eventType", eventType))
	}
}

// Subscribe registers a handler for events of the specified type
func (eb *WatermillEventBus[T]) Subscribe(
	eventType string,
	handler func(ctx context.Context, event T) error,
	filter ...func(ctx context.Context, event T) bool,
) func() {
	messages, err := eb.pubSub.Subscribe(context.Background(), eventType)
	if err != nil {
		eb.logger.Error("Failed to subscribe",
			zap.Error(err),
			zap.String("eventType", eventType))
		return func() {}
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-messages:
				if !ok {
					return
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

				// Call handler
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
	return cancel
}

// Close shuts down the event bus
func (eb *WatermillEventBus[T]) Close() error {
	return eb.pubSub.Close()
}
