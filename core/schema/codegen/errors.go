package codegen

import (
	"errors"
	"strings"
)

// CodegenError represents errors specific to code generation operations.
type CodegenError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *CodegenError) Error() string {
	var b strings.Builder
	b.WriteString(e.Operation)
	b.WriteString(" operation failed")

	if e.Key != "" {
		b.WriteString(" for key '")
		b.WriteString(e.Key)
		b.WriteString("': ")
	} else {
		b.WriteString(": ")
	}
	b.WriteString(e.Message)

	if e.Cause != nil {
		b.WriteString(" (caused by: ")
		b.WriteString(e.Cause.Error())
		b.WriteString(")")
	}
	return b.String()
}

func (e *CodegenError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the codegen package.
var (
	ErrSchemaValidationFailed = errors.New("schema validation failed")
	ErrUnsupportedFieldType   = errors.New("unsupported field type")
	ErrPrimitiveFieldHasSchemaRef = errors.New("primitive field type cannot have schema references")
	ErrArraySetNoItemsType    = errors.New("array/set field has schema reference but no ItemsType specified")
	ErrObjectReferencesLiteralSchema = errors.New("object field cannot reference literal nested schema - only structured schemas allowed")
	ErrUnknownNestedSchema    = errors.New("references unknown nested schema")
	ErrUnsupportedItemsType   = errors.New("unsupported ItemsType")
	ErrFailedToApplyProjectionToNestedSchema = errors.New("failed to apply projection to nested schema")
	ErrNestedSchemaNoStructuredFields = errors.New("nested schema has no structured fields to project")
	ErrFailedToApplyRecursiveNestedExclusion = errors.New("failed to apply recursive nested exclusion")
	ErrFailedToApplyRecursiveNestedProjection = errors.New("failed to apply recursive nested projection")
	ErrFieldInProjectionIncludeDoesNotExist = errors.New("field specified in projection include does not exist")
	ErrComputedFieldConflictsWithExistingField = errors.New("computed field conflicts with existing field")
	ErrFailedToParseSchemaJSON = errors.New("failed to parse schema JSON")
)
