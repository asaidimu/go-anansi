package migration_test

import (
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	. "github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"

	"github.com/google/go-cmp/cmp" // Added for better comparisons
)

func TestMigrationGenerator_Generate_NoChangesDetected(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration != nil {
		t.Errorf("Expected no migration for no changes, got %v", migration)
	}
}

func TestMigrationGenerator_Generate_SingleFieldAdded(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["email"] = &schema.FieldDefinition{Name: "email", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)}

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if migration.Version.Source != "1.0.0" {
		t.Errorf("Expected source version 1.0.0, got %s", migration.Version.Source)
	}
	if *migration.Version.Target != "2.0.0" { // Major bump for adding a required field without default
		t.Errorf("Expected target version 2.0.0, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeAddField {
		t.Errorf("Expected change type AddField, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "email" {
		t.Errorf("Expected field ID 'email', got %s", *migration.Changes[0].ID)
	}
}

func TestMigrationGenerator_Generate_SingleFieldRemoved(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	delete(newSchema.Fields, "name") // Remove an existing field

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if *migration.Version.Target != "2.0.0" { // Major bump for removing a field
		t.Errorf("Expected target version 2.0.0, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeRemoveField {
		t.Errorf("Expected change type RemoveField, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "name" {
		t.Errorf("Expected field ID 'name', got %s", *migration.Changes[0].ID)
	}
}

func TestMigrationGenerator_Generate_FieldModified(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["name"].Required = utils.BoolPtr(false) // Modify an existing field

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if *migration.Version.Target != "1.1.0" { // Minor bump for making field optional
		t.Errorf("Expected target version 1.1.0, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeModifyField {
		t.Errorf("Expected change type ModifyField, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "name" {
		t.Errorf("Expected field ID 'name', got %s", *migration.Changes[0].ID)
	}
	if *migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Required != false {
		t.Errorf("Expected Required change to false, got %v", *migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Required)
	}
}

func TestMigrationGenerator_Generate_SchemaNameMismatch(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Name = "DifferentSchemaName" // Mismatch in name

	generator := NewMigrationGenerator(GeneratorOptions{})
	_, err := generator.Generate(oldSchema, newSchema)

	assertSystemErrorCode(t, err, "ERR_SCHEMA_NAME_MISMATCH")
}

func TestMigrationGenerator_Generate_RollbackGeneratedForAddField(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["email"] = &schema.FieldDefinition{Name: "email", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)}

	generator := NewMigrationGenerator(GeneratorOptions{GenerateRollback: true})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if len(migration.Rollback) != 1 {
		t.Fatalf("Expected 1 rollback change, got %d", len(migration.Rollback))
	}
	if migration.Rollback[0].Type != schema.SchemaChangeTypeRemoveField {
		t.Errorf("Expected rollback change type RemoveField, got %s", migration.Rollback[0].Type)
	}
	if *migration.Rollback[0].ID != "email" {
		t.Errorf("Expected rollback field ID 'email', got %s", *migration.Rollback[0].ID)
	}
}

func TestMigrationGenerator_GenerateMigrationSequence_ValidSequence(t *testing.T) {
	// Base schema 1.0.0
	schemaV1_0_0 := createTestSchema("1.0.0")
	schemaV1_0_0.Description = utils.StringPtr("Initial schema") // Set a distinct description

	// Schema 1.1.0: Derived from 1.0.0, adds an optional field (minor bump)
	schemaV1_1_0_base := createTestSchema("1.0.0") // Get a fresh clone
	schemaV1_1_0_base.Description = utils.StringPtr("Initial schema")
	schemaV1_1_0 := &schema.SchemaDefinition{}
	*schemaV1_1_0 = *schemaV1_1_0_base // Shallow copy
	schemaV1_1_0.Version = "1.1.0"
	schemaV1_1_0.Fields = make(map[string]*schema.FieldDefinition)
	for k, v := range schemaV1_1_0_base.Fields { // Deep copy fields map
		fieldCopy := *v
		schemaV1_1_0.Fields[k] = &fieldCopy
	}
	schemaV1_1_0.Fields["email"] = &schema.FieldDefinition{Name: "email", Type: schema.FieldTypeString, Required: utils.BoolPtr(false)}

	// Schema 1.1.1: Derived from 1.1.0, modifies description (patch bump)
	schemaV1_1_1 := &schema.SchemaDefinition{}
	*schemaV1_1_1 = *schemaV1_1_0 // Shallow copy
	schemaV1_1_1.Version = "1.1.1"
	schemaV1_1_1.Fields = make(map[string]*schema.FieldDefinition)
	for k, v := range schemaV1_1_0.Fields { // Deep copy fields map
		fieldCopy := *v
		schemaV1_1_1.Fields[k] = &fieldCopy
	}
	schemaV1_1_1.Description = utils.StringPtr("Updated schema description")

	schemas := []*schema.SchemaDefinition{schemaV1_0_0, schemaV1_1_0, schemaV1_1_1}
	options := GeneratorOptions{GenerateRollback: true} // Also test rollback generation

	migrations, err := GenerateMigrationSequence(schemas, options)

	if err != nil {
		t.Fatalf("GenerateMigrationSequence failed: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("Expected 2 migrations, got %d", len(migrations))
	}

	// Migration 1: 1.0.0 -> 1.1.0 (AddField: email)
	if migrations[0].Version.Source != "1.0.0" || *migrations[0].Version.Target != "1.1.0" {
		t.Errorf("Migration 0: Expected version 1.0.0 -> 1.1.0, got %s -> %s", migrations[0].Version.Source, *migrations[0].Version.Target)
	}
	if len(migrations[0].Changes) != 1 || migrations[0].Changes[0].Type != schema.SchemaChangeTypeAddField {
		t.Errorf("Migration 0: Expected 1 AddField change, got %v", migrations[0].Changes)
	}
	if len(migrations[0].Rollback) != 1 || migrations[0].Rollback[0].Type != schema.SchemaChangeTypeRemoveField {
		t.Errorf("Migration 0: Expected 1 RemoveField rollback, got %v", migrations[0].Rollback)
	}

	// Migration 2: 1.1.0 -> 1.1.1 (ModifyProperty: description)
	if migrations[1].Version.Source != "1.1.0" || *migrations[1].Version.Target != "1.1.1" {
		t.Errorf("Migration 1: Expected version 1.1.0 -> 1.1.1, got %s -> %s", migrations[1].Version.Source, *migrations[1].Version.Target)
	}
	if len(migrations[1].Changes) != 1 || migrations[1].Changes[0].Type != schema.SchemaChangeTypeModifyProperty {
		t.Errorf("Migration 1: Expected 1 ModifyProperty change, got %v", migrations[1].Changes)
	}
	if len(migrations[1].Rollback) != 1 || migrations[1].Rollback[0].Type != schema.SchemaChangeTypeModifyProperty {
		t.Errorf("Migration 1: Expected 1 ModifyProperty rollback, got %v", migrations[1].Rollback)
	}
}

func TestMigrationGenerator_GenerateMigrationSequence_InsufficientSchemas(t *testing.T) {
	schemas := []*schema.SchemaDefinition{createTestSchema("1.0.0")} // Only one schema
	options := GeneratorOptions{}

	_, err := GenerateMigrationSequence(schemas, options)

	assertSystemErrorCode(t, err, "ERR_INSUFFICIENT_SCHEMAS")
}

func TestMigrationGenerator_Generate_ChecksumGeneration(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["new_field"] = &schema.FieldDefinition{Name: "new_field", Type: schema.FieldTypeString}

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if migration.Checksum == "" {
		t.Error("Expected checksum to be generated, but it's empty")
	}

	// Verify checksum consistency (generating the same migration twice should yield the same checksum)
	// Note: Because migration ID and CreatedAt are time-based, they will differ.
	// To truly test checksum consistency, one would need to control time or make ID/CreatedAt deterministic.
	// For now, we just check it's non-empty.
}

func TestMigrationGenerator_Generate_ModifyProperty(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Description = utils.StringPtr("A new description")

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if *migration.Version.Target != "1.0.1" { // Patch bump for description change
		t.Errorf("Expected target version 1.0.1, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeModifyProperty {
		t.Errorf("Expected change type ModifyProperty, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "description" {
		t.Errorf("Expected property ID 'description', got %s", *migration.Changes[0].ID)
	}
	if *migration.Changes[0].SchemaChangeModifyPropertyPayload.Value.(*string) != "A new description" {
		t.Errorf("Expected property value 'A new description', got %s", *migration.Changes[0].SchemaChangeModifyPropertyPayload.Value.(*string))
	}
}

func TestMigrationGenerator_Generate_RollbackGeneratedForRemoveField(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	delete(newSchema.Fields, "name")

	generator := NewMigrationGenerator(GeneratorOptions{GenerateRollback: true})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if len(migration.Rollback) != 1 {
		t.Fatalf("Expected 1 rollback change, got %d", len(migration.Rollback))
	}
	if migration.Rollback[0].Type != schema.SchemaChangeTypeAddField {
		t.Errorf("Expected rollback change type AddField, got %s", migration.Rollback[0].Type)
	}
	if *migration.Rollback[0].ID != "name" {
		t.Errorf("Expected rollback field ID 'name', got %s", *migration.Rollback[0].ID)
	}
	if migration.Rollback[0].SchemaChangeAddFieldPayload.Definition.Name != "name" {
		t.Errorf("Expected rollback field definition name 'name', got %s", migration.Rollback[0].SchemaChangeAddFieldPayload.Definition.Name)
	}
}

func TestMigrationGenerator_Generate_RollbackGeneratedForModifyField(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["name"].Required = utils.BoolPtr(false) // Modify an existing field

	generator := NewMigrationGenerator(GeneratorOptions{GenerateRollback: true})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if len(migration.Rollback) != 1 {
		t.Fatalf("Expected 1 rollback change, got %d", len(migration.Rollback))
	}
	if migration.Rollback[0].Type != schema.SchemaChangeTypeModifyField {
		t.Errorf("Expected rollback change type ModifyField, got %s", migration.Rollback[0].Type)
	}
	if *migration.Rollback[0].ID != "name" {
		t.Errorf("Expected rollback field ID 'name', got %s", *migration.Rollback[0].ID)
	}
	if *migration.Rollback[0].SchemaChangeModifyFieldPayload.Changes.Required != true {
		t.Errorf("Expected rollback required change to true, got %v", *migration.Rollback[0].SchemaChangeModifyFieldPayload.Changes.Required)
	}
}

func TestMigrationGenerator_Generate_IgnoreMetadata(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Metadata = map[string]any{"author": "test"}

	generator := NewMigrationGenerator(GeneratorOptions{IgnoreMetadata: true})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration != nil {
		t.Errorf("Expected no migration when ignoring metadata, got %v", migration)
	}
}

// Ensure the ID and CreatedAt fields are generated time-based (non-deterministic)
// and Checksum is calculated based on current state.
// This test is more about ensuring the structure is filled, rather than deterministic values.
func TestMigrationGenerator_Generate_MetadataFields(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["new_field"] = &schema.FieldDefinition{Name: "new_field", Type: schema.FieldTypeString}

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}

	if migration.ID == "" {
		t.Error("Migration ID should not be empty")
	}
	if _, err := time.Parse(time.RFC3339, migration.CreatedAt); err != nil {
		t.Errorf("Migration CreatedAt format invalid: %v", err)
	}
	if migration.Status != "pending" {
		t.Errorf("Migration Status expected 'pending', got '%s'", migration.Status)
	}
	if migration.Transform != "" {
		t.Errorf("Migration Transform expected empty, got '%s'", migration.Transform)
	}
}

func TestMigrationGenerator_Generate_FieldTypeModified(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["name"].Type = schema.FieldTypeInteger // Change type from string to integer

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if *migration.Version.Target != "2.0.0" { // Major bump for type change
		t.Errorf("Expected target version 2.0.0, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeModifyField {
		t.Errorf("Expected change type ModifyField, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "name" {
		t.Errorf("Expected field ID 'name', got %s", *migration.Changes[0].ID)
	}
	if migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Type == nil {
		t.Fatal("Expected Type change in partial field definition, got nil")
	}
	if *migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Type != schema.FieldTypeInteger {
		t.Errorf("Expected Type change to %s, got %s", schema.FieldTypeInteger, *migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Type)
	}
}

func TestMigrationGenerator_Generate_ModifyProperty_MetadataChange(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	oldSchema.Metadata = map[string]any{"author": "old author", "version": float64(1)}

	newSchema := createTestSchema("1.0.0")
	newSchema.Metadata = map[string]any{"author": "new author", "version": float64(2), "new_key": "new_value"}

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if *migration.Version.Target != "1.0.1" { // Patch bump for metadata change
		t.Errorf("Expected target version 1.0.1, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeModifyProperty {
		t.Errorf("Expected change type ModifyProperty, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "metadata" {
		t.Errorf("Expected property ID 'metadata', got %s", *migration.Changes[0].ID)
	}

	// Compare metadata map content
	expectedMetadata := map[string]any{"author": "new author", "version": float64(2), "new_key": "new_value"} // JSON unmarshals numbers to float64
	actualMetadata, ok := migration.Changes[0].SchemaChangeModifyPropertyPayload.Value.(map[string]any)
	if !ok {
		t.Fatalf("Expected metadata value to be map[string]any, got %T", migration.Changes[0].SchemaChangeModifyPropertyPayload.Value)
	}
	if !cmp.Equal(actualMetadata, expectedMetadata) {
		t.Errorf("Metadata mismatch.\nExpected: %+v\nActual: %+v", expectedMetadata, actualMetadata)
	}
}

func TestMigrationGenerator_Generate_ModifyField_RequiredAdded(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	oldSchema.Fields["name"].Required = utils.BoolPtr(false) // Make 'name' optional initially

	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["name"].Required = utils.BoolPtr(true) // Make 'name' required

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if *migration.Version.Target != "2.0.0" { // Major bump for making a field required
		t.Errorf("Expected target version 2.0.0, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeModifyField {
		t.Errorf("Expected change type ModifyField, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "name" {
		t.Errorf("Expected field ID 'name', got %s", *migration.Changes[0].ID)
	}
	if migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Required == nil {
		t.Fatal("Expected Required change in partial field definition, got nil")
	}
	if *migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Required != true {
		t.Errorf("Expected Required change to true, got %v", *migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Required)
	}
}

func TestMigrationGenerator_Generate_ModifyField_RequiredRemoved(t *testing.T) {
	oldSchema := createTestSchema("1.0.0")
	oldSchema.Fields["name"].Required = utils.BoolPtr(true) // Make 'name' required initially

	newSchema := createTestSchema("1.0.0")
	newSchema.Fields["name"].Required = nil // Make 'name' optional (unset)

	generator := NewMigrationGenerator(GeneratorOptions{})
	migration, err := generator.Generate(oldSchema, newSchema)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if migration == nil {
		t.Fatal("Expected migration, got nil")
	}
	if *migration.Version.Target != "1.1.0" { // Minor bump for making a field optional
		t.Errorf("Expected target version 1.1.0, got %s", *migration.Version.Target)
	}
	if len(migration.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(migration.Changes))
	}
	if migration.Changes[0].Type != schema.SchemaChangeTypeModifyField {
		t.Errorf("Expected change type ModifyField, got %s", migration.Changes[0].Type)
	}
	if *migration.Changes[0].ID != "name" {
		t.Errorf("Expected field ID 'name', got %s", *migration.Changes[0].ID)
	}
	if !cmp.Equal(migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Unset, []string{"required"}) {
		t.Errorf("Expected Unset to contain 'required', got %+v", migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Unset)
	}
	if migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Required != nil {
		t.Errorf("Expected Required to be nil for Unset operation, got %v", migration.Changes[0].SchemaChangeModifyFieldPayload.Changes.Required)
	}
}