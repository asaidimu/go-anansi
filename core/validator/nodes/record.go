package nodes

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// RecordValidationNode validates each value in a record map against a schema.
type RecordValidationNode struct {
    BaseNode[map[string]any]
	fieldDef *schema.FieldDefinition
	schema   *schema.SchemaDefinition
	issues   []types.Issue
}

func (n *RecordValidationNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	// end of preamble
	return n.execute(ctx, false, func(recordMap map[string]any,  _ reflect.Type) *types.NodeResult {
		// Case 1: If schema is not defined, the value must be a map.
		// The TypeCheckNode has already confirmed this, so we are done.
		if n.fieldDef.Schema == nil {
			return &types.NodeResult{Success: true}
		}

		// Case 2: If schema is defined, all values must conform.
		ref, ok := n.fieldDef.Schema.(schema.NestedSchemaReference)
		if !ok {
			return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "INVALID_RECORD_SCHEMA", Message: "Record schema must be a NestedSchemaReference", Path: n.path}}}
		}

		nestedDef, exists := n.schema.FindNestedSchema(ref.ID)
		if !exists {
			return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "NESTED_SCHEMA_NOT_FOUND", Message: fmt.Sprintf("Nested schema '%s' not found for record items", ref.ID), Path: n.path}}}
		}

		var allIssues []types.Issue

		for key, itemValue := range recordMap {
			itemPath := utils.BuildPath(n.path, key)
			// Create a temporary validator for each value in the record.
			var tempRootField schema.FieldDefinition
			if nestedDef.IsStructured != nil && *nestedDef.IsStructured {
				// Handle structured item values (i.e., objects).
				tempRootField = schema.FieldDefinition{Name: "item", Type: schema.FieldTypeObject, Schema: ref}
			} else if nestedDef.Type != nil {
				// Handle unstructured item values (i.e., literals like string, number).
				tempRootField = schema.FieldDefinition{Name: "item", Type: *nestedDef.Type, Schema: nestedDef.Schema, ItemsType: nestedDef.ItemsType}
			} else {
				continue // Should not happen with a valid nested schema.
			}

			tempSchema := &schema.SchemaDefinition{
				Name:          "temp_record_item_check",
				Fields:        map[string]*schema.FieldDefinition{"item": &tempRootField},
				NestedSchemas: n.schema.NestedSchemas,
			}

			validator, err := n.validatorFactory(tempSchema)
			if err != nil {
				return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "VALIDATOR_CREATION_ERROR", Message: err.Error(), Path: itemPath}}}
			}
			itemIssues, _ := validator.Validate(map[string]any{"item": itemValue}, false)

			// Rewrite paths from "item.subpath" to "recordName.key.subpath" for correct error reporting.
			for j := range itemIssues {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
			}
			allIssues = append(allIssues, itemIssues...)
		}

		return &types.NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
	})

}
