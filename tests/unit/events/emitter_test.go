package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	anansievents "github.com/asaidimu/go-anansi/v8/core/events"
	"github.com/asaidimu/go-anansi/v8/core/persistence/events"
	"github.com/asaidimu/go-anansi/v8/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestEvent is a simple struct to use as a generic event for testing
type TestEvent struct {
	ID      string
	Message string
	Context map[string]any
}

// MockEventBus is a mock implementation of the EventBus interface for testing
type MockEventBus[T any] struct {
	EmitFunc      func(eventType string, event T)
	SubscribeFunc func(req anansievents.SubscriptionRequest[T]) func()
}

func (m *MockEventBus[T]) Emit(_ context.Context, eventType string, event T) {
	if m.EmitFunc != nil {
		m.EmitFunc(eventType, event)
	}
}

func (m *MockEventBus[T]) Subscribe(req anansievents.SubscriptionRequest[T]) func() {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(req)
	}
	return func() {} // No-op unsubscribe
}

// newTestAdapter builds an in-memory v2 go-events bus wrapped in the anansi
// adapter, plus a cleanup function that closes the underlying bus.
func newTestAdapter(t *testing.T) (anansievents.EventBus[TestEvent], func()) {
	t.Helper()
	bus, err := utils.NewInMemoryGoEventsBus("test")
	require.NoError(t, err)
	adapter := events.NewGoEventsBusAdapter[TestEvent](bus)
	return adapter, func() { _ = bus.Close() }
}

// waitFor polls pred until it returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestNewEventEmitter(t *testing.T) {
	mockBus := &MockEventBus[TestEvent]{}
	logger := zaptest.NewLogger(t)
	emitter := anansievents.NewEventEmitter(mockBus, nil, logger)

	assert.NotNil(t, emitter)
}

func TestEventEmitter_EmitEvent(t *testing.T) {
	emittedEvents := make(chan TestEvent, 2)
	mockBus := &MockEventBus[TestEvent]{
		EmitFunc: func(eventType string, event TestEvent) {
			emittedEvents <- event
		},
	}
	logger := zaptest.NewLogger(t)
	emitter := anansievents.NewEventEmitter(mockBus, nil, logger)

	testEvent := TestEvent{ID: "123", Message: "Hello"}
	emitter.EmitEvent(context.Background(), "test.event", testEvent)

	select {
	case emitted := <-emittedEvents:
		assert.Equal(t, testEvent, emitted)
	case <-time.After(time.Second):
		t.Fatal("EmitEvent did not emit event")
	}
}

func TestGoEventsBusAdapter_Emit(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	emittedEvent := make(chan TestEvent, 1)
	unsubscribe := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "test.event",
		Handler: func(_ context.Context, event TestEvent) error {
			emittedEvent <- event
			return nil
		},
	})
	defer unsubscribe()

	testEvent := TestEvent{ID: "456", Message: "World"}
	adapter.Emit(context.Background(), "test.event", testEvent)

	select {
	case emitted := <-emittedEvent:
		assert.Equal(t, testEvent, emitted)
	case <-time.After(2 * time.Second):
		t.Fatal("Adapter Emit did not emit event")
	}
}

func TestGoEventsBusAdapter_Subscribe(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	handler := func(_ context.Context, event TestEvent) error {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		return nil
	}

	unsubscribe := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{EventType: "test.subscribe", Handler: handler})
	defer unsubscribe()

	// Emit some events
	adapter.Emit(context.Background(), "test.subscribe", TestEvent{ID: "s1", Message: "Subscribed 1"})
	adapter.Emit(context.Background(), "other.event", TestEvent{ID: "o1", Message: "Other 1"}) // Should not be received
	adapter.Emit(context.Background(), "test.subscribe", TestEvent{ID: "s2", Message: "Subscribed 2"})

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedEvents) >= 2
	})

	mu.Lock()
	assert.Len(t, receivedEvents, 2)
	assert.Contains(t, receivedEvents, TestEvent{ID: "s1", Message: "Subscribed 1"})
	assert.Contains(t, receivedEvents, TestEvent{ID: "s2", Message: "Subscribed 2"})
	mu.Unlock()

	// Test unsubscribe
	unsubscribe()
	adapter.Emit(context.Background(), "test.subscribe", TestEvent{ID: "s3", Message: "Subscribed 3 - after unsubscribe"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Len(t, receivedEvents, 2) // Should still be 2
	mu.Unlock()
}

func TestGoEventsBusAdapter_SubscribeWithFilter(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	handler := func(_ context.Context, event TestEvent) error {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		return nil
	}

	filter := func(_ context.Context, event TestEvent) bool {
		return event.ID == "filtered"
	}

	unsubscribe := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "test.filtered",
		Handler:   handler,
		Filters:   []func(context.Context, TestEvent) bool{filter},
	})
	defer unsubscribe()

	adapter.Emit(context.Background(), "test.filtered", TestEvent{ID: "unfiltered", Message: "Should not pass"})
	adapter.Emit(context.Background(), "test.filtered", TestEvent{ID: "filtered", Message: "Should pass"})
	adapter.Emit(context.Background(), "test.filtered", TestEvent{ID: "another", Message: "Should not pass"})

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedEvents) >= 1
	})

	mu.Lock()
	assert.Len(t, receivedEvents, 1)
	assert.Contains(t, receivedEvents, TestEvent{ID: "filtered", Message: "Should pass"})
	mu.Unlock()
}

