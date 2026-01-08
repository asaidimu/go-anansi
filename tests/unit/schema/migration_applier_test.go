package schema_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//
// Helper Functions
//

func getBaseSchemaForApplier(version string) *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:        "ApplierTestSchema",
		Version:     version,
		Description: utils.StringPtr("A base schema for applier testing"),
		Fields: map[string]*schema.FieldDefinition{
			"stable_id_1": {
				Name:        "field1",
				Type:        schema.FieldTypeString,
				Description: utils.StringPtr("Initial field 1"),
				Required:    utils.BoolPtr(true),
			},
			"stable_id_2": {
				Name:        "field2",
				Type:        schema.FieldTypeInteger,
				Description: utils.StringPtr("Initial field 2"),
				Required:    utils.BoolPtr(false),
			},
		},
		Indexes: []schema.IndexOrReference{
			{
				Index: &schema.IndexDefinition{
					Name:   "idx_field1",
					Fields: []string{"stable_id_1"},
					Type:   "normal",
					Unique: utils.BoolPtr(false),
				},
			},
		},
		Constraints: schema.SchemaConstraint{
			{
				Constraint: &schema.Constraint{
					Name:      "cns_field1_len",
					Predicate: "minLength",
					Field:     utils.StringPtr("stable_id_1"),
					Parameters: map[string]any{
						"value": 5,
					},
				},
			},
		},
	}
}

func createApplierMigration(sourceVersion, targetVersion string, changes ...schema.SchemaChange) *schema.Migration {
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

//
// Test Cases
//

func TestApplyMigration_VersionMismatch(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.1", "1.0.2") // Source version mismatch

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	_, err := applier.ApplyMigration(sourceSchema, mig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "migration source version 1.0.1 does not match schema version 1.0.0")
}

func TestApplyMigration_NilTargetVersion(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.0", "1.0.0") // Target will be overwritten to nil for this test
	mig.Version.Target = nil

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	_, err := applier.ApplyMigration(sourceSchema, mig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "migration target version is nil")
}

func TestApplyMigration_ModifyProperty(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	newDescription := "Updated description for schema"
	mig := createApplierMigration("1.0.0", "1.0.1", schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyProperty,
		ID:   utils.StringPtr("description"),
		SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
			Value: &newDescription,
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.0.1", targetSchema.Version)
	assert.Equal(t, newDescription, *targetSchema.Description)
	assert.Equal(t, "ApplierTestSchema", targetSchema.Name) // Name should remain unchanged
}

func TestApplyMigration_AddField(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddField,
		ID:   utils.StringPtr("stable_id_3"),
		SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
			Definition: schema.FieldDefinition{
				Name:     "field3",
				Type:     schema.FieldTypeBoolean,
				Required: utils.BoolPtr(true),
			},
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	assert.Contains(t, targetSchema.Fields, "stable_id_3")
	assert.Equal(t, "field3", targetSchema.Fields["stable_id_3"].Name)
	assert.Equal(t, schema.FieldTypeBoolean, targetSchema.Fields["stable_id_3"].Type)
}

func TestApplyMigration_RemoveField(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.0", "2.0.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveField,
		ID:   utils.StringPtr("stable_id_1"),
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "2.0.0", targetSchema.Version)
	assert.NotContains(t, targetSchema.Fields, "stable_id_1")

	// Verify associated index is removed
	foundIndex := false
	for _, ior := range targetSchema.Indexes {
		if ior.IsIndex() && ior.Index.Name == "idx_field1" {
			foundIndex = true
			break
		}
	}
	assert.False(t, foundIndex, "Index 'idx_field1' should have been removed")

	// Verify associated constraint is removed
	foundConstraint := false
	for _, rule := range targetSchema.Constraints {
		if rule.IsConstraint() && rule.Constraint.Name == "cns_field1_len" {
			foundConstraint = true
			break
		}
	}
	assert.False(t, foundConstraint, "Constraint 'cns_field1_len' should have been removed")
}

func TestApplyMigration_ModifyField_Rename(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	newName := "renamedField1"
	mig := createApplierMigration("1.0.0", "2.0.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("stable_id_1"),
		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
			Changes: schema.PartialFieldDefinition{
				Name: &newName,
			},
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "2.0.0", targetSchema.Version)
	require.Contains(t, targetSchema.Fields, "stable_id_1")
	assert.Equal(t, newName, targetSchema.Fields["stable_id_1"].Name)
}

func TestApplyMigration_ModifyField_TypeChange(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	newType := schema.FieldTypeNumber
	mig := createApplierMigration("1.0.0", "2.0.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("stable_id_2"),
		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
			Changes: schema.PartialFieldDefinition{
				Type: &newType,
			},
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "2.0.0", targetSchema.Version)
	require.Contains(t, targetSchema.Fields, "stable_id_2")
	assert.Equal(t, newType, targetSchema.Fields["stable_id_2"].Type)
}

/* func TestApplyMigration_DeprecateField(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("stable_id_1"),
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	require.Contains(t, targetSchema.Fields, "stable_id_1")
	assert.True(t, *targetSchema.Fields["stable_id_1"].Deprecated)
} */

func TestApplyMigration_AddIndex(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	newIndex := schema.IndexDefinition{
		Name:   "idx_field2",
		Fields: []string{"stable_id_2"},
		Type:   "normal",
	}
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
			Definition: newIndex,
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	assert.Len(t, targetSchema.Indexes, 2)
	found := false
	for _, ior := range targetSchema.Indexes {
		if ior.IsIndex() && ior.Index.Name == newIndex.Name {
			found = true
			assert.Equal(t, newIndex.Fields, ior.Index.Fields)
			break
		}
	}
	assert.True(t, found, "New index should be present")
}

func TestApplyMigration_RemoveIndex(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.0", "2.0.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveIndex,
		Name: utils.StringPtr("idx_field1"),
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "2.0.0", targetSchema.Version)
	assert.Empty(t, targetSchema.Indexes) // Should have one index removed
}

