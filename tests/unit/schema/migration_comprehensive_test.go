package schema_test

import (
	"encoding/json"
	"fmt" // Used in createMigration helper
	"testing"
	"time" // Used in createMigration helper

	scjson "github.com/asaidimu/go-anansi/v6/core/json"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper Functions for Comprehensive Tests

func createMigration(sourceVersion, targetVersion string, changes ...schema.SchemaChange) *schema.Migration {
	target := targetVersion
	return &schema.Migration{
		ID: fmt.Sprintf("%d", time.Now().UnixNano()),
		Version: schema.MigrationVersion{
			Source: sourceVersion,
			Target: &target,
		},
		Changes:   changes,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
}

type testMigrationScenario struct {
	name                     string
	oldSchemaJSON            string
	newSchemaJSON            string
	expectedNumChanges       int
	expectedTargetVersion    string
	validateGeneratedChanges func(t *testing.T, changes []schema.SchemaChange)
	validateResultingSchema  func(t *testing.T, resultingSchema *schema.SchemaDefinition)
}

// createMigrationFromJSONs encapsulates migration generation from JSON strings
func createMigrationFromJSONs(t *testing.T, engine migration.MigrationEngine, oldSchemaJSON, newSchemaJSON string) (
	*schema.SchemaDefinition,
	*schema.SchemaDefinition,
	*schema.Migration,
) {
	oldSchemaDef, err := schema.From([]byte(oldSchemaJSON))
	require.NoError(t, err, "Failed to unmarshal oldSchemaJSON into SchemaDefinition using schema.From")
	require.NotNil(t, oldSchemaDef, "oldSchemaDef should not be nil")

	newSchemaDef, err := schema.From([]byte(newSchemaJSON))
	require.NoError(t, err, "Failed to unmarshal newSchemaJSON into SchemaDefinition using schema.From")
	require.NotNil(t, newSchemaDef, "newSchemaDef should not be nil")

	mig, err := engine.Diff(*oldSchemaDef, *newSchemaDef)
	require.NoError(t, err, "Failed to generate migration using MigrationEngine.Diff")
	require.NotNil(t, mig, "Migration should not be nil")

	return oldSchemaDef, newSchemaDef, mig
}

// applyMigrationViaDirectApplier applies migration using the MigrationEngine's Apply method
func applyMigrationViaDirectApplier(t *testing.T, engine migration.MigrationEngine, sourceSchema *schema.SchemaDefinition, mig *schema.Migration) *schema.SchemaDefinition {
	targetSchema, err := engine.Apply(sourceSchema, mig)
	require.NoError(t, err, "Failed to apply migration via direct applier")
	return targetSchema
}

// applyMigrationViaJsonPatch applies migration using JSON Patch
func applyMigrationViaJsonPatch(t *testing.T, engine migration.MigrationEngine, sourceSchemaJSON string, mig *schema.Migration, oldSchemaDef *schema.SchemaDefinition) string {
	oldSchemaDefForPatch, err := schema.From([]byte(sourceSchemaJSON))
	require.NoError(t, err, "Failed to unmarshal sourceSchemaJSON into SchemaDefinition for patching")

	allPatches := make([]scjson.PatchOperation, 0)
	for _, change := range mig.Changes {
		// Pass the original oldSchemaDef for context-dependent patch generation (e.g., removing index by name)
		patches, err := migration.SchemaChangeToPatch(change, oldSchemaDef)
		require.NoError(t, err, "Failed to convert schema change to patch")
		allPatches = append(allPatches, patches...)
	}

	require.NotEmpty(t, allPatches, "Should have generated JSON patches")

	patchedSchemaDef, err := engine.Patch(oldSchemaDefForPatch, allPatches)
	require.NoError(t, err, "Failed to apply patches via JSON Patch using MigrationEngine")

	// The version is calculated by the migrator but not included in JSON patches.
	// Explicitly set it on the patched map for comparison.
	if mig.Version.Target != nil {
		patchedSchemaDef.Version = *mig.Version.Target
	}

	patchedBytes, err := json.Marshal(patchedSchemaDef)
	require.NoError(t, err, "Failed to marshal patched schema map")

	return string(patchedBytes)
}

//
// Comprehensive Test Suite
//

func TestMigration_Comprehensive(t *testing.T) {
	// Initialize a single migration engine for all test cases
	engine := migration.NewDefaultMigrationEngine(migration.GeneratorOptions{}, migration.ApplierOptions{ValidateResult: true})
	baseOldSchemaJSON := `{
		"name": "BaseTestSchema",
		"version": "1.0.0",
		"description": "A base schema for comprehensive testing",
		"fields": {
			"stable_f1": {
				"name": "field1",
				"type": "string",
				"description": "An initial string field",
				"required": true
			},
			"stable_f2": {
				"name": "field2",
				"type": "integer",
				"description": "An initial integer field",
				"required": false
			}
		},
		"indexes": [
			{
				"name": "idx_f1",
				"fields": ["stable_f1"],
				"type": "normal",
				"unique": false
			}
		],
		"constraints": [
			{
					"name": "c_f1_minlen",
					"predicate": "minLength",
					"field": "stable_f1",
					"parameters": {"value": 3},
					"type": "schema"
			}
		],
		"nestedSchemas": {
			"nested_s1": {
				"name": "NestedSchema1",
				"fields": {
					"nested_f1": {"name": "nestedField1", "type": "string"}
				}
			}
		}
	}`

	testCases := []testMigrationScenario{
		{
			name: "ModifyNestedSchema_ChangeFieldType",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {},
				"nestedSchemas": {
					"nested_modify": {
						"name": "NestedToModify",
						"fields": {
							"nested_field": {"name": "nestedField", "type": "string"}
						}
					}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {},
				"nestedSchemas": {
					"nested_modify": {
						"name": "NestedToModify",
						"fields": {
							"nested_field": {"name": "nestedField", "type": "integer"}
						}
					}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Changing field type in nested schema is a breaking change
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifySchema, changes[0].Type)
				assert.Equal(t, "nested_modify", *changes[0].ID)
				require.Len(t, changes[0].SchemaChangeModifySchemaPayload.Changes, 1)
				nestedChange := changes[0].SchemaChangeModifySchemaPayload.Changes[0]
				assert.Equal(t, schema.SchemaChangeTypeModifyField, nestedChange.Type)
				assert.Equal(t, "nested_field", *nestedChange.ID)
				assert.NotNil(t, nestedChange.SchemaChangeModifyFieldPayload.Changes.Type)
				assert.Equal(t, schema.FieldTypeInteger, *nestedChange.SchemaChangeModifyFieldPayload.Changes.Type)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.NotNil(t, resultingSchema.NestedSchemas)
				assert.Contains(t, resultingSchema.NestedSchemas, "nested_modify")
				nestedSchema := resultingSchema.NestedSchemas["nested_modify"]
				assert.NotNil(t, nestedSchema.Fields)
				assert.Contains(t, nestedSchema.Fields.FieldsMap, "nested_field")
				field := nestedSchema.Fields.FieldsMap["nested_field"]
				assert.Equal(t, schema.FieldTypeInteger, field.Type)
			},
		},
		{
			name: "RemoveNestedSchema",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {},
				"nestedSchemas": {
					"nested_to_remove": {
						"name": "NestedToRemove",
						"fields": {
							"f1": {"name": "field1", "type": "string"}
						}
					}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {},
				"nestedSchemas": {}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Removing a nested schema is a major breaking change
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeRemoveSchema, changes[0].Type)
				assert.Equal(t, "nested_to_remove", *changes[0].ID)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Empty(t, resultingSchema.NestedSchemas)
			},
		},
		{
			name: "ModifyConstraint_MoreRestrictive",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				},
				"constraints": [
					{
						"name": "c_modify",
						"predicate": "minLength",
						"field": "field1",
						"parameters": {"value": 5}
					}
				]
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				},
				"constraints": [
					{
						"name": "c_modify",
						"predicate": "minLength",
						"field": "field1",
						"parameters": {"value": 10}
					}
				]
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Making a constraint more restrictive is a breaking change
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyConstraint, changes[0].Type)
				assert.Equal(t, "c_modify", *changes[0].Name)
				assert.NotNil(t, changes[0].SchemaChangeModifyConstraintPayload.Changes.Parameters)
				params, ok := changes[0].SchemaChangeModifyConstraintPayload.Changes.Parameters.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, float64(10), params["value"]) // JSON unmarshals numbers to float64
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Len(t, resultingSchema.Constraints, 1)
				c := resultingSchema.Constraints[0].Constraint
				assert.Equal(t, "c_modify", c.Name)
				params, ok := c.Parameters.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, float64(10), params["value"])
			},
		},
		{
			name: "RemoveConstraint",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				},
				"constraints": [
					{
						"name": "c_to_remove",
						"predicate": "minLength",
						"field": "field1",
						"parameters": {"value": 5}
					}
				]
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				},
				"constraints": []
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.1.0", // Removing a constraint is loosening strictness, thus minor
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeRemoveConstraint, changes[0].Type)
				assert.Equal(t, "c_to_remove", *changes[0].Name)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Empty(t, resultingSchema.Constraints)
			},
		},
		{
			name: "ModifyIndex_ChangeUnique",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				},
				"indexes": [
					{
						"name": "idx_modify",
						"fields": ["field1"],
						"type": "normal",
						"unique": false
					}
				]
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.1.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				},
				"indexes": [
					{
						"name": "idx_modify",
						"fields": ["field1"],
						"type": "normal",
						"unique": true
					}
				]
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyIndex, changes[0].Type)
				assert.Equal(t, "idx_modify", *changes[0].Name)
				assert.NotNil(t, changes[0].SchemaChangeModifyIndexPayload.Changes.Unique)
				assert.True(t, *changes[0].SchemaChangeModifyIndexPayload.Changes.Unique)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Len(t, resultingSchema.Indexes, 1)
				idx := resultingSchema.Indexes[0].Index
				assert.Equal(t, "idx_modify", idx.Name)
				assert.NotNil(t, idx.Unique)
				assert.True(t, *idx.Unique)
			},
		},
		{
			name: "RemoveIndex",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				},
				"indexes": [
					{
						"name": "idx_to_remove",
						"fields": ["field1"],
						"type": "normal"
					}
				]
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"field1": {"name": "field1", "type": "string"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.0.1",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeRemoveIndex, changes[0].Type)
				assert.Equal(t, "idx_to_remove", *changes[0].Name)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Empty(t, resultingSchema.Indexes)
			},
		},
		{
			name: "ModifyProperty_ChangeSchemaDescription",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"description": "old schema description",
				"fields": {}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.1",
				"description": "new schema description",
				"fields": {}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.0.1",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyProperty, changes[0].Type)
				assert.Equal(t, "description", *changes[0].ID)
				assert.Equal(t, utils.StringPtr("new schema description"), changes[0].SchemaChangeModifyPropertyPayload.Value)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.NotNil(t, resultingSchema.Description)
				assert.Equal(t, utils.StringPtr("new schema description"), resultingSchema.Description)
			},
		},
		{
			name: "ModifyField_ChangeArraySchema",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"array_field": {
						"name": "arrayField",
						"type": "array",
						"itemsType": "string"
					}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"fields": {
					"array_field": {
						"name": "arrayField",
						"type": "array",
						"itemsType": "integer"
					}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Changing internal schema is breaking
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "array_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.ItemsType)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "array_field")
				field, ok := resultingSchema.Fields["array_field"]
				assert.True(t, ok, "Expected inline schema to be map[string]any")
				assert.Equal(t, schema.FieldType("integer"), *field.ItemsType)
			},
		},
		{
			name: "ModifyField_ChangeDescription",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"desc_field": {"name": "descField", "type": "string", "description": "old description"}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.1",
				"fields": {
					"desc_field": {"name": "descField", "type": "string", "description": "new description"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.0.1",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "desc_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Description)
				assert.Equal(t, "new description", *changes[0].SchemaChangeModifyFieldPayload.Changes.Description)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "desc_field")
				assert.NotNil(t, resultingSchema.Fields["desc_field"].Description)
				assert.Equal(t, "new description", *resultingSchema.Fields["desc_field"].Description)
			},
		},
		{
			name: "ModifyField_UniqueTrueToFalse",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"unique_field": {"name": "uniqueField", "type": "string", "unique": true}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.1.0",
				"fields": {
					"unique_field": {"name": "uniqueField", "type": "string", "unique": false}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.1.0", // Less restrictive, so minor
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "unique_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Unique)
				assert.False(t, *changes[0].SchemaChangeModifyFieldPayload.Changes.Unique)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "unique_field")
				assert.NotNil(t, resultingSchema.Fields["unique_field"].Unique)
				assert.False(t, *resultingSchema.Fields["unique_field"].Unique)
			},
		},
		{
			name: "ModifyField_UniqueFalseToTrue",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"unique_field": {"name": "uniqueField", "type": "string", "unique": false}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"fields": {
					"unique_field": {"name": "uniqueField", "type": "string", "unique": true}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // More restrictive, so major
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "unique_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Unique)
				assert.True(t, *changes[0].SchemaChangeModifyFieldPayload.Changes.Unique)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "unique_field")
				assert.NotNil(t, resultingSchema.Fields["unique_field"].Unique)
				assert.True(t, *resultingSchema.Fields["unique_field"].Unique)
			},
		},
		{
			name: "ModifyField_RequiredTrueToFalse",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "required": true}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.1",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "required": false}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.1.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "test_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Required)
				assert.False(t, *changes[0].SchemaChangeModifyFieldPayload.Changes.Required)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "test_field")
				assert.NotNil(t, resultingSchema.Fields["test_field"].Required)
				assert.False(t, *resultingSchema.Fields["test_field"].Required)
			},
		},
		{
			name: "ModifyField_RequiredFalseToTrue",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "required": false}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "required": true}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "test_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Required)
				assert.True(t, *changes[0].SchemaChangeModifyFieldPayload.Changes.Required)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "test_field")
				assert.NotNil(t, resultingSchema.Fields["test_field"].Required)
				assert.True(t, *resultingSchema.Fields["test_field"].Required)
			},
		},
		{
			name: "ModifyField_AddDefaultValue",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"test_field": {"name": "testField", "type": "string"}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.1",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "default": "default_val"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.0.1",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "test_field", *changes[0].ID)
				assert.Equal(t, "default_val", changes[0].SchemaChangeModifyFieldPayload.Changes.Default)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "test_field")
				assert.Equal(t, "default_val", resultingSchema.Fields["test_field"].Default)
			},
		},
		{
			name: "ModifyField_ChangeDefaultValue",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "default": "old_default"}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.1",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "default": "new_default"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.0.1",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "test_field", *changes[0].ID)
				assert.Equal(t, "new_default", changes[0].SchemaChangeModifyFieldPayload.Changes.Default)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "test_field")
				assert.Equal(t, "new_default", resultingSchema.Fields["test_field"].Default)
			},
		},
		{
			name: "ModifyField_RemoveDefaultValue",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"test_field": {"name": "testField", "type": "string", "default": "default_val"}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.1",
				"fields": {
					"test_field": {"name": "testField", "type": "string"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "test_field", *changes[0].ID)
				assert.Nil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Default)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "test_field")
				assert.Nil(t, resultingSchema.Fields["test_field"].Default)
			},
		},
		{
			name: "ModifyField_AddEnumValue",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"enum_field": {"name": "enumField", "type": "enum", "values": ["value1", "value2"]}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.1.0",
				"fields": {
					"enum_field": {"name": "enumField", "type": "enum", "values": ["value1", "value2", "value3"]}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.1.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "enum_field", *changes[0].ID)
				assert.Contains(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Values, "value3")
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "enum_field")
				assert.Len(t, resultingSchema.Fields["enum_field"].Values, 3)
				assert.Contains(t, resultingSchema.Fields["enum_field"].Values, "value3")
			},
		},
		{
			name: "ModifyField_RemoveEnumValue",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"enum_field": {"name": "enumField", "type": "enum", "values": ["value1", "value2", "value3"]}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"fields": {
					"enum_field": {"name": "enumField", "type": "enum", "values": ["value1", "value3"]}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Major because removing enum value is a breaking change
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "enum_field", *changes[0].ID)
				assert.NotContains(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Values, "value2")
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "enum_field")
				assert.Len(t, resultingSchema.Fields["enum_field"].Values, 2)
				assert.NotContains(t, resultingSchema.Fields["enum_field"].Values, "value2")
			},
		},
		{
			name: "ModifyField_ReorderEnumValues",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"enum_field": {"name": "enumField", "type": "enum", "values": ["value1", "value2", "value3"]}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.1.0",
				"fields": {
					"enum_field": {"name": "enumField", "type": "enum", "values": ["value3", "value1", "value2"]}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.0.1",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "enum_field", *changes[0].ID)
				// DeepEqual is used for slice comparison, so order matters
				assert.Equal(t, []any{"value3", "value1", "value2"}, changes[0].SchemaChangeModifyFieldPayload.Changes.Values)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "enum_field")
				assert.Equal(t, []any{"value3", "value1", "value2"}, resultingSchema.Fields["enum_field"].Values)
			},
		},
		{
			name: "ModifyField_ChangeItemsType",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"array_field": {"name": "arrayField", "type": "array", "itemsType": "string"}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"fields": {
					"array_field": {"name": "arrayField", "type": "array", "itemsType": "integer"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Breaking change
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "array_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.ItemsType)
				assert.Equal(t, schema.FieldTypeInteger, *changes[0].SchemaChangeModifyFieldPayload.Changes.ItemsType)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "array_field")
				assert.NotNil(t, resultingSchema.Fields["array_field"].ItemsType)
				assert.Equal(t, schema.FieldTypeInteger, *resultingSchema.Fields["array_field"].ItemsType)
			},
		},
		{
			name: "ModifyField_AddItemsType",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"array_field": {"name": "arrayField", "type": "array"}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"fields": {
					"array_field": {"name": "arrayField", "type": "array", "itemsType": "string"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Breaking change (making it more specific)
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "array_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.ItemsType)
				assert.Equal(t, schema.FieldTypeString, *changes[0].SchemaChangeModifyFieldPayload.Changes.ItemsType)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "array_field")
				assert.NotNil(t, resultingSchema.Fields["array_field"].ItemsType)
				assert.Equal(t, schema.FieldTypeString, *resultingSchema.Fields["array_field"].ItemsType)
			},
		},
		{
			name: "ModifyField_RemoveItemsType",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"array_field": {"name": "arrayField", "type": "array", "itemsType": "string"}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"fields": {
					"array_field": {"name": "arrayField", "type": "array"}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0", // Major change (making it less specific, breaking)
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "array_field", *changes[0].ID)
				assert.Nil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.ItemsType)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "array_field")
				assert.Nil(t, resultingSchema.Fields["array_field"].ItemsType)
			},
		},
		{
			      name: "ModifyField_DeprecateField",
			      oldSchemaJSON: `{
			        "name": "BaseTestSchema",
			        "version": "1.0.0",
			        "fields": {
			          "old_field": {"name": "oldField", "type": "string"}
			        }
			      }`,
			      newSchemaJSON: `{
			        "name": "BaseTestSchema",
			        "version": "1.0.1",
			        "fields": {
			          "old_field": {"name": "oldField", "type": "string", "deprecated": true}
			        }
			      }`,
			      expectedNumChanges:    1,
			      expectedTargetVersion: "1.1.0",			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "old_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Deprecated)
				assert.True(t, *changes[0].SchemaChangeModifyFieldPayload.Changes.Deprecated)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "old_field")
				assert.NotNil(t, resultingSchema.Fields["old_field"].Deprecated)
				assert.True(t, *resultingSchema.Fields["old_field"].Deprecated)
			},
		},
		{
			name: "ModifyField_UnDeprecateField",
			oldSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.0",
				"fields": {
					"old_field": {"name": "oldField", "type": "string", "deprecated": true}
				}
			}`,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.0.1",
				"fields": {
					"old_field": {"name": "oldField", "type": "string", "deprecated": false}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.1.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "old_field", *changes[0].ID)
				assert.NotNil(t, changes[0].SchemaChangeModifyFieldPayload.Changes.Deprecated)
				assert.False(t, *changes[0].SchemaChangeModifyFieldPayload.Changes.Deprecated)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "old_field")
				assert.NotNil(t, resultingSchema.Fields["old_field"].Deprecated)
				assert.False(t, *resultingSchema.Fields["old_field"].Deprecated)
			},
		},
		{
			name:          "AddField",
			oldSchemaJSON: baseOldSchemaJSON,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.1.0",
				"description": "A base schema for comprehensive testing",
				"fields": {
					"stable_f1": {
						"name": "field1",
						"type": "string",
						"description": "An initial string field",
						"required": true
					},
					"stable_f2": {
						"name": "field2",
						"type": "integer",
						"description": "An initial integer field",
						"required": false
					},
					"stable_f3": {
						"name": "field3",
						"type": "boolean",
						"required": true
					}
				},
				"indexes": [
					{
						"name": "idx_f1",
						"fields": ["stable_f1"],
						"type": "normal",
						"unique": false
					}
				],
				"constraints": [
					{
						"name": "c_f1_minlen",
						"predicate": "minLength",
						"field": "stable_f1",
						"parameters": {"value": 3},
						"type": "schema"
					}
				],
				"nestedSchemas": {
					"nested_s1": {
						"name": "NestedSchema1",
						"fields": {
							"nested_f1": {"name": "nestedField1", "type": "string"}
						}
					}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeAddField, changes[0].Type)
				assert.Equal(t, "stable_f3", *changes[0].ID)
				assert.Equal(t, "field3", changes[0].SchemaChangeAddFieldPayload.Definition.Name)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "stable_f3")
				assert.Equal(t, "field3", resultingSchema.Fields["stable_f3"].Name)
				assert.Equal(t, schema.FieldTypeBoolean, resultingSchema.Fields["stable_f3"].Type)
			},
		},
		{
			name:          "RemoveField",
			oldSchemaJSON: baseOldSchemaJSON,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"description": "A base schema for comprehensive testing",
				"fields": {
					"stable_f2": {
						"name": "field2",
						"type": "integer",
						"description": "An initial integer field",
						"required": false
					}
				},
				"nestedSchemas": {
					"nested_s1": {
						"name": "NestedSchema1",
						"fields": {
							"nested_f1": {"name": "nestedField1", "type": "string"}
						}
					}
				}
			}`,
			expectedNumChanges:    3, // RemoveField, RemoveIndex, RemoveConstraint (due to cleanup)
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				// We expect RemoveField, RemoveIndex, RemoveConstraint. Order might vary.
				var foundFieldRemove, foundIndexRemove, foundConstraintRemove bool
				for _, change := range changes {
					switch change.Type {
					case schema.SchemaChangeTypeRemoveField:
						assert.Equal(t, "stable_f1", *change.ID)
						foundFieldRemove = true
					case schema.SchemaChangeTypeRemoveIndex:
						assert.Equal(t, "idx_f1", *change.Name)
						foundIndexRemove = true
					case schema.SchemaChangeTypeRemoveConstraint:
						assert.Equal(t, "c_f1_minlen", *change.Name)
						foundConstraintRemove = true
					}
				}
				assert.True(t, foundFieldRemove, "Should find RemoveField change")
				assert.True(t, foundIndexRemove, "Should find RemoveIndex change")
				assert.True(t, foundConstraintRemove, "Should find RemoveConstraint change")
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.NotContains(t, resultingSchema.Fields, "stable_f1")
				assert.Empty(t, resultingSchema.Indexes)
				assert.Empty(t, resultingSchema.Constraints)
			},
		},
		{
			name:          "ModifyField_Rename",
			oldSchemaJSON: baseOldSchemaJSON,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"description": "A base schema for comprehensive testing",
				"fields": {
					"stable_f1": {
						"name": "new_field1_name",
						"type": "string",
						"description": "An initial string field",
						"required": true
					},
					"stable_f2": {
						"name": "field2",
						"type": "integer",
						"description": "An initial integer field",
						"required": false
					}
				},
				"indexes": [
					{
						"name": "idx_f1",
						"fields": ["stable_f1"],
						"type": "normal",
						"unique": false
					}
				],
				"constraints": [
					{
						"name": "c_f1_minlen",
						"predicate": "minLength",
						"field": "stable_f1",
						"parameters": {"value": 3},
						"type": "schema"
					}
				],
				"nestedSchemas": {
					"nested_s1": {
						"name": "NestedSchema1",
						"fields": {
							"nested_f1": {"name": "nestedField1", "type": "string"}
						}
					}
				}
			}`,
			expectedNumChanges:    1, // Only field name change
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "stable_f1", *changes[0].ID)
				assert.Equal(t, "new_field1_name", *changes[0].SchemaChangeModifyFieldPayload.Changes.Name)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "stable_f1")
				assert.Equal(t, "new_field1_name", resultingSchema.Fields["stable_f1"].Name)
			},
		},
		{
			name:          "ModifyField_TypeChange",
			oldSchemaJSON: baseOldSchemaJSON,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"description": "A base schema for comprehensive testing",
				"fields": {
					"stable_f1": {
						"name": "field1",
						"type": "integer",
						"description": "An initial string field",
						"required": true
					},
					"stable_f2": {
						"name": "field2",
						"type": "integer",
						"description": "An initial integer field",
						"required": false
					}
				},
				"indexes": [
					{
						"name": "idx_f1",
						"fields": ["stable_f1"],
						"type": "normal",
						"unique": false
					}
				],
				"constraints": [
					{
						"name": "c_f1_minlen",
						"predicate": "minLength",
						"field": "stable_f1",
						"parameters": {"value": 3},
						"type": "schema"
					}
				],
				"nestedSchemas": {
					"nested_s1": {
						"name": "NestedSchema1",
						"fields": {
							"nested_f1": {"name": "nestedField1", "type": "string"}
						}
					}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeModifyField, changes[0].Type)
				assert.Equal(t, "stable_f1", *changes[0].ID)
				assert.Equal(t, schema.FieldTypeInteger, *changes[0].SchemaChangeModifyFieldPayload.Changes.Type)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Contains(t, resultingSchema.Fields, "stable_f1")
				assert.Equal(t, schema.FieldTypeInteger, resultingSchema.Fields["stable_f1"].Type)
			},
		},
		{
			name:          "AddIndex",
			oldSchemaJSON: baseOldSchemaJSON,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.1.0",
				"description": "A base schema for comprehensive testing",
				"fields": {
					"stable_f1": {
						"name": "field1",
						"type": "string",
						"description": "An initial string field",
						"required": true
					},
					"stable_f2": {
						"name": "field2",
						"type": "integer",
						"description": "An initial integer field",
						"required": false
					}
				},
				"indexes": [
					{
						"name": "idx_f1",
						"fields": ["stable_f1"],
						"type": "normal",
						"unique": false
					},
					{
						"name": "idx_f2",
						"fields": ["stable_f2"],
						"type": "normal",
						"unique": true
					}
				],
				"constraints": [
					{
						"name": "c_f1_minlen",
						"predicate": "minLength",
						"field": "stable_f1",
						"parameters": {"value": 3},
						"type": "schema"
					}
				],
				"nestedSchemas": {
					"nested_s1": {
						"name": "NestedSchema1",
						"fields": {
							"nested_f1": {"name": "nestedField1", "type": "string"}
						}
					}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeAddIndex, changes[0].Type)
				assert.Equal(t, "idx_f2", changes[0].SchemaChangeAddIndexPayload.Definition.Name)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Len(t, resultingSchema.Indexes, 2)
				// Check existence of idx_f2
				var found bool
				for _, ior := range resultingSchema.Indexes {
					if ior.IsIndex() && ior.Index.Name == "idx_f2" {
						found = true
						assert.Equal(t, []string{"stable_f2"}, ior.Index.Fields)
						assert.True(t, *ior.Index.Unique)
						break
					}
				}
				assert.True(t, found, "idx_f2 should be present")
			},
		},
		{
			name:          "AddConstraint",
			oldSchemaJSON: baseOldSchemaJSON,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "2.0.0",
				"description": "A base schema for comprehensive testing",
				"fields": {
					"stable_f1": {
						"name": "field1",
						"type": "string",
						"description": "An initial string field",
						"required": true
					},
					"stable_f2": {
						"name": "field2",
						"type": "integer",
						"description": "An initial integer field",
						"required": false
					}
				},
				"indexes": [
					{
						"name": "idx_f1",
						"fields": ["stable_f1"],
						"type": "normal",
						"unique": false
					}
				],
				"constraints": [
					{
							"name": "c_f1_minlen",
							"predicate": "minLength",
							"field": "stable_f1",
							"parameters": {"value": 3},
							"type": "schema"
					},
					{
						"name": "c_f2_minval",
						"predicate": "min",
						"field": "stable_f2",
						"parameters": {"value": 0},
						"type": "schema"
					}
				],
				"nestedSchemas": {
					"nested_s1": {
						"name": "NestedSchema1",
						"fields": {
							"nested_f1": {"name": "nestedField1", "type": "string"}
						}
					}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "2.0.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeAddConstraint, changes[0].Type)
				assert.Equal(t, "c_f2_minval", changes[0].SchemaChangeAddConstraintPayload.Constraint.Constraint.Name)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.Len(t, resultingSchema.Constraints, 2)
				var found bool
				for _, c := range resultingSchema.Constraints {
					if c.IsConstraint() && c.Constraint.Name == "c_f2_minval" {
						found = true
						assert.Equal(t, "min", c.Constraint.Predicate)
						break
					}
				}
				assert.True(t, found, "c_f2_minval constraint should be present")
			},
		},
		{
			name:          "AddNestedSchema",
			oldSchemaJSON: baseOldSchemaJSON,
			newSchemaJSON: `{
				"name": "BaseTestSchema",
				"version": "1.1.0",
				"description": "A base schema for comprehensive testing",
				"fields": {
					"stable_f1": {
						"name": "field1",
						"type": "string",
						"description": "An initial string field",
						"required": true
					},
					"stable_f2": {
						"name": "field2",
						"type": "integer",
						"description": "An initial integer field",
						"required": false
					}
				},
				"indexes": [
					{
						"name": "idx_f1",
						"fields": ["stable_f1"],
						"type": "normal",
						"unique": false
					}
				],
				"constraints": [
					{
						"name": "c_f1_minlen",
						"predicate": "minLength",
						"field": "stable_f1",
						"parameters": {"value": 3},
						"type": "schema"
					}
				],
				"nestedSchemas": {
					"nested_s1": {
						"name": "NestedSchema1",
						"fields": {
							"nested_f1": {"name": "nestedField1", "type": "string"}
						}
					},
					"nested_s2": {
						"name": "NestedSchema2",
						"fields": {
							"nested_f2": {"name": "nestedField2", "type": "boolean"}
						}
					}
				}
			}`,
			expectedNumChanges:    1,
			expectedTargetVersion: "1.1.0",
			validateGeneratedChanges: func(t *testing.T, changes []schema.SchemaChange) {
				assert.Equal(t, schema.SchemaChangeTypeAddSchema, changes[0].Type)
				assert.Equal(t, "nested_s2", *changes[0].ID)
				assert.Equal(t, "NestedSchema2", changes[0].SchemaChangeAddSchemaPayload.Definition.Name)
			},
			validateResultingSchema: func(t *testing.T, resultingSchema *schema.SchemaDefinition) {
				assert.NotNil(t, resultingSchema.NestedSchemas)
				assert.Contains(t, resultingSchema.NestedSchemas, "nested_s2")
				assert.Equal(t, "NestedSchema2", resultingSchema.NestedSchemas["nested_s2"].Name)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldSchemaDef, newSchemaDef, mig := createMigrationFromJSONs(t, engine, tc.oldSchemaJSON, tc.newSchemaJSON)

			// --- Test via Direct Applier ---
			t.Run("DirectApplier", func(t *testing.T) {
				appliedSchema := applyMigrationViaDirectApplier(t, engine, oldSchemaDef, mig)

				assert.Equal(t, tc.expectedTargetVersion, appliedSchema.Version)
				assert.Equal(t, tc.expectedNumChanges, len(mig.Changes))
				if tc.validateGeneratedChanges != nil {
					tc.validateGeneratedChanges(t, mig.Changes)
				}
				if tc.validateResultingSchema != nil {
					tc.validateResultingSchema(t, appliedSchema)
				}
				// Deep comparison of the entire schema against the expected new schema definition
				// Marshal/Unmarshal to normalize before deep comparison
				expectedBytes, err := json.Marshal(newSchemaDef)
				require.NoError(t, err)
				appliedBytes, err := json.Marshal(appliedSchema)
				require.NoError(t, err)

				var finalExpectedMap, finalAppliedMap map[string]any
				require.NoError(t, json.Unmarshal(expectedBytes, &finalExpectedMap))
				require.NoError(t, json.Unmarshal(appliedBytes, &finalAppliedMap))
				finalExpectedMap["version"] = appliedSchema.Version
				assert.Equal(t, finalExpectedMap, finalAppliedMap, "Direct applied schema should match new schema definition")
			})

			// --- Test via JSON Patch ---
			t.Run("JsonPatchApplier", func(t *testing.T) {
				appliedSchemaJSON := applyMigrationViaJsonPatch(t, engine, tc.oldSchemaJSON, mig, oldSchemaDef)

				var appliedSchemaDef schema.SchemaDefinition
				err := json.Unmarshal([]byte(appliedSchemaJSON), &appliedSchemaDef)
				require.NoError(t, err)

				assert.Equal(t, tc.expectedTargetVersion, appliedSchemaDef.Version)
				assert.Equal(t, tc.expectedNumChanges, len(mig.Changes))
				if tc.validateGeneratedChanges != nil {
					tc.validateGeneratedChanges(t, mig.Changes)
				}
				if tc.validateResultingSchema != nil {
					tc.validateResultingSchema(t, &appliedSchemaDef)
				}

				// Deep comparison of the entire schema against the expected new schema JSON
				// Normalize both JSON strings by unmarshaling and re-marshaling
				expectedBytes, err := json.Marshal(newSchemaDef)
				require.NoError(t, err)

				var finalExpectedMap, finalAppliedMap map[string]any
				require.NoError(t, json.Unmarshal(expectedBytes, &finalExpectedMap))
				require.NoError(t, json.Unmarshal([]byte(appliedSchemaJSON), &finalAppliedMap))
				finalExpectedMap["version"] = finalAppliedMap["version"]
				assert.Equal(t, finalExpectedMap, finalAppliedMap, "JSON Patch applied schema should match new schema JSON")
			})
		})
	}
}