func TestGoEventsBusAdapter_LiveOnlyByDefault(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	// Emitted before subscription: must NOT be replayed by a live-only subscriber.
	adapter.Emit(context.Background(), "live", TestEvent{ID: "before"})

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	unsubscribe := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "live",
		Handler: func(_ context.Context, event TestEvent) error {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
			return nil
		},
	})
	defer unsubscribe()

	adapter.Emit(context.Background(), "live", TestEvent{ID: "after"})

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedEvents) >= 1
	})

	mu.Lock()
	assert.Len(t, receivedEvents, 1)
	assert.Equal(t, "after", receivedEvents[0].ID)
	mu.Unlock()
}

func TestGoEventsBusAdapter_Replay(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	// Emitted before subscription: must be replayed when Replay is opted into.
	adapter.Emit(context.Background(), "replay", TestEvent{ID: "old"})

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	unsubscribe := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "replay",
		Handler: func(_ context.Context, event TestEvent) error {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
			return nil
		},
		Replay: true,
	})
	defer unsubscribe()

	waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedEvents) >= 1
	})

	mu.Lock()
	assert.Len(t, receivedEvents, 1)
	assert.Equal(t, "old", receivedEvents[0].ID)
	mu.Unlock()
}

func TestGoEventsBusAdapter_ReplayFromCursor(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	adapter.Emit(context.Background(), "cursor", TestEvent{ID: "old"})
	time.Sleep(20 * time.Millisecond) // ensure distinct UUIDv7 timestamps

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	unsubscribe := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "cursor",
		Handler: func(_ context.Context, event TestEvent) error {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
			return nil
		},
		Replay: true,
		Cursor: time.Now(),
	})
	defer unsubscribe()

	adapter.Emit(context.Background(), "cursor", TestEvent{ID: "new"})

	waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedEvents) >= 1
	})

	mu.Lock()
	assert.Len(t, receivedEvents, 1)
	assert.Equal(t, "new", receivedEvents[0].ID)
	mu.Unlock()
}

