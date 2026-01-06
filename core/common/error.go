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
	Code        string `json:"code"`                  // Code is a machine-readable identifier for the type of issue (e.g., "RESOURCE_LOCKED").
	Message     string `json:"message"`               // Message is a human-readable description of the issue.
	Path        string `json:"path,omitempty"`        // Path indicates the location in a document (e.g., "user.email").
	Severity    string `json:"severity,omitempty"`    // Severity indicates the seriousness: "error", "warning", "info".
	Description string `json:"description,omitempty"` // Description provides a more detailed, potentially multi-line explanation.
	Cause       *Issue `json:"cause,omitempty"`       // Cause represents the nested issue that caused this one.
}

// String implements the fmt.Stringer interface for a single Issue.
// It returns a human-readable representation in the format: [CODE] Message at 'Path'.
// It recursively renders nested causes to provide a clear audit trail of the failure.
func (i Issue) String() string {
	var sb strings.Builder

	if i.Code != "" {
		sb.WriteString(fmt.Sprintf("[%s] ", i.Code))
	}

	msg := i.Message
	if msg == "" {
		msg = "An issue occurred"
	}
	sb.WriteString(msg)

	if i.Path != "" {
		sb.WriteString(fmt.Sprintf(" at '%s'", i.Path))
	}

	if i.Cause != nil {
		sb.WriteString("\n  caused by: ")
		sb.WriteString(i.Cause.String())
	}

	return sb.String()
}

// Issues is a named slice of Issue objects that supports pretty-printing.
// This allows for cleaner logging when dealing with multiple validation errors.
type Issues []Issue

