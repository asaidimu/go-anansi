package schema

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
)

var (
	ErrSchemaViolation                            = common.NewSystemError("ERR_SCHEMA_VIOLATION", "schema violation")
	ErrSystemErrorDuringValidation                = common.NewSystemError("ERR_SCHEMA_SYSTEM_ERROR_DURING_VALIDATION", "system error during document validation")
	ErrFailedToResolvePhysicalName                = common.NewSystemError("ERR_SCHEMA_FAILED_TO_RESOLVE_PHYSICAL_NAME", "failed to resolve physical name")
	ErrInvalidOrMissingMetadataVersion            = common.NewSystemError("ERR_SCHEMA_INVALID_OR_MISSING_METADATA_VERSION", "invalid or missing version in metadata")
	ErrExplicitMetadataProjectionForbidden        = common.NewSystemError("ERR_SCHEMA_EXPLICIT_METADATA_PROJECTION_FORBIDDEN", "users must not explicitly include _metadata_ in projection")
	ErrGeneratingField                            = common.NewSystemError("ERR_SCHEMA_GENERATING_FIELD", "error generating field")
	ErrUnsupportedFieldType                       = common.NewSystemError("ERR_SCHEMA_UNSUPPORTED_FIELD_TYPE", "unsupported field type")
	ErrPrimitiveFieldSchemaReference              = common.NewSystemError("ERR_SCHEMA_PRIMITIVE_FIELD_SCHEMA_REFERENCE", "primitive field type cannot have schema references")
	ErrArraySetMissingItemsType                   = common.NewSystemError("ERR_SCHEMA_ARRAY_SET_MISSING_ITEMS_TYPE", "array/set field has schema reference but no ItemsType specified")
	ErrObjectFieldLiteralSchemaReference          = common.NewSystemError("ERR_SCHEMA_OBJECT_FIELD_LITERAL_SCHEMA_REFERENCE", "object field cannot reference literal nested schema - only structured schemas allowed")
	ErrUnknownNestedSchemaReference               = common.NewSystemError("ERR_SCHEMA_UNKNOWN_NESTED_SCHEMA_REFERENCE", "field references unknown nested schema")
	ErrUnknownSchemaChangeType                    = common.NewSystemError("ERR_SCHEMA_UNKNOWN_SCHEMA_CHANGE_TYPE", "unknown schema change type")
	ErrFieldTypeCannotHaveSchemaReference         = common.NewSystemError("ERR_SCHEMA_FIELD_TYPE_CANNOT_HAVE_SCHEMA_REFERENCE", "field of this type cannot have a 'schema' reference")
	ErrFailedToUnmarshalFieldSchema               = common.NewSystemError("ERR_SCHEMA_FAILED_TO_UNMARSHAL_FIELD_SCHEMA", "failed to unmarshal FieldDefinition.Schema")
	ErrFieldTypeCannotHaveItemsType               = common.NewSystemError("ERR_SCHEMA_FIELD_TYPE_CANNOT_HAVE_ITEMS_TYPE", "field of this type cannot have an 'itemsType'")
	ErrNestedSchemaFieldsAndTypeConflict          = common.NewSystemError("ERR_SCHEMA_NESTED_SCHEMA_FIELDS_AND_TYPE_CONFLICT", "NestedSchemaDefinition cannot have both 'fields' and 'type'")
	ErrFailedToUnmarshalNestedSchemaFields        = common.NewSystemError("ERR_SCHEMA_FAILED_TO_UNMARSHAL_NESTED_SCHEMA_FIELDS", "failed to unmarshal NestedSchemaDefinition.fields")
	ErrNestedSchemaMissingFieldsOrType            = common.NewSystemError("ERR_SCHEMA_NESTED_SCHEMA_MISSING_FIELDS_OR_TYPE", "NestedSchemaDefinition must contain either 'fields' or 'type'")
	ErrFailedToCloneSchema                        = common.NewSystemError("ERR_SCHEMA_FAILED_TO_CLONE_SCHEMA", "failed to clone schema")
	ErrFieldAlreadyExists                         = common.NewSystemError("ERR_SCHEMA_FIELD_ALREADY_EXISTS", "field already exists in schema")
	ErrInvalidSchema                              = common.NewSystemError("ERR_SCHEMA_INVALID_SCHEMA", "invalid schema")
	ErrPhysicalNameResolverNotSet                 = common.NewSystemError("ERR_SCHEMA_PHYSICAL_NAME_RESOLVER_NOT_SET", "physical name resolver function is not set")
	ErrFailedToUnmarshalSchema                    = common.NewSystemError("ERR_SCHEMA_FAILED_TO_UNMARSHAL_SCHEMA", "failed to unmarshal schema")
	ErrNestedSchemaDefCannotHaveBothFieldsAndType = common.NewSystemError("ERR_SCHEMA_NESTED_SCHEMA_DEF_CANNOT_HAVE_BOTH_FIELDS_AND_TYPE", "NestedSchemaDefinition cannot have both 'fields' and 'type'")
	ErrFailedToUnmarshalNestedSchemaDefFields     = common.NewSystemError("ERR_SCHEMA_FAILED_TO_UNMARSHAL_NESTED_SCHEMA_DEF_FIELDS", "failed to unmarshal NestedSchemaDefinition.fields")
	ErrFailedToUnmarshalNestedSchemaDefSchema     = common.NewSystemError("ERR_SCHEMA_FAILED_TO_UNMARSHAL_NESTED_SCHEMA_DEF_SCHEMA", "failed to unmarshal NestedSchemaDefinition.schema")
	ErrValidatorSchemaValidationFailed            = common.NewSystemError("ERR_SCHEMA_VALIDATOR_SCHEMA_VALIDATION_FAILED", "schema validation failed during validator creation")
	ErrValidatorCircularDependency                = common.NewSystemError("ERR_SCHEMA_VALIDATOR_CIRCULAR_DEPENDENCY", "circular dependency detected in validation graph")
	ErrValidatorCreationFailed                    = common.NewSystemError("ERR_SCHEMA_VALIDATOR_CREATION_FAILED", "failed to create nested validator")
	ErrSchemaEmptyInput                           = common.NewSystemError("ERR_SCHEMA_EMPTY_INPUT", "schema definition cannot be empty")
	ErrSchemaMetaSchemaReadFailed                 = common.NewSystemError("ERR_SCHEMA_META_SCHEMA_READ_FAILED", "failed to read embedded meta-schema file: definition.json")
	ErrSchemaMetaSchemaCompileFailed              = common.NewSystemError("ERR_SCHEMA_META_SCHEMA_COMPILE_FAILED", "failed to compile meta-schema")
	ErrSchemaInternalInitFailed                   = common.NewSystemError("ERR_SCHEMA_INTERNAL_INIT_FAILED", "schema system initialization failed")
	ErrSchemaValidationFailed                     = common.NewSystemError("ERR_SCHEMA_VALIDATION_FAILED", "schema definition does not match required structure")
	ErrSchemaUnmarshalFailed                      = common.NewSystemError("ERR_SCHEMA_UNMARSHAL_FAILED", "failed to parse schema definition")

	ErrFieldIDEmpty = common.NewSystemError("ERR_SCHEMA_FIELD_ID_EMPTY", "field ID cannot be empty")

	ErrConditionalFieldsEmpty                  = common.NewSystemError("ERR_SCHEMA_CONDITIONAL_FIELDS_EMPTY", "conditional field set must define fields")
	ErrConditionalWhenFieldEmpty               = common.NewSystemError("ERR_SCHEMA_CONDITIONAL_WHEN_FIELD_EMPTY", "conditional field 'when.field' cannot be empty")
	ErrConditionalWhenFieldInvalidIdentifier   = common.NewSystemError("ERR_SCHEMA_CONDITIONAL_WHEN_FIELD_INVALID_IDENTIFIER", "conditional field 'when.field' must be a valid identifier")
	ErrConditionalWhenFieldNotFound            = common.NewSystemError("ERR_SCHEMA_CONDITIONAL_WHEN_FIELD_NOT_FOUND", "conditional field 'when.field' references non-existent parent field")
	ErrConditionalWhenFieldSelfReference       = common.NewSystemError("ERR_SCHEMA_CONDITIONAL_WHEN_FIELD_SELF_REFERENCE", "conditional field 'when.field' must not reference a field defined within the conditional field set itself")
	ErrConditionalWhenValueTypeMismatch        = common.NewSystemError("ERR_SCHEMA_CONDITIONAL_WHEN_VALUE_TYPE_MISMATCH", "conditional field 'when.value' type is incompatible with parent field's type")
	ErrConditionalFieldRedefinesBaseField      = common.NewSystemError("ERR_SCHEMA_CONDITIONAL_FIELD_REDEFINES_BASE_FIELD", "conditional field cannot redefine a field already present in the NestedSchemaDefinition's base fields")
)


