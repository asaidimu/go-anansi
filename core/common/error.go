package common

import (
	"errors"
	"fmt"
	"strings"
)

// Issue represents a detailed validation or operational issue. It is used to provide
// structured, machine-readable feedback about problems encountered during an operation,
// which is particularly useful for form validation or API error responses.
// This is a DTO (Data Transfer Object) for serialization, not a Go error type.
type Issue struct {
	Code        string `json:"code"`                  // Code is a machine-readable identifier for the type of issue (e.g., "RESOURCE_LOCKED", "VALIDATION_ERROR").
	Message     string `json:"message"`               // Message is a human-readable description of the issue.
	Path        string `json:"path,omitempty"`        // Path indicates the location in a document (e.g., "user.email", "items[0].price").
	Severity    string `json:"severity,omitempty"`    // Severity indicates the seriousness: "error", "warning", "info".
	Description string `json:"description,omitempty"` // Description provides a more detailed, potentially multi-line explanation of the issue and how to resolve it.
	Cause       *Issue `json:"cause,omitempty"`       // Cause represents the nested issue that caused this one, for API response serialization.
}

// SystemError is the primary error type for all operational and validation errors.
// It implements the standard Go error interface and can be converted to structured
// Issue representations for API responses.
type SystemError struct {
	Path      string   // Path indicates the location in a document (e.g., "user.email", "items[0].price")
	Operation string   // Operation describes where the error originated (e.g., "repository.Insert", "document.Validate")
	Message   string   // Message is the human-readable error message
	Code      string   // Code is the machine-readable error code (e.g., "RESOURCE_LOCKED", "VALIDATION_ERROR")
	Severity  string   // Severity indicates the seriousness: "error", "warning", "info"
	Issues    []Issue  // Issues contains multiple structured issues (e.g., multiple validation failures)
	Cause     error    // Cause is the underlying error that caused this one (standard Go error wrapping)
}

// Error implements the error interface.
// It provides a human-readable error message suitable for logging and debugging.
func (e *SystemError) Error() string {
	var parts []string

	if e.Operation != "" {
		parts = append(parts, e.Operation)
	}

	if e.Path != "" {
		parts = append(parts, fmt.Sprintf("at '%s'", e.Path))
	}

	if e.Code != "" {
		parts = append(parts, fmt.Sprintf("[%s]", e.Code))
	}

	prefix := ""
	if len(parts) > 0 {
		prefix = strings.Join(parts, " ") + ": "
	}

	message := e.Message
	if message == "" && len(e.Issues) > 0 {
		message = e.Issues[0].Message
	}
	if message == "" && e.Cause != nil {
		message = e.Cause.Error()
	}
	if message == "" {
		message = "an error occurred"
	}

	// If there's an underlying cause, append its message for more context.
	if e.Cause != nil {
		return fmt.Sprintf("%s%s: %v", prefix, message, e.Cause)
	}

	return prefix + message
}

// Unwrap returns the underlying cause error.
// This allows errors.Is() and errors.As() to work correctly with the error chain.
func (e *SystemError) Unwrap() error {
	return e.Cause
}

// ToIssue converts this SystemError into a single Issue for API responses.
// The depth parameter controls how deep to traverse the error cause chain:
//   - depth < 0: traverse the entire chain (default behavior)
//   - depth = 0: only convert this error, don't include any causes
//   - depth > 0: traverse up to depth levels deep
//
// If the error contains multiple Issues, it returns the first one with the others nested.
func (e *SystemError) ToIssue(depth ...int) Issue {
	maxDepth := 0
	if len(depth) > 0 {
		maxDepth = depth[0]
	}

	return e.toIssueWithDepth(maxDepth, 0)
}

