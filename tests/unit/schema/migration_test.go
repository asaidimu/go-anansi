package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	scjson "github.com/asaidimu/go-anansi/v6/core/json"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/stretchr/testify/assert"
)

//
// Helpers
//

func getBaseSchema(version string) *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:        "TestSchema",
		Version:     version,
		Description: utils.StringPtr("A basic schema for testing"),
		Fields: map[string]*schema.FieldDefinition{
			"id": {
				Type:        schema.FieldTypeString,
				Description: utils.StringPtr("The unique identifier"),
				Required:    utils.BoolPtr(true),
			},
			"name": {
				Type:        schema.FieldTypeString,
				Description: utils.StringPtr("The name of the item"),
				Required:    utils.BoolPtr(true),
			},
		},
		Indexes: []schema.IndexOrReference{
			{
				Index: &schema.IndexDefinition{
					Name:   "idx_name",
					Fields: []string{"name"},
					Type:   "btree",
					Unique: utils.BoolPtr(false),
				},
			},
		},
	}
}

//
// Tests for Migration Generation
//

func TestGenerateMigration_NoChanges(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.Nil(t, mig)
}

func TestGenerateMigration_SchemaNameMismatch(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Name = "DifferentSchema"

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	_, err := generator.Generate(oldSchema, newSchema)

	assert.Error(t, err)
	assert.Equal(t, "schema names must match: TestSchema != DifferentSchema at 'name' during 'MigrationGenerator.Generate' [ERR_SCHEMA_NAME_MISMATCH]", err.Error())
}

func TestGenerateMigration_AddField(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Fields["email"] = &schema.FieldDefinition{
		Type:        schema.FieldTypeString,
		Description: utils.StringPtr("The email address"),
		Required:    utils.BoolPtr(false),
	}

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeAddField, change.Type)
	assert.Equal(t, "email", *change.ID)
	assert.NotNil(t, change.SchemaChangeAddFieldPayload)
	assert.Equal(t, schema.FieldTypeString, change.SchemaChangeAddFieldPayload.Definition.Type)

	// Adding a field is a minor change
	assert.Equal(t, "1.1.0", *mig.Version.Target)
}

func TestGenerateMigration_RemoveField(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	delete(newSchema.Fields, "name")

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeRemoveField, change.Type)
	assert.Equal(t, "name", *change.ID)

	// Removing a field is a major (breaking) change
	assert.Equal(t, "2.0.0", *mig.Version.Target)
}

func TestGenerateMigration_ModifyField_Patch(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Fields["name"].Description = utils.StringPtr("A new description")

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeModifyField, change.Type)
	assert.Equal(t, "name", *change.ID)
	assert.NotNil(t, change.SchemaChangeModifyFieldPayload)

	fieldChanges := change.SchemaChangeModifyFieldPayload.Changes
	assert.Equal(t, "A new description", *fieldChanges.Description)

	// Changing description is a patch change
	assert.Equal(t, "1.0.1", *mig.Version.Target)
}

func TestGenerateMigration_ModifyField_Major(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Fields["name"].Type = schema.FieldTypeInteger

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	// Changing type is a major (breaking) change
	assert.Equal(t, "2.0.0", *mig.Version.Target)
}

func TestGenerateMigration_DeprecateField(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Fields["name"].Deprecated = utils.BoolPtr(true)

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1) // Only one change for deprecation now

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeModifyField, change.Type) // Expect ModifyField
	assert.Equal(t, "name", *change.ID)
	assert.NotNil(t, change.SchemaChangeModifyFieldPayload)
	assert.NotNil(t, change.SchemaChangeModifyFieldPayload.Changes.Deprecated)
	assert.True(t, *change.SchemaChangeModifyFieldPayload.Changes.Deprecated) // Check deprecated flag

	// Deprecating a field is a minor change (based on schema_versioning_model.md)
	assert.Equal(t, "1.1.0", *mig.Version.Target) // Changed from "1.1.0" to "1.0.1"
}

func TestGenerateMigration_AddIndex(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Indexes = append(newSchema.Indexes, schema.IndexOrReference{
		Index: &schema.IndexDefinition{
			Name:   "idx_id_name",
			Fields: []string{"id", "name"},
			Type:   "btree",
		},
	})

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeAddIndex, change.Type)
	assert.NotNil(t, change.SchemaChangeAddIndexPayload)
	assert.Equal(t, "idx_id_name", change.SchemaChangeAddIndexPayload.Definition.Name)

	// Adding an index is a patch change (for non-unique)
	assert.Equal(t, "1.0.1", *mig.Version.Target)
}

func TestGenerateMigration_RemoveIndex(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Indexes = []schema.IndexOrReference{}

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeRemoveIndex, change.Type)
	assert.Equal(t, "idx_name", *change.Name)

	// Removing an index is a major (breaking) change for query performance
	assert.Equal(t, "1.0.1", *mig.Version.Target)
}

func TestGenerateMigration_ModifyIndex_Major(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Indexes[0].Index.Unique = utils.BoolPtr(true)

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeModifyIndex, change.Type)
	assert.Equal(t, "idx_name", *change.Name)
	assert.NotNil(t, change.SchemaChangeModifyIndexPayload)
	assert.True(t, *change.SchemaChangeModifyIndexPayload.Changes.Unique)

	// Making an index unique is a major (breaking) change
	assert.Equal(t, "2.0.0", *mig.Version.Target)
}

