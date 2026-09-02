package common

import "slices"

import "context"

// ContextKey is a type for context keys to avoid collisions
type ContextKey string

const CollectionNameContextKey ContextKey = "anansi.collection.name"
const SanitizationScopeContextKey ContextKey = "anansi.sanitization.scope"
const RetryContextKey ContextKey = "anansi.retry.context"

// ============================================================================
// Context Helpers
// ============================================================================

// ContextWithSanitizationScope adds a sanitization scope identifier to the context.
// This determines which scoped sanitizer (if any) will be used for documents.
// If multiple scopes are added, they are all considered for sanitization and
// the most restrictive policy wins.
func ContextWithSanitizationScope(ctx context.Context, scopeID string) context.Context {
        if scopeID == "" {
                return ctx
        }

        var existingScopes []string
        if val := ctx.Value(SanitizationScopeContextKey); val != nil {
                if s, ok := val.([]string); ok {
                        existingScopes = s
                }
        }

        if found := slices.Contains(existingScopes, scopeID); !found {
                newScopes := make([]string, len(existingScopes)+1)
                copy(newScopes, existingScopes)
                newScopes[len(existingScopes)] = scopeID
                return context.WithValue(ctx, SanitizationScopeContextKey, newScopes)
        }

        return ctx
}

// ContextWithCollectionName adds a collection  identifier to the context.
// Collections are the default sanitization scopes in anansi so we add that as well
func ContextWithCollectionName(ctx context.Context, collectionName string) context.Context {
        return ContextWithSanitizationScope(
                context.WithValue(ctx, CollectionNameContextKey, collectionName),
                collectionName,
        )
}

// SanitizationScopesFromContext retrieves scopes from the context, if present.
func SanitizationScopesFromContext(ctx context.Context) ([]string) {

        if val := ctx.Value(SanitizationScopeContextKey); val != nil {
                if s, ok := val.([]string); ok {
                        return s
                }
        }

        return make([]string, 0)
}

// CollectionNameFromContext retrieves the collection name from the context, if present.
func CollectionNameFromContext(ctx context.Context) (string, bool) {
        name, ok := ctx.Value(CollectionNameContextKey).(string)
        return name, ok
}

// ============================================================================
// Retry Context — decorator side-effect suppression
// ============================================================================

// RetryContext carries retry metadata that the pipeline injects into the context
// on each retry attempt. Decorators (audit, metrics, caching) inspect this to
// suppress non-idempotent side effects during retries.
//
// Example usage in a decorator:
//
//      rc := common.RetryContextFromContext(ctx)
//      if rc != nil && rc.Attempt > 0 {
//              // Skip audit log / metric emission on retry — the first attempt
//              // already recorded it, and replaying would create duplicates.
//              return next.Read(ctx, query)
//      }
type RetryContext struct {
        // Attempt is the 0-based attempt index. 0 = first attempt, 1+ = retries.
        Attempt int
}

// WithRetryContext stores a RetryContext in the context.
// The Retry value is embedded by value, so callers cannot mutate the stored copy.
func WithRetryContext(ctx context.Context, rc RetryContext) context.Context {
        return context.WithValue(ctx, RetryContextKey, rc)
}

// RetryContextFromContext retrieves the RetryContext from the context, if present.
// Returns nil when not in a retry loop (i.e. on the first attempt or when
// retry is disabled).
func RetryContextFromContext(ctx context.Context) *RetryContext {
        if val := ctx.Value(RetryContextKey); val != nil {
                rc, ok := val.(RetryContext)
                if ok {
                        return &rc
                }
        }
        return nil
}

// ============================================================================
// ExecuteWithContext — context-cancellable function execution
// ============================================================================

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
