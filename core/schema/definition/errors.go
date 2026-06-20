package definition

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
)

var (
	ErrMarshalFailed       = common.NewSystemError("ERR_SCHEMA_MARSHAL_FAILED", "failed to marshal the provided schema component")
	ErrUnmarshalFailed     = common.NewSystemError("ERR_SCHEMA_UNMARSHAL_FAILED", "failed to unmarshal the provided schema component")
	ErrInvalidLiteralValue = common.NewSystemError("ERR_INVALID_LITERAL_VALUE", "invalid literal value")
	ErrInvalidContraint    = common.NewSystemError("ERR_INVALID_CONSTRAINT", "invalid constraint definition")
	ErrTypeMismatch        = common.NewSystemError("ERR_TYPE_MISMATCH", "type mismatch encountered")
	ErrInvalidFieldType    = common.NewSystemError("ERR_SCHEMA_INVALID_FIELD_TYPE", "unsupported field type")
	ErrSchemaNotFound      = common.NewSystemError("ERR_SCHEMA_FAILED_TO_RESOLVE_NESTED_SCHEMA", "failed to resolve nested schema")
	ErrFieldNotFound       = common.NewSystemError("ERR_SCHEMA_FAILED_TO_RESOLVE_FIELD", "failed to resolve a field")

	ErrInvalidSchema = common.NewSystemError("ERR_SCHEMA_INVALID_SCHEMA", "invalid schema")
	ErrValidationCircularDependency = common.NewSystemError("ERR_VALIDATION_CIRCULAR_DEPENDENCY", "circular dependency detected in validation graph")
)