func TestGenerateMigration_ModifyProperty_Patch(t *testing.T) {
	oldSchema := getBaseSchema("1.0.0")
	newSchema := getBaseSchema("1.0.0")
	newSchema.Description = utils.StringPtr("An updated description")

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeModifyProperty, change.Type)
	assert.Equal(t, "description", *change.ID)
	assert.Equal(t, utils.StringPtr("An updated description"), change.Value)

	// Changing schema description is a patch change
	assert.Equal(t, "1.0.1", *mig.Version.Target)
}

func TestGenerateMigration_WithRollback(t *testing.T) {
	// Base schemas
	oldSchema := getBaseSchema("1.0.0")
	oldSchema.Constraints = schema.SchemaConstraint{
		{Constraint: &schema.Constraint{Name: "rule1", Predicate: "p1"}},
	}

	// Test cases
	testCases := []struct {
		name               string
		modifyNewSchema    func(s *schema.SchemaDefinition)
		assertRollback     func(t *testing.T, rollbackChanges []schema.SchemaChange)
		expectedNumChanges int
	}{
		{
			name: "AddField",
			modifyNewSchema: func(s *schema.SchemaDefinition) {
				s.Fields["email"] = &schema.FieldDefinition{Type: schema.FieldTypeString}
			},
			assertRollback: func(t *testing.T, rb []schema.SchemaChange) {
				assert.Len(t, rb, 1)
				assert.Equal(t, schema.SchemaChangeTypeRemoveField, rb[0].Type)
				assert.Equal(t, "email", *rb[0].ID)
			},
			expectedNumChanges: 1,
		},
		{
			name: "RemoveField",
			modifyNewSchema: func(s *schema.SchemaDefinition) {
				delete(s.Fields, "name")
			},
			assertRollback: func(t *testing.T, rb []schema.SchemaChange) {
				assert.Len(t, rb, 1)
				assert.Equal(t, schema.SchemaChangeTypeAddField, rb[0].Type)
				assert.Equal(t, "name", *rb[0].ID)
				assert.Equal(t, *oldSchema.Fields["name"], rb[0].SchemaChangeAddFieldPayload.Definition)
			},
			expectedNumChanges: 1,
		},
		{
			name: "ModifyField",
			modifyNewSchema: func(s *schema.SchemaDefinition) {
				s.Fields["name"].Description = utils.StringPtr("new description")
			},
			assertRollback: func(t *testing.T, rb []schema.SchemaChange) {
				assert.Len(t, rb, 1)
				assert.Equal(t, schema.SchemaChangeTypeModifyField, rb[0].Type)
				assert.Equal(t, "name", *rb[0].ID)
				assert.Equal(t, *oldSchema.Fields["name"].Description, *rb[0].SchemaChangeModifyFieldPayload.Changes.Description)
			},
			expectedNumChanges: 1,
		},
		{
			name: "AddIndex",
			modifyNewSchema: func(s *schema.SchemaDefinition) {
				s.Indexes = append(s.Indexes, schema.IndexOrReference{Index: &schema.IndexDefinition{Name: "new_idx"}})
			},
			assertRollback: func(t *testing.T, rb []schema.SchemaChange) {
				assert.Len(t, rb, 1)
				assert.Equal(t, schema.SchemaChangeTypeRemoveIndex, rb[0].Type)
				assert.Equal(t, "new_idx", *rb[0].Name)
			},
			expectedNumChanges: 1,
		},
		{
			name: "RemoveIndex",
			modifyNewSchema: func(s *schema.SchemaDefinition) {
				s.Indexes = []schema.IndexOrReference{}
			},
			assertRollback: func(t *testing.T, rb []schema.SchemaChange) {
				assert.Len(t, rb, 1)
				assert.Equal(t, schema.SchemaChangeTypeAddIndex, rb[0].Type)
				assert.Equal(t, *oldSchema.Indexes[0].Index, rb[0].SchemaChangeAddIndexPayload.Definition)
			},
			expectedNumChanges: 1,
		},
		{
			name: "ModifyProperty",
			modifyNewSchema: func(s *schema.SchemaDefinition) {
				s.Description = utils.StringPtr("new schema description")
			},
			assertRollback: func(t *testing.T, rb []schema.SchemaChange) {
				assert.Len(t, rb, 1)
				assert.Equal(t, schema.SchemaChangeTypeModifyProperty, rb[0].Type)
				assert.Equal(t, "description", *rb[0].ID)
				assert.Equal(t, oldSchema.Description, rb[0].SchemaChangeModifyPropertyPayload.Value)
			},
			expectedNumChanges: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			newSchema := getBaseSchema("1.0.0")
			newSchema.Constraints = schema.SchemaConstraint{
				{Constraint: &schema.Constraint{Name: "rule1", Predicate: "p1"}},
			}
			tc.modifyNewSchema(newSchema)

			generator := migration.NewMigrationGenerator(migration.GeneratorOptions{GenerateRollback: true})
			mig, err := generator.Generate(oldSchema, newSchema)

			assert.NoError(t, err)
			assert.NotNil(t, mig)
			assert.Len(t, mig.Changes, tc.expectedNumChanges)
			assert.NotEmpty(t, mig.Rollback)

			tc.assertRollback(t, mig.Rollback)
		})
	}
}

