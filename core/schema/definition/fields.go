package definition

import (
	"encoding/json"
)

// FieldName represents the name of a field in a schema.
type FieldName string

// FieldId represents the id of a field in a schema.
// IT IS A UUID v7 AND IS DISTINCT FROM NAME
type FieldId string

type FieldProperties struct {
	Default LiteralValue         `json:"default"`
	Schema  FieldSchemaReference `json:"schema"`
	Type    FieldType            `json:"type,omitempty"`
}

// Field defines a field within a schema.
type Field struct {
	Name        FieldName      `json:"name"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Deprecated  bool           `json:"deprecated,omitempty"`
	Unique      bool           `json:"unique,omitempty"`
	Nullable    *bool          `json:"nullable,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`

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

// ResolvedNullable returns the effective nullable value.
// Absent (nil) defaults to true for backward compatibility.
func (f *Field) ResolvedNullable() bool {
	return f.Nullable == nil || *f.Nullable
}

// BoolPtr returns a pointer to the given bool value.
func BoolPtr(b bool) *bool { return &b }

// cloneBoolPtr returns a deep copy of a *bool.
func cloneBoolPtr(p *bool) *bool {
	if p == nil {
		return nil
	}
	b := *p
	return &b
}