// NewFieldNotFoundError creates an error for when a field is not found
func NewFieldNotFoundError(id string) *common.SystemError {
	return common.NewSystemError("ERR_FIELD_NOT_FOUND").
		WithMessagef("field with ID '%s' not found", id)
}

// NewFieldNameNotFoundError creates an error for when a field name is not found
func NewFieldNameNotFoundError(name string) *common.SystemError {
	return common.NewSystemError("ERR_FIELD_NAME_NOT_FOUND").
		WithMessagef("field with name '%s' not found", name)
}

// NewFieldAlreadyExistsError creates an error for when a field already exists
func NewFieldAlreadyExistsError(id string) *common.SystemError {
	return common.NewSystemError("ERR_FIELD_ALREADY_EXISTS").
		WithMessagef("field with ID '%s' already exists", id)
}

// NewFieldNameAlreadyExistsError creates an error for when a field name already exists
func NewFieldNameAlreadyExistsError(name string) *common.SystemError {
	return common.NewSystemError("ERR_FIELD_NAME_ALREADY_EXISTS").
		WithMessagef("field with name '%s' already exists", name)
}

// NewIndexNotFoundError creates an error for when an index is not found
func NewIndexNotFoundError(name string) *common.SystemError {
	return common.NewSystemError("ERR_INDEX_NOT_FOUND").
		WithMessagef("index with name '%s' not found", name)
}

