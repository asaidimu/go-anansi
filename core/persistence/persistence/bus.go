package persistence

import (
	"context"
	"log/slog"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-events"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

func createEventBus(z *zap.Logger) (*events.TypedEventBus[base.PersistenceEvent], error) {
	handler := zapslog.NewHandler(z.Core())
	logger := slog.New(handler)
	cfg := events.DefaultConfig()
	cfg.Async = true
	cfg.BatchSize = 10                        // Process events in batches of 10
	cfg.BatchDelay = 50 * time.Millisecond    // Wait 50ms to collect batches
	cfg.MaxQueueSize = 100                    // Allow up to 100 events in the queue
	cfg.BlockOnFullQueue = false              // Drop events if queue is full
	cfg.AsyncWorkerPoolSize = 3               // Use 3 worker goroutines
	cfg.ShutdownTimeout = 2 * time.Second     // Allow 2s for graceful shutdown
	cfg.EventTimeout = 500 * time.Millisecond // Timeout handlers after 500ms
	cfg.Logger = nil
	cfg.ErrorHandler = func(e *events.EventError) {
		logger.Error("Event processing error",
			"event", e.EventName,
			"error", e.Err,
			"timestamp", e.Timestamp)
	}
	cfg.DeadLetterHandler = func(ctx context.Context, event events.Event, finalErr error) {
		logger.Warn("Event sent to DLQ",
			"event", event.Name,
			"error", finalErr)
	}

	return events.NewTypedEventBus[base.PersistenceEvent](cfg)
}