// String implements the fmt.Stringer interface for a collection of Issues.
// It renders a numbered list of issues, each on a new line.
func (is Issues) String() string {
	if len(is) == 0 {
		return "no issues found"
	}
	var sb strings.Builder
	for idx, iss := range is {
		sb.WriteString(fmt.Sprintf("%d. %s", idx+1, iss.String()))
		if idx < len(is)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// SystemError is the primary error type for all operational and validation errors.
// It implements the standard Go error interface and can be converted to structured
// Issue representations for API responses.
type SystemError struct {
	Path      string   // Path indicates the location in a document (e.g., "items[0].price")
	Operation string   // Operation describes where the error originated (e.g., "repository.Insert")
	Message   string   // Message is the human-readable error message
	Code      string   // Code is the machine-readable error code (e.g., "VALIDATION_ERROR")
	Severity  string   // Severity indicates the seriousness: "error", "warning", "info"
	Issues    Issues   // Issues contains multiple structured issues (e.g., multiple validation failures)
	Cause     error    // Cause is the underlying error that caused this one (standard Go error wrapping)
}

// Error implements the error interface.
// It provides a human-readable error message formatted as: Message at 'Path' during 'Operation' [Code].
func (e *SystemError) Error() string {
	var parts []string

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
	parts = append(parts, message)

	if e.Path != "" {
		parts = append(parts, fmt.Sprintf("at '%s'", e.Path))
	}

	if e.Operation != "" {
		parts = append(parts, fmt.Sprintf("during '%s'", e.Operation))
	}

	if e.Code != "" {
		parts = append(parts, fmt.Sprintf("[%s]", e.Code))
	}

	res := strings.Join(parts, " ")

	// If there's an underlying cause, append its message for more context.
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", res, e.Cause)
	}

	return res
}

// Unwrap returns the underlying cause error.
// This allows errors.Is() and errors.As() to work correctly with the error chain.
func (e *SystemError) Unwrap() error {
	return e.Cause
}

// SystemErrorFrom converts any error into a SystemError.
// If the error is already a SystemError, it returns a DUPLICATE to ensure immutability.
// It uses struct dereferencing to ensure all metadata (Path, Operation, Issues) is preserved.
func SystemErrorFrom(err error, code ...string) *SystemError {
	if err == nil {
		return nil
	}

	var sysErr *SystemError
	if errors.As(err, &sysErr) {
		// Create a shallow copy of the struct to ensure immutability.
		// This copies all fields including the Issues slice header.
		newErr := *sysErr

		if len(code) > 0 && code[0] != "" {
			newErr.Code = code[0]
		}
		if len(code) > 1 && code[1] != "" {
			newErr.Severity = code[1]
		}

		return &newErr
	}

	errorCode := "INTERNAL_ERROR"
	severity := "error"

	if len(code) > 0 && code[0] != "" {
		errorCode = code[0]
	}
	if len(code) > 1 && code[1] != "" {
		severity = code[1]
	}

	return &SystemError{
		Code:     errorCode,
		Message:  err.Error(),
		Severity: severity,
		Cause:    err,
	}
}

// NewSystemError creates a new SystemError with the given parameters.
func NewSystemError(code, message string) *SystemError {
	return &SystemError{
		Code:     code,
		Message:  message,
		Severity: "error",
	}
}

// --- Fluent API Methods (Safe for concurrent use) ---

// WithPath returns a copy of the error with the specified path.
func (e *SystemError) WithPath(path string) *SystemError {
	newErr := *e
	newErr.Path = path
	return &newErr
}

// WithOperation returns a copy of the error with the specified operation.
func (e *SystemError) WithOperation(operation string) *SystemError {
	newErr := *e
	newErr.Operation = operation
	return &newErr
}

// WithCause returns a copy of the error with the specified cause.
func (e *SystemError) WithCause(cause error) *SystemError {
	newErr := *e
	newErr.Cause = cause
	return &newErr
}

// WithIssue returns a copy of the error with the specified issue appended.
// It creates a new underlying slice to ensure true immutability.
func (e *SystemError) WithIssue(issue Issue) *SystemError {
	newErr := *e
	newIssues := make(Issues, len(e.Issues), len(e.Issues)+1)
	copy(newIssues, e.Issues)
	newErr.Issues = append(newIssues, issue)
	return &newErr
}

// WithIssues returns a copy of the error with multiple issues appended.
func (e *SystemError) WithIssues(issues []Issue) *SystemError {
	newErr := *e
	newIssues := make(Issues, len(e.Issues), len(e.Issues)+len(issues))
	copy(newIssues, e.Issues)
	newErr.Issues = append(newIssues, issues...)
	return &newErr
}

// WithCode returns a copy of the error with the specified code.
func (e *SystemError) WithCode(code string) *SystemError {
	newErr := *e
	newErr.Code = code
	return &newErr
}

// WithSeverity returns a copy of the error with the specified severity.
func (e *SystemError) WithSeverity(severity string) *SystemError {
	newErr := *e
	newErr.Severity = severity
	return &newErr
}

// WithMessage returns a copy of the error with the specified message.
func (e *SystemError) WithMessage(message string) *SystemError {
	newErr := *e
	newErr.Message = message
	return &newErr
}

// --- Helper Functions and DTO conversion ---

// ToIssue converts this SystemError into a single Issue for API responses.
func (e *SystemError) ToIssue(depth ...int) Issue {
	maxDepth := 0
	if len(depth) > 0 {
		maxDepth = depth[0]
	}
	return e.toIssueWithDepth(maxDepth, 0)
}

func (e *SystemError) toIssueWithDepth(maxDepth, currentDepth int) Issue {
	shouldTraverseCause := maxDepth < 0 || currentDepth < maxDepth

	if len(e.Issues) > 0 {
		primary := e.Issues[0]
		if e.Path != "" && primary.Path == "" {
			primary.Path = e.Path
		}

		if len(e.Issues) > 1 {
			current := &primary
			for i := 1; i < len(e.Issues); i++ {
				issueCopy := e.Issues[i]
				current.Cause = &issueCopy
				current = &issueCopy
			}
		}

		if e.Cause != nil && shouldTraverseCause {
			current := &primary
			for current.Cause != nil {
				current = current.Cause
			}
			current.Cause = errorToIssueWithDepth(e.Cause, maxDepth, currentDepth+1)
		}
		return primary
	}

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

	if e.Cause != nil && shouldTraverseCause {
		issue.Cause = errorToIssueWithDepth(e.Cause, maxDepth, currentDepth+1)
	}

	return issue
}

func errorToIssueWithDepth(err error, maxDepth, currentDepth int) *Issue {
	if err == nil {
		return nil
	}
	if maxDepth >= 0 && currentDepth >= maxDepth {
		return nil
	}

	var sysErr *SystemError
	if errors.As(err, &sysErr) {
		issue := sysErr.toIssueWithDepth(maxDepth, currentDepth)
		return &issue
	}

	if unwrappedErr := errors.Unwrap(err); unwrappedErr != nil {
		return errorToIssueWithDepth(unwrappedErr, maxDepth, currentDepth)
	}

	return &Issue{
		Code:     "INTERNAL_ERROR",
		Message:  err.Error(),
		Severity: "error",
	}
}

// Pre-defined common errors.
var (
	ErrInputCannotBeNil        = NewSystemError("ERR_COMMON_INPUT_CANNOT_BE_NIL", "input record cannot be nil")
	ErrInputCannotBeNilPointer = NewSystemError("ERR_COMMON_INPUT_CANNOT_BE_NIL_POINTER", "input record cannot be a nil pointer to a struct")
	ErrNoMetadata              = NewSystemError("ERR_COMMON_NO_METADATA", "no metadata found")
)
