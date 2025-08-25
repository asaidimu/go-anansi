package ephemeral

import (
	"errors"
	"fmt"
)

// EphemeralError represents errors specific to ephemeral operations.
type EphemeralError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *EphemeralError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s operation failed for key '%s': %s (caused by: %v)",
			e.Operation, e.Key, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s operation failed for key '%s': %s", e.Operation, e.Key, e.Message)
}

func (e *EphemeralError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the ephemeral package.
var (
	ErrCollectionNotFound = errors.New("collection not found")
)