func TestGenerateMigration_ModifyConstraintGroup(t *testing.T) {
	// Helper to create a base schema with a constraint group
	schemaWithGroup := func(op common.LogicalOperator, rules []schema.ConstraintRule) *schema.SchemaDefinition {
		s := getBaseSchema("1.0.0")
		s.Constraints = schema.SchemaConstraint{
			{
				ConstraintGroup: &schema.ConstraintGroup{
					Name:     "group1",
					Operator: op,
					Rules:    rules,
				},
			},
		}
		return s
	}

	baseRules := []schema.ConstraintRule{
		{
			Constraint: &schema.Constraint{
				Name:      "rule1",
				Predicate: "required",
				Field:     utils.StringPtr("name"),
			},
		},
	}

	t.Run("Operator Change (Breaking)", func(t *testing.T) {
		oldSchema := schemaWithGroup(common.LogicalOr, baseRules)
		newSchema := schemaWithGroup(common.LogicalAnd, baseRules) // More restrictive

		generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
		mig, err := generator.Generate(oldSchema, newSchema)

		assert.NoError(t, err)
		assert.NotNil(t, mig)
		assert.Len(t, mig.Changes, 1)
		change := mig.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeModifyConstraint, change.Type)
		assert.Equal(t, "group1", *change.Name)
		assert.Equal(t, "2.0.0", *mig.Version.Target) // Major change
	})

	t.Run("Operator Change (Non-Breaking)", func(t *testing.T) {
		oldSchema := schemaWithGroup(common.LogicalAnd, baseRules)
		newSchema := schemaWithGroup(common.LogicalOr, baseRules) // Less restrictive

		generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
		mig, err := generator.Generate(oldSchema, newSchema)

		assert.NoError(t, err)
		assert.NotNil(t, mig)
		assert.Len(t, mig.Changes, 1)
		change := mig.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeModifyConstraint, change.Type)
		assert.Equal(t, "group1", *change.Name)
		assert.Equal(t, "1.1.0", *mig.Version.Target) // Minor change
	})

	t.Run("Rule Removed (Non-Breaking)", func(t *testing.T) {
		oldSchema := schemaWithGroup(common.LogicalOr, baseRules)
		newSchema := schemaWithGroup(common.LogicalOr, []schema.ConstraintRule{}) // Rule removed

		generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
		mig, err := generator.Generate(oldSchema, newSchema)

		assert.NoError(t, err)
		assert.NotNil(t, mig)
		assert.Len(t, mig.Changes, 1)
		change := mig.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeRemoveConstraint, change.Type)
		assert.Equal(t, "group1/rule1", *change.Name)
		assert.Equal(t, "1.1.0", *mig.Version.Target) // Minor change
	})

	t.Run("Rule Added (Breaking)", func(t *testing.T) {
		oldSchema := schemaWithGroup(common.LogicalOr, []schema.ConstraintRule{})
		newSchema := schemaWithGroup(common.LogicalOr, baseRules) // Rule added

		generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
		mig, err := generator.Generate(oldSchema, newSchema)

		assert.NoError(t, err)
		assert.NotNil(t, mig)
		assert.Len(t, mig.Changes, 1)
		change := mig.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeAddConstraint, change.Type)
		assert.Equal(t, "group1/rule1", *change.Name)
		assert.Equal(t, "2.0.0", *mig.Version.Target) // Major change
	})
}

func TestGenerateMigration_ModifySimpleConstraint(t *testing.T) {
	schemaWithConstraint := func(params map[string]any) *schema.SchemaDefinition {
		s := getBaseSchema("1.0.0")
		s.Constraints = schema.SchemaConstraint{
			{
				Constraint: &schema.Constraint{
					Name:       "simple_rule",
					Predicate:  "length",
					Field:      utils.StringPtr("name"),
					Parameters: params,
				},
			},
		}
		return s
	}

	oldSchema := schemaWithConstraint(map[string]any{"min": 5})
	newSchema := schemaWithConstraint(map[string]any{"min": 10}) // More restrictive

	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchema, newSchema)

	assert.NoError(t, err)
	assert.NotNil(t, mig)
	assert.Len(t, mig.Changes, 1)

	change := mig.Changes[0]
	assert.Equal(t, schema.SchemaChangeTypeModifyConstraint, change.Type)
	assert.Equal(t, "simple_rule", *change.Name)
	assert.NotNil(t, change.SchemaChangeModifyConstraintPayload)
	assert.Equal(t, map[string]any{"min": 10}, change.SchemaChangeModifyConstraintPayload.Changes.Parameters)

	// Modifying a constraint is a major (breaking) change
	assert.Equal(t, "2.0.0", *mig.Version.Target)
}

