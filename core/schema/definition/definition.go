package definition

import "github.com/asaidimu/go-anansi/v6/core/common"

type BaseSchema struct {
	Name        string                      `json:"name"`
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
	Values []LiteralValue `json:"values,omitempty"`
}

type Schema struct {
	BaseSchema
	Version common.Version            `json:"version"`
	Schemas map[SchemaId]NestedSchema `json:"schema,omitempty"`
}

func (n NestedSchema) Mode() NestedSchemaMode {
	if n.BaseSchema.Fields != nil {
		return NestedSchemaModeSchema
	}
	return NestedSchemaModeField
}
