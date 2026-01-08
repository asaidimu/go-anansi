package migration_test

import (
	"encoding/json"
	"reflect" // Keep reflect for other tests
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	mg "github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"

	"github.com/google/go-cmp/cmp" // Added for better comparisons
)

func TestMigrationApplier_ApplyMigration_Successful(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddField,
			ID:   utils.StringPtr("email"),
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "email", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			},
		},
		{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("description"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
				Value: "Updated schema description",
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{ValidateResult: true})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.1.0" {
		t.Errorf("Expected target schema version 1.1.0, got %s", targetSchema.Version)
	}
	if sourceSchema.Version != "1.0.0" {
		t.Errorf("Source schema version changed, expected 1.0.0, got %s", sourceSchema.Version)
	}
	if targetSchema.Fields["email"] == nil {
		t.Error("Expected field 'email' to be added to target schema")
	}
	if *targetSchema.Description != "Updated schema description" {
		t.Errorf("Expected description to be updated, got %s", *targetSchema.Description)
	}
	if targetSchema == sourceSchema {
		t.Error("Target schema should be a deep clone, not the same object as source")
	}
}

func TestMigrationApplier_ApplyMigration_NoChanges(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{}) // Empty changes

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.0.1" {
		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)
	}
	if !reflect.DeepEqual(sourceSchema.Fields, targetSchema.Fields) {
		t.Error("Fields should be identical when no changes are applied")
	}
	if !reflect.DeepEqual(sourceSchema.Indexes, targetSchema.Indexes) {
		t.Error("Indexes should be identical when no changes are applied")
	}
	sourceConstraintsJSON, _ := json.Marshal(sourceSchema.Constraints)
	targetConstraintsJSON, _ := json.Marshal(targetSchema.Constraints)
	if string(sourceConstraintsJSON) != string(targetConstraintsJSON) {
		t.Error("Constraints should be identical when no changes are applied (via JSON comparison)")
	}
	if !reflect.DeepEqual(sourceSchema.NestedSchemas, targetSchema.NestedSchemas) {
		t.Error("Nested schemas should be identical when no changes are applied")
	}
}

func TestMigrationApplier_ApplyMigration_StrictMode_AddField_AlreadyExists(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0") // Has "name" field
	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddField,
			ID:   utils.StringPtr("name"), // Field "name" already exists
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "name", Type: schema.FieldTypeString},
			},
		},
	})

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{StrictMode: true})
	_, err := applier.ApplyMigration(sourceSchema, migration)

	assertSystemErrorCode(t, err, "ERR_FIELD_ALREADY_EXISTS")
}

func TestMigrationApplier_ApplyMigration_StrictMode_RemoveField_NotFound(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeRemoveField,
			ID:   utils.StringPtr("non_existent_field"),
		},
	})

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{StrictMode: true})
	_, err := applier.ApplyMigration(sourceSchema, migration)

	assertSystemErrorCode(t, err, "ERR_FIELD_NOT_FOUND")
}

func TestMigrationApplier_ApplyMigration_VersionMismatch(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.1.0", "1.2.0", []schema.SchemaChange{}) // Migration expects 1.1.0, source is 1.0.0

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{})
	_, err := applier.ApplyMigration(sourceSchema, migration)

	sysErr, ok := err.(*common.SystemError)
	if !ok || sysErr.Code != "ERR_MIGRATION_VERSION_MISMATCH" {
		t.Errorf("Expected ERR_MIGRATION_VERSION_MISMATCH error, got %v", err)
	}
}

func TestMigrationApplier_ApplyMigration_NilTargetVersion(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := &schema.Migration{
		ID: "test_migration_1",
		Version: schema.MigrationVersion{
			Source: "1.0.0",
			Target: nil, // Nil target version
		},
		Changes: []schema.SchemaChange{},
	}

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{})
	_, err := applier.ApplyMigration(sourceSchema, migration)

	sysErr, ok := err.(*common.SystemError)
	if !ok || sysErr.Code != "ERR_MIGRATION_INVALID_TARGET_VERSION" {
		t.Errorf("Expected ERR_MIGRATION_INVALID_TARGET_VERSION error, got %v", err)
	}
}

