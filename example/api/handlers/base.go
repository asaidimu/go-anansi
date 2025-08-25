package handlers

import (
    "context"
    "encoding/json"
	"io"
    "net/http"
    "time"

    "github.com/asaidimu/go-anansi/v6/example/api/types"
    "github.com/asaidimu/go-anansi/v6/core/persistence/base"
    "go.uber.org/zap"
)

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
    persistence base.Persistence
    logger      *zap.Logger
    timeout     time.Duration
}

func NewBaseHandler(persistence base.Persistence, logger *zap.Logger) *BaseHandler {
    return &BaseHandler{
        persistence: persistence,
        logger:      logger,
        timeout:     30 * time.Second,
    }
}

// WithTimeout adds timeout to context
func (h *BaseHandler) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
    return context.WithTimeout(ctx, h.timeout)
}

// ParseJSON safely parses JSON with size limits
func (h *BaseHandler) ParseJSON(w http.ResponseWriter, r *http.Request, v interface{}, maxSize int64) *types.APIError {
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		if err == io.EOF {
			return types.NewClientError(types.ErrCodeInvalidJSON, "Empty request body")
		}
		return types.NewClientError(types.ErrCodeInvalidJSON, "Invalid JSON format", err.Error())
	}

	return nil
}

// WriteResponse writes standardized responses
func (h *BaseHandler) WriteResponse(w http.ResponseWriter, data interface{}) {
	response := types.APIResponse{
		Success: true,
		Data:    data,
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

// WriteError writes standardized error responses
func (h *BaseHandler) WriteError(w http.ResponseWriter, err *types.APIError) {
	response := types.APIResponse{
		Success: false,
		Error:   err,
	}
	h.logger.Error(err.Message, zap.Error(err), zap.String("details", err.Details))
	h.writeJSONResponse(w, err.HTTPStatus(), response)
}

// writeJSONResponse writes a JSON response
func (h *BaseHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

// ValidateMethod ensures correct HTTP method
func (h *BaseHandler) ValidateMethod(r *http.Request, allowed ...types.HTTPMethod) *types.APIError {
	for _, method := range allowed {
		if r.Method == string(method) {
			return nil
		}
	}
	return types.NewClientError(types.ErrCodeMethodNotAllowed, "Method not allowed")
}