// toIssueWithDepth is the internal recursive implementation of ToIssue.
func (e *SystemError) toIssueWithDepth(maxDepth, currentDepth int) Issue {
	// If we've reached the maximum depth, stop traversing
	shouldTraverseCause := maxDepth < 0 || currentDepth < maxDepth

	// If we have explicit Issues, use the first one as the primary
	if len(e.Issues) > 0 {
		primary := e.Issues[0]

		// Preserve the document path if present
		if e.Path != "" && primary.Path == "" {
			primary.Path = e.Path
		}

		// If there are additional issues, chain them as causes
		if len(e.Issues) > 1 {
			current := &primary
			for i := 1; i < len(e.Issues); i++ {
				issueCopy := e.Issues[i]
				current.Cause = &issueCopy
				current = &issueCopy
			}
		}

		// If there's also an underlying error cause and we should traverse it, append it to the chain
		if e.Cause != nil && shouldTraverseCause {
			current := &primary
			for current.Cause != nil {
				current = current.Cause
			}
			current.Cause = errorToIssueWithDepth(e.Cause, maxDepth, currentDepth+1)
		}

		return primary
	}

	// Otherwise, construct an Issue from the SystemError fields
	issue := Issue{
		Code:     e.Code,
		Message:  e.Message,
		Path:     e.Path,
		Severity: e.Severity,
	}

	if issue.Code == "" {
		issue.Code = "INTERNAL_ERROR"
	}
	if issue.Severity == "" {
		issue.Severity = "error"
	}
	if issue.Message == "" && e.Cause != nil {
		issue.Message = e.Cause.Error()
	}

	// Convert the cause chain into nested Issues if we should traverse
	if e.Cause != nil && shouldTraverseCause {
		issue.Cause = errorToIssueWithDepth(e.Cause, maxDepth, currentDepth+1)
	}

	return issue
}

// ToIssues returns all Issues contained in this SystemError.
// The depth parameter controls how deep to traverse the error cause chain (same as ToIssue).
// This is useful when you want to return multiple validation errors or operational issues.
func (e *SystemError) ToIssues(depth ...int) []Issue {
	maxDepth := -1 // Default: traverse entire chain
	if len(depth) > 0 {
		maxDepth = depth[0]
	}

	shouldTraverseCause := maxDepth < 0 || 0 < maxDepth

	if len(e.Issues) > 0 {
		// If we have explicit issues, return them with the path preserved
		issues := make([]Issue, len(e.Issues))
		copy(issues, e.Issues)

		// If the SystemError has a Path and an issue doesn't, use it
		if e.Path != "" {
			for i := range issues {
				if issues[i].Path == "" {
					issues[i].Path = e.Path
				}
			}
		}

		if e.Cause != nil && shouldTraverseCause {
			// Append the cause to the last issue
			lastIdx := len(issues) - 1
			issues[lastIdx].Cause = errorToIssueWithDepth(e.Cause, maxDepth, 1)
		}

		return issues
	}

	// Otherwise return a single issue constructed from this error
	return []Issue{e.toIssueWithDepth(maxDepth, 0)}
}

// errorToIssueWithDepth converts any error into an Issue with depth control.
func errorToIssueWithDepth(err error, maxDepth, currentDepth int) *Issue {
	if err == nil {
		return nil
	}

	// Check if we've reached the maximum depth
	if maxDepth >= 0 && currentDepth >= maxDepth {
		return nil
	}

	// Try to find a SystemError in the error chain
	var sysErr *SystemError
	if errors.As(err, &sysErr) {
		issue := sysErr.toIssueWithDepth(maxDepth, currentDepth)
		return &issue
	}

	// If not a SystemError, but it's a wrapped error, try unwrapping
	if unwrappedErr := errors.Unwrap(err); unwrappedErr != nil {
		// Recursively call with the unwrapped error
		return errorToIssueWithDepth(unwrappedErr, maxDepth, currentDepth)
	}

	// For any other error (not SystemError, not unwrappable), create a generic issue
	return &Issue{
		Code:     "INTERNAL_ERROR",
		Message:  err.Error(),
		Severity: "error",
	}
}

