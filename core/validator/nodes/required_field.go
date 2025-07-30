package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// RequiredFieldNode checks for the presence of a required field.
type RequiredFieldNode struct {
	BaseNode[map[string]any]
	fieldName string
}

func (n *RequiredFieldNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	parentPath := getScopedPath(n.path)
	ctx.Path = &parentPath

	return n.execute(ctx, false, func(dataMap map[string]any, _ reflect.Type) *types.NodeResult {
		fieldName := n.path[len(parentPath)+1:]
		if parentPath == "" {
			fieldName = n.path
		}
		if _, exists := dataMap[fieldName]; !exists {
			return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "REQUIRED_FIELD_MISSING", Message: fmt.Sprintf("Required field '%s' is missing", fieldName), Path: n.path}}}
		}
		return &types.NodeResult{Success: true}

	})
}
