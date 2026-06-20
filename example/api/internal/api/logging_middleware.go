package api

import (
	"bytes"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// responseWriter is a wrapper for http.ResponseWriter that captures the status code and body.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status code
		body:           new(bytes.Buffer),
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b) // Capture body
	return rw.ResponseWriter.Write(b)
}

// LoggingMiddleware logs every HTTP request and its corresponding response.
func LoggingMiddleware(logger *zap.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Log Request
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "N/A"
		}
		logger.Info("Incoming Request",
			zap.String("request_id", requestID),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
			zap.Any("headers", r.Header),
		)

		// Create a wrapped ResponseWriter to capture the response details
		wrappedWriter := newResponseWriter(w)

		// Serve the request
		next.ServeHTTP(wrappedWriter, r)

		// Log Response
		duration := time.Since(start)
		logger.Info("Outgoing Response",
			zap.String("request_id", requestID),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", wrappedWriter.statusCode),
			zap.String("duration", duration.String()),
			zap.Any("response_headers", w.Header()), // Use original writer's headers to get final headers
			zap.String("response_body", wrappedWriter.body.String()), // Log captured body
		)
	})
}
