package migration_test

import (
	"context"
	"encoding/json" // Added for json.Marshal and json.Unmarshal
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"

	scjson "github.com/asaidimu/go-anansi/v6/core/json"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	mg "github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

func TestDefaultMigrationEngine_Diff_Successful(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["description"] = &schema.FieldDefinition{Name: "description", Type: schema.FieldTypeString}

	engine := mg.NewDefaultMigrationEngine(mg.GeneratorOptions{}, mg.ApplierOptions{})
	migration, err := engine.Diff(*oldSchema, *newSchema)

	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeAddField {
		t.Errorf("Expected AddField change, got %s", migration.Changes[0].Type)
	}
}

func TestDefaultMigrationEngine_Diff_NoChanges(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")

	engine := mg.NewDefaultMigrationEngine(mg.GeneratorOptions{}, mg.ApplierOptions{})
	migration, err := engine.Diff(*oldSchema, *newSchema)

	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if migration != nil {
		t.Errorf("Expected no migration for no changes, got %v", migration)
	}
}

func TestDefaultMigrationEngine_Apply_Successful(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddField,
			ID:   utils.StringPtr("newField"),
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "newField", Type: schema.FieldTypeString},
			},
		},
	})

	engine :=mg.NewDefaultMigrationEngine(mg.GeneratorOptions{},mg.ApplierOptions{})
	resultSchema, err := engine.Apply(baseSchema, migration)

	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if resultSchema.Version != "1.1.0" {
		t.Errorf("Expected result schema version 1.1.0, got %s", resultSchema.Version)
	}
	if resultSchema.Fields["newField"] == nil {
		t.Error("Expected newField to be present in result schema")
	}
}

func TestDefaultMigrationEngine_Patch_Successful(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	patches := []scjson.PatchOperation{
		{Op: "add", Path: "/fields/age", Value: map[string]any{"name": "age", "type": "integer", "required": true}},
		{Op: "replace", Path: "/description", Value: "Patched schema"},
	}

	engine :=mg.NewDefaultMigrationEngine(mg.GeneratorOptions{},mg.ApplierOptions{})
	patchedSchema, err := engine.Patch(baseSchema, patches)

	if err != nil {
		sysErr, ok := err.(*common.SystemError)
		if ok && sysErr.Code == "ERR_SCHEMA_FROM" && sysErr.Cause != nil {
				t.Logf("Patch failed (ERR_SCHEMA_FROM): %v. Cause: %v", err, sysErr.ToIssue())
		}
		t.Fatalf("Patch failed: %v", err)
	}
	if patchedSchema.Fields["age"] == nil {
		t.Error("Expected field 'age' to be added")
	}
	if *patchedSchema.Description != "Patched schema" {
		t.Errorf("Expected description 'Patched schema', got '%s'", *patchedSchema.Description)
	}
}

func TestDefaultMigrationEngine_Patch_Invalid(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	// Invalid path for add operation
	patches := []scjson.PatchOperation{
		{Op: "add", Path: "/nonExistentPath/field", Value: "value"},
	}

	engine :=mg.NewDefaultMigrationEngine(mg.GeneratorOptions{},mg.ApplierOptions{})
	_, err := engine.Patch(baseSchema, patches)

	assertSystemErrorCode(t, err, "ERR_JSON_PATCH_APPLY")
}

func TestDefaultMigrationEngine_Plan_InPlaceStrategy(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	targetSchema := createTestSchema("1.0.1")
	targetSchema.Description = utils.StringPtr("New Description")

	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("description"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
				Value: "New Description",
			},
		},
	})

	engine :=mg.NewDefaultMigrationEngine(mg.GeneratorOptions{},mg.ApplierOptions{})
	plan, err := engine.Plan(context.Background(), sourceSchema, targetSchema, migration)

	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if plan.Strategy != mg.MigrationStrategyInPlace {
		t.Errorf("Expected InPlace strategy, got %s", plan.Strategy)
	}
	if len(plan.Changes) != 0 {
		t.Fatalf("Expected 1 change in plan, got %d", len(plan.Changes))
	}

}

