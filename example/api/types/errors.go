package types

import (
    "fmt"
    "net/http"
)

// ErrorCode represents standardized error codes
type ErrorCode string

const (
    // Client errors (4xx)
    ErrCodeInvalidJSON       ErrorCode = "INVALID_JSON"
    ErrCodeInvalidPath       ErrorCode = "INVALID_PATH"
    ErrCodeCollectionNotFound ErrorCode = "COLLECTION_NOT_FOUND"
    ErrCodeValidationFailed   ErrorCode = "VALIDATION_FAILED"
    ErrCodeMethodNotAllowed   ErrorCode = "METHOD_NOT_ALLOWED"
    
    // Server errors (5xx)
    ErrCodeInternalError     ErrorCode = "INTERNAL_ERROR"
    ErrCodeDatabaseError     ErrorCode = "DATABASE_ERROR"
    ErrCodeTransactionFailed ErrorCode = "TRANSACTION_FAILED"
)

// APIError represents structured error information
type APIError struct {
    Code       ErrorCode `json:"code"`
    Message    string    `json:"message"`
    Details    string    `json:"details,omitempty"`
    StatusCode int       `json:"-"`
}

func (e APIError) Error() string {
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error constructors
func NewClientError(code ErrorCode, message string, details ...string) *APIError {
	apiErr := &APIError{
		Code:    code,
		Message: message,
	}
	if len(details) > 0 {
		apiErr.Details = details[0]
	}
	return apiErr
}

func NewServerError(code ErrorCode, message string, internalErr error) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
		Details: internalErr.Error(), // For internal logging, not exposed to the client
	}
}

// HTTPStatus maps an APIError to an HTTP status code.
func (e APIError) HTTPStatus() int {
	switch e.Code {
	// Client errors
	case ErrCodeInvalidJSON:
		return http.StatusBadRequest
	case ErrCodeInvalidPath:
		return http.StatusNotFound
	case ErrCodeCollectionNotFound:
		return http.StatusNotFound
	case ErrCodeValidationFailed:
		return http.StatusBadRequest
	case ErrCodeMethodNotAllowed:
		return http.StatusMethodNotAllowed

	// Server errors
	case ErrCodeInternalError:
		return http.StatusInternalServerError
	case ErrCodeDatabaseError:
		return http.StatusInternalServerError
	case ErrCodeTransactionFailed:
		return http.StatusInternalServerError

	default:
		return http.StatusInternalServerError
	}
}
