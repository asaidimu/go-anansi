package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/google/uuid"
)

type APIResponse struct {
	Data any  `json:"data,omitempty"`
	Meta Meta `json:"meta"`
}

type ErrorResponse struct {
	Error apiError `json:"error"`
	Meta  Meta     `json:"meta"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type Meta struct {
	Count     int    `json:"count,omitempty"`
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request"`
}

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) WriteJSON(w http.ResponseWriter, status int, data any, r *http.Request, count ...int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse{Data: data, Meta: h.generateMeta(r, count...)})
}

func (h *Handler) WriteError(w http.ResponseWriter, status int, err any, r *http.Request) {
	apiErr, finalStatus := h.translateError(err, status)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(finalStatus)
	if encodeErr := json.NewEncoder(w).Encode(ErrorResponse{Error: apiErr, Meta: h.generateMeta(r)}); encodeErr != nil {
		log.Printf("Error writing JSON error response: %v", encodeErr)
	}
}

func (h *Handler) generateMeta(_ *http.Request, count ...int) Meta {
	result := Meta{
		Timestamp: time.Now().Format(time.RFC3339),
		RequestID: uuid.Must(uuid.NewV7()).String(),
	}
	if len(count) > 0 {
		result.Count = count[0]
	}
	return result
}


func (h *Handler) translateError(er any, defaultStatus int) (apiError, int) {
	var sysErr *common.SystemError

	// 1. Convert the input into a SystemError to leverage its structure
	switch e := er.(type) {
	case *common.SystemError:
		sysErr = e
	case error:
		if !errors.As(e, &sysErr) {
			sysErr = common.SystemErrorFrom(e)
		}
	default:
		sysErr = common.NewSystemError("INTERNAL_ERROR", fmt.Sprintf("%v", e))
	}

	issue := sysErr.ToIssue()

	return apiError{
		Code:    issue.Code,
		Message: issue.Message,
		Details: issue.Cause, // This will now be a JSON array []
	}, defaultStatus
}
