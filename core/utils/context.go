package utils

import (
	"maps"
	"context"
	"sync"
)

// ExecuteWithContext executes a function and waits for it to complete or for the context to be canceled.
func ExecuteWithContext[T any](ctx context.Context, f func() (T, error)) (T, error) {
	done := make(chan struct{})
	var result T
	var err error

	go func() {
		defer close(done)
		result, err = f()
	}()

	select {
	case <-done:
		return result, err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// ContextData manages structured data within Go contexts with optional mode-based gating.
// It maintains an internal store of key-value pairs that are merged with context-specific
// data using Last-Write-Wins semantics, where context values take precedence over store values.
//
// ContextData is safe for concurrent use across multiple goroutines.
type ContextData struct {
	key    any
	values map[string]any
	mu     sync.RWMutex
	mode   any
}

// NewContextData creates a new ContextData instance with the specified key and optional mode gating.
//
// The key parameter should be unique to avoid conflicts with other ContextData instances.
// If mode is non-nil, operations that modify context will only succeed if the context
// contains the specified mode key.
//
// Initial data maps are merged in order, with later maps taking precedence for duplicate keys.
func NewContextData(key string, mode any, initialData ...map[string]any) *ContextData {
	values := make(map[string]any)
	for _, data := range initialData {
		maps.Copy(values, data)
	}

	return &ContextData{
		key:    struct{ key string }{key},
		values: values,
		mode:   mode,
	}
}

// SetValue adds or updates a value in the internal store.
// This operation is atomic and safe for concurrent use.
func (cd *ContextData) SetValue(key string, value any) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.values[key] = value
}

// SetValues atomically sets multiple key-value pairs in the internal store.
// Existing values with the same keys will be overwritten.
func (cd *ContextData) SetValues(data map[string]any) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	maps.Copy(cd.values, data)
}

// ClearValue removes a value from the internal store.
// This operation is atomic and safe for concurrent use.
func (cd *ContextData) ClearValue(key string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.values, key)
}

// SetMode updates the mode key used for gating context operations.
// Set to nil to disable mode checking.
func (cd *ContextData) SetMode(mode any) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.mode = mode
}

// HasMode reports whether this ContextData instance is configured for the specified mode.
func (cd *ContextData) HasMode(mode any) bool {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return cd.mode == mode
}

// withModeCheck executes the provided action only if mode validation passes.
// If mode is nil, the action is always executed.
// If mode is non-nil but the context does not contain the mode key, the original context is returned.
func (cd *ContextData) withModeCheck(ctx context.Context, action func() context.Context) context.Context {
	cd.mu.RLock()
	mode := cd.mode
	cd.mu.RUnlock()

	if mode != nil && ctx.Value(mode) == nil {
		return ctx
	}

	return action()
}

// WithValue returns a new context with the specified key-value pair added to the data.
// The returned context contains merged data from the internal store and existing context data,
// with context values taking precedence (Last-Write-Wins).
//
// If mode gating is enabled and the context does not contain the required mode key,
// the original context is returned unchanged.
func (cd *ContextData) WithValue(ctx context.Context, key string, value any) context.Context {
	return cd.withModeCheck(ctx, func() context.Context {
		existing := cd.getContextData(ctx)
		merged := make(map[string]any)

		cd.mu.RLock()
		maps.Copy(merged, cd.values)
		cd.mu.RUnlock()

		maps.Copy(merged, existing)

		merged[key] = value

		return context.WithValue(ctx, cd.key, merged)
	})
}

// WithValues returns a new context with multiple key-value pairs added to the data.
// The returned context contains merged data from the internal store, existing context data,
// and the provided data map, with later sources taking precedence (Last-Write-Wins).
//
// If mode gating is enabled and the context does not contain the required mode key,
// the original context is returned unchanged.
func (cd *ContextData) WithValues(ctx context.Context, data map[string]any) context.Context {
	return cd.withModeCheck(ctx, func() context.Context {
		existing := cd.getContextData(ctx)
		merged := make(map[string]any)

		cd.mu.RLock()
		maps.Copy(merged, cd.values)
		cd.mu.RUnlock()

		maps.Copy(merged, existing)

		maps.Copy(merged, data)

		return context.WithValue(ctx, cd.key, merged)
	})
}

// Data retrieves the complete merged data from the context.
// The returned map contains data from both the internal store and the context,
// with context values taking precedence over store values.
//
// The returned map is a copy and safe to modify without affecting the original data.
func (cd *ContextData) Data(ctx context.Context) map[string]any {
	existing := cd.getContextData(ctx)
	result := make(map[string]any)

	cd.mu.RLock()
	maps.Copy(result, cd.values)
	cd.mu.RUnlock()

	maps.Copy(result, existing)

	return result
}

// HasValue reports whether the specified key exists in the merged data.
func (cd *ContextData) HasValue(ctx context.Context, key string) bool {
	data := cd.Data(ctx)
	_, exists := data[key]
	return exists
}

// FromContext creates a new ContextData instance from existing context data.
// This is useful for reconstructing a ContextData manager from a context that
// already contains structured data.
//
// The returned ContextData will have no mode gating and will use the existing
// context data as its initial store values.
func FromContext(ctx context.Context, key string) *ContextData {
	contextKey := struct{ key string }{key}

	var initialValues map[string]any
	if data, ok := ctx.Value(contextKey).(map[string]any); ok {
		initialValues = make(map[string]any)
		maps.Copy(initialValues, data)
	}

	return &ContextData{
		key:    contextKey,
		values: initialValues,
	}
}

// getContextData retrieves the raw data map stored in the context for this ContextData instance.
// Returns an empty map if no data is found.
func (cd *ContextData) getContextData(ctx context.Context) map[string]any {
	if data, ok := ctx.Value(cd.key).(map[string]any); ok {
		return data
	}
	return make(map[string]any)
}
