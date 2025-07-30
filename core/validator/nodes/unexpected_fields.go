package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// UnexpectedFieldsNode checks for fields not defined in the schema.
type UnexpectedFieldsNode struct {
	BaseNode[map[string]any]
	expectedFields map[string]bool
}

func (n *UnexpectedFieldsNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, false, func(data map[string]any, _ reflect.Type) *types.NodeResult {
		var issues []types.Issue
		for key := range data {
			if !n.expectedFields[key] {
				issues = append(issues, types.Issue{Code: "UNEXPECTED_FIELD", Message: fmt.Sprintf("Unexpected field '%s'", key), Path: utils.BuildPath(n.path, key)})
			}
		}
		return &types.NodeResult{Success: len(issues) == 0, Issues: issues}
	})
}
