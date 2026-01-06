package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// This module provides a structured error handling system designed for complex
// operational and validation scenarios. The core design supports:
//
//   - Hierarchical error representation with context (path, operation, code)
//   - Multiple validation issues attached to a single error
//   - Causal chains showing why errors occurred
//   - Immutable fluent API for building rich error contexts
//   - Severity levels (error, warning, info) for nuanced feedback
//   - Internationalization support with format string translation
//
// Example usage:
//
//	err := NewSystemError("VALIDATION_FAILED").
//		WithMessagef("document validation failed for %d documents", count).
//		WithPath("users[0]").
//		WithOperation("CreateUser").
//		WithIssue(Issue{
//			Code:    "REQUIRED_FIELD_MISSING",
//			Message: "email is required",
//			Path:    "email",
//		})
//
// The error can represent complex scenarios like batch operations where one
// document fails due to multiple underlying validation issues:

// ============================================================================
// Severity Constants
// ============================================================================

const (
	// SeverityError indicates a critical failure that prevents operation completion.
	// Use for validation failures, resource conflicts, permission denials, etc.
	SeverityError = "error"

	// SeverityWarning indicates a non-critical issue that doesn't prevent operation
	// completion but may require attention. Use for deprecation notices, suboptimal
	// configurations, or recoverable inconsistencies.
	SeverityWarning = "warning"

	// SeverityInfo provides informational feedback about the operation without
	// indicating problems. Use for progress updates, suggestions, or contextual notes.
	SeverityInfo = "info"
)

// ============================================================================
// Issue - Structured Validation/Operational Feedback
// ============================================================================

// Issue represents a detailed validation or operational problem. Issues are the
// building blocks of structured error feedback, designed to be machine-readable
// while remaining human-friendly.
//
// An Issue can have multiple causes (represented as nested Issues), allowing you
// to express complex error scenarios. For example, a document creation failure
// might be caused by both a missing required field AND an unexpected field.
//
// Fields:
//   - Code: Machine-readable identifier (e.g., "REQUIRED_FIELD_MISSING")
//   - Message: Human-readable description
//   - Path: Field path in a document (e.g., "email", "user.name", "address.city")
//   - Index: Optional array index when the issue relates to a specific element
//   - Severity: One of SeverityError, SeverityWarning, or SeverityInfo
//   - Description: Optional detailed explanation, can be multi-line
//   - Cause: Optional nested issues that caused this issue
//
// Path Normalization:
//
//	Paths are automatically normalized to use '.' as separator when marshaled to JSON
//	or converted to string. Internally, paths may use '/' (JSON Pointer style) or '.'
//	(object notation), but consumers always receive normalized '.' paths.
//
//	Examples of normalization:
//	  "/fields/user_id/type" → "fields.user_id.type"
//	  "fields/user_id/type"  → "fields.user_id.type"
//	  "fields.user_id.type"  → "fields.user_id.type" (already normalized)
//
// For internationalization, Issues support format string translation:
//
//	issue := Issue{
//		Code: "FIELD_TOO_LONG",
//		messageFormat: "field exceeds maximum length of %d characters",
//		messageArgs: []any{255},
//	}
//	// Can be translated to: "sehemu imezidi urefu wa juu wa herufi %d"
//
// Path vs Index Usage:
//
//	// Field validation (no index needed)
//	Issue{Path: "email", Code: "INVALID_FORMAT"}
//
//	// Array element validation (index + path)
//	Issue{Index: intPtr(1), Path: "role", Code: "REQUIRED_FIELD_MISSING"}
//	// Client knows this is at array[1].role without parsing strings
//
//	// Array-level issue (index only, no path)
//	Issue{Index: intPtr(1), Code: "DOCUMENT_CREATION_FAILED"}
type Issue struct {
	Code          string  `json:"code,omitempty"`        // Machine-readable identifier (e.g., "RESOURCE_LOCKED").
	// Message is the rendered error message. This field is populated by WithMessagef
    // and should not be modified directly if you plan to use Translate(), as the
    // translation uses the stored format string and arguments, not this field.
	Message       string  `json:"message,omitempty"`     // Human-readable description.
	Path          string  `json:"path,omitempty"`        // Field path (e.g., "email", "user.name").
	Index         *int    `json:"index,omitempty"`       // Array index if applicable (e.g., 0, 1, 2).
	Severity      string  `json:"severity,omitempty"`    // Seriousness: "error", "warning", "info".
	Description   string  `json:"description,omitempty"` // Detailed, potentially multi-line explanation.
	Cause         *Issues `json:"cause,omitempty"`       // Nested issues that caused this one.
	messageFormat string  // Private: format string for translation
	messageArgs   []any   // Private: arguments for format string
}