func TestMigrationApplier_ApplyMigration_ValidateResult_IndexReferencesNonExistentField(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddIndex,
			SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
				Definition: schema.IndexDefinition{Name: "idx_non_existent", Fields: []string{"nonExistentField"}},
			},
		},
	})

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{ValidateResult: true})
	_, err := applier.ApplyMigration(sourceSchema, migration)

	assertSystemErrorCode(t, err, "ERR_INDEX_REFERENCES_NON_EXISTENT_FIELD")
}

func TestMigrationApplier_ApplyMigration_ValidateResult_ConstraintReferencesNonExistentField(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddConstraint,
			SchemaChangeAddConstraintPayload: &schema.SchemaChangeAddConstraintPayload{
				Constraint: schema.ConstraintRule{
					Constraint: &schema.Constraint{Name: "non_existent_field_constraint", Predicate: "exists", Field: utils.StringPtr("anotherNonExistentField")},
				},
			},
		},
	})

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{ValidateResult: true})
	_, err := applier.ApplyMigration(sourceSchema, migration)

	assertSystemErrorCode(t, err, "ERR_CONSTRAINT_REFERENCES_NON_EXISTENT_FIELD")
}

func TestMigrationApplier_applyRemoveField_CleansUpReferences(t *testing.T) {
	initialSchema := createTestSchema("1.0.0")
	// Add an index and a constraint that use the "name" field directly to the initial schema
	initialSchema.Indexes = append(initialSchema.Indexes, schema.IndexOrReference{
		Index: &schema.IndexDefinition{Name: "idx_on_name", Fields: []string{"name"}},
	})
	initialSchema.Constraints = append(initialSchema.Constraints, schema.ConstraintRule{
		Constraint: &schema.Constraint{Name: "name_not_empty", Predicate: "not_empty", Field: utils.StringPtr("name")},
	})

	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveField,
		ID:   utils.StringPtr("name"),
	}

	applier :=mg.NewMigrationApplier(mg.ApplierOptions{})
	// ApplyMigration will clone initialSchema, and then apply the change to the clone
	migratedSchema, err := applier.ApplyMigration(initialSchema, createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{change}))
	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if _, exists := migratedSchema.Fields["name"]; exists {
		t.Error("Field 'name' was not removed from migrated schema")
	}
	if len(migratedSchema.Indexes) != 0 {
		t.Errorf("Expected 0 indexes after removing 'name', got %d", len(migratedSchema.Indexes))
	}
	if len(migratedSchema.Constraints) != 0 {
		t.Errorf("Expected 0 constraints after removing 'name', got %d", len(migratedSchema.Constraints))
	}
	// Check specifically that 'idx_on_name' and 'name_not_empty' are gone
	for _, idx := range migratedSchema.Indexes {
		if idx.IsIndex() && idx.Index.Name == "idx_on_name" {
			t.Error("Index 'idx_on_name' was not removed")
		}
	}
	for _, constraint := range migratedSchema.Constraints {
		if constraint.IsConstraint() && constraint.Constraint.Name == "name_not_empty" {
			t.Error("Constraint 'name_not_empty' was not removed")
		}
	}
}

func TestMigrationApplier_ApplyMigration_StrictMode_IgnoresAddingExistingField(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0") // Has "name" field (string)
	originalNameFieldType := sourceSchema.Fields["name"].Type

	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddField,
			ID:   utils.StringPtr("name"), // Field "name" already exists
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "name", Type: schema.FieldTypeNumber}, // Attempt to change type to number
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{StrictMode: false})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Fields["name"] == nil {
		t.Error("Expected field 'name' to exist in target schema")
	}
	// Verify that the field type was updated as per the migration payload when StrictMode is false
	if targetSchema.Fields["name"].Type != schema.FieldTypeNumber {
		t.Errorf("Expected field 'name' type to be %s, got %s", schema.FieldTypeNumber, targetSchema.Fields["name"].Type)
	}
	if originalNameFieldType == targetSchema.Fields["name"].Type {
		t.Errorf("Field 'name' type should have changed from %s to %s, but remained the same", originalNameFieldType, targetSchema.Fields["name"].Type)
	}
}

