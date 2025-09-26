package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	anansievents "github.com/asaidimu/go-anansi/v6/core/events" // Alias to avoid conflict
	"github.com/asaidimu/go-anansi/v6/core/persistence/events"
	goevents "github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
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
	SubscribeFunc func(eventType string, handler func(ctx context.Context, event T) error, filter ...func(ctx context.Context, event T) bool) func()
}

func (m *MockEventBus[T]) Emit(eventType string, event T) {
	if m.EmitFunc != nil {
		m.EmitFunc(eventType, event)
	}
}

func (m *MockEventBus[T]) Subscribe(eventType string, handler func(ctx context.Context, event T) error, filter ...func(ctx context.Context, event T) bool) func() {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(eventType, handler, filter...)
	}
	return func() {} // No-op unsubscribe
}

func TestNewEventEmitter(t *testing.T) {
	mockBus := &MockEventBus[TestEvent]{}
	logger := zaptest.NewLogger(t)
	emitter := anansievents.NewEventEmitter[TestEvent](mockBus, logger)

	assert.NotNil(t, emitter)
}

func TestEventEmitter_EmitEvent(t *testing.T) {
	emittedEvents := make(chan TestEvent, 1)
	mockBus := &MockEventBus[TestEvent]{
		EmitFunc: func(eventType string, event TestEvent) {
			emittedEvents <- event
		},
	}
	logger := zaptest.NewLogger(t)
	emitter := anansievents.NewEventEmitter[TestEvent](mockBus, logger)

	testEvent := TestEvent{ID: "123", Message: "Hello"}
	emitter.EmitEvent("test.event", testEvent)

	select {
	case emitted := <-emittedEvents:
		assert.Equal(t, testEvent, emitted)
	case <-time.After(time.Second):
		t.Fatal("EmitEvent did not emit event")
	}
}

func TestGoEventsBusAdapter_Emit(t *testing.T) {
	typedBus, err := goevents.NewTypedEventBus[TestEvent](goevents.DefaultConfig())
	assert.NoError(t, err)
	adapter := events.NewGoEventsBusAdapter[TestEvent](typedBus)

	emittedEvent := make(chan TestEvent, 1)
	typedBus.Subscribe("test.event", func(ctx context.Context, event TestEvent) error {
		emittedEvent <- event
		return nil
	})

	testEvent := TestEvent{ID: "456", Message: "World"}
	adapter.Emit("test.event", testEvent)

	select {
	case emitted := <-emittedEvent:
		assert.Equal(t, testEvent, emitted)
	case <-time.After(time.Second):
		t.Fatal("Adapter Emit did not emit event")
	}
}

func TestGoEventsBusAdapter_Subscribe(t *testing.T) {
	typedBus, err := goevents.NewTypedEventBus[TestEvent](goevents.DefaultConfig())
	assert.NoError(t, err)
	adapter := events.NewGoEventsBusAdapter[TestEvent](typedBus)

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	handler := func(ctx context.Context, event TestEvent) error {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		return nil
	}

	unsubscribe := adapter.Subscribe("test.subscribe", handler)
	defer unsubscribe()

	// Emit some events
	typedBus.Emit("test.subscribe", TestEvent{ID: "s1", Message: "Subscribed 1"})
	typedBus.Emit("other.event", TestEvent{ID: "o1", Message: "Other 1"}) // Should not be received
	typedBus.Emit("test.subscribe", TestEvent{ID: "s2", Message: "Subscribed 2"})

	// Give some time for events to propagate
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.Len(t, receivedEvents, 2)
	assert.Contains(t, receivedEvents, TestEvent{ID: "s1", Message: "Subscribed 1"})
	assert.Contains(t, receivedEvents, TestEvent{ID: "s2", Message: "Subscribed 2"})
	mu.Unlock()

	// Test unsubscribe
	unsubscribe()
	typedBus.Emit("test.subscribe", TestEvent{ID: "s3", Message: "Subscribed 3 - after unsubscribe"})
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.Len(t, receivedEvents, 2) // Should still be 2
	mu.Unlock()
}

