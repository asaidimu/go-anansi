package api

import (
	"net/http"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

// collectionContextMiddleware is a middleware that injects the collection name from the URL
// into the request's context. This makes the collection name available to downstream
// handlers and services for purposes like logging, authorization, or per-collection logic.
func collectionContextMiddleware(next http.Handler) http.Handler {
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

// corsMiddleware adds CORS headers to responses and handles preflight OPTIONS requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5678")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
