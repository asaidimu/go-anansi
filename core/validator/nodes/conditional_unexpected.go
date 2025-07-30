package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// Add this new node type for conditional unexpected fields checking
type ConditionalUnexpectedFieldsNode struct {
	BaseNode[map[string]any]
	conditionalFields map[string]*schema.FieldInclusionCondition // field -> condition
	baseFields        map[string]bool                            // unconditional fields
}

// Execute method for ConditionalUnexpectedFieldsNode
func (n *ConditionalUnexpectedFieldsNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, false, func(dataMap map[string]any, _ reflect.Type) *types.NodeResult {
		var issues []types.Issue
		// Check each field in the data
		for fieldName := range dataMap {
			isExpected := false

			// Check if it's a base field (always allowed)
			if n.baseFields[fieldName] {
				isExpected = true
			} else {
				// Check if it's a conditional field whose condition is met
				if condition, exists := n.conditionalFields[fieldName]; exists {

					if condition.Evaluate(dataMap) {
						isExpected = true
					}
				}
			}

			if !isExpected {
				issues = append(issues, types.Issue{
					Code:    "UNEXPECTED_FIELD",
					Message: fmt.Sprintf("Unexpected field '%s'", fieldName),
					Path:    n.buildPath(fieldName),
				})
			}
		}

		return &types.NodeResult{Success: len(issues) == 0, Issues: issues}
	})
}