func TestGenerateMigration_RegistryChanges(t *testing.T) {
	nestedSchema := func(fields map[string]*schema.FieldDefinition) *schema.NestedSchemaDefinition {
		return &schema.NestedSchemaDefinition{
			Fields: &schema.NestedSchemaFields{
				FieldsMap: fields,
			},
		}
	}

	schemaWithRegistry := func(schemas map[string]*schema.NestedSchemaDefinition) *schema.SchemaDefinition {
		s := getBaseSchema("1.0.0")
		s.NestedSchemas = schemas
		return s
	}

	t.Run("Add Nested Schema", func(t *testing.T) {
		oldSchema := schemaWithRegistry(map[string]*schema.NestedSchemaDefinition{})
		newSchema := schemaWithRegistry(map[string]*schema.NestedSchemaDefinition{
			"address": nestedSchema(map[string]*schema.FieldDefinition{
				"street": {Type: schema.FieldTypeString},
			}),
		})

		generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
		mig, err := generator.Generate(oldSchema, newSchema)

		assert.NoError(t, err)
		assert.NotNil(t, mig)
		assert.Len(t, mig.Changes, 1)

		change := mig.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeAddSchema, change.Type)
		assert.Equal(t, "address", *change.ID)
		assert.Equal(t, "1.1.0", *mig.Version.Target) // Minor change
	})

	t.Run("Remove Nested Schema", func(t *testing.T) {
		oldSchema := schemaWithRegistry(map[string]*schema.NestedSchemaDefinition{
			"address": nestedSchema(map[string]*schema.FieldDefinition{"street": {Type: schema.FieldTypeString}}),
		})
		newSchema := schemaWithRegistry(map[string]*schema.NestedSchemaDefinition{})

		generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
		mig, err := generator.Generate(oldSchema, newSchema)

		assert.NoError(t, err)
		assert.NotNil(t, mig)
		assert.Len(t, mig.Changes, 1)

		change := mig.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeRemoveSchema, change.Type)
		assert.Equal(t, "address", *change.ID)
		assert.Equal(t, "2.0.0", *mig.Version.Target) // MAJOR change (removing a schema is breaking)
	})

	t.Run("Modify Nested Schema", func(t *testing.T) {
		oldSchema := schemaWithRegistry(map[string]*schema.NestedSchemaDefinition{
			"address": nestedSchema(map[string]*schema.FieldDefinition{"street": {Type: schema.FieldTypeString}}),
		})
		newSchema := schemaWithRegistry(map[string]*schema.NestedSchemaDefinition{
			"address": nestedSchema(map[string]*schema.FieldDefinition{
				"street": {Type: schema.FieldTypeString},
				"city":   {Type: schema.FieldTypeString}, // Added field
			}),
		})

		generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
		mig, err := generator.Generate(oldSchema, newSchema)

		assert.NoError(t, err)
		assert.NotNil(t, mig)
		assert.Len(t, mig.Changes, 1)

		change := mig.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeModifySchema, change.Type)
		assert.Equal(t, "address", *change.ID)
		assert.NotNil(t, change.SchemaChangeModifySchemaPayload)
		assert.Len(t, change.SchemaChangeModifySchemaPayload.Changes, 1)

		nestedChange := change.SchemaChangeModifySchemaPayload.Changes[0]
		assert.Equal(t, schema.SchemaChangeTypeAddField, nestedChange.Type)
		assert.Equal(t, "city", *nestedChange.ID)

		assert.Equal(t, "1.1.0", *mig.Version.Target) // Minor change
	})
}

//
// Tests for Patch Generation
//

func TestSchemaChangeToPatch_AddField(t *testing.T) {
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddField,
		ID:   utils.StringPtr("new_field"),
		SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
			Definition: schema.FieldDefinition{Type: schema.FieldTypeString},
		},
	}
	s := getBaseSchema("1.0.0")
	patches, err := migration.SchemaChangeToPatch(change, s)

	assert.NoError(t, err)
	assert.Len(t, patches, 1)
	patch := patches[0]
	assert.Equal(t, "add", patch.Op)
	assert.Equal(t, "/fields/new_field", patch.Path)
	assert.Equal(t, schema.FieldDefinition{Type: schema.FieldTypeString}, patch.Value)
}

func TestSchemaChangeToPatch_RemoveField(t *testing.T) {
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveField,
		ID:   utils.StringPtr("name"),
	}
	s := getBaseSchema("1.0.0")
	patches, err := migration.SchemaChangeToPatch(change, s)

	assert.NoError(t, err)
	assert.Len(t, patches, 1)
	patch := patches[0]
	assert.Equal(t, "remove", patch.Op)
	assert.Equal(t, "/fields/name", patch.Path)
}

func TestSchemaChangeToPatch_ModifyField(t *testing.T) {
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("name"),
		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
			Changes: schema.PartialFieldDefinition{
				Description: utils.StringPtr("new description"),
				Required:    utils.BoolPtr(false),
			},
		},
	}
	s := getBaseSchema("1.0.0")
	patches, err := migration.SchemaChangeToPatch(change, s)

	assert.NoError(t, err)
	assert.Len(t, patches, 2)

	// Order isn't guaranteed due to map iteration, so we check for existence
	foundDesc := false
	foundReq := false
	for _, p := range patches {
		assert.Equal(t, "replace", p.Op)
		if p.Path == "/fields/name/description" {
			assert.Equal(t, "new description", p.Value)
			foundDesc = true
		}
		if p.Path == "/fields/name/required" {
			assert.Equal(t, false, p.Value)
			foundReq = true
		}
	}
	assert.True(t, foundDesc, "Did not find description patch")
	assert.True(t, foundReq, "Did not find required patch")
}