// NewIndexAlreadyExistsError creates an error for when an index already exists
func NewIndexAlreadyExistsError(name string) *common.SystemError {
	return common.NewSystemError("ERR_INDEX_ALREADY_EXISTS").
		WithMessagef("index with name '%s' already exists", name)
}

// NewConstraintNotFoundError creates an error for when a constraint is not found
func NewConstraintNotFoundError(name string) *common.SystemError {
	return common.NewSystemError("ERR_CONSTRAINT_NOT_FOUND").
		WithMessagef("constraint with name '%s' not found", name)
}

// NewConstraintAlreadyExistsError creates an error for when a constraint already exists
func NewConstraintAlreadyExistsError(name string) *common.SystemError {
	return common.NewSystemError("ERR_CONSTRAINT_ALREADY_EXISTS").
		WithMessagef("constraint with name '%s' already exists", name)
}

// NewNestedSchemaNotFoundError creates an error for when a nested schema is not found
func NewNestedSchemaNotFoundError(id string) *common.SystemError {
	return common.NewSystemError("ERR_NESTED_SCHEMA_NOT_FOUND").
		WithMessagef("nested schema with ID '%s' not found", id)
}

// NewNestedSchemaNameNotFoundError creates an error for when a nested schema name is not found
func NewNestedSchemaNameNotFoundError(name string) *common.SystemError {
	return common.NewSystemError("ERR_NESTED_SCHEMA_NAME_NOT_FOUND").
		WithMessagef("nested schema with name '%s' not found", name)
}

// NewNestedSchemaAlreadyExistsError creates an error for when a nested schema already exists
func NewNestedSchemaAlreadyExistsError(id string) *common.SystemError {
	return common.NewSystemError("ERR_NESTED_SCHEMA_ALREADY_EXISTS").
		WithMessagef("nested schema with ID '%s' already exists", id)
}
