package common

// Pre-defined errors for logical operator evaluation.
var (
	ErrEmptConditions    = NewSystemError("ERR_COMMON_EMPTY_CONDITIONS", "conditions slice cannot be empty")
	ErrInvalidOperator   = NewSystemError("ERR_COMMON_INVALID_OPERATOR",)
	ErrInvalidNotOperand = NewSystemError("ERR_COMMON_INVALID_NOT_OPERAND", "logical NOT requires exactly one result")
	ErrUnmarshalFailed   = NewSystemError("ERR_COMMON_UNMARSHAL_FAILED")
	ErrMarshalFailed   = NewSystemError("ERR_COMMON_MARSHAL_FAILED")
)