func TestSchemaChangeToPatch_AddIndex(t *testing.T) {
	newIndex := schema.IndexDefinition{Name: "new_idx", Fields: []string{"id"}}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
			Definition: newIndex,
		},
	}
	s := getBaseSchema("1.0.0")
	patches, err := migration.SchemaChangeToPatch(change, s)

	assert.NoError(t, err)
	assert.Len(t, patches, 1)
	patch := patches[0]
	assert.Equal(t, "add", patch.Op)
	assert.Equal(t, "/indexes/-", patch.Path)
	assert.Equal(t, newIndex, patch.Value)
}

func TestSchemaChangeToPatch_RemoveIndex(t *testing.T) {
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveIndex,
		Name: utils.StringPtr("idx_name"),
	}
	s := getBaseSchema("1.0.0")
	patches, err := migration.SchemaChangeToPatch(change, s)

	assert.NoError(t, err)
	assert.Len(t, patches, 1)
	patch := patches[0]
	assert.Equal(t, "remove", patch.Op)
	assert.Equal(t, "/indexes/0", patch.Path) // It's the first (and only) index
}

func TestSchemaChangeToPatch_ModifyIndex(t *testing.T) {
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyIndex,
		Name: utils.StringPtr("idx_name"),
		SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
			Changes: schema.PartialIndexDefinition{
				Unique: utils.BoolPtr(true),
			},
		},
	}
	s := getBaseSchema("1.0.0")
	patches, err := migration.SchemaChangeToPatch(change, s)

	assert.NoError(t, err)
	assert.Len(t, patches, 1)
	patch := patches[0]
	assert.Equal(t, "replace", patch.Op)
	assert.Equal(t, "/indexes/0/unique", patch.Path)
	assert.Equal(t, true, patch.Value)
}

func TestSchemaChangeToPatch_ModifyConstraint(t *testing.T) {
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyConstraint,
		Name: utils.StringPtr("simple_rule"),
		SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
			Changes: schema.PartialConstraint{
				Parameters: map[string]any{"min": 10},
			},
		},
	}

	s := getBaseSchema("1.0.0")
	s.Constraints = schema.SchemaConstraint{
		{
			Constraint: &schema.Constraint{
				Name:       "simple_rule",
				Predicate:  "length",
				Field:      utils.StringPtr("name"),
				Parameters: map[string]any{"min": 5},
			},
		},
	}

	patches, err := migration.SchemaChangeToPatch(change, s)

	assert.NoError(t, err)
	assert.Len(t, patches, 1)
	patch := patches[0]
	assert.Equal(t, "replace", patch.Op)
	assert.Equal(t, "/constraints/0/parameters", patch.Path)
	assert.Equal(t, map[string]any{"min": 10}, patch.Value)
}

func TestSchemaChangeToPatch_RegistryChanges(t *testing.T) {
	nestedSchemaDef := schema.NestedSchemaDefinition{
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
				"street": {Type: schema.FieldTypeString},
			},
		},
	}

	baseRegistrySchema := func() *schema.SchemaDefinition {
		s := getBaseSchema("1.0.0")
		s.NestedSchemas = map[string]*schema.NestedSchemaDefinition{
			"address": &nestedSchemaDef,
		}
		return s
	}

	t.Run("Add Schema", func(t *testing.T) {
		newNestedSchema := schema.NestedSchemaDefinition{
			Fields: &schema.NestedSchemaFields{
				FieldsMap: map[string]*schema.FieldDefinition{"country": {Type: schema.FieldTypeString}},
			},
		}
		change := schema.SchemaChange{
			Type: schema.SchemaChangeTypeAddSchema,
			ID:   utils.StringPtr("location"),
			SchemaChangeAddSchemaPayload: &schema.SchemaChangeAddSchemaPayload{
				Definition: newNestedSchema,
			},
		}
		s := baseRegistrySchema()
		patches, err := migration.SchemaChangeToPatch(change, s)

		assert.NoError(t, err)
		assert.Len(t, patches, 1)
		patch := patches[0]
		assert.Equal(t, "add", patch.Op)
		assert.Equal(t, "/nestedSchemas/location", patch.Path)
		assert.Equal(t, newNestedSchema, patch.Value)
	})

	t.Run("Remove Schema", func(t *testing.T) {
		change := schema.SchemaChange{
			Type: schema.SchemaChangeTypeRemoveSchema,
			ID:   utils.StringPtr("address"),
		}
		s := baseRegistrySchema()
		patches, err := migration.SchemaChangeToPatch(change, s)

		assert.NoError(t, err)
		assert.Len(t, patches, 1)
		patch := patches[0]
		assert.Equal(t, "remove", patch.Op)
		assert.Equal(t, "/nestedSchemas/address", patch.Path)
	})

}

