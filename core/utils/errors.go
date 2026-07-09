package utils

import "github.com/asaidimu/go-anansi/v8/core/common"

// Pre-defined errors for the utils package.
var (
	ErrUnmarshalJSON              = common.NewSystemError("ERROR_UNMARSHALJSON", "error unmarshaling JSON")
	ErrMarshalJSON                = common.NewSystemError("ERROR_MARSHALJSON", "error marshaling to JSON")
	ErrInputNil                   = common.NewSystemError("ERROR_INPUTNIL", "input record cannot be nil")
	ErrInputNilPointer            = common.NewSystemError("ERROR_INPUTNILPOINTER", "input record cannot be a nil pointer to a struct")
	ErrInputNotStruct             = common.NewSystemError("ERROR_INPUTNOTSTRUCT", "input record must be a struct or a pointer to a struct")
	ErrMapToStructInputNil        = common.NewSystemError("ERROR_MAPTOSTRUCTINPUTNIL", "MapToStruct: input map cannot be nil")
	ErrMapToStructTargetNotStruct = common.NewSystemError("ERROR_MAP_TO_STRUCT_TARGETNOTSTRUCT", "MapToStruct: generic type T must be a struct type (or pointer to struct)")
)
