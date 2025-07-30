package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// ConstraintNode executes a single predicate function.
type ConstraintNode struct {
	BaseNode[any]
	constraint schema.Constraint[schema.FieldType]
	fmap       *schema.FunctionMap
}

func (n *ConstraintNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	predicateFunc, exists := (*n.fmap)[n.constraint.Predicate]
	if !exists {
		return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "MISSING_PREDICATE", Message: fmt.Sprintf("Predicate '%s' not found", n.constraint.Predicate), Path: n.path}}}
	}

	predicate, ok := predicateFunc.(func(schema.PredicateParams[any]) bool)
	if !ok {
		return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "INVALID_PREDICATE_TYPE", Message: fmt.Sprintf("Predicate '%s' has invalid type", n.constraint.Predicate), Path: n.path}}}
	}

	return n.execute(ctx, false, func(predicateData any, _ reflect.Type) *types.NodeResult {

		// If the constraint targets a sub-field, the predicateData is already the container.
		// If it doesn't, predicateData is the value to be tested itself.
		// The predicate function's implementation handles this distinction with the 'Field' parameter.

		params := schema.PredicateParams[any]{
			Data:  predicateData,
			Field: n.constraint.Field,
			Args:  n.constraint.Parameters,
		}

		if !predicate(params) {
			message := fmt.Sprintf("Constraint '%s' failed", n.constraint.Name)
			if n.constraint.ErrorMessage != nil {
				message = *n.constraint.ErrorMessage
			}

			// The path of the issue should be more specific if a field is targeted.
			issuePath := n.path
			if n.constraint.Field != nil && *n.constraint.Field != "" {
				// Check if predicateData is a map to avoid panic
				if _, ok := predicateData.(map[string]any); ok {
					// Only append the field to the path if it's not a global constraint (path is empty)
					if n.path != "" {
						issuePath = n.buildPath(*n.constraint.Field)
					} else {
						issuePath = *n.constraint.Field
					}
				}
			}
			return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "CONSTRAINT_VIOLATION", Message: message, Path: issuePath}}}
		}
		return &types.NodeResult{Success: true}
	})
}