// NormalizedPath returns the path with consistent '.' separators.
// Converts both JSON Pointer style (/fields/name) and mixed formats to dot notation.
func (i Issue) NormalizedPath() string {
	return normalizePath(i.Path)
}

// normalizePath converts various path formats to consistent dot notation.
// Supports:
//   - JSON Pointer style: "/fields/user_id/type" → "fields.user_id.type"
//   - Mixed separators: "fields/user_id.type" → "fields.user_id.type"
//   - Already normalized: "fields.user_id.type" → "fields.user_id.type"
//
// Leading slashes are removed, and all forward slashes are converted to dots.
func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	// Remove leading slash (JSON Pointer style)
	normalized := strings.TrimPrefix(path, "/")
	// Convert all forward slashes to dots
	normalized = strings.ReplaceAll(normalized, "/", ".")
	return normalized
}

// MarshalJSON implements custom JSON marshaling to normalize paths.
func (i Issue) MarshalJSON() ([]byte, error) {
	type Alias Issue
	return json.Marshal(&struct {
		Path string `json:"path,omitempty"`
		*Alias
	}{
		Path:  i.NormalizedPath(),
		Alias: (*Alias)(&i),
	})
}

// String implements the fmt.Stringer interface for a single Issue.
// Produces a human-readable representation suitable for logging or display.
// Paths are normalized to use '.' separators for consistency.
//
// Format: [CODE] message at 'path' (or at index N if applicable)
//
// If the issue has causes, they are indented and prefixed with "caused by:".
func (i Issue) String() string {
	var sb strings.Builder
	if i.Code != "" {
		sb.WriteString(fmt.Sprintf("[%s] ", i.Code))
	}
	msg := i.Message
	if msg == "" {
		msg = "an issue occurred"
	}
	sb.WriteString(msg)

	// Show location information with normalized path
	normalizedPath := i.NormalizedPath()
	if i.Index != nil && normalizedPath != "" {
		sb.WriteString(fmt.Sprintf(" at index %d, path '%s'", *i.Index, normalizedPath))
	} else if i.Index != nil {
		sb.WriteString(fmt.Sprintf(" at index %d", *i.Index))
	} else if normalizedPath != "" {
		sb.WriteString(fmt.Sprintf(" at '%s'", normalizedPath))
	}

	if i.Cause != nil {
		sb.WriteString("\n  caused by: ")
		sb.WriteString(i.Cause.String())
	}
	return sb.String()
}

// IsError returns true if this issue has error severity.
func (i Issue) IsError() bool {
	return i.Severity == SeverityError || i.Severity == ""
}

// IsWarning returns true if this issue has warning severity.
func (i Issue) IsWarning() bool {
	return i.Severity == SeverityWarning
}

// IsInfo returns true if this issue has info severity.
func (i Issue) IsInfo() bool {
	return i.Severity == SeverityInfo
}

// WithMessagef sets the message using a format string and arguments.
// The format and arguments are stored for later translation.
//
// Example:
// issue.WithMessagef("field exceeds maximum length of %d characters", 255)
func (i Issue) WithMessagef(format string, args ...any) Issue {
	i.messageFormat = format
	i.messageArgs = args
	i.Message = fmt.Sprintf(format, args...)
	return i
}

