package utils

import (
	"errors"
	"fmt"
)

// UtilityError represents errors specific to utility operations.
type UtilityError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *UtilityError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s operation failed for key '%s': %s (caused by: %v)",
			e.Operation, e.Key, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s operation failed for key '%s': %s", e.Operation, e.Key, e.Message)
}

func (e *UtilityError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the utils package.
var (
	ErrUnmarshalJSON = errors.New("error unmarshaling JSON")
	ErrMarshalJSON   = errors.New("error marshaling to JSON")
	ErrInputNil      = errors.New("input record cannot be nil")
	ErrInputNilPointer = errors.New("input record cannot be a nil pointer to a struct")
	ErrInputNotStruct = errors.New("input record must be a struct or a pointer to a struct")
	ErrMapToStructInputNil = errors.New("MapToStruct: input map cannot be nil")
	ErrMapToStructTargetNotStruct = errors.New("MapToStruct: generic type T must be a struct type (or pointer to struct)")
)
