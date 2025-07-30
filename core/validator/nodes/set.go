package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// SetValidationNode validates uniqueness in a set.
type SetValidationNode struct {
	BaseNode[[]any]
}

func (n *SetValidationNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, false, func(items []any, dataType reflect.Type) *types.NodeResult {
		seen := make(map[string]bool)
		for i, item := range items {
			key := fmt.Sprintf("%v", item)
			if seen[key] {
				return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "SET_DUPLICATE", Message: fmt.Sprintf("Duplicate value found in set at index %d", i), Path: n.path}}}
			}
			seen[key] = true
		}
		return &types.NodeResult{Success: true}
	})
}
