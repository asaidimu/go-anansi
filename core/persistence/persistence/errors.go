package persistence

import (
	"fmt"
)

// PersistenceError represents errors specific to persistence operations.
type PersistenceError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *PersistenceError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s operation failed for key '%s': %s (caused by: %v)",
			e.Operation, e.Key, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s operation failed for key '%s': %s", e.Operation, e.Key, e.Message)
}

func (e *PersistenceError) Unwrap() error {
	return e.Cause
}
