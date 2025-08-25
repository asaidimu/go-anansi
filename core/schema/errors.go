package schema

import (
	"errors"
	"strings"
)

// SchemaError represents errors specific to schema operations.
type SchemaError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *SchemaError) Error() string {
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

func (e *SchemaError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the schema package.
var (
	ErrFieldTypeCannotHaveSchemaRef = errors.New("field of type cannot have a 'schema' reference")
	ErrFailedToUnmarshalSchema      = errors.New("failed to unmarshal FieldDefinition.Schema into expected types or generic any")
	ErrFieldTypeCannotHaveItemsType = errors.New("field of type cannot have an 'itemsType'")
	ErrNestedSchemaDefCannotHaveBothFieldsAndType = errors.New("NestedSchemaDefinition cannot have both 'fields' and 'type'")
	ErrFailedToUnmarshalNestedSchemaDefFields = errors.New("failed to unmarshal NestedSchemaDefinition.fields")
	ErrNestedSchemaDefMustContainFieldsOrType = errors.New("NestedSchemaDefinition must contain either 'fields' or 'type'")
	ErrUnknownSchemaChangeType = errors.New("unknown schema change type")
	ErrFailedToUnmarshalNestedSchemaDefSchema = errors.New("failed to unmarshal NestedSchemaDefinition.Schema")
)
