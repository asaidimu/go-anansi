package response

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// APIResponse represents the standard success response structure.
type APIResponse struct {
	Data any `json:"data,omitempty"`
	Meta Meta `json:"meta"`
}

// ErrorResponse represents the standard error response structure.
type ErrorResponse struct {
	Error APIError `json:"error"`
	Meta  Meta     `json:"meta"`
}

// APIError represents the detailed error information.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Meta contains metadata about the response.
type Meta struct {
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request"`
}

// Handler provides methods for writing standardized API responses.
type Handler struct{}

// NewHandler creates a new ResponseHandler.
func NewHandler() *Handler {
	return &Handler{}
}

// WriteJSON writes a JSON success response to the http.ResponseWriter.
func (h *Handler) WriteJSON(w http.ResponseWriter, status int, data any, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(APIResponse{Data: data, Meta: h.generateMeta(r)}); err != nil {
		log.Printf("Error writing JSON response: %v", err)
	}
}

// WriteError writes a JSON error response to the http.ResponseWriter.
func (h *Handler) WriteError(w http.ResponseWriter, status int, apiError APIError, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: apiError, Meta: h.generateMeta(r)}); err != nil {
		log.Printf("Error writing JSON error response: %v", err)
	}
}

// generateMeta creates a Meta object for responses.
func (h *Handler) generateMeta(_ *http.Request) Meta {
	id := uuid.Must(uuid.NewV7())
	// In a real application, RequestID would be generated uniquely per request
	// and timestamp would be more precise.
	return Meta{
		Timestamp: time.Now().Format(time.RFC3339),
		RequestID: id.String(),
	}
}
