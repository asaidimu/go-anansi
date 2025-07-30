package nodes

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// ArrayValidationNode validates each item in an array.
type ArrayValidationNode struct {
	BaseNode[[]any]
	base BaseNode[[]any]
	fieldDef *schema.FieldDefinition
	schema   *schema.SchemaDefinition
}

func (n *ArrayValidationNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, false, func(items []any, _ reflect.Type) *types.NodeResult {
		var allIssues []types.Issue
		itemType := *n.fieldDef.ItemsType

		for i, item := range items {
			itemPath := fmt.Sprintf("%s[%d]", n.path, i)

			// Create a temporary schema and validator for this single item.
			tempRootField := schema.FieldDefinition{
				Name:      "item",
				Type:      itemType,
				Schema:    n.fieldDef.Schema, // Pass along schema ref for Object/Record/Union
				ItemsType: nil,               // ItemsType is not nested within another array validator
			}

			tempSchema := &schema.SchemaDefinition{
				Name:          "temp_array_item_check",
				Fields:        map[string]*schema.FieldDefinition{"item": &tempRootField},
				NestedSchemas: n.schema.NestedSchemas, // Provide access to all known nested schemas
			}

			validator, err := n.validatorFactory(tempSchema)
			if err != nil {
				return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "VALIDATOR_CREATION_ERROR", Message: err.Error(), Path: itemPath}}}
			}
			itemIssues, _ := validator.Validate(map[string]any{"item": item}, false)

			// The temporary validator reports paths starting with "item". We must rewrite them
			// to correspond to the correct array index path.
			for j := range itemIssues {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
			}
			allIssues = append(allIssues, itemIssues...)
		}

		return &types.NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
	})
}
