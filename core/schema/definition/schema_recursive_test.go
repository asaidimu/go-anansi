package definition_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecursiveSchema tests the validator with a recursive schema definition
func TestRecursiveSchema(t *testing.T) {
	// Define a simple recursive schema: Node has a name and an array of children Nodes
	recursiveSchema := definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "RecursiveNodeSchema",
			Fields: map[definition.FieldId]definition.Field{
				"name": {
					Name:     "name",
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"children": {
					Name:     "children",
					Required: false,
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeArray,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Node"}), // Reference to itself
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"Node": {
				BaseSchema: definition.BaseSchema{ // This is the recursive part
					Name: "Node",
					Fields: map[definition.FieldId]definition.Field{
						"name": {
							Name:     "name",
							Required: true,
							FieldProperties: definition.FieldProperties{
								Type: definition.FieldTypeString,
							},
						},
						"children": {
							Name:     "children",
							Required: false,
							FieldProperties: definition.FieldProperties{
								Type:   definition.FieldTypeArray,
								Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Node"}),
							},
						},
					},
				},
			},
		},
	}

	// Create validator with the recursive schema
	validator, err := definition.NewDocumentValidator(&recursiveSchema, nil)
	require.NoError(t, err, "Failed to create validator for recursive schema")

	t.Run("Valid deeply nested data", func(t *testing.T) {
		validData := map[string]any{
			"name": "Root",
			"children": []any{
				map[string]any{
					"name": "Child 1",
					"children": []any{
						map[string]any{
							"name": "Grandchild 1.1",
						},
						map[string]any{
							"name": "Grandchild 1.2",
							"children": []any{
								map[string]any{"name": "Great-grandchild 1.2.1"},
								map[string]any{"name": "Great-grandchild 1.2.2"},
							},
						},
					},
				},
				map[string]any{"name": "Child 2"},
			},
		}

		issues, isValid := validator.Validate(validData)
		assert.True(t, isValid, "Valid data should pass validation, issues: %v", issues)
		assert.Empty(t, issues, "Valid data should have no issues")
	})

	t.Run("Error at deep level - Type Mismatch", func(t *testing.T) {
		invalidData := map[string]any{
			"name": "Root",
			"children": []any{
				map[string]any{
					"name": "Child 1",
					"children": []any{
						map[string]any{
							"name": "Grandchild 1.1",
						},
						map[string]any{
							"name": 123, // ERROR: name should be string
							"children": []any{
								map[string]any{"name": "Great-grandchild 1.2.1"},
							},
						},
					},
				},
			},
		}

		issues, isValid := validator.Validate(invalidData)
		assert.False(t, isValid, "Invalid data should not pass validation")
		require.Len(t, issues, 1, "Expected exactly one issue")
		assert.Equal(t, "TYPE_MISMATCH", issues[0].Code)
		assert.Equal(t, "Expected string, got int", issues[0].Message)
		assert.Equal(t, "children[0].children[1].name", issues[0].Path)
	})

	t.Run("Error at deep level - Required Field Missing", func(t *testing.T) {
		invalidData := map[string]any{
			"name": "Root",
			"children": []any{
				map[string]any{
					"name": "Child 1",
					"children": []any{
						map[string]any{
							"name": "Grandchild 1.1",
						},
						map[string]any{
							// "name" field is missing here, but it's required
							"children": []any{
								map[string]any{"name": "Great-grandchild 1.2.1"},
							},
						},
					},
				},
			},
		}

		issues, isValid := validator.Validate(invalidData)
		assert.False(t, isValid, "Invalid data should not pass validation")
		require.Len(t, issues, 1, "Expected exactly one issue")
		assert.Equal(t, "REQUIRED_FIELD_MISSING", issues[0].Code)
		assert.Equal(t, "Required field 'name' is missing", issues[0].Message)
		assert.Equal(t, "children[0].children[1].name", issues[0].Path)
	})

	t.Run("MaxDepth Exceeded", func(t *testing.T) {
		deeplyNestedData := map[string]any{
			"name": "Root",
			"children": []any{
				map[string]any{
					"name": "Child 1",
					"children": []any{
						map[string]any{
							"name": "Grandchild 1.1",
							"children": []any{
								map[string]any{
									"name": "Great-grandchild 1.1.1",
									"children": []any{
										map[string]any{"name": "GG-grandchild 1.1.1.1"},
										map[string]any{"name": "GG-grandchild 1.1.1.2"},
									},
								},
							},
						},
					},
				},
			},
		}

		// Configure validator with a low MaxDepth (e.g., 2 levels of children, total 3 levels deep)
		config := definition.DefaultValidationConfig()
		config.MaxDepth = 3 // Corresponds to path depth up to Grandchild. Great-grandchild path depth (6) should exceed.

		depthValidator, err := definition.NewDocumentValidatorWithConfig(&recursiveSchema, nil, config)
		require.NoError(t, err, "Failed to create validator with MaxDepth config")

		issues, isValid := depthValidator.Validate(deeplyNestedData)
		assert.False(t, isValid, "Validation should fail due to MaxDepth")
		require.GreaterOrEqual(t, len(issues), 1, "Expected at least one issue due to MaxDepth")

		// Check for MAX_DEPTH_EXCEEDED error for paths exceeding max depth
		foundMaxDepthError := false
		for _, issue := range issues {
			if issue.Code == "MAX_DEPTH_EXCEEDED" {
				foundMaxDepthError = true
				assert.Contains(t, issue.Path, "children[0].children", "MaxDepthExceeded error path should be correct")
			}
		}
		assert.True(t, foundMaxDepthError, "Expected MAX_DEPTH_EXCEEDED error")
	})

	t.Run("Cyclic Schema Definition (handled by visitedSchemas during graph build)", func(t *testing.T) {
		// The `recursiveSchema` itself has a direct cycle (Node references Node).
		// If visitedSchemas in buildFromSchema were not working, NewDocumentValidator would panic or loop infinitely.
		// Since the above tests passed, it implies visitedSchemas is working during graph construction.
		// This test explicitly asserts that the validator creation with the cyclic schema does not error.
		_, err := definition.NewDocumentValidator(&recursiveSchema, nil)
		assert.NoError(t, err, "Validator creation with cyclic schema should not error due to graph build cycle")
	})
}
