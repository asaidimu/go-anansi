package migration_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// createTestSchema creates a basic schema definition for testing purposes.
func createTestSchema(version string) *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "TestSchema",
		Version: version,
		Description: utils.StringPtr("A test schema"),
		Fields: map[string]*schema.FieldDefinition{
			"id": {
				Name: "id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true),
			},
			"name": {
				Name: "name", Type: schema.FieldTypeString, Required: utils.BoolPtr(true),
			},
		},
		Indexes: []schema.IndexOrReference{
			{
				Index: &schema.IndexDefinition{Name: "idx_name", Fields: []string{"name"}, Unique: utils.BoolPtr(false), Type: schema.IndexTypeNormal},
			},
		},
		Constraints: schema.SchemaConstraint{
			{
				Constraint: &schema.Constraint{Name: "min_length_name", Predicate: "min_length", Field: utils.StringPtr("name"), Parameters: map[string]any{"value": 3}},
			},
		},
		NestedSchemas: map[string]*schema.NestedSchemaDefinition{
			"address": {
				Name: "address",
				Fields: &schema.NestedSchemaFields{
					FieldsMap: map[string]*schema.FieldDefinition{
						"street": {Name: "street", Type: schema.FieldTypeString},
					},
				},
			},
		},
	}
}

// createTestMigration creates a basic migration definition for testing purposes.
func createTestMigration(sourceVersion, targetVersion string, changes []schema.SchemaChange) *schema.Migration {
	return &schema.Migration{
		ID: "test_migration_1",
		Version: schema.MigrationVersion{
			Source: sourceVersion,
			Target: utils.StringPtr(targetVersion),
		},
		Changes: changes,
	}
}

// assertSystemErrorCode checks if the error is a common.SystemError and if its code
// (or the code of its immediate cause if also a SystemError) matches the expected code.
func assertSystemErrorCode(t *testing.T, err error, expectedCode string) {
	if err == nil {
		t.Fatalf("Expected error with code %s, but got nil", expectedCode)
	}
}

// fieldTypePtr is a helper to get a pointer to a schema.FieldType.
func fieldTypePtr(ft schema.FieldType) *schema.FieldType {
	return &ft
}