func TestApplyMigration_ModifyIndex(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	newType := schema.IndexTypeUnique
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyIndex,
		Name: utils.StringPtr("idx_field1"),
		SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
			Changes: schema.PartialIndexDefinition{
				Type: &newType,
			},
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	require.Len(t, targetSchema.Indexes, 1)
	assert.Equal(t, newType, targetSchema.Indexes[0].Index.Type)
}

func TestApplyMigration_AddConstraint(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	newConstraint := schema.ConstraintRule{
		Constraint: &schema.Constraint{
			Name:      "cns_field2_gt0",
			Predicate: "greaterThan",
			Field:     utils.StringPtr("stable_id_2"),
			Parameters: map[string]any{
				"value": 0,
			},
		},
	}
	mig := createApplierMigration("1.0.0", "2.0.0", schema.SchemaChange{ // Adding constraint is major
		Type: schema.SchemaChangeTypeAddConstraint,
		SchemaChangeAddConstraintPayload: &schema.SchemaChangeAddConstraintPayload{
			Constraint: newConstraint,
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "2.0.0", targetSchema.Version)
	assert.Len(t, targetSchema.Constraints, 2)
	found := false
	for _, rule := range targetSchema.Constraints {
		if rule.IsConstraint() && rule.Constraint.Name == newConstraint.Constraint.Name {
			found = true
			assert.Equal(t, newConstraint.Constraint.Predicate, rule.Constraint.Predicate)
			break
		}
	}
	assert.True(t, found, "New constraint should be present")
}

func TestApplyMigration_RemoveConstraint(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{ // Removing constraint is minor
		Type: schema.SchemaChangeTypeRemoveConstraint,
		Name: utils.StringPtr("cns_field1_len"),
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	assert.Empty(t, targetSchema.Constraints) // Should have one constraint removed
}

func TestApplyMigration_ModifyConstraint(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	newParamValue := 10
	mig := createApplierMigration("1.0.0", "2.0.0", schema.SchemaChange{ // Modifying constraint is major
		Type: schema.SchemaChangeTypeModifyConstraint,
		Name: utils.StringPtr("cns_field1_len"),
		SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
			Changes: schema.PartialConstraint{
				Parameters: map[string]any{
					"value": newParamValue,
				},
			},
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "2.0.0", targetSchema.Version)
	require.Len(t, targetSchema.Constraints, 1)
	require.NotNil(t, targetSchema.Constraints[0].Constraint)
	assert.Equal(t, newParamValue, targetSchema.Constraints[0].Constraint.Parameters.(map[string]any)["value"])
}

func TestApplyMigration_AddSchema(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	nestedSchema := schema.NestedSchemaDefinition{
		Name:        "Address",
		Description: utils.StringPtr("Address details"),
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
				"street": {Name: "street", Type: schema.FieldTypeString},
			},
		},
	}
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddSchema,
		ID:   utils.StringPtr("address_id"),
		SchemaChangeAddSchemaPayload: &schema.SchemaChangeAddSchemaPayload{
			Definition: nestedSchema,
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	require.NotNil(t, targetSchema.NestedSchemas)
	require.Contains(t, targetSchema.NestedSchemas, "address_id")
	assert.Equal(t, "Address", targetSchema.NestedSchemas["address_id"].Name)
}

func TestApplyMigration_RemoveSchema(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	sourceSchema.NestedSchemas = map[string]*schema.NestedSchemaDefinition{
			"address_id": { // Corrected: provide Fields or Type
				Name:        "Address",
				Description: utils.StringPtr("Address details"),
				Fields: &schema.NestedSchemaFields{
					FieldsMap: map[string]*schema.FieldDefinition{
						"street": {Name: "street", Type: schema.FieldTypeString},
					},
				},
			},
		}
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveSchema,
		ID:   utils.StringPtr("address_id"),
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	require.NotNil(t, targetSchema.NestedSchemas)
	assert.NotContains(t, targetSchema.NestedSchemas, "address_id")
}

func TestApplyMigration_ModifySchema(t *testing.T) {
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	sourceSchema.NestedSchemas = map[string]*schema.NestedSchemaDefinition{
			"address_id": {
				Name:        "Address",
				Description: utils.StringPtr("Address details"),
				Fields: &schema.NestedSchemaFields{
					FieldsMap: map[string]*schema.FieldDefinition{
						"street": {Name: "street", Type: schema.FieldTypeString},
					},
				},
			},
	}
	newField := schema.FieldDefinition{Name: "city", Type: schema.FieldTypeString}
	mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifySchema,
		ID:   utils.StringPtr("address_id"),
		SchemaChangeModifySchemaPayload: &schema.SchemaChangeModifySchemaPayload{
			Changes: []schema.SchemaChange{
				{
					Type: schema.SchemaChangeTypeAddField,
					ID:   utils.StringPtr("city_field_id"),
					SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
						Definition: newField,
					},
				},
			},
		},
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	assert.Equal(t, "1.1.0", targetSchema.Version)
	require.NotNil(t, targetSchema.NestedSchemas)
	require.Contains(t, targetSchema.NestedSchemas, "address_id")
	nestedSchema := targetSchema.NestedSchemas["address_id"]
	require.NotNil(t, nestedSchema.Fields)
	require.Contains(t, nestedSchema.Fields.FieldsMap, "city_field_id")
	assert.Equal(t, "city", nestedSchema.Fields.FieldsMap["city_field_id"].Name)
}

func TestApplyMigration_StrictMode(t *testing.T) {
	t.Run("AddField_ExistingField", func(t *testing.T) {
		sourceSchema := getBaseSchemaForApplier("1.0.0")
		mig := createApplierMigration("1.0.0", "1.1.0", schema.SchemaChange{
			Type: schema.SchemaChangeTypeAddField,
			ID:   utils.StringPtr("stable_id_1"), // Already exists
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "field1_new", Type: schema.FieldTypeString},
			},
		})

		applier := migration.NewMigrationApplier(migration.ApplierOptions{StrictMode: true})
		_, err := applier.ApplyMigration(sourceSchema, mig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field stable_id_1 already exists")
	})

	t.Run("RemoveField_NonExistentField", func(t *testing.T) {
		sourceSchema := getBaseSchemaForApplier("1.0.0")
		mig := createApplierMigration("1.0.0", "1.0.1", schema.SchemaChange{
			Type: schema.SchemaChangeTypeRemoveField,
			ID:   utils.StringPtr("non_existent_field"),
		})

		applier := migration.NewMigrationApplier(migration.ApplierOptions{StrictMode: true})
		_, err := applier.ApplyMigration(sourceSchema, mig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field non_existent_field does not exist")
	})

	t.Run("AddIndex_ExistingIndex", func(t *testing.T) {
		sourceSchema := getBaseSchemaForApplier("1.0.0")
		mig := createApplierMigration("1.0.0", "1.0.1", schema.SchemaChange{
			Type: schema.SchemaChangeTypeAddIndex,
			SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
				Definition: schema.IndexDefinition{Name: "idx_field1", Fields: []string{"stable_id_1"}, Type: "normal"},
			},
		})

		applier := migration.NewMigrationApplier(migration.ApplierOptions{StrictMode: true})
		_, err := applier.ApplyMigration(sourceSchema, mig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "index idx_field1 already exists")
	})
}

func TestApplyMigration_ValidateResult(t *testing.T) {
	// Create a migration that removes a field, verify no error as applier cleans up indexes
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	// This would remove stable_id_1, making idx_field1 invalid without cleanup
	mig := createApplierMigration("1.0.0", "2.0.0", schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveField,
		ID:   utils.StringPtr("stable_id_1"),
	})

	applier := migration.NewMigrationApplier(migration.ApplierOptions{ValidateResult: true})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig) // Expect no error due to applier's cleanup

	require.NoError(t, err) // Assert no error
	assert.NotNil(t, targetSchema)
	// Additional checks can be added here if needed, but the primary goal is to ensure no validation error.
}

func TestApplyMigration_CleanupOrphans(t *testing.T) {
	// CleanupOrphans is a placeholder in applier.go, so this test will ensure it doesn't error
	sourceSchema := getBaseSchemaForApplier("1.0.0")
	mig := createApplierMigration("1.0.0", "1.0.1")

	applier := migration.NewMigrationApplier(migration.ApplierOptions{CleanupOrphans: true})
	targetSchema, err := applier.ApplyMigration(sourceSchema, mig)

	require.NoError(t, err)
	assert.NotNil(t, targetSchema)
	// No specific checks, just that it doesn't error
}

