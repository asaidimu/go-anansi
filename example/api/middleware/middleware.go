package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/asaidimu/go-anansi/v6/example/api/types"
	"go.uber.org/zap"
)

// Chain represents a middleware chain
type Chain struct {
	middlewares []func(http.Handler) http.Handler
}

func New(middlewares ...func(http.Handler) http.Handler) *Chain {
	return &Chain{middlewares}
}

func (c *Chain) Then(h http.Handler) http.Handler {
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

// Common middleware factories
func WithTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func WithRecovery(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("Panic recovered", zap.Any("error", err))
					// Create a generic error response
					_ = types.NewServerError(types.ErrCodeInternalError, "Internal server error", fmt.Errorf("%v", err))
					// You would need a way to write this error response.
					// This might require passing a BaseHandler or a writer function to the middleware.
					// For now, we'll just log it.
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func WithRequestLogging(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("Request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

func WithValidation() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Example: Limit request body size
			r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB
			next.ServeHTTP(w, r)
		})
	}
}
