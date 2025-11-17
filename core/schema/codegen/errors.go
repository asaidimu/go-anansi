package codegen

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
)


// CodegenError was a custom error type for code generation operations, now replaced by common.SystemError.

// Pre-defined errors for the codegen package.
var (
	ErrSchemaValidationFailed = common.NewSystemError("ERR_CODEGEN_SCHEMA_VALIDATION_FAILED", "schema validation failed")
	ErrPrimitiveFieldHasSchemaRef = common.NewSystemError("ERR_CODEGEN_PRIMITIVE_FIELD_HAS_SCHEMA_REF", "primitive field type cannot have schema references")
	ErrArraySetNoItemsType    = common.NewSystemError("ERR_CODEGEN_ARRAY_SET_NO_ITEMS_TYPE", "array/set field has schema reference but no ItemsType specified")
	ErrObjectReferencesLiteralSchema = common.NewSystemError("ERR_CODEGEN_OBJECT_REFERENCES_LITERAL_SCHEMA", "object field cannot reference literal nested schema - only structured schemas allowed")
	ErrUnknownNestedSchema    = common.NewSystemError("ERR_CODEGEN_UNKNOWN_NESTED_SCHEMA", "references unknown nested schema")
	ErrUnsupportedItemsType   = common.NewSystemError("ERR_CODEGEN_UNSUPPORTED_ITEMS_TYPE", "unsupported ItemsType")
	ErrFailedToApplyProjectionToNestedSchema = common.NewSystemError("ERR_CODEGEN_FAILED_TO_APPLY_PROJECTION_TO_NESTED_SCHEMA", "failed to apply projection to nested schema")
	ErrNestedSchemaNoStructuredFields = common.NewSystemError("ERR_CODEGEN_NESTED_SCHEMA_NO_STRUCTURED_FIELDS", "nested schema has no structured fields to project")
	ErrFailedToApplyRecursiveNestedExclusion = common.NewSystemError("ERR_CODEGEN_FAILED_TO_APPLY_RECURSIVE_NESTED_EXCLUSION", "failed to apply recursive nested exclusion")
	ErrFailedToApplyRecursiveNestedProjection = common.NewSystemError("ERR_CODEGEN_FAILED_TO_APPLY_RECURSIVE_NESTED_PROJECTION", "failed to apply recursive nested projection")
	ErrFieldInProjectionIncludeDoesNotExist = common.NewSystemError("ERR_CODEGEN_FIELD_IN_PROJECTION_INCLUDE_DOES_NOT_EXIST", "field specified in projection include does not exist")
	ErrComputedFieldConflictsWithExistingField = common.NewSystemError("ERR_CODEGEN_COMPUTED_FIELD_CONFLICTS_WITH_EXISTING_FIELD", "computed field conflicts with existing field")
	ErrFailedToParseSchemaJSON = common.NewSystemError("ERR_CODEGEN_FAILED_TO_PARSE_SCHEMA_JSON", "failed to parse schema JSON")
)