func TestMigrationApplier_ApplyMigration_StrictMode_IgnoresRemovingNonExistentField(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0") // Does NOT have "non_existent_field"

	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeRemoveField,
			ID:   utils.StringPtr("non_existent_field"),
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{StrictMode: false})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	// Assert that the schema remains unchanged (except for version)
	if targetSchema.Version != "1.1.0" {
		t.Errorf("Expected target schema version 1.1.0, got %s", targetSchema.Version)
	}
	// Deep equality check for fields, indexes, constraints, nested schemas etc.
	// Since createTestSchema creates a base schema, we can compare relevant parts
	if !reflect.DeepEqual(sourceSchema.Fields, targetSchema.Fields) {
		t.Error("Fields should be identical when removing non-existent field with StrictMode false")
	}
	if !reflect.DeepEqual(sourceSchema.Indexes, targetSchema.Indexes) {
		t.Error("Indexes should be identical when removing non-existent field with StrictMode false")
	}
	sourceConstraintsJSON, _ := json.Marshal(sourceSchema.Constraints)
	targetConstraintsJSON, _ := json.Marshal(targetSchema.Constraints)
	if string(sourceConstraintsJSON) != string(targetConstraintsJSON) {
		t.Error("Constraints should be identical when removing non-existent field with StrictMode false")
	}
	if !reflect.DeepEqual(sourceSchema.NestedSchemas, targetSchema.NestedSchemas) {
		t.Error("Nested schemas should be identical when removing non-existent field with StrictMode false")
	}
}

func TestMigrationApplier_ApplyMigration_ModifyProperty_DescriptionChange(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	originalDescription := *sourceSchema.Description
	newDescription := "This is a new schema description."

	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("description"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
				Value: newDescription,
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.0.1" {
		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)
	}
	if *targetSchema.Description != newDescription {
		t.Errorf("Expected description to be '%s', got '%s'", newDescription, *targetSchema.Description)
	}
	if *sourceSchema.Description != originalDescription {
		t.Errorf("Source schema description should not have changed, expected '%s', got '%s'", originalDescription, *sourceSchema.Description)
	}
}

func TestMigrationApplier_ApplyMigration_ModifyProperty_UnknownProperty(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("unknownProperty"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
				Value: "some value",
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{})
	_, err := applier.ApplyMigration(sourceSchema, migration)

	assertSystemErrorCode(t, err, "ERR_UNKNOWN_PROPERTY")
}

func TestMigrationApplier_ApplyMigration_AddField_SuccessfulAddition(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0")
	newField := schema.FieldDefinition{Name: "new_field", Type: schema.FieldTypeInteger, Required: utils.BoolPtr(true)}

	migration := createTestMigration("1.0.0", "1.1.0", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddField,
			ID:   utils.StringPtr("new_field"),
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: newField,
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.1.0" {
		t.Errorf("Expected target schema version 1.1.0, got %s", targetSchema.Version)
	}
	addedField, exists := targetSchema.Fields["new_field"]
	if !exists {
		t.Error("Expected new_field to be added to target schema")
	}
	if !reflect.DeepEqual(*addedField, newField) {
		t.Errorf("Added field definition mismatch. Expected %+v, got %+v", newField, *addedField)
	}
	if sourceSchema.Fields["new_field"] != nil {
		t.Error("Source schema should not have new_field")
	}
}

func TestMigrationApplier_ApplyMigration_ModifyField_SuccessfulModification(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0") // Has 'name' field, Required: true, Type: string
	originalField := *sourceSchema.Fields["name"]

	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyField,
			ID:   utils.StringPtr("name"),
			SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
				Changes: schema.PartialFieldDefinition{
					Required: utils.BoolPtr(false), // Change required to false
					Type:     fieldTypePtr(schema.FieldTypeInteger), // Change type to integer
				},
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.0.1" {
		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)
	}
	modifiedField, exists := targetSchema.Fields["name"]
	if !exists {
		t.Fatal("Expected 'name' field to exist in target schema")
	}

	// Assert modified properties
	if *modifiedField.Required != false {
		t.Errorf("Expected 'name' field Required to be false, got %v", *modifiedField.Required)
	}
	if modifiedField.Type != schema.FieldTypeInteger {
		t.Errorf("Expected 'name' field Type to be %s, got %s", schema.FieldTypeInteger, modifiedField.Type)
	}

	// Assert other properties remain unchanged
	if modifiedField.Name != originalField.Name {
		t.Errorf("Expected 'name' field Name to be %s, got %s", originalField.Name, modifiedField.Name)
	}
	// Example: assert that Values (if present) remain unchanged if not modified
	if !cmp.Equal(modifiedField.Values, originalField.Values) {
		t.Errorf("Expected 'name' field Values to be unchanged, got %+v", modifiedField.Values)
	}
}