func TestDefaultMigrationEngine_Plan_BlueGreenStrategy(t *testing.T) {
	tests := []struct {
		name         string
		sourceSchema *schema.SchemaDefinition
		targetSchema *schema.SchemaDefinition
		migration    *schema.Migration
	}{
		{
			name:         "RemoveField",
			sourceSchema: createTestSchema("1.0.0"),
			targetSchema: func() *schema.SchemaDefinition {
				s := createTestSchema("1.1.0")
				delete(s.Fields, "name")
				return s
			}(),
			migration: createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
				{
					Type: schema.SchemaChangeTypeRemoveField,
					ID:   utils.StringPtr("name"),
				},
			}),
		},
		{
			name:         "AddField_RequiredWithoutDefault",
			sourceSchema: createTestSchema("1.0.0"),
			targetSchema: func() *schema.SchemaDefinition {
				s := createTestSchema("1.1.0")
				s.Fields["new_required"] = &schema.FieldDefinition{Name: "new_required", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)}
				return s
			}(),
			migration: createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
				{
					Type: schema.SchemaChangeTypeAddField,
					ID:   utils.StringPtr("new_required"),
					SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
						Definition: schema.FieldDefinition{Name: "new_required", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
					},
				},
			}),
		},
		{
			name:         "ModifyField_TypeChange",
			sourceSchema: createTestSchema("1.0.0"),
			targetSchema: func() *schema.SchemaDefinition {
				s := createTestSchema("1.1.0")
				s.Fields["name"].Type = schema.FieldTypeInteger
				return s
			}(),
			migration: createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
				{
					Type: schema.SchemaChangeTypeModifyField,
					ID:   utils.StringPtr("name"),
					SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
						Changes: schema.PartialFieldDefinition{
							Type: fieldTypePtr(schema.FieldTypeInteger),
						},
					},
				},
			}),
		},
	}

	engine :=mg.NewDefaultMigrationEngine(mg.GeneratorOptions{},mg.ApplierOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := engine.Plan(context.Background(), tt.sourceSchema, tt.targetSchema, tt.migration)

			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			if plan.Strategy !=mg.MigrationStrategyBlueGreen {
				t.Errorf("Expected BlueGreen strategy, got %s", plan.Strategy)
			}
			if len(plan.Changes) != 0 {
				t.Errorf("Expected 0 changes in plan for BlueGreen strategy, got %d", len(plan.Changes))
			}
		})
	}
}

func TestDefaultMigrationEngine_Plan_NilSourceSchemaInput(t *testing.T) {
	engine := mg.NewDefaultMigrationEngine(mg.GeneratorOptions{}, mg.ApplierOptions{})
	// Assuming targetSchema and migration are not nil for this test's focus
	targetSchema := createTestSchema("1.1.0")
	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{})

	_, err := engine.Plan(context.Background(), nil, targetSchema, migration)

	assertSystemErrorCode(t, err, "ERR_PLAN_NIL_SOURCE_SCHEMA")
}

func TestDefaultMigrationEngine_Apply_ErrorPropagation(t *testing.T) {
	// Setup: Create a base schema
	baseSchema := createTestSchema("1.0.0")

	// Setup: Create a migration that would cause an error in the applier (e.g., version mismatch)
	// The applier expects baseSchema.Version to match migration.Version.Source
	// Here we make them mismatch (1.0.0 vs 1.1.0)
	migration := createTestMigration("1.1.0", "1.2.0", []schema.SchemaChange{})

	// Create an engine with default options
	engine := mg.NewDefaultMigrationEngine(mg.GeneratorOptions{}, mg.ApplierOptions{})

	// Attempt to apply the migration
	_, err := engine.Apply(baseSchema, migration)

	// Assert that an error is returned and it's the expected applier error
	assertSystemErrorCode(t, err, "ERR_MIGRATION_VERSION_MISMATCH")
}

