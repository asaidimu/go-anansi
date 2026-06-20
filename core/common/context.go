package common

import "slices"

import "context"

// ContextKey is a type for context keys to avoid collisions
type ContextKey string

const CollectionNameContextKey ContextKey = "anansi.collection.name"
const SanitizationScopeContextKey ContextKey = "anansi.sanitization.scope"

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