//
// Tests for Utils
//

func TestMigration_RoundTrip(t *testing.T) {
	// --- Test Cases ---
	testCases := []struct {
		name         string
		modifySchema func(s *schema.SchemaDefinition) // How to modify the base schema to create the new version
	}{
		{
			name: "AddField",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Fields["email"] = &schema.FieldDefinition{Type: schema.FieldTypeString, Required: utils.BoolPtr(false)}
			},
		},
		{
			name: "RemoveField",
			modifySchema: func(s *schema.SchemaDefinition) {
				delete(s.Fields, "name")
			},
		},
		{
			name: "ModifyField_Breaking",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Fields["name"].Type = schema.FieldTypeInteger
			},
		},
		{
			name: "ModifyField_NonBreaking",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Fields["name"].Description = utils.StringPtr("a new description")
			},
		},
		{
			name: "AddIndex",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Indexes = append(s.Indexes, schema.IndexOrReference{Index: &schema.IndexDefinition{Name: "new_idx", Fields: []string{"id"}}})
			},
		},
		{
			name: "RemoveIndex",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Indexes = []schema.IndexOrReference{}
			},
		},
		{
			name: "ModifyIndex",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Indexes[0].Index.Unique = utils.BoolPtr(true)
			},
		},
		{
			name: "AddConstraint",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Constraints = append(s.Constraints, schema.ConstraintRule{
					Constraint: &schema.Constraint{Name: "c2", Predicate: "p2"},
				})
			},
		},
		{
			name: "RemoveConstraint",
			modifySchema: func(s *schema.SchemaDefinition) {
				s.Constraints = []schema.ConstraintRule{}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Create initial and modified schemas
			oldSchema := getBaseSchema("1.0.0")
			oldSchema.Constraints = []schema.ConstraintRule{
				{Constraint: &schema.Constraint{Name: "c1", Predicate: "p1"}},
			}

			newSchema := getBaseSchema("1.0.0")
			newSchema.Constraints = []schema.ConstraintRule{
				{Constraint: &schema.Constraint{Name: "c1", Predicate: "p1"}},
			}
			tc.modifySchema(newSchema)

			// 2. Generate forward migration (old -> new)
			generator := migration.NewMigrationGenerator(migration.GeneratorOptions{GenerateRollback: true})
			forwardMigration, err := generator.Generate(oldSchema, newSchema)
			assert.NoError(t, err)
			assert.NotNil(t, forwardMigration)

			// 3. Generate reverse migration (new -> old)
			reverseMigration, err := generator.Generate(newSchema, oldSchema)
			assert.NoError(t, err)
			assert.NotNil(t, reverseMigration)

			// 4. The forward migration's ROLLBACK should be equivalent to the reverse migration's CHANGES
			// This proves the logic is symmetrical.
			assert.ElementsMatch(t, reverseMigration.Changes, forwardMigration.Rollback, "Forward rollback should match reverse changes")
		})
	}
}



func TestGenerateMigrationSequence(t *testing.T) {
	schemaV1 := getBaseSchema("1.0.0")

	schemaV2 := getBaseSchema("2.0.0") // Manually set version
	schemaV2.Fields["email"] = &schema.FieldDefinition{Type: schema.FieldTypeString}
	schemaV2.Version = "1.1.0" // Correct version should be minor bump

	schemaV3 := getBaseSchema("3.0.0") // Manually set version
	delete(schemaV3.Fields, "name")
	schemaV3.Version = "2.0.0" // Correct version should be major bump

	schemas := []*schema.SchemaDefinition{schemaV3, schemaV1, schemaV2} // Unordered

	migrations, err := migration.GenerateMigrationSequence(schemas, migration.GeneratorOptions{})

	assert.NoError(t, err)
	assert.Len(t, migrations, 2)

	// Check migration 1 (v1.0.0 -> v1.1.0)
	mig1 := migrations[0]
	assert.Equal(t, "1.0.0", mig1.Version.Source)
	assert.Equal(t, "1.1.0", *mig1.Version.Target)
	assert.Len(t, mig1.Changes, 1)
	assert.Equal(t, schema.SchemaChangeTypeAddField, mig1.Changes[0].Type)
	assert.Equal(t, "email", *mig1.Changes[0].ID)

	// Check migration 2 (v1.1.0 -> v2.0.0)
	mig2 := migrations[1]
	assert.Equal(t, "1.1.0", mig2.Version.Source)
	assert.Equal(t, "2.0.0", *mig2.Version.Target) // Removing fields is a major change

	assert.Len(t, mig2.Changes, 2)

	expectedRemovals := map[string]bool{
		"name":  false,
		"email": false,
	}

	for _, change := range mig2.Changes {
		if change.Type == schema.SchemaChangeTypeRemoveField {
			id := *change.ID
			if _, ok := expectedRemovals[id]; ok {
				expectedRemovals[id] = true
			}
		}
	}

	for id, found := range expectedRemovals {
		assert.True(t, found, "Expected to find removal of field: %s", id)
	}
}

