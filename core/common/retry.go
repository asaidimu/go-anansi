package common

import (
        "context"
        "errors"
        "math/rand/v2"
        "time"
)

// RetryableFunc is any function that returns a result and an error.
// The retry executor inspects the error for retryability via
// SystemError.IsRetryable().
type RetryableFunc[T any] func() (T, error)

// RetryConfig holds retry parameters for a single retry invocation.
// Usually sourced from Capabilities.Retry, but can be overridden.
type RetryConfig[T any] struct {
        Policy  RetryPolicy
        OnRetry func(attempt int, err error) // Optional callback for logging/metrics.
}

// Retry executes f with exponential backoff according to cfg.Policy.
// It stops on success, non-retryable error, context cancellation,
// or exhausting MaxAttempts / MaxTotalDuration.
//
// Retry is generic over T so it can wrap any operation signature
// (single return, tuple struct, etc.) without type assertions.
//
// The jitter mode randomizes each delay to [0, calculated_delay) to prevent
// thundering herd when multiple goroutines retry simultaneously.
func Retry[T any](ctx context.Context, cfg RetryConfig[T], f RetryableFunc[T]) (T, error) {
        var zero T
        policy := cfg.Policy
        if policy.MaxAttempts < 1 {
                policy.MaxAttempts = 1
        }

        // MaxTotalDuration is an absolute ceiling: if the cumulative time spent
        // retrying exceeds this budget, the operation fails immediately.
        // This prevents a single operation from retrying indefinitely under
        // sustained overload (the "retry storm" problem).
        var deadline time.Time
        if policy.MaxTotalDuration > 0 {
                deadline = time.Now().Add(policy.MaxTotalDuration)
        }

        var lastErr error
        for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
                // Check circuit-breaker deadline before each attempt.
                if !deadline.IsZero() && time.Now().After(deadline) {
                        return zero, lastErr
                }

                if attempt > 0 {
                        delay := CalculateBackoff(policy, attempt)

                        // Enforce circuit-breaker: don't sleep past the deadline.
                        if !deadline.IsZero() {
                                remaining := time.Until(deadline)
                                if remaining <= 0 {
                                        return zero, lastErr
                                }
                                if delay > remaining {
                                        delay = remaining
                                }
                        }

                        if policy.Jitter {
                                delay = time.Duration(JitterDelay(delay))
                                if delay == 0 {
                                        delay = time.Nanosecond // avoid zero-sleep busy loop
                                }
                        }

                        timer := time.NewTimer(delay)
                        select {
                        case <-ctx.Done():
                                timer.Stop()
                                return zero, ctx.Err()
                        case <-timer.C:
                        }

                        if cfg.OnRetry != nil {
                                cfg.OnRetry(attempt, lastErr)
                        }
                }

                result, err := f()
                if err == nil {
                        return result, nil
                }

                lastErr = err
                if !isRetryableError(err) {
                        return zero, err
                }
        }

        return zero, lastErr
}

// IsRetryableError checks if any SystemError in the error chain is marked
// retryable. Non-SystemError values are never retried.
// This is exported so callers can check retryability without using Retry.
func IsRetryableError(err error) bool {
        return isRetryableError(err)
}

// isRetryableError is the internal implementation.
func isRetryableError(err error) bool {
        var sysErr *SystemError
        if errors.As(err, &sysErr) {
                return sysErr.IsRetryable()
        }
        return false
}

// CalculateBackoff computes the exponential backoff delay for the given
// retry attempt (1-indexed: attempt=1 is the first retry, not the first try).
// Formula: BaseDelay * 2^(attempt-1), capped at MaxDelay.
//
// Exported so that callers who need context-injection (e.g. transaction retry
// with RetryContext) can build their own loop instead of using Retry[T].
func CalculateBackoff(p RetryPolicy, attempt int) time.Duration {
        if p.BaseDelay <= 0 {
                return 0
        }
        d := p.BaseDelay << (attempt - 1)
        if d <= 0 || d > p.MaxDelay {
                d = p.MaxDelay
        }
        return d
}

// JitterDelay randomizes a delay to [0, delay) to prevent thundering herd.
// Returns the jittered duration in nanoseconds.
// A minimum of 1ns is returned to avoid zero-sleep busy loops.
func JitterDelay(delay time.Duration) int64 {
        j := rand.Int64N(int64(delay))
        if j == 0 {
                return 1
        }
        return j
}
