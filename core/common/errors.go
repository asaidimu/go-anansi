package common

import "errors"

// Errors for common package
var (
	ErrInputCannotBeNil             = errors.New("input record cannot be nil")
	ErrInputCannotBeNilPointer      = errors.New("input record cannot be a nil pointer to a struct")
	ErrInputMustBeStruct            = errors.New("input record must be a struct or a pointer to a struct")
	ErrFailedToMarshalInput         = errors.New("failed to marshal input record to JSON")
	ErrFailedToUnmarshalInput       = errors.New("failed to unmarshal JSON to map[string]any")
	ErrMapToStructInputCannotBeNil  = errors.New("MapToStruct: input map cannot be nil")
	ErrMapToStructTargetNotStruct   = errors.New("MapToStruct: generic type T must be a struct type (or pointer to struct)")
	ErrMapToStructFailedToMarshal   = errors.New("MapToStruct: failed to marshal input map to JSON")
	ErrMapToStructFailedToUnmarshal = errors.New("MapToStruct: failed to unmarshal JSON to target struct")
	ErrNoMetadata                   = errors.New("no metadata found")
	ErrMetadataKeyNotFound          = errors.New("metadata key not found")
	ErrFailedToCalculateHash        = errors.New("failed to calculate hash")
	ErrHashMismatch                 = errors.New("hash mismatch")
)
