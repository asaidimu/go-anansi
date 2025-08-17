package data

import (
	"errors"
	"fmt"
)

// DocumentError represents errors specific to document operations.
type DocumentError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *DocumentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s operation failed for key '%s': %s (caused by: %v)",
			e.Operation, e.Key, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s operation failed for key '%s': %s", e.Operation, e.Key, e.Message)
}

func (e *DocumentError) Unwrap() error {
	return e.Cause
}

// Common errors
var (
	ErrKeyNotFound             = errors.New("key not found")
	ErrTypeMismatch            = errors.New("type mismatch")
	ErrInvalidPath             = errors.New("invalid path")
	ErrSchemaViolation         = errors.New("schema violation")
	ErrInvalidQuery            = errors.New("invalid query")
	ErrFailedToUnmarshalJSON   = errors.New("failed to unmarshal JSON")
	ErrKeyEmpty                = errors.New("key cannot be empty")
	ErrTypeConversion          = errors.New("type conversion failed") // General for type coercion issues
	ErrPathSegmentNotFound     = errors.New("path segment not found")
	ErrCannotTraverse          = errors.New("cannot traverse into non-document type")
	ErrParentNotMap            = errors.New("parent is not a map")
	ErrNoMetadata              = errors.New("no metadata found")
	ErrMetadataValueCoercion   = errors.New("metadata value cannot be coerced")
	ErrMetadataKeyNotFound     = errors.New("metadata key not found")
	ErrFailedToMarshalStruct   = errors.New("failed to marshal struct")
	ErrFailedToMarshalJSON     = errors.New("failed to marshal to JSON")
	ErrFailedToUnmarshalStruct = errors.New("failed to unmarshal to struct")
	ErrMetadataProviderFailed  = errors.New("metadata provider failed")
	ErrConflictingMetadataField = errors.New("conflicting metadata field")
	ErrFailedToMarshalMetadata = errors.New("failed to marshal metadata to JSON")
	ErrInvalidTargetType       = errors.New("invalid target type")
	ErrRequiredFieldNotFound   = errors.New("required field not found")
	ErrFailedToSetField        = errors.New("failed to set field")
	ErrTypeConversionFailed    = errors.New("type conversion failed") // More specific than ErrTypeConversion
)