func TestMigrationApplier_ApplyMigration_AddIndex_SuccessfulAddition(t *testing.T) {
	// Custom sourceSchema without initial indexes
	sourceSchema := &schema.SchemaDefinition{
		Name:    "TestSchema",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"name": {Name: "name", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
		},
		Indexes:        []schema.IndexOrReference{}, // Explicitly empty
		Constraints:    []schema.ConstraintRule{},
		NestedSchemas: make(map[string]*schema.NestedSchemaDefinition),
	}
	originalSourceSchemaFields := sourceSchema.Fields // Keep a copy to compare later

	newIndex := schema.IndexDefinition{Name: "new_index", Fields: []string{"name"}, Type: schema.IndexTypeNormal}

	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddIndex,
			SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
				Definition: newIndex,
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.0.1" {
		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)
	}

	found := false
	for _, idx := range targetSchema.Indexes {
		if idx.IsIndex() && idx.Index.Name == newIndex.Name {
			if !cmp.Equal(*idx.Index, newIndex) {
				t.Errorf("Added index definition mismatch. Expected %+v, got %+v", newIndex, *idx.Index)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected new_index to be added to target schema")
	}

	// Ensure source schema indexes are unchanged (should still be empty)
	if len(sourceSchema.Indexes) != 0 {
		t.Errorf("Source schema indexes should still be empty, got %d", len(sourceSchema.Indexes))
	}
	// Also ensure other parts of the source schema are untouched
	if !reflect.DeepEqual(sourceSchema.Fields, originalSourceSchemaFields) {
		t.Error("Source schema fields should be unchanged")
	}
}

func TestMigrationApplier_ApplyMigration_RemoveIndex_SuccessfulRemoval(t *testing.T) {
	// Source schema with an initial index
	sourceSchema := createTestSchema("1.0.0") // This includes "idx_name" by default
	originalIndexCount := len(sourceSchema.Indexes)

	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeRemoveIndex,
			Name: utils.StringPtr("idx_name"), // Remove the existing index
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.0.1" {
		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)
	}

	for _, idx := range targetSchema.Indexes {
		if idx.IsIndex() && idx.Index.Name == "idx_name" {
			t.Error("Expected idx_name to be removed from target schema, but it still exists")
		}
	}
	if len(targetSchema.Indexes) != originalIndexCount-1 {
		t.Errorf("Expected %d indexes in target schema, got %d", originalIndexCount-1, len(targetSchema.Indexes))
	}

	// Ensure source schema indexes are unchanged
	if len(sourceSchema.Indexes) != originalIndexCount {
		t.Errorf("Source schema index count changed, expected %d, got %d", originalIndexCount, len(sourceSchema.Indexes))
	}
	if sourceSchema.Indexes[0].Index.Name != "idx_name" { // Assuming original first index is idx_name
		t.Error("Source schema index content changed")
	}
}

func TestMigrationApplier_ApplyMigration_ModifyIndex_SuccessfulModification(t *testing.T) {
	sourceSchema := createTestSchema("1.0.0") // Has 'idx_name' index, Unique: false, Type: Normal
	originalIndex := sourceSchema.Indexes[0].Index // Assume idx_name is the first index

	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyIndex,
			Name: utils.StringPtr("idx_name"),
			SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
				Changes: schema.PartialIndexDefinition{
					Unique: utils.BoolPtr(true), // Change Unique to true
					Type:   indexTypePtr(schema.IndexTypeFullText), // Change type to FullText
				},
			},
		},
	})

	applier := mg.NewMigrationApplier(mg.ApplierOptions{})
	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)

	if err != nil {
		t.Fatalf("ApplyMigration failed: %v", err)
	}

	if targetSchema.Version != "1.0.1" {
		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)
	}

	modifiedIndex := &schema.IndexDefinition{}
	found := false
	for _, idx := range targetSchema.Indexes {
		if idx.IsIndex() && idx.Index.Name == "idx_name" {
			modifiedIndex = idx.Index
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Expected 'idx_name' to exist in target schema")
	}

	// Assert modified properties
	if *modifiedIndex.Unique != true {
		t.Errorf("Expected 'idx_name' Unique to be true, got %v", *modifiedIndex.Unique)
	}
	if modifiedIndex.Type != schema.IndexTypeFullText {
		t.Errorf("Expected 'idx_name' Type to be %s, got %s", schema.IndexTypeFullText, modifiedIndex.Type)
	}

	// Assert other properties remain unchanged
	if modifiedIndex.Name != originalIndex.Name {
		t.Errorf("Expected 'idx_name' Name to be %s, got %s", originalIndex.Name, modifiedIndex.Name)
	}
	// Note: Fields are not modified in this partial definition, so they should remain the same.
	if !cmp.Equal(modifiedIndex.Fields, originalIndex.Fields) {
		t.Errorf("Expected 'idx_name' Fields to be unchanged, got %+v", modifiedIndex.Fields)
	}
}

