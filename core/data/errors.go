package data

import (
	"errors"
	"fmt"
)

// DocumentError represents errors specific to document operations.
type DocumentError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *DocumentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s operation failed for key '%s': %s (caused by: %v)",
			e.Operation, e.Key, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s operation failed for key '%s': %s", e.Operation, e.Key, e.Message)
}

func (e *DocumentError) Unwrap() error {
	return e.Cause
}

// Common errors
var (
	ErrKeyNotFound             = errors.New("key not found")
	ErrTypeMismatch            = errors.New("type mismatch")
	ErrInvalidPath             = errors.New("invalid path")
	ErrSchemaViolation         = errors.New("schema violation")
	ErrInvalidQuery            = errors.New("invalid query")
	ErrFailedToUnmarshalJSON   = errors.New("failed to unmarshal JSON")
	ErrKeyEmpty                = errors.New("key cannot be empty")
	ErrReadOnlyField           = errors.New("field is read-only")
	ErrTypeConversion          = errors.New("type conversion failed") // General for type coercion issues
	ErrPathSegmentNotFound     = errors.New("path segment not found")
	ErrCannotTraverse          = errors.New("cannot traverse into non-document type")
	ErrParentNotMap            = errors.New("parent is not a map")
	ErrNoMetadata              = errors.New("no metadata found")
	ErrMetadataValueCoercion   = errors.New("metadata value cannot be coerced")
	ErrMetadataKeyNotFound     = errors.New("metadata key not found")
	ErrFailedToMarshalStruct   = errors.New("failed to marshal struct")
	ErrFailedToMarshalJSON     = errors.New("failed to marshal to JSON")
	ErrFailedToUnmarshalStruct = errors.New("failed to unmarshal to struct")
	ErrMetadataProviderFailed  = errors.New("metadata provider failed")
	ErrConflictingMetadataField = errors.New("conflicting metadata field")
	ErrFailedToMarshalMetadata = errors.New("failed to marshal metadata to JSON")
	ErrInvalidTargetType       = errors.New("invalid target type")
	ErrRequiredFieldNotFound   = errors.New("required field not found")
	ErrFailedToSetField        = errors.New("failed to set field")
	ErrTypeConversionFailed    = errors.New("type conversion failed") // More specific than ErrTypeConversion

	ErrFactoryAlreadyConfigured          = errors.New("document factory already configured")
	ErrConfigurationNotApplied           = errors.New("configuration was not applied")
	ErrFactoryNotConfigured              = errors.New("document factory not configured")
	ErrFailedToCalculateHash             = errors.New("failed to calculate hash")
	ErrHashMismatch                      = errors.New("hash mismatch")
	ErrSignatureInvalid 				 = errors.New("Invalid signature")
	ErrInvalidJSONPathSyntax             = errors.New("invalid JSONPath syntax")
	ErrNoNumericValuesForAggregation     = errors.New("no numeric values found for aggregation")
	ErrSystemErrorDuringValidation       = errors.New("system error during document validation")
	ErrPhysicalNameResolverNotSet        = errors.New("physical name resolver function is not set")
	ErrFailedToResolvePhysicalName       = errors.New("failed to resolve physical name")
	ErrInvalidOrMissingMetadataVersion   = errors.New("invalid or missing version in metadata")
	ErrExplicitMetadataProjectionForbidden = errors.New("users must not explicitly include _metadata_ in projection")
	ErrGeneratingField                   = errors.New("error generating field")
	ErrUnsupportedFieldType              = errors.New("unsupported field type")
	ErrPrimitiveFieldSchemaReference     = errors.New("primitive field type cannot have schema references")
	ErrArraySetMissingItemsType          = errors.New("array/set field has schema reference but no ItemsType specified")
	ErrObjectFieldLiteralSchemaReference = errors.New("object field cannot reference literal nested schema - only structured schemas allowed")
	ErrUnknownNestedSchemaReference      = errors.New("field references unknown nested schema")
	ErrUnknownSchemaChangeType           = errors.New("unknown schema change type")
	ErrFieldTypeCannotHaveSchemaReference = errors.New("field of this type cannot have a 'schema' reference")
	ErrFailedToUnmarshalFieldSchema      = errors.New("failed to unmarshal FieldDefinition.Schema")
	ErrFieldTypeCannotHaveItemsType      = errors.New("field of this type cannot have an 'itemsType'")
	ErrNestedSchemaFieldsAndTypeConflict = errors.New("NestedSchemaDefinition cannot have both 'fields' and 'type'")
	ErrFailedToUnmarshalNestedSchemaFields = errors.New("failed to unmarshal NestedSchemaDefinition.fields")
	ErrNestedSchemaMissingFieldsOrType   = errors.New("NestedSchemaDefinition must contain either 'fields' or 'type'")
	ErrFailedToCloneSchema               = errors.New("failed to clone schema")
	ErrFieldAlreadyExists                = errors.New("field already exists in schema")
	ErrErrorAfterScanningRows            = errors.New("error after scanning rows")
	ErrFailedToGetColumns                = errors.New("failed to get columns")
	ErrFailedToScanRow                   = errors.New("failed to scan row")
	ErrFailedToCreateInteractorForTransaction = errors.New("failed to create new interactor for transaction")
)
