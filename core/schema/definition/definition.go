package definition

import (
	"encoding/json"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

type BaseSchema struct {
	Name        string                      `json:"name,omitempty"`
	Description string                      `json:"description,omitempty"`
	Fields      map[FieldId]Field           `json:"fields,omitempty"`
	Indexes     map[IndexId]Index           `json:"indexes,omitempty"`
	Constraints map[ConstraintId]Constraint `json:"constraints,omitempty"`
	Metadata    map[string]any              `json:"metadata,omitempty"`
}

type NestedSchemaMode byte

const (
	NestedSchemaModeField NestedSchemaMode = iota + 1
	NestedSchemaModeSchema
)

type NestedSchema struct {
	// The shema part
	BaseSchema

	// The field definition part
	FieldProperties

	Values   []LiteralValue `json:"values,omitempty"`
	Concrete bool           `json:"concrete,omitempty"`
}

func (ns NestedSchema) MarshalJSON() ([]byte, error) {
	type Alias NestedSchema

	proxy := struct {
		Alias
		Default *LiteralValue         `json:"default,omitempty"`
		Schema  *FieldSchemaReference `json:"schema,omitempty"`
	}{
		Alias: Alias(ns),
	}

	if !ns.Default.IsZero() && !ns.Default.IsNull() {
		proxy.Default = &ns.FieldProperties.Default
	}

	if !ns.Schema.IsZero() {
		proxy.Schema = &ns.FieldProperties.Schema
	}

	return json.Marshal(proxy)
}

type Schema struct {
	Version *common.Version `json:"version"`
	BaseSchema
	Schemas map[SchemaId]NestedSchema `json:"schemas,omitempty"`
}

// Helper function to check if schema effectively represents an object
func (schema NestedSchema) isEffectivelyObject(parentSchema *Schema, depth ...int) bool {
	d := 0
	if len(depth) > 0 {
		d = depth[0]
	}
	if d > 500 {
		return false
	}

	// Schema mode: has Fields
	if len(schema.Fields) > 0 {
		return true
	}

	// Type mode: check the type
	switch schema.Type {
	case FieldTypeObject, FieldTypeRecord:
		return true
	case FieldTypeComposite:
		if schema.Schema.IsZero() || !schema.Schema.IsMultiple() {
			return false
		}

		refs, err := FieldSchemaAs[[]SchemaReference](schema.Schema)
		if err != nil {
			return false
		}
		for _, componentRef := range refs {
			componentSchema, exists := parentSchema.Schemas[componentRef.ID]
			if !exists {
				return false
			}
			if !componentSchema.isEffectivelyObject(parentSchema, d+1) {
				return false
			}
		}
		return true

	case FieldTypeUnion:
		if schema.Schema.IsZero() || !schema.Schema.IsMultiple() {
			return false
		}

		refs, err := FieldSchemaAs[[]SchemaReference](schema.Schema)
		if err != nil {
			return false
		}
		for _, variantRef := range refs {
			variantSchema, exists := parentSchema.Schemas[variantRef.ID]
			if !exists || !variantSchema.isEffectivelyObject(parentSchema, d+1) {
				return false
			}
		}
		return true

	default:
		return false
	}
}
