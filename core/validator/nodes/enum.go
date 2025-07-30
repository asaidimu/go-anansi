package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// EnumValidationNode validates that a field's value is one of the allowed enum values.
type EnumValidationNode struct {
	BaseNode[any]
	fieldDef *schema.FieldDefinition // FieldDef contains the Values
}

func (n *EnumValidationNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, false, func(value any, _ reflect.Type) *types.NodeResult {
		for _, allowedValue := range n.fieldDef.Values {
			if reflect.DeepEqual(value, allowedValue) {
				return &types.NodeResult{Success: true}
			}
		}
		return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "ENUM_VIOLATION", Message: fmt.Sprintf("Value must be one of: %v", n.fieldDef.Values), Path: n.path}}}
	})
}
