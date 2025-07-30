package nodes

import "github.com/asaidimu/go-anansi/v6/core/validator/types"

// NestedSchemaNode triggers validation of a nested schema.
type NestedSchemaNode struct {
	BaseNode[any]
	// This node is a marker; its dependencies trigger the actual nested validation.
}


func (n *NestedSchemaNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return &types.NodeResult{Success: true}
}

