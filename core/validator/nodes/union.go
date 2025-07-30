package nodes

import (
	"reflect"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// UnionValidationNode validates a value against one of several possible schemas.
type UnionValidationNode struct {
	BaseNode[any]
	fieldDef *schema.FieldDefinition
	schema   *schema.SchemaDefinition
}

func (n *UnionValidationNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, false, func(value any, _ reflect.Type) *types.NodeResult {
		schemas, ok := n.fieldDef.Schema.([]schema.NestedSchemaReference)
		if !ok {
			return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "INVALID_UNION_SCHEMA", Path: n.path}}}
		}

		var specificConstraintViolations []types.Issue // Stores issues where type matched, but constraints failed

		for _, schemaRef := range schemas {
			nestedDef, exists := n.schema.FindNestedSchema(schemaRef.ID)
			if !exists {
				continue // Skip if nested schema definition not found
			}

			var tempRootField schema.FieldDefinition
			if nestedDef.IsStructured != nil && *nestedDef.IsStructured {
				tempRootField = schema.FieldDefinition{Name: "root", Type: schema.FieldTypeObject, Schema: schemaRef}
			} else if nestedDef.Type != nil {
				// FIX: For literal nested schemas, include the constraints
				tempRootField = schema.FieldDefinition{
					Name:        "root",
					Type:        *nestedDef.Type,
					Schema:      nestedDef.Schema,
					ItemsType:   nestedDef.ItemsType,
					Constraints: nestedDef.Constraints, // Include constraints from nested schema
				}
			} else {
				continue // Should not happen with a valid nested schema.
			}

			tempSchema := &schema.SchemaDefinition{
				Name:          "temp_union_check",
				Fields:        map[string]*schema.FieldDefinition{"root": &tempRootField},
				NestedSchemas: n.schema.NestedSchemas,
			}

			validator, err := n.validatorFactory(tempSchema)
			if err != nil {
				// If validator creation fails for this union option, it cannot be matched.
				continue
			}

			// Perform validation for this specific union branch
			itemIssues, matched := validator.Validate(map[string]any{"root": value}, false)

			// If this branch fully matched, we are done.
			if matched {
				return &types.NodeResult{Success: true}
			}

			// If not matched, we need to categorize the issues.
			// Rewrite paths first to be consistent with the parent path.
			for i := range itemIssues {
				itemIssues[i].Path = strings.Replace(itemIssues[i].Path, "root", n.path, 1)
			}

			// Only collect constraint violations if there were no structural issues
			hasStructuralIssues := false
			var constraintViolations []types.Issue

			for _, issue := range itemIssues {
				switch issue.Code {
				case "TYPE_MISMATCH", "UNEXPECTED_FIELD", "REQUIRED_FIELD_MISSING", "ENUM_VIOLATION":
					// These indicate structural incompatibility with this union branch
					hasStructuralIssues = true
				case "CONSTRAINT_VIOLATION":
					// This indicates the structure matched but business rules failed
					constraintViolations = append(constraintViolations, issue)
				}
			}

			// Only collect constraint violations if the branch was structurally compatible
			if !hasStructuralIssues && len(constraintViolations) > 0 {
				specificConstraintViolations = append(specificConstraintViolations, constraintViolations...)
			}
		}

		// After trying all schemas in the union:
		if len(specificConstraintViolations) > 0 {
			// If at least one schema was structurally compatible but failed constraints,
			// return those specific constraint violations.
			return &types.NodeResult{Success: false, Issues: specificConstraintViolations}
		} else {
			// If no schema fully matched, and no structurally compatible branches had constraint violations,
			// then it means all branches were either structurally incompatible or had no issues at all.
			// In this case, return a single UNION_NO_MATCH issue.
			return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "UNION_NO_MATCH", Message: "Value does not match any of the union schemas", Path: n.path}}}
		}
	})
}