// SystemErrorFrom converts any error into a SystemError.
// If the error is already a SystemError, it returns a DUPLICATE SystemError.
// If the error is nil, it returns nil.
// Otherwise, it wraps the error in a new SystemError with the provided code and severity.
// If code is not provided, defaults to "INTERNAL_ERROR".
// If severity is not provided, defaults to "error".
// This ensures immutability of the original SystemError object if it was passed in.
func SystemErrorFrom(err error, code ...string) *SystemError {
	if err == nil {
		return nil
	}

	// Check if it's already a SystemError
	var sysErr *SystemError
	if errors.As(err, &sysErr) {
		// Found an existing SystemError.
		// To ensure immutability, we return a duplicate/copy instead of the original.

		// The function needs to respect the optional 'code' and 'severity'
		// if they are explicitly provided, otherwise use the existing sysErr's values.

		// Determine code and severity from optional parameters,
		// falling back to the existing SystemError's fields.
		errorCode := sysErr.Code
		severity := sysErr.Severity

		if len(code) > 0 && code[0] != "" {
			errorCode = code[0] // Override code if provided
		}
		if len(code) > 1 && code[1] != "" {
			severity = code[1] // Override severity if provided
		}

		// Return a NEW SystemError (a duplicate)
		return &SystemError{
			Code:     errorCode,
			Message:  sysErr.Message, // Keep the original message
			Severity: severity,
			Cause:    sysErr.Cause,    // Keep the original cause, which might be another error
		}
	}

	// --- Original logic for non-SystemError errors remains the same ---

	// Determine code and severity from optional parameters, falling back to defaults
	errorCode := "INTERNAL_ERROR"
	severity := "error"

	if len(code) > 0 && code[0] != "" {
		errorCode = code[0]
	}
	if len(code) > 1 && code[1] != "" {
		severity = code[1]
	}

	// Otherwise, wrap it in a new SystemError
	return &SystemError{
		Code:     errorCode,
		Message:  err.Error(),
		Severity: severity,
		Cause:    err,
	}
}

// NewSystemError creates a new SystemError with the given parameters.
// This is a convenience constructor for common error creation patterns.
func NewSystemError(code, message string) *SystemError {
	return &SystemError{
		Code:     code,
		Message:  message,
		Severity: "error",
	}
}

// WithPath creates a new SystemError with the same properties as the original, but with the specified path.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithPath(path string) *SystemError {
	newErr := *e
	newErr.Path = path
	return &newErr
}

// WithOperation creates a new SystemError with the same properties as the original, but with the specified operation.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithOperation(operation string) *SystemError {
	newErr := *e
	newErr.Operation = operation
	return &newErr
}

// WithCause creates a new SystemError with the same properties as the original, but with the specified cause.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithCause(cause error) *SystemError {
	newErr := *e
	newErr.Cause = cause
	return &newErr
}

// WithIssue creates a new SystemError with the same properties as the original, but with the specified issue appended.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithIssue(issue Issue) *SystemError {
	newErr := *e
	newErr.Issues = append(e.Issues, issue)
	return &newErr
}

// WithIssues creates a new SystemError with the same properties as the original, but with the specified issues appended.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithIssues(issues []Issue) *SystemError {
	newErr := *e
	newErr.Issues = append(e.Issues, issues...)
	return &newErr
}

// WithCode creates a new SystemError with the same properties as the original, but with the specified code.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithCode(code string) *SystemError {
	newErr := *e
	newErr.Code = code
	return &newErr
}

// WithSeverity creates a new SystemError with the same properties as the original, but with the specified severity.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithSeverity(severity string) *SystemError {
	newErr := *e
	newErr.Severity = severity
	return &newErr
}

// WithMessage creates a new SystemError with the same properties as the original, but with the specified message.
// This is safe for concurrent use as it does not modify the original error.
func (e *SystemError) WithMessage(message string) *SystemError {
	newErr := *e
	newErr.Message = message
	return &newErr
}

