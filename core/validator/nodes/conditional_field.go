package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

type ConditionalFieldNode struct {
	BaseNode[map[string]any]
	condition *schema.FieldInclusionCondition
	fieldName string
}

// Execute method for ConditionalFieldNode
func (n *ConditionalFieldNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, false, func(dataMap map[string]any, dataType reflect.Type) *types.NodeResult {

		// Evaluate the condition
		conditionMet := n.condition.Evaluate(dataMap)

		if !conditionMet {
			// Condition not met, this field should not be present
			if _, fieldExists := dataMap[n.fieldName]; fieldExists {
				return &types.NodeResult{
					Success: false,
					Issues: []types.Issue{{
						Code:    "CONDITIONAL_FIELD_PRESENT",
						Message: fmt.Sprintf("Field '%s' should not be present when %s != %v", n.fieldName, n.condition.Field, n.condition.Value),
						Path:    n.buildPath(n.fieldName),
					}},
				}
			}
			// Field correctly absent
			return &types.NodeResult{Success: true}
		}
		// Condition met, field validation will be handled by regular field nodes
		return &types.NodeResult{Success: true}
	})
}