// Translate returns a new Issue with the message translated using the provided
// translation catalog. If no translation is found for the code, the original
// message is preserved.
//
// The translation uses the stored format string and arguments, allowing the
// same data to be presented in different languages:
//
//	// English: "user 123 not found"
//	// Swahili: "mtumiaji 123 hajapatikana"
func (i Issue) Translate(locale string, catalog TranslationCatalog) Issue {
	newIssue := i
	if i.messageFormat != "" {
		if template := catalog.Get(locale, i.Code); template != "" {
			newIssue.Message = fmt.Sprintf(template, i.messageArgs...)
		}
	}

	// Recursively translate nested causes
	if i.Cause != nil {
		translatedCauses := make(Issues, len(*i.Cause))
		for idx, cause := range *i.Cause {
			translatedCauses[idx] = cause.Translate(locale, catalog)
		}
		newIssue.Cause = &translatedCauses
	}

	return newIssue
}

// ============================================================================
// Issues - Collection of Issue Objects
// ============================================================================

// Issues is a collection of Issue objects that supports pretty-printing and
// provides convenience methods for filtering and inspection.
type Issues []Issue

// String implements the fmt.Stringer interface for a slice of Issues.
// Produces a numbered list of issues, suitable for displaying multiple
// validation errors or operational problems.
//
// Example output:
//
//  1. [REQUIRED_FIELD_MISSING] email is required at 'user.email'
//  2. [INVALID_FORMAT] phone number format is invalid at 'user.phone'
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

// HasErrors returns true if any issue in the collection has error severity.
// This is useful for determining if an operation should be aborted.
func (is Issues) HasErrors() bool {
	for _, issue := range is {
		if issue.IsError() {
			return true
		}
	}
	return false
}

