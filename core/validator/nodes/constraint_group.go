package nodes

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// ConstraintGroupNode evaluates a logical group of constraints.
type ConstraintGroupNode struct {
	BaseNode[any]
	base BaseNode[any]
	group     schema.ConstraintGroup[schema.FieldType]
	memberIDs []string
}

func (n *ConstraintGroupNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	var results []bool

	for _, depID := range n.deps {
		if result, ok := ctx.Results[depID]; ok {
			results = append(results, result.Success)
		} else {
			results = append(results, false)
		}
	}

	if !schema.EvaluateLogicalOperator(n.group.Operator, results) {
		// If the group constraint fails, add the group violation issue
		groupViolationIssue := types.Issue{Code: "CONSTRAINT_GROUP_VIOLATION", Message: fmt.Sprintf("Constraint group '%s' failed", n.group.Name), Path: n.path}
		return &types.NodeResult{Success: false, Issues: []types.Issue{groupViolationIssue}}
	}

	return &types.NodeResult{Success: true}
}