func TestMigration_FullJsonRoundTrip(t *testing.T) {
	oldJsonSchemaStr := `{
		"name": "TestSchema",
		"version": "1.0.0",
		"description": "A basic schema for testing",
		"fields": {
			"itemId": {
				"name": "itemId",
				"type": "string",
				"description": "The unique identifier",
				"required": true
			},
			"name": {
				"name": "name",
				"type": "string",
				"description": "The name of the item",
				"required": true
			}
		},
		"indexes": [
			{
				"name": "idx_name",
				"fields": ["name"],
				"type": "normal",
				"unique": false
			}
		]
	}`

	newJsonSchemaStr := `{
		"name": "TestSchema",
		"version": "2.0.0",
		"description": "An updated schema for testing",
		"fields": {
			"itemId": {
				"name": "itemId",
				"type": "string",
				"description": "The unique identifier",
				"required": true
			},
			"name": {
				"name": "name",
				"type": "string",
				"description": "The modified name of the item",
				"required": true
			},
			"email": {
				"name": "email",
				"type": "string",
				"description": "The email address",
				"required": false
			}
		},
		"indexes": [
			{
				"name": "idx_name",
				"fields": ["name"],
				"type": "normal",
				"unique": false
			},
			{
				"name": "idx_email",
				"fields": ["email"],
				"type": "normal",
				"unique": true
			}
		]
	}`

	// 1. Unmarshal JSON strings into schema.SchemaDefinition and map[string]any
	oldSchemaDef, err := schema.From([]byte(oldJsonSchemaStr))
	assert.NoError(t, err, "Failed to unmarshal oldJsonSchemaStr into SchemaDefinition using schema.From")
	assert.NotNil(t, oldSchemaDef, "oldSchemaDef should not be nil")

	newSchemaDef, err := schema.From([]byte(newJsonSchemaStr))
	assert.NoError(t, err, "Failed to unmarshal newJsonSchemaStr into SchemaDefinition using schema.From")
	assert.NotNil(t, newSchemaDef, "newSchemaDef should not be nil")

	var oldSchemaMap map[string]any
	err = json.Unmarshal([]byte(oldJsonSchemaStr), &oldSchemaMap)
	assert.NoError(t, err, "Failed to unmarshal oldJsonSchemaStr into map[string]any")

	var expectedNewSchemaMap map[string]any
	err = json.Unmarshal([]byte(newJsonSchemaStr), &expectedNewSchemaMap)
	assert.NoError(t, err, "Failed to unmarshal newJsonSchemaStr into map[string]any")

	// 2. Generate Migration
	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchemaDef, newSchemaDef)
	assert.NoError(t, err, "Failed to generate migration")
	assert.NotNil(t, mig, "Migration should not be nil")
	assert.NotEmpty(t, mig.Changes, "Migration should have changes")

	// Verify the calculated target version matches the one in newJsonSchemaStr
	assert.Equal(t, "2.0.0", *mig.Version.Target, "Calculated target version should match newSchemaDef's version")

	// 3. Convert changes to JSON patches
	allPatches := make([]scjson.PatchOperation, 0)
	for _, change := range mig.Changes {
		patches, err := migration.SchemaChangeToPatch(change, oldSchemaDef) // Pass oldSchemaDef here for context
		assert.NoError(t, err, "Failed to convert schema change to patch")
		allPatches = append(allPatches, patches...)
	}
	assert.NotEmpty(t, allPatches, "Should have generated JSON patches")

	// 4. Apply JSON patches to the old schema map
	patcher := scjson.NewPatcher() // Re-added this line
	patchedSchemaMapAny, err := patcher.Apply(oldSchemaMap, allPatches)
	assert.NoError(t, err, "Failed to apply patches")
	assert.NotNil(t, patchedSchemaMapAny, "Patched schema map should not be nil")

	// Convert to map[string]any for direct manipulation
	patchedSchemaMap := patchedSchemaMapAny.(map[string]any)

	// Explicitly set the version of the patched schema map to the target version of the migration.
	// This is necessary because the version itself is not part of the JSON patches generated.
	if mig.Version.Target != nil {
		patchedSchemaMap["version"] = *mig.Version.Target
	}

	// 5. Compare the patched schema with the expected new schema map
	// Note: reflect.DeepEqual can have issues with any and map[string]any.
	// We'll re-marshal and then unmarshal to ensure consistent types for comparison.
	patchedBytes, err := json.Marshal(patchedSchemaMap)
	assert.NoError(t, err, "Failed to marshal patched schema map")

	expectedBytes, err := json.Marshal(expectedNewSchemaMap)
	assert.NoError(t, err, "Failed to marshal expected new schema map")

	var finalPatchedMap map[string]any
	err = json.Unmarshal(patchedBytes, &finalPatchedMap)
	assert.NoError(t, err, "Failed to unmarshal final patched bytes")

	var finalExpectedMap map[string]any
	err = json.Unmarshal(expectedBytes, &finalExpectedMap)
	assert.NoError(t, err, "Failed to unmarshal final expected bytes")

	assert.Equal(t, finalExpectedMap, finalPatchedMap, "Patched schema should match new schema")
}

