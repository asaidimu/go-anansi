package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

type ConditionalRequiredFieldNode struct {
	BaseNode[map[string]any]
	fieldName string
	condition *schema.FieldInclusionCondition
}

func (n *ConditionalRequiredFieldNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	parentPath := getScopedPath(n.path)
	ctx.Path = &parentPath

	return n.execute(ctx, false, func(dataMap map[string]any, _ reflect.Type) *types.NodeResult {
		// Check if the condition is met
		conditionMet := n.condition.Evaluate(dataMap)

		if conditionMet {
			// Condition met, field is required
			fieldName := n.path[len(parentPath)+1:]
			if parentPath == "" {
				fieldName = n.path
			}

			if _, exists := dataMap[fieldName]; !exists {
				return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "REQUIRED_FIELD_MISSING", Message: fmt.Sprintf("Required field '%s' is missing", fieldName), Path: n.path}}}
			}
		}

		// Either condition not met (field optional) or field present when required
		return &types.NodeResult{Success: true}
	})
}
