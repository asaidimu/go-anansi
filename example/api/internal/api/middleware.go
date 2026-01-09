package api

import (
	"net/http"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// CollectionContextMiddleware is a middleware that injects the collection name from the URL
// into the request's context. This makes the collection name available to downstream
// handlers and services for purposes like logging, authorization, or per-collection logic.
func CollectionContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectionName := r.PathValue("collection")

		if collectionName != "" {
			ctx := common.ContextWithCollectionName(r.Context(), collectionName)
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			next.ServeHTTP(w, r)
		}
	})
}
