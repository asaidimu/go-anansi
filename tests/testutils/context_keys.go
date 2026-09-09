package testutils

type ContextKey string

const (
	TraceIDKey     ContextKey = "traceID"
	ReadTraceIDKey ContextKey = "readTraceID"
)
