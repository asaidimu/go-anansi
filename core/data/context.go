package data

import (
	"context"
	"sync"
	"time"
)

// =============================================================================
// 1. CONTEXT-AWARE DOCUMENT
// =============================================================================

// ContextualDocument wraps a Document with context awareness
type ContextualDocument struct {
	Document
	ctx context.Context
}

// NewContextualDocument creates a new ContextualDocument from a map[string]any.
func NewContextualDocument(ctx context.Context, data map[string]any) (ContextualDocument, error) {
	if data == nil {
		data = make(map[string]any)
	}

	d, err := getFactory().newDocument(ctx, data)
	if err != nil {
		return ContextualDocument{}, err
	}

	return ContextualDocument{
		Document: d,
		ctx:      ctx,
	}, nil
}

// MustNewContextualDocument creates a new ContextualDocument from various map forms, panics on failure.
func MustNewContextualDocument(ctx context.Context, data any) ContextualDocument {
	doc, err := convertToDocumentMap(data)
	if err != nil {
		panic(err)
	}

	d, err := getFactory().newDocument(ctx, doc)
	if err != nil {
		panic(err)
	}

	return ContextualDocument{
		Document: d,
		ctx:      ctx,
	}
}

// WithContext creates a new ContextualDocument with the given context
func (d Document) WithContext(ctx context.Context) *ContextualDocument {
	return &ContextualDocument{
		Document: d,
		ctx:      ctx,
	}
}

// Context returns the attached context
func (cd *ContextualDocument) Context() context.Context {
	return cd.ctx
}

// ContextBuilder helps build context with common values and manages cancel functions
type ContextBuilder struct {
	ctx         context.Context
	cancelFuncs []context.CancelFunc
	mu          sync.Mutex // Protects cancelFuncs slice
	buildOnce   sync.Once  // Ensures build operations happen only once
}

// NewContextBuilder creates a new context builder
func NewContextBuilder(parent context.Context) *ContextBuilder {
	if parent == nil {
		parent = context.Background()
	}
	return &ContextBuilder{
		ctx:         parent,
		cancelFuncs: make([]context.CancelFunc, 0),
	}
}

// WithTimeout adds a timeout to the context
func (cb *ContextBuilder) WithTimeout(timeout time.Duration) *ContextBuilder {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ctx, cancel := context.WithTimeout(cb.ctx, timeout)
	cb.ctx = ctx
	cb.cancelFuncs = append(cb.cancelFuncs, cancel)
	return cb
}

// WithDeadline adds a deadline to the context
func (cb *ContextBuilder) WithDeadline(deadline time.Time) *ContextBuilder {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ctx, cancel := context.WithDeadline(cb.ctx, deadline)
	cb.ctx = ctx
	cb.cancelFuncs = append(cb.cancelFuncs, cancel)
	return cb
}

// WithCancel adds cancellation capability to the context
func (cb *ContextBuilder) WithCancel() *ContextBuilder {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ctx, cancel := context.WithCancel(cb.ctx)
	cb.ctx = ctx
	cb.cancelFuncs = append(cb.cancelFuncs, cancel)
	return cb
}

// WithValue adds a key-value pair to the context
func (cb *ContextBuilder) WithValue(key, value any) *ContextBuilder {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.ctx = context.WithValue(cb.ctx, key, value)
	return cb
}

// Build returns the built context and prevents further modifications
func (cb *ContextBuilder) Build() context.Context {
	var result context.Context
	cb.buildOnce.Do(func() {
		cb.mu.Lock()
		result = cb.ctx
		cb.mu.Unlock()
	})
	return result
}

// BuildWithCleanup returns the context and a cleanup function that cancels all associated contexts
func (cb *ContextBuilder) BuildWithCleanup() (context.Context, func()) {
	var result context.Context
	var cleanup func()

	cb.buildOnce.Do(func() {
		cb.mu.Lock()
		result = cb.ctx

		// Create a cleanup function that calls all cancel functions
		cleanup = func() {
			cb.mu.Lock()
			defer cb.mu.Unlock()

			for _, cancel := range cb.cancelFuncs {
				if cancel != nil {
					cancel()
				}
			}
			// Clear the slice to prevent double-cancellation
			cb.cancelFuncs = cb.cancelFuncs[:0]
		}
		cb.mu.Unlock()
	})

	return result, cleanup
}

// Cancel immediately cancels all contexts created by this builder
func (cb *ContextBuilder) Cancel() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	for _, cancel := range cb.cancelFuncs {
		if cancel != nil {
			cancel()
		}
	}
	// Clear the slice
	cb.cancelFuncs = cb.cancelFuncs[:0]
}

// CancelCount returns the number of cancel functions being managed
func (cb *ContextBuilder) CancelCount() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return len(cb.cancelFuncs)
}
