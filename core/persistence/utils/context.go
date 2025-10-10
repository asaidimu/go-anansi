package utils

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
)

// ContextWithCollectionName returns a new context with the collection name set.
func ContextWithCollectionName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, base.CollectionNameContextKey, name)
}

// CollectionNameFromContext retrieves the collection name from the context, if present.
func CollectionNameFromContext(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(base.CollectionNameContextKey).(string)
	return name, ok
}
