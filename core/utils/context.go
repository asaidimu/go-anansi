package utils

import (
	"context"
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
