package definition

import (
	"encoding/json"
)

// FieldName represents the name of a field in a schema.
type FieldName string

// FieldId represents the id of a field in a schema.
type FieldId string

type FieldProperties struct {
	Default LiteralValue         `json:"default"`
	Schema  FieldSchemaReference `json:"schema"`
	Type    FieldType            `json:"type,omitempty"`
}

// Field defines a field within a schema.
type Field struct {
	Name        FieldName `json:"name"`
	Description string    `json:"description,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Deprecated  bool      `json:"deprecated,omitempty"`
	Unique      bool      `json:"unique,omitempty"`

	FieldProperties
}

func (f Field) MarshalJSON() ([]byte, error) {
	type Alias Field

	proxy := struct {
		Alias
		Default *LiteralValue         `json:"default,omitempty"`
		Schema  *FieldSchemaReference `json:"schema,omitempty"`
	}{
		Alias: Alias(f),
	}

	if !f.Default.IsZero() && !f.Default.IsNull() {
		proxy.Default = &f.FieldProperties.Default
	}

	if !f.Schema.IsZero() {
		proxy.Schema = &f.FieldProperties.Schema
	}

	return json.Marshal(proxy)
}