// FilterBySeverity returns a new Issues slice containing only issues
// matching the specified severity level.
func (is Issues) FilterBySeverity(severity string) Issues {
	var filtered Issues
	for _, issue := range is {
		if issue.Severity == severity {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

// ContainsCode returns true if any issue in the collection has the specified code.
// This search is non-recursive and only checks top-level issues.
func (is Issues) ContainsCode(code string) bool {
	for _, issue := range is {
		if issue.Code == code {
			return true
		}
	}
	return false
}

// Translate returns a new Issues slice with all issues translated to the
// specified locale using the provided translation catalog.
func (is Issues) Translate(locale string, catalog TranslationCatalog) Issues {
	translated := make(Issues, len(is))
	for i, issue := range is {
		translated[i] = issue.Translate(locale, catalog)
	}
	return translated
}

// ============================================================================
// SystemError - Primary Error Type
// ============================================================================

// SystemError is the primary error type for all operational and validation errors.
// It provides rich contextual information and supports immutable composition through
// a fluent API.
//
// Design principles:
//   - Immutability: All With* methods return new instances, leaving the original unchanged
//   - Context: Captures where (Path), what (Operation), why (Code), and details (Issues)
//   - Composition: Can wrap other errors via Cause, preserving the error chain
//   - Machine-readable: Structured fields enable programmatic error handling
//   - Human-friendly: Error() method produces natural language output
//   - Internationalization: Supports format string translation with stored arguments
//
// Example:
//
//	err := NewSystemError("VALIDATION_FAILED").
//		WithMessagef("user validation failed for %d fields", 3).
//		WithPath("users[0]").
//		WithOperation("CreateBatch").
//		WithIssue(Issue{
//			Code:    "REQUIRED_FIELD_MISSING",
//			Message: "email is required",
//			Path:    "email",
//			Severity: SeverityError,
//		})
type SystemError struct {
	Path          string // Location where error occurred (e.g., "users[0].email")
	Operation     string // Operation being performed (e.g., "CreateUser", "ValidateDocument")
	Message       string // Primary human-readable error message
	Code          string // Machine-readable error code (e.g., "VALIDATION_FAILED")
	Severity      string // Error severity: SeverityError, SeverityWarning, or SeverityInfo
	Issues        Issues // Detailed issues providing structured feedback
	Cause         error  // Underlying error that caused this one (supports error wrapping)
	messageFormat string // Private: format string for translation
	messageArgs   []any  // Private: arguments for format string
}

// ============================================================================
// SystemError - Core Methods
// ============================================================================

// Error implements the error interface with natural language phrasing.
// Produces output like: "user validation failed at 'users[0]' during 'CreateBatch' [VALIDATION_FAILED]"
//
// The method intelligently falls back through available information:
//  1. Uses Message if present
//  2. Falls back to first Issue's message
//  3. Falls back to Cause's error message
//  4. Uses generic "an error occurred" as last resort
func (e *SystemError) Error() string {
	var parts []string
	msg := e.Message
	if msg == "" && len(e.Issues) > 0 {
		msg = e.Issues[0].Message
	}
	if msg == "" && e.Cause != nil {
		msg = e.Cause.Error()
	}
	if msg == "" {
		msg = "an error occurred"
	}
	parts = append(parts, msg)
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
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", res, e.Cause)
	}
	return res
}

// Unwrap allows errors.Is and errors.As to traverse the error chain.
// This enables standard Go error handling patterns like:
//
//	if errors.Is(err, ErrNotFound) { ... }
//	var sysErr *SystemError
//	if errors.As(err, &sysErr) { ... }
func (e *SystemError) Unwrap() error { return e.Cause }

// IsError returns true if this error has error severity (or unspecified severity,
// which defaults to error).
func (e *SystemError) IsError() bool {
	return e.Severity == SeverityError || e.Severity == ""
}

// IsWarning returns true if this error has warning severity.
func (e *SystemError) IsWarning() bool {
	return e.Severity == SeverityWarning
}

// IsInfo returns true if this error has info severity.
func (e *SystemError) IsInfo() bool {
	return e.Severity == SeverityInfo
}

// HasErrors returns true if this SystemError or any of its Issues have error severity.
// This is useful for determining if an operation should fail based on the error tree.
func (e *SystemError) HasErrors() bool {
	if e.IsError() {
		return true
	}
	return e.Issues.HasErrors()
}

// ContainsCode recursively searches for the specified code in this error and all
// nested issues. Returns true if found anywhere in the error tree.
func (e *SystemError) ContainsCode(code string) bool {
	if e.Code == code {
		return true
	}
	if e.Issues.ContainsCode(code) {
		return true
	}
	// Recursively check causes
	for _, issue := range e.Issues {
		if issue.Cause != nil && issue.Cause.ContainsCode(code) {
			return true
		}
	}
	return false
}

// ============================================================================
// SystemError - Constructors
// ============================================================================

// NewSystemError creates a new SystemError instance with the specified code
// and optional message. The error defaults to SeverityError. Use the fluent
// With* methods to add additional context.
//
// The message parameter is optional for backward compatibility:
//
//	// With message (traditional API)
//	err := NewSystemError("NOT_FOUND", "user not found")
//
//	// Without message (use WithMessagef for i18n)
//	err := NewSystemError("NOT_FOUND").
//		WithMessagef("user %d not found", userID)
func NewSystemError(code string, message ...string) *SystemError {
	err := &SystemError{Code: code, Severity: SeverityError}
	if len(message) > 0 {
		err.Message = message[0]
	}
	return err
}

// SystemErrorFrom converts any error into a SystemError. If the error is already
// a SystemError, it creates a copy and optionally updates the code and severity.
// Otherwise, it wraps the error in a new SystemError.
//
// Parameters:
//   - err: The error to convert
//   - code: Optional variadic parameters where code[0] is the error code and
//     code[1] is the severity. If not provided, defaults to "INTERNAL_ERROR"
//     and "error" respectively.
//
// This function preserves the entire error structure including all Issues and nested
// causes by creating a proper copy of the SystemError fields.
//
// Example:
//
//	// Convert with default code
//	sysErr := SystemErrorFrom(someError)
//
//	// Convert with custom code
//	sysErr := SystemErrorFrom(someError, "DATABASE_ERROR")
//
//	// Convert with custom code and severity
//	sysErr := SystemErrorFrom(someError, "DEPRECATED_API", "warning")
func SystemErrorFrom(err error, code ...string) *SystemError {
	if err == nil {
		return nil
	}
	var sysErr *SystemError
	if errors.As(err, &sysErr) {
		// Create a complete copy to preserve all fields
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
	severity := SeverityError
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

// ============================================================================
// SystemError - Fluent API (Immutable Builders)
// ============================================================================
//
// All With* methods follow an immutable pattern: they create and return a new
// SystemError instance with the updated field, leaving the original unchanged.
// This prevents accidental mutation and enables safe concurrent use.
//
// Example of chaining:
//
//	baseErr := NewSystemError("VALIDATION_FAILED")
//	userErr := baseErr.WithMessagef("validation failed for user %s", "john").
//		WithPath("users[0]").
//		WithOperation("CreateUser")
//	adminErr := baseErr.WithMessagef("validation failed for admin %s", "jane").
//		WithPath("admins[0]").
//		WithOperation("CreateAdmin")
//	// baseErr remains unchanged, userErr and adminErr are independent copies

// WithPath returns a new SystemError with the specified path.
// The path typically indicates the location in a document or data structure
// where the error occurred (e.g., "users[0].email", "config.database.host").
func (e *SystemError) WithPath(path string) *SystemError {
	newErr := *e
	newErr.Path = path
	return &newErr
}

// WithOperation returns a new SystemError with the specified operation name.
// The operation describes what was being attempted when the error occurred
// (e.g., "CreateUser", "ValidateDocument", "UpdatePayment").
func (e *SystemError) WithOperation(op string) *SystemError {
	newErr := *e
	newErr.Operation = op
	return &newErr
}

// WithCause returns a new SystemError wrapping the specified cause.
// This enables error chaining and preserves the full context of nested errors.
// The cause can be retrieved using errors.Unwrap or checked with errors.Is/As.
func (e *SystemError) WithCause(cause error) *SystemError {
	newErr := *e
	newErr.Cause = cause
	return &newErr
}

// WithIssue returns a new SystemError with the specified issue appended to the
// Issues collection. This is useful for adding detailed validation feedback or
// operational problems to the error.
func (e *SystemError) WithIssue(issue Issue) *SystemError {
	newErr := *e
	newIssues := make(Issues, len(e.Issues), len(e.Issues)+1)
	copy(newIssues, e.Issues)
	newErr.Issues = append(newIssues, issue)
	return &newErr
}

// WithIssues returns a new SystemError with the specified issues appended to the
// existing Issues collection. This is useful for batch operations where multiple
// validation problems need to be reported together.
func (e *SystemError) WithIssues(issues []Issue) *SystemError {
	newErr := *e
	newIssues := make(Issues, len(e.Issues), len(e.Issues)+len(issues))
	copy(newIssues, e.Issues)
	newErr.Issues = append(newIssues, issues...)
	return &newErr
}

// WithMessage returns a new SystemError with the specified message.
// Use this to set a static message without format arguments.
func (e *SystemError) WithMessage(msg string) *SystemError {
	newErr := *e
	newErr.Message = msg
	newErr.messageFormat = ""
	newErr.messageArgs = nil
	return &newErr
}

// WithMessagef returns a new SystemError with a formatted message.
// The format string and arguments are stored privately to enable translation
// while preserving the original data.
//
// Example:
//
//	err := NewSystemError("USER_NOT_FOUND").
//		WithMessagef("user with id %d not found", 123)
//
//	// Later, can be translated:
//	// English: "user with id 123 not found"
//	// Swahili: "mtumiaji mwenye nambari 123 hajapatikana"
func (e *SystemError) WithMessagef(format string, args ...any) *SystemError {
	newErr := *e
	newErr.messageFormat = format
	newErr.messageArgs = args
	newErr.Message = fmt.Sprintf(format, args...)
	return &newErr
}

// WithCode returns a new SystemError with the specified error code.
// Error codes should be machine-readable identifiers that enable programmatic
// error handling (e.g., "NOT_FOUND", "VALIDATION_FAILED", "PERMISSION_DENIED").
func (e *SystemError) WithCode(code string) *SystemError {
	newErr := *e
	newErr.Code = code
	return &newErr
}

// WithSeverity returns a new SystemError with the specified severity level.
// Use SeverityError, SeverityWarning, or SeverityInfo constants.
func (e *SystemError) WithSeverity(severity string) *SystemError {
	newErr := *e
	newErr.Severity = severity
	return &newErr
}

// ============================================================================
// SystemError - Internationalization
// ============================================================================

// TranslationCatalog defines the interface for looking up translated format strings.
// Implementations typically load translations from files, databases, or embedded resources.
//
// Example implementation:
//
//	type SimpleCatalog map[string]map[string]string
//
//	func (c SimpleCatalog) Get(locale, code string) string {
//		if translations, ok := c[locale]; ok {
//			return translations[code]
//		}
//		return ""
//	}
type TranslationCatalog interface {
	// Get retrieves the format string for the given locale and error code.
	// Returns empty string if no translation is found.
	Get(locale, code string) string
}

// Translate returns a new SystemError with the message translated to the specified
// locale using the provided translation catalog. If no translation is found for the
// error code, the original message is preserved.
//
// The translation replays the stored format string and arguments, allowing the same
// data to be presented in different languages while maintaining information density:
//
//	// English: "user 123 not found"
//	// Swahili: "mtumiaji 123 hajapatikana"
//
// Example:
//
//	catalog := map[string]map[string]string{
//		"en": {"USER_NOT_FOUND": "user %d not found"},
//		"sw": {"USER_NOT_FOUND": "mtumiaji %d hajapatikana"},
//	}
//
//	err := NewSystemError("USER_NOT_FOUND").WithMessagef("user %d not found", 123)
//	swahiliErr := err.Translate("sw", catalog)
//	// swahiliErr.Message == "mtumiaji 123 hajapatikana"
func (e *SystemError) Translate(locale string, catalog TranslationCatalog) *SystemError {
	newErr := *e

	// Translate the main message if we have format info
	if e.messageFormat != "" {
		if template := catalog.Get(locale, e.Code); template != "" {
			newErr.Message = fmt.Sprintf(template, e.messageArgs...)
		}
	}

	// Translate all issues
	if len(e.Issues) > 0 {
		newErr.Issues = e.Issues.Translate(locale, catalog)
	}

	// Recursively translate the cause if it's a SystemError
	if e.Cause != nil {
		if sysErr, ok := e.Cause.(*SystemError); ok {
			newErr.Cause = sysErr.Translate(locale, catalog)
		}
	}

	return &newErr
}

// ============================================================================
// SystemError - Transformation and Conversion
// ============================================================================

// TransformFunc is a function that transforms a string, typically used for
// sanitization or redaction of sensitive information in error messages.
type TransformFunc func(string) string

// Sanitize recursively transforms all string fields in the error tree using the
// provided transformation function. This is useful for:
//   - Removing sensitive information (passwords, tokens, PII)
//   - Redacting internal paths or implementation details
//   - Normalizing error messages for public APIs
//
// The transformation is applied to:
//   - Message, Code, Path in SystemError
//   - Message, Code, Path, Description in all Issues
//   - Nested causes (recursively)
//
// Example:
//
//	redact := func(s string) string {
//		return strings.ReplaceAll(s, "secret_key_123", "[REDACTED]")
//	}
//	sanitized := err.Sanitize(redact)
func (e *SystemError) Sanitize(tf TransformFunc) *SystemError {
	if e == nil {
		return nil
	}
	newErr := *e
	newErr.Message = tf(e.Message)
	newErr.Code = tf(e.Code)
	newErr.Path = tf(e.Path)

	if len(e.Issues) > 0 {
		newIssues := make(Issues, len(e.Issues))
		for i, iss := range e.Issues {
			newIssues[i] = sanitizeIssue(iss, tf)
		}
		newErr.Issues = newIssues
	}

	if e.Cause != nil {
		if sysErr, ok := e.Cause.(*SystemError); ok {
			newErr.Cause = sysErr.Sanitize(tf)
		} else {
			newErr.Cause = errors.New(tf(e.Cause.Error()))
		}
	}
	return &newErr
}

// sanitizeIssue recursively applies the transformation function to all fields
// in an Issue and its nested causes.
func sanitizeIssue(i Issue, tf TransformFunc) Issue {
	i.Message = tf(i.Message)
	i.Code = tf(i.Code)
	i.Path = tf(i.Path)
	i.Description = tf(i.Description)

	// Recursively sanitize nested causes
	if i.Cause != nil {
		sanitizedCauses := make(Issues, len(*i.Cause))
		for idx, childIssue := range *i.Cause {
			sanitizedCauses[idx] = sanitizeIssue(childIssue, tf)
		}
		i.Cause = &sanitizedCauses
	}

	return i
}

// ToIssue converts a SystemError into an Issue. This is useful when you need to
// nest a SystemError as a cause within another Issue or SystemError.
//
// If the SystemError has multiple Issues, they are attached as the cause of the
// returned Issue. If the SystemError has a Cause, it is recursively converted and
// attached as nested Issue causes.
func (e *SystemError) ToIssue() Issue {
	if len(e.Issues) > 0 {
		return Issue{
			Code:     e.Code,
			Message:  e.Message,
			Path:     e.Path,
			Severity: e.Severity,
			Cause:    &e.Issues, // Attach sibling issues as the cause
		}
	}

	// Fallback for standard recursive errors
	iss := Issue{
		Code:     e.Code,
		Message:  e.Message,
		Path:     e.Path,
		Severity: e.Severity,
	}

	if e.Cause != nil {
		var next *SystemError
		if errors.As(e.Cause, &next) {
			child := next.ToIssue()
			iss.Cause = &Issues{child}
		} else {
			iss.Cause = &Issues{{Message: e.Cause.Error(), Code: "INTERNAL_ERROR"}}
		}
	}
	return iss
}

// ============================================================================
// Predefined Common Errors
// ============================================================================
//
// These are common error instances that can be used as-is or extended with
// additional context using the fluent API. They follow a consistent naming
// convention: Err<Domain><Condition>
//
// Example usage:
//
//	return ErrInputCannotBeNil.
//		WithPath("user").
//		WithOperation("CreateUser")

var (
	// ErrInputCannotBeNil indicates that a required input parameter was nil.
	ErrInputCannotBeNil = NewSystemError("ERR_COMMON_INPUT_CANNOT_BE_NIL").
				WithMessage("input record cannot be nil")

	// ErrInputCannotBeNilPointer indicates that a required input parameter was a nil pointer.
	ErrInputCannotBeNilPointer = NewSystemError("ERR_COMMON_INPUT_CANNOT_BE_NIL_POINTER").
					WithMessage("input record cannot be a nil pointer")

	// ErrNoMetadata indicates that expected metadata was not found.
	ErrNoMetadata = NewSystemError("ERR_COMMON_NO_METADATA").
			WithMessage("no metadata found")

	// ErrMetadataKeyNotFound indicates that a specific metadata key was not found.
	ErrMetadataKeyNotFound = NewSystemError("ERR_COMMON_METADATA_KEY_NOT_FOUND").
				WithMessage("metadata key not found")

	// ErrFailedToCalculateHash indicates that a hash calculation operation failed.
	ErrFailedToCalculateHash = NewSystemError("ERR_COMMON_FAILED_TO_CALCULATE_HASH").
					WithMessage("failed to calculate hash")

	// ErrHashMismatch indicates that a hash comparison failed (data integrity issue).
	ErrHashMismatch = NewSystemError("ERR_COMMON_HASH_MISMATCH").
			WithMessage("hash mismatch")
)