// Pre-defined common errors.
var (
	ErrInputCannotBeNil             = NewSystemError("ERR_COMMON_INPUT_CANNOT_BE_NIL", "input record cannot be nil")
	ErrInputCannotBeNilPointer      = NewSystemError("ERR_COMMON_INPUT_CANNOT_BE_NIL_POINTER", "input record cannot be a nil pointer to a struct")
	ErrInputMustBeStruct            = NewSystemError("ERR_COMMON_INPUT_MUST_BE_STRUCT", "input record must be a struct or a pointer to a struct")
	ErrFailedToMarshalInput         = NewSystemError("ERR_COMMON_FAILED_TO_MARSHAL_INPUT", "failed to marshal input record to JSON")
	ErrFailedToUnmarshalInput       = NewSystemError("ERR_COMMON_FAILED_TO_UNMARSHAL_INPUT", "failed to unmarshal JSON to map[string]any")
	ErrMapToStructInputCannotBeNil  = NewSystemError("ERR_COMMON_MAPTOSTRUCT_INPUT_CANNOT_BE_NIL", "MapToStruct: input map cannot be nil")
	ErrMapToStructTargetNotStruct   = NewSystemError("ERR_COMMON_MAPTOSTRUCT_TARGET_NOT_STRUCT", "MapToStruct: generic type T must be a struct type (or pointer to struct)")
	ErrMapToStructFailedToMarshal   = NewSystemError("ERR_COMMON_MAPTOSTRUCT_FAILED_TO_MARSHAL", "MapToStruct: failed to marshal input map to JSON")
	ErrMapToStructFailedToUnmarshal = NewSystemError("ERR_COMMON_MAPTOSTRUCT_FAILED_TO_UNMARSHAL", "MapToStruct: failed to unmarshal JSON to target struct")
	ErrNoMetadata                   = NewSystemError("ERR_COMMON_NO_METADATA", "no metadata found")
	ErrMetadataKeyNotFound          = NewSystemError("ERR_COMMON_METADATA_KEY_NOT_FOUND", "metadata key not found")
	ErrFailedToCalculateHash        = NewSystemError("ERR_COMMON_FAILED_TO_CALCULATE_HASH", "failed to calculate hash")
	ErrHashMismatch                 = NewSystemError("ERR_COMMON_HASH_MISMATCH", "hash mismatch")
)

type TransformFunc func(string) string

// Sanitize recurses through the error tree to transform messages, codes, and paths.
func (e *SystemError) Sanitize(tf TransformFunc) *SystemError {
	if e == nil {
		return nil
	}

	newErr := *e
	newErr.Message = tf(e.Message)
	newErr.Code = tf(e.Code)
	newErr.Path = tf(e.Path)

	// 1. Sanitize the Issues slice
	if len(e.Issues) > 0 {
		newIssues := make([]Issue, len(e.Issues))
		for i, iss := range e.Issues {
			newIssues[i] = sanitizeIssue(iss, tf)
		}
		newErr.Issues = newIssues
	}

	// 2. Sanitize the Cause (if it's a SystemError)
	if e.Cause != nil {
		var sysErr *SystemError
		if errors.As(e.Cause, &sysErr) {
			newErr.Cause = sysErr.Sanitize(tf)
		} else {
			// If it's a raw Go error, we wrap it in a sanitized SystemError
			// to ensure the message is cleaned.
			newErr.Cause = errors.New(tf(e.Cause.Error()))
		}
	}

	return &newErr
}

// sanitizeIssue is a helper for the recursive DTO cleanup
func sanitizeIssue(iss Issue, tf TransformFunc) Issue {
	iss.Message = tf(iss.Message)
	iss.Path = tf(iss.Path)
	iss.Code = tf(iss.Code)
	iss.Description = tf(iss.Description)
	if iss.Cause != nil {
		cleanedCause := sanitizeIssue(*iss.Cause, tf)
		iss.Cause = &cleanedCause
	}
	return iss
}
