package collection

import (
	"fmt"
)

// CollectionError represents errors specific to collection operations.
type CollectionError struct {
	Operation string
	Message   string
	Cause     error
}

func (e *CollectionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s operation failed %s (caused by: %v)",
			e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s operation failed %s", e.Operation,  e.Message)
}

func (e *CollectionError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the collection package.
var (
	// Add any specific collection errors here if needed
)
