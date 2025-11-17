package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/google/uuid"
)

// APIResponse represents the standard success response structure.
type APIResponse struct {
	Data any  `json:"data,omitempty"`
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
	Details any `json:"details,omitempty"`
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
func (h *Handler) WriteError(w http.ResponseWriter, status int, apiError any, r *http.Request) {
	finalError, _ := h.translateError(apiError)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: finalError, Meta: h.generateMeta(r)}); err != nil {
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

// TranslateError converts any Go error into a standard API response and status code.
func (h *Handler) translateError(er any) (apiErr APIError, httpStatus int) {
	if e, ok := er.(APIError); ok {
		// If it's already an APIError, ensure its Details field conforms to the new type
		if _, isSlice := e.Details.([]common.Issue); !isSlice {
			// If Details is not already a []common.Issue, try to convert it
			if strDetails, isString := e.Details.(string); isString {
				e.Details = []common.Issue{{Message: strDetails}}
			} else if e.Details != nil {
				// If it's something else, wrap it in a generic issue
				e.Details = []common.Issue{{Message: fmt.Sprintf("%v", e.Details)}}
			} else {
				e.Details = []common.Issue{}
			}
		}
		return e, 0
	}

	err := er.(error)
	// 1. Start with a generic fallback for unknown errors
	apiErr = APIError{
		Code:    "INTERNAL_SERVER_ERROR",
		Message: "An unexpected error occurred.",
		Details: err.Error(),
	}
	httpStatus = http.StatusInternalServerError // 500

	// 2. Translate common/custom errors
	var sysErr *common.SystemError
	if errors.As(err, &sysErr) {
		// Example translation for custom SystemError
		apiErr.Code = sysErr.Code
		apiErr.Message = sysErr.Message

		// Map SystemError to a likely HTTP status (e.g., if it's a validation error)
		httpStatus = http.StatusBadRequest // 400
		apiErr.Details = sysErr.ToIssues()
		return // Return immediately after handling a known custom type
	}

	// 3. Translate standard library or well-known errors (e.g., database/SQL errors)
	// Example: check for a generic "not found" pattern
	// if errors.Is(err, sql.ErrNoRows) {
	//    httpStatus = http.StatusNotFound
	//    apiErr.Code = "RESOURCE_NOT_FOUND"
	//    apiErr.Message = "The requested resource does not exist."
	// }

	return // Returns the default 500 if no specific translation was found
}