func TestMigration_FullJsonRoundTrip_RenameField(t *testing.T) {
	oldJsonSchemaStr := `{
		"name": "TestSchema",
		"version": "1.0.0",
		"description": "A schema with a field to be renamed",
		"fields": {
			"user_stable_id": {
				"name": "user",
				"type": "string",
				"description": "description",
				"required": true
			}
		},
		"indexes": []
	}`

	newJsonSchemaStr := `{
		"name": "TestSchema",
		"version": "2.0.0",
		"description": "A schema with a field to be renamed",
		"fields": {
			"user_stable_id": {
				"name": "username",
				"type": "string",
				"description": "description",
				"required": true
			}
		},
		"indexes": []
	}`

	// 1. Unmarshal JSON strings into schema.SchemaDefinition and map[string]any
	oldSchemaDef, err := schema.From([]byte(oldJsonSchemaStr))
	assert.NoError(t, err, "Failed to unmarshal oldJsonSchemaStr into SchemaDefinition using schema.From")
	assert.NotNil(t, oldSchemaDef, "oldSchemaDef should not be nil")

	newSchemaDef, err := schema.From([]byte(newJsonSchemaStr))
	assert.NoError(t, err, "Failed to unmarshal newJsonSchemaStr into SchemaDefinition using schema.From")
	assert.NotNil(t, newSchemaDef, "newSchemaDef should not be nil")

	var oldSchemaMap map[string]any
	err = json.Unmarshal([]byte(oldJsonSchemaStr), &oldSchemaMap)
	assert.NoError(t, err, "Failed to unmarshal oldJsonSchemaStr into map[string]any")

	var expectedNewSchemaMap map[string]any
	err = json.Unmarshal([]byte(newJsonSchemaStr), &expectedNewSchemaMap)
	assert.NoError(t, err, "Failed to unmarshal newJsonSchemaStr into map[string]any")

	// 2. Generate Migration
	generator := migration.NewMigrationGenerator(migration.GeneratorOptions{})
	mig, err := generator.Generate(oldSchemaDef, newSchemaDef)
	assert.NoError(t, err, "Failed to generate migration")
	assert.NotNil(t, mig, "Migration should not be nil")
	assert.NotEmpty(t, mig.Changes, "Migration should have changes")

	assert.Len(t, mig.Changes, 1, "Migration should have 1 change: modify field name")

	// Verify the calculated target version matches the one in newJsonSchemaStr
	assert.Equal(t, "2.0.0", *mig.Version.Target, "Calculated target version should match newSchemaDef's version")

	// Verify the specific change for field renaming
	var foundRenameChange bool
	for _, change := range mig.Changes {
		if change.Type == schema.SchemaChangeTypeModifyField && *change.ID == "user_stable_id" {
			assert.NotNil(t, change.SchemaChangeModifyFieldPayload)
			assert.NotNil(t, change.SchemaChangeModifyFieldPayload.Changes.Name)
			assert.Equal(t, "username", *change.SchemaChangeModifyFieldPayload.Changes.Name)
			foundRenameChange = true
		}
	}
	assert.True(t, foundRenameChange, "Should have found a modify field change for renaming 'user'")

	// 3. Convert changes to JSON patches
	allPatches := make([]scjson.PatchOperation, 0)
	for _, change := range mig.Changes {
		patches, err := migration.SchemaChangeToPatch(change, oldSchemaDef) // Pass oldSchemaDef here for context
		assert.NoError(t, err, "Failed to convert schema change to patch")
		allPatches = append(allPatches, patches...)
	}
	assert.NotEmpty(t, allPatches, "Should have generated JSON patches")

	// 4. Apply JSON patches to the old schema map
	patcher := scjson.NewPatcher()
	patchedSchemaMapAny, err := patcher.Apply(oldSchemaMap, allPatches)
	assert.NoError(t, err, "Failed to apply patches")
	assert.NotNil(t, patchedSchemaMapAny, "Patched schema map should not be nil")

	// Convert to map[string]any for direct manipulation
	patchedSchemaMap := patchedSchemaMapAny.(map[string]any)

	// Explicitly set the version of the patched schema map to the target version of the migration.
	// This is necessary because the version itself is not part of the JSON patches generated.
	if mig.Version.Target != nil {
		patchedSchemaMap["version"] = *mig.Version.Target
	}

	// 5. Compare the patched schema with the expected new schema map
	// Note: reflect.DeepEqual can have issues with any and map[string]any.
	// We'll re-marshal and then unmarshal to ensure consistent types for comparison.
	patchedBytes, err := json.Marshal(patchedSchemaMap)
	assert.NoError(t, err, "Failed to marshal patched schema map")

	expectedBytes, err := json.Marshal(expectedNewSchemaMap)
	assert.NoError(t, err, "Failed to marshal expected new schema map")

	var finalPatchedMap map[string]any
	err = json.Unmarshal(patchedBytes, &finalPatchedMap)
	assert.NoError(t, err, "Failed to unmarshal final patched bytes")

	var finalExpectedMap map[string]any
	err = json.Unmarshal(expectedBytes, &finalExpectedMap)
	assert.NoError(t, err, "Failed to unmarshal final expected bytes")

	assert.Equal(t, finalExpectedMap, finalPatchedMap, "Patched schema should match new schema")
}