func TestGoEventsBusAdapter_SubscribeWithFilter(t *testing.T) {
	typedBus, err := goevents.NewTypedEventBus[TestEvent](goevents.DefaultConfig())
	assert.NoError(t, err)
	adapter := events.NewGoEventsBusAdapter[TestEvent](typedBus)

	var mu sync.Mutex
	receivedEvents := []TestEvent{}

	handler := func(ctx context.Context, event TestEvent) error {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		return nil
	}

	filter := func(ctx context.Context, event TestEvent) bool {
		// Simulate a contextual check, though context.Background() is passed by adapter
		return event.ID == "filtered"
	}

	unsubscribe := adapter.Subscribe("test.filtered", handler, filter)
	defer unsubscribe()

	typedBus.Emit("test.filtered", TestEvent{ID: "unfiltered", Message: "Should not pass"})
	typedBus.Emit("test.filtered", TestEvent{ID: "filtered", Message: "Should pass"})
	typedBus.Emit("test.filtered", TestEvent{ID: "another", Message: "Should not pass"})

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.Len(t, receivedEvents, 1)
	assert.Contains(t, receivedEvents, TestEvent{ID: "filtered", Message: "Should pass"})
	mu.Unlock()
}

func TestEventEmitter_WithEventEmission(t *testing.T) {
	mockBus := &MockEventBus[TestEvent]{}
	logger := zaptest.NewLogger(t)
	emitter := anansievents.NewEventEmitter[TestEvent](mockBus, logger)

	var emittedEvents []TestEvent
	var emittedEventTypes []string
	mockBus.EmitFunc = func(eventType string, event TestEvent) {
		emittedEventTypes = append(emittedEventTypes, eventType)
		emittedEvents = append(emittedEvents, event)
	}

	// Mock factory for TestEvent
	eventFactory := func(eventType string, operation string, input any, output any, query any, errorMsg *string, transactionID *string, startTime time.Time, duration *int64, contextMap map[string]any) TestEvent {
		return TestEvent{
			ID:      operation,
			Message: eventType,
			Context: contextMap,
		}
	}

	// Mock transaction ID extractor
	extractTransactionID := func(ctx context.Context) *string {
		txID := "mock-tx-123"
		return &txID
	}

	ctx := anansievents.WithEventContextValue(context.Background(), "userID", "user-abc")
	ctx = anansievents.WithEventContextValue(ctx, "requestID", "req-xyz")

	config := anansievents.OperationConfig{
		Operation:        "TestOperation",
		StartEventType:   "op.start",
		SuccessEventType: "op.success",
		FailedEventType:  "op.failed",
		Input:            map[string]string{"data": "input"},
	}

	// Test successful operation
	result, err := emitter.WithEventEmission(ctx, config, func() (any, error) {
		return "operation_output", nil
	}, eventFactory, extractTransactionID)

	assert.NoError(t, err)
	assert.Equal(t, "operation_output", result)
	assert.Len(t, emittedEvents, 2)
	assert.Contains(t, emittedEventTypes, "op.start")
	assert.Contains(t, emittedEventTypes, "op.success")

	// Check start event
	startEvent := emittedEvents[0]
	assert.Equal(t, "TestOperation", startEvent.ID)
	assert.Equal(t, "op.start", startEvent.Message)
	assert.Contains(t, startEvent.Context, "userID")
	assert.Contains(t, startEvent.Context, "requestID")

	// Check success event
	successEvent := emittedEvents[1]
	assert.Equal(t, "TestOperation", successEvent.ID)
	assert.Equal(t, "op.success", successEvent.Message)
	assert.Contains(t, successEvent.Context, "userID")
	assert.Contains(t, successEvent.Context, "requestID")

	// Test failed operation
	emittedEvents = []TestEvent{} // Reset
	emittedEventTypes = []string{}
	expectedErr := errors.New("something went wrong")
	result, err = emitter.WithEventEmission(ctx, config, func() (any, error) {
		return nil, expectedErr
	}, eventFactory, extractTransactionID)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Len(t, emittedEvents, 2)
	assert.Contains(t, emittedEventTypes, "op.start")
	assert.Contains(t, emittedEventTypes, "op.failed")

	// Check failed event
	failedEvent := emittedEvents[1]
	assert.Equal(t, "TestOperation", failedEvent.ID)
	assert.Equal(t, "op.failed", failedEvent.Message)
	assert.Contains(t, failedEvent.Context, "userID")
	assert.Contains(t, failedEvent.Context, "requestID")
}