func TestDefaultMigrationEngine_Patch_CleanupEmptyCollectionsEffect(t *testing.T) {
	// Create a schema that has some collections which we'll make empty
	baseSchema := &schema.SchemaDefinition{
		Name:    "TestSchema",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
		},
		Indexes: []schema.IndexOrReference{
			{Index: &schema.IndexDefinition{Name: "idx_1", Fields: []string{"id"}}},
		},
		Constraints: schema.SchemaConstraint{
			{Constraint: &schema.Constraint{Name: "const_1", Predicate: "exists", Field: utils.StringPtr("id")}},
		},
		NestedSchemas: map[string]*schema.NestedSchemaDefinition{
			"address": {Name: "address", Fields: &schema.NestedSchemaFields{
				FieldsMap: map[string]*schema.FieldDefinition{"street": {Name: "street", Type: schema.FieldTypeString}},
			}},
		},
	}

	// Patches to remove all fields, indexes, constraints, and nested schemas
	patches := []scjson.PatchOperation{
		{Op: "remove", Path: "/fields/id"},
		{Op: "remove", Path: "/fields/name"},
		{Op: "remove", Path: "/indexes/0"},
		{Op: "remove", Path: "/constraints/0"},
		{Op: "remove", Path: "/nestedSchemas/address"},
	}

	engine := mg.NewDefaultMigrationEngine(mg.GeneratorOptions{}, mg.ApplierOptions{})
	patchedSchema, err := engine.Patch(baseSchema, patches)
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Marshal the patched schema to JSON and check if empty collections are absent
	patchedJSON, err := json.Marshal(patchedSchema)
	if err != nil {
		t.Fatalf("Failed to marshal patched schema to JSON: %v", err)
	}

	patchedMap := make(map[string]any)
	if err := json.Unmarshal(patchedJSON, &patchedMap); err != nil {
		t.Fatalf("Failed to unmarshal patched JSON to map: %v", err)
	}

	// Assert that "fields", "indexes", "constraints", "nestedSchemas" keys are absent or empty
	// when they are expected to be removed by omitempty
	if _, ok := patchedMap["fields"]; ok {
		t.Errorf("Expected 'fields' to be cleaned up (absent from JSON), but it's present: %+v", patchedMap["fields"])
	}
	if _, ok := patchedMap["indexes"]; ok {
		t.Errorf("Expected 'indexes' to be cleaned up (absent from JSON), but it's present: %+v", patchedMap["indexes"])
	}
	if _, ok := patchedMap["constraints"]; ok {
		t.Errorf("Expected 'constraints' to be cleaned up (absent from JSON), but it's present: %+v", patchedMap["constraints"])
	}
	if _, ok := patchedMap["nestedSchemas"]; ok {
		t.Errorf("Expected 'nestedSchemas' to be cleaned up (absent from JSON), but it's present: %+v", patchedMap["nestedSchemas"])
	}

	// To make it more robust, also check the empty objects that are left behind by nested schemas for FieldsMap
	// For instance, if a NestedSchemaFields becomes empty, it should be marshalled as null or absent.
	// This is often handled by custom JSON marshallers or by the omitempty on the FieldsMap struct.
	// Since the SchemaDefinition struct has omitempty tags, directly checking the marshaled map is a good way.
}

func TestDefaultMigrationEngine_Transform_NotImplementedError(t *testing.T) {
	engine := mg.NewDefaultMigrationEngine(mg.GeneratorOptions{}, mg.ApplierOptions{})
	// Transform method expects old and new schemas, but the error should occur before processing them.
	_, err := engine.Transform(context.Background(), nil, nil, "") // Pass "" for direction

	assertSystemErrorCode(t, err, "ERR_NOT_IMPLEMENTED")
}