func TestEventEmitter_WithEventEmission(t *testing.T) {
	mockBus := &MockEventBus[TestEvent]{}
	logger := zaptest.NewLogger(t)

	// Mock factory for TestEvent
	eventFactory := func(ctx context.Context, eventType string, operation string, input any, output any, errorMsg *string, startTime time.Time, duration *int64, extra map[string]any) TestEvent {
		contextMap := map[string]any{}
		return TestEvent{
			ID:      operation,
			Message: eventType,
			Context: contextMap,
		}
	}

	emitter := anansievents.NewEventEmitter(mockBus, eventFactory, logger)

	var emittedEvents []TestEvent
	var emittedEventTypes []string
	mockBus.EmitFunc = func(eventType string, event TestEvent) {
		emittedEventTypes = append(emittedEventTypes, eventType)
		emittedEvents = append(emittedEvents, event)
	}

	ctx := context.Background()

	config := anansievents.OperationConfig{
		Operation:         "TestOperation",
		StartEventTypes:   []string{"op.start"},
		SuccessEventTypes: []string{"op.success"},
		FailedEventTypes:  []string{"op.failed"},
		Input:             map[string]string{"data": "input"},
		Extra:             nil,
	}

	// Test successful operation
	result, err := emitter.WithEventEmission(ctx, config, func() (any, error) {
		return "operation_output", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "operation_output", result)
	assert.Len(t, emittedEvents, 8, "Expected 8 events for successful operation (op.start, *, op.success, *)")

	// Find and check start event
	var startEvent TestEvent
	for _, ev := range emittedEvents {
		if ev.Message == "op.start" {
			startEvent = ev
			break
		}
	}
	assert.Equal(t, "TestOperation", startEvent.ID)
	assert.Equal(t, "op.start", startEvent.Message)

	// Find and check success event
	var successEvent TestEvent
	for _, ev := range emittedEvents {
		if ev.Message == "op.success" {
			successEvent = ev
			break
		}
	}
	assert.Equal(t, "TestOperation", successEvent.ID)
	assert.Equal(t, "op.success", successEvent.Message)

	// Test failed operation
	emittedEvents = []TestEvent{} // Reset
	emittedEventTypes = []string{}
	expectedErr := errors.New("something went wrong")
	_, err = emitter.WithEventEmission(ctx, config, func() (any, error) {
		return nil, expectedErr
	})

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Len(t, emittedEvents, 8, "Expected 8 events for failed operation (op.start, *, op.failed, *)")

	// Find and check start event for failed operation
	for _, ev := range emittedEvents {
		if ev.Message == "op.start" {
			startEvent = ev
			break
		}
	}
	assert.Equal(t, "TestOperation", startEvent.ID)
	assert.Equal(t, "op.start", startEvent.Message)

	// Find and check failed event
	var failedEvent TestEvent
	for _, ev := range emittedEvents {
		if ev.Message == "op.failed" {
			failedEvent = ev
			break
		}
	}
	assert.Equal(t, "TestOperation", failedEvent.ID)
	assert.Equal(t, "op.failed", failedEvent.Message)
}

// ---- New tests for v2 event bus ----

func TestEventEmitter_WithV2Bus_Emit(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)

	factory := func(ctx context.Context, eventType string, operation string, input any, output any, errorMsg *string, startTime time.Time, duration *int64, extra map[string]any) TestEvent {
		return TestEvent{ID: operation, Message: eventType, Context: map[string]any{}}
	}

	emitter := anansievents.NewEventEmitter(adapter, factory, logger)

	received := make(chan TestEvent, 1)
	cancel := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "test.event",
		Handler: func(_ context.Context, event TestEvent) error {
			received <- event
			return nil
		},
	})
	defer cancel()

	testEvent := TestEvent{ID: "123", Message: "Hello"}
	emitter.EmitEvent(context.Background(), "test.event", testEvent)

	select {
	case emitted := <-received:
		assert.Equal(t, testEvent, emitted)
	case <-time.After(2 * time.Second):
		t.Fatal("EmitEvent did not emit event on v2 bus via adapter")
	}
}

func TestEventEmitter_WithV2Bus_OperationSuccess(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)

	factory := func(ctx context.Context, eventType string, operation string, input any, output any, errorMsg *string, startTime time.Time, duration *int64, extra map[string]any) TestEvent {
		return TestEvent{ID: operation, Message: eventType, Context: map[string]any{}}
	}

	emitter := anansievents.NewEventEmitter(adapter, factory, logger)

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	cancelStart := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "op.start",
		Handler: func(_ context.Context, event TestEvent) error {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
			return nil
		},
	})
	defer cancelStart()

	cancelSuccess := adapter.Subscribe(anansievents.SubscriptionRequest[TestEvent]{
		EventType: "op.success",
		Handler: func(_ context.Context, event TestEvent) error {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
			return nil
		},
	})
	defer cancelSuccess()

	config := anansievents.OperationConfig{
		Operation:         "TestOperation",
		StartEventTypes:   []string{"op.start"},
		SuccessEventTypes: []string{"op.success"},
		FailedEventTypes:  []string{"op.failed"},
		Input:             map[string]string{"data": "input"},
		Extra:             nil,
	}

	result, err := emitter.WithEventEmission(context.Background(), config, func() (any, error) {
		return "operation_output", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "operation_output", result)

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedEvents) >= 2
	})

	mu.Lock()
	var hasStart, hasSuccess bool
	for _, ev := range receivedEvents {
		if ev.Message == "op.start" {
			hasStart = true
			assert.Equal(t, "TestOperation", ev.ID)
		}
		if ev.Message == "op.success" {
			hasSuccess = true
			assert.Equal(t, "TestOperation", ev.ID)
		}
	}
	assert.True(t, hasStart)
	assert.True(t, hasSuccess)
	mu.Unlock()
}
