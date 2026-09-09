package utils

import (
	"context"
	"sync"
)

// DeferredActionQueue queues functions to be executed later.
// This is useful for any scenario where actions need to be deferred
// until some condition is met (e.g., transaction commit, batch completion, etc.)
type DeferredActionQueue struct {
	actions []func()
	mu      sync.Mutex
}

func NewDeferredActionQueue() *DeferredActionQueue {
	return &DeferredActionQueue{
		actions: make([]func(), 0),
	}
}

func (q *DeferredActionQueue) Enqueue(action func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.actions = append(q.actions, action)
}

func (q *DeferredActionQueue) ExecuteAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, action := range q.actions {
		action()
	}
	q.actions = nil
}

func (q *DeferredActionQueue) DiscardAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.actions = nil
}

// Context helpers - generic, no mention of transactions or events
type deferredActionsKey struct{}

func AttachToContext(ctx context.Context, queue *DeferredActionQueue) context.Context {
	return context.WithValue(ctx, deferredActionsKey{}, queue)
}

func FromContext(ctx context.Context) (*DeferredActionQueue, bool) {
	queue, ok := ctx.Value(deferredActionsKey{}).(*DeferredActionQueue)
	return queue, ok
}