func TestMigrationApplier_ApplyMigration_AddConstraint_SuccessfulAddition(t *testing.T) {

	sourceSchema := createTestSchema("1.0.0") // Has "min_length_name" already

	originalConstraintCount := len(sourceSchema.Constraints)



	newConstraint := schema.ConstraintRule{

		Constraint: &schema.Constraint{

			Name:      "new_constraint",

			Predicate: "max_value",

			Field:     utils.StringPtr("id"),

			Parameters: map[string]any{"value": 100},

		},

	}



	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{

		{

			Type: schema.SchemaChangeTypeAddConstraint,

			SchemaChangeAddConstraintPayload: &schema.SchemaChangeAddConstraintPayload{

				Constraint: newConstraint,

			},

		},

	})



	applier := mg.NewMigrationApplier(mg.ApplierOptions{})

	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)



	if err != nil {

		t.Fatalf("ApplyMigration failed: %v", err)

	}



	if targetSchema.Version != "1.0.1" {

		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)

	}



	found := false

	for _, rule := range targetSchema.Constraints {

		if rule.IsConstraint() && rule.Constraint.Name == newConstraint.Constraint.Name {

			newConstraintJSON, err := json.Marshal(newConstraint)

			if err != nil {

				t.Fatalf("Failed to marshal newConstraint to JSON: %v", err)

			}

			ruleJSON, err := json.Marshal(rule)

			if err != nil {

				t.Fatalf("Failed to marshal actual rule to JSON: %v", err)

			}



			if string(newConstraintJSON) != string(ruleJSON) {

				t.Errorf("Added constraint definition mismatch.\nExpected: %s\nActual: %s", string(newConstraintJSON), string(ruleJSON))

			}

			found = true

			break

		}

	}

	if !found {

		t.Error("Expected new_constraint to be added to target schema")

	}



	// Ensure source schema constraints are unchanged

	if len(sourceSchema.Constraints) != originalConstraintCount {

		t.Errorf("Source schema constraint count changed, expected %d, got %d", originalConstraintCount, len(sourceSchema.Constraints))

	}

}



func TestMigrationApplier_ApplyMigration_RemoveConstraint_SuccessfulRemovalSimpleName(t *testing.T) {

	sourceSchema := createTestSchema("1.0.0") // Has "min_length_name" already

	originalConstraintCount := len(sourceSchema.Constraints)



	migration := createTestMigration("1.0.0", "1.0.1", []schema.SchemaChange{

		{

			Type: schema.SchemaChangeTypeRemoveConstraint,

			Name: utils.StringPtr("min_length_name"), // Remove the existing constraint

		},

	})



	applier := mg.NewMigrationApplier(mg.ApplierOptions{})

	targetSchema, err := applier.ApplyMigration(sourceSchema, migration)



	if err != nil {

		t.Fatalf("ApplyMigration failed: %v", err)

	}



	if targetSchema.Version != "1.0.1" {

		t.Errorf("Expected target schema version 1.0.1, got %s", targetSchema.Version)

	}



	for _, rule := range targetSchema.Constraints {

		if rule.IsConstraint() && rule.Constraint.Name == "min_length_name" {

			t.Error("Expected 'min_length_name' to be removed from target schema, but it still exists")

		}

	}

	if len(targetSchema.Constraints) != originalConstraintCount-1 {

		t.Errorf("Expected %d constraints in target schema, got %d", originalConstraintCount-1, len(targetSchema.Constraints))

	}



	// Ensure source schema constraints are unchanged

	if len(sourceSchema.Constraints) != originalConstraintCount {

		t.Errorf("Source schema constraint count changed, expected %d, got %d", originalConstraintCount, len(sourceSchema.Constraints))

	}

}







