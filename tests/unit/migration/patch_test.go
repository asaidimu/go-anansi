package migration_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	scjson "github.com/asaidimu/go-anansi/v6/core/json"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	mg "github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// Helper to compare JSON patch operations.
func comparePatches(t *testing.T, actual, expected []scjson.PatchOperation) {
	if len(actual) != len(expected) {
		t.Fatalf("Expected %d patches, got %d", len(expected), len(actual))
	}

	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal actual patches: %v", err)
	}
	expectedJSON, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal expected patches: %v", err)
	}

	// Basic string comparison after marshalling, order matters in JSON patch
	if string(actualJSON) != string(expectedJSON) {
		t.Errorf("Patches do not match.\nExpected:\n%s\nActual:\n%s", string(expectedJSON), string(actualJSON))
	}
}

func TestPatchConverter_Convert_UnknownChangeType(t *testing.T) {
	change := schema.SchemaChange{
		Type: "UNKNOWN_CHANGE_TYPE", // An unknown type
	}
	converter :=mg.NewPatchConverter(createTestSchema("1.0.0"))
	_, err := converter.Convert(change)

	assertSystemErrorCode(t, err, "ERR_CREATE_MIGRATION_PATCH")
}

func TestPatchConverter_convertModifyProperty_ReplaceDescription(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyProperty,
		ID:   utils.StringPtr("description"),
		SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
			Value: "New Description for Schema",
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "replace", Path: "/description", Value: "New Description for Schema"},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertAddField_Add(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	newFieldDef := schema.FieldDefinition{Name: "new_field", Type: schema.FieldTypeString, Required: utils.BoolPtr(false)}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddField,
		ID:   utils.StringPtr("new_field"),
		SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
			Definition: newFieldDef,
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "add", Path: "/fields/new_field", Value: newFieldDef},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertRemoveField_Remove(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // Has 'name' field
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveField,
		ID:   utils.StringPtr("name"),
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "remove", Path: "/fields/name"},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyField_ReplaceType(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // 'name' is FieldTypeString
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("name"),
		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
			Changes: schema.PartialFieldDefinition{
				Type: fieldTypePtr(schema.FieldTypeInteger),
			},
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "replace", Path: "/fields/name/type", Value: schema.FieldTypeInteger},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyField_AddRequired(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // 'name' is Required: true, let's make a field that's not
	baseSchema.Fields["optional_field"] = &schema.FieldDefinition{Name: "optional_field", Type: schema.FieldTypeString, Required: utils.BoolPtr(false)}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("optional_field"),
		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
			Changes: schema.PartialFieldDefinition{
				Required: utils.BoolPtr(true),
			},
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "replace", Path: "/fields/optional_field/required", Value: true},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyField_UnsetDescription(t *testing.T) {

	baseSchema := createTestSchema("1.0.0")

	baseSchema.Fields["id"].Description = utils.StringPtr("The unique ID")

	change := schema.SchemaChange{

		Type: schema.SchemaChangeTypeModifyField,

		ID:   utils.StringPtr("id"),

		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{

			Changes: schema.PartialFieldDefinition{

				Unset: []string{"description"},

			},

		},

	}

	converter := mg.NewPatchConverter(baseSchema)

	actualPatches, err := converter.Convert(change)



	if err != nil {

		t.Fatalf("Convert failed: %v", err)

	}

	expectedPatches := []scjson.PatchOperation{

		{Op: "remove", Path: "/fields/id/description"},

	}

	comparePatches(t, actualPatches, expectedPatches)

}



func TestPatchConverter_convertModifyField_UnsetDescription_PreviouslyNil(t *testing.T) {

	baseSchema := createTestSchema("1.0.0")

	// Ensure description is nil for 'id' field

	baseSchema.Fields["id"].Description = nil

	change := schema.SchemaChange{

		Type: schema.SchemaChangeTypeModifyField,

		ID:   utils.StringPtr("id"),

		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{

			Changes: schema.PartialFieldDefinition{

				Unset: []string{"description"},

			},

		},

	}

	converter := mg.NewPatchConverter(baseSchema)

	actualPatches, err := converter.Convert(change)



	if err != nil {

		t.Fatalf("Convert failed: %v", err)

	}

	// No patch should be generated if the field was already nil

	if len(actualPatches) != 0 {

		t.Errorf("Expected 0 patches, got %d", len(actualPatches))

	}

}

func TestPatchConverter_convertAddIndex_AddFirstIndex(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	baseSchema.Indexes = nil // Ensure indexes array is nil for this test
	newIndexDef := schema.IndexDefinition{Name: "new_idx", Fields: []string{"new_field"}, Type: schema.IndexTypeNormal}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
			Definition: newIndexDef,
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "add", Path: "/indexes", Value: []any{}}, // Initialize array
		{Op: "add", Path: "/indexes/-", Value: newIndexDef},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertAddIndex_AddSubsequentIndex(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // Already has an index
	newIndexDef := schema.IndexDefinition{Name: "another_idx", Fields: []string{"another_field"}, Type: schema.IndexTypeUnique, Unique: utils.BoolPtr(true)}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
			Definition: newIndexDef,
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "add", Path: "/indexes/-", Value: newIndexDef},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertRemoveIndex_RemoveExisting(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // Has 'idx_name'
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveIndex,
		Name: utils.StringPtr("idx_name"),
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "remove", Path: "/indexes/0"}, // Assuming 'idx_name' is at index 0
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyIndex_ReplaceType(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // 'idx_name' is IndexTypeNormal
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyIndex,
		Name: utils.StringPtr("idx_name"),
		SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
			Changes: schema.PartialIndexDefinition{
				Type: indexTypePtr(schema.IndexTypeUnique),
			},
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "replace", Path: "/indexes/0/type", Value: schema.IndexTypeUnique},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertAddConstraint_AddFirstConstraint(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	baseSchema.Constraints = nil // Ensure constraints array is nil for this test
	newConstraint := schema.ConstraintRule{
		Constraint: &schema.Constraint{Name: "new_cons", Predicate: "regex", Parameters: map[string]any{"pattern": "[a-z]"}},
	}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddConstraint,
		SchemaChangeAddConstraintPayload: &schema.SchemaChangeAddConstraintPayload{
			Constraint: newConstraint,
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "add", Path: "/constraints", Value: []any{}}, // Initialize array
		{Op: "add", Path: "/constraints/-", Value: newConstraint},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertRemoveConstraint_RemoveSimple(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // Has 'min_length_name'
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveConstraint,
		Name: utils.StringPtr("min_length_name"),
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "remove", Path: "/constraints/0"}, // Assuming 'min_length_name' is at index 0
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertRemoveConstraint_RemoveHierarchical(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	// Add a constraint group with a nested constraint
	baseSchema.Constraints = append(baseSchema.Constraints,
	schema.ConstraintRule{
		ConstraintGroup: &schema.ConstraintGroup{
			Name: "group1",
			Operator: common.LogicalAnd,
			Rules: []schema.ConstraintRule{
				{
					Constraint: &schema.Constraint{
Name: "nested_cons", Predicate: "min", Field: utils.StringPtr("value"), Parameters: map[string]any{"min": 10},
					},
				},
			},
		},
	})
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveConstraint,
		Name: utils.StringPtr("group1/nested_cons"),
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "remove", Path: "/constraints/1/rules/0"}, // Index 1 for group1, then rule 0
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyConstraint_ReplacePredicate(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // 'min_length_name' is 'min_length'
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyConstraint,
		Name: utils.StringPtr("min_length_name"),
		SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
			Changes: schema.PartialConstraint{
				Predicate: utils.StringPtr("max_length"),
			},
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "replace", Path: "/constraints/0/predicate", Value: "max_length"},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertAddSchema_AddNested(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	baseSchema.NestedSchemas = nil // Ensure nil for this test
	newNestedSchemaDef := schema.NestedSchemaDefinition{Name: "user_details"}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddSchema,
		ID:   utils.StringPtr("user_details"),
		SchemaChangeAddSchemaPayload: &schema.SchemaChangeAddSchemaPayload{
			Definition: newNestedSchemaDef,
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "add", Path: "/nestedSchemas", Value: map[string]any{}},
		{Op: "add", Path: "/nestedSchemas/user_details", Value: newNestedSchemaDef},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertRemoveSchema_RemoveNested(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // Has 'address' nested schema
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveSchema,
		ID:   utils.StringPtr("address"),
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "remove", Path: "/nestedSchemas/address"},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifySchema_ModifyFieldInNested(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // Has 'address' nested schema with 'street'
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifySchema,
		ID:   utils.StringPtr("address"),
		SchemaChangeModifySchemaPayload: &schema.SchemaChangeModifySchemaPayload{
			Changes: []schema.SchemaChange{
				{
					Type: schema.SchemaChangeTypeModifyField,
					ID:   utils.StringPtr("street"),
					SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
						Changes: schema.PartialFieldDefinition{
							Required: utils.BoolPtr(true),
						},
					},
				},
			},
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "add", Path: "/nestedSchemas/address/fields/street/required", Value: true},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifySchemaReference_ModifyIndexInFieldSchemaRef(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	// Add a field that references a nested schema
	baseSchema.Fields["profile"] = &schema.FieldDefinition{
		Name: "profile",
		Type: schema.FieldTypeObject,
		Schema: schema.NestedSchemaReference{
			ID: "address",
			Indexes: []schema.IndexOrReference{
				{Index: &schema.IndexDefinition{Name: "idx_street", Fields: []string{"street"}, Type: schema.IndexTypeNormal}},
			},
		},
	}
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifySchemaReference,
		SchemaChangeModifySchemaReferencePayload: &schema.SchemaChangeModifySchemaReferencePayload{
			Field: "profile",
			ID:    utils.StringPtr("address"),
			Changes: []schema.SchemaChange{
				{
					Type: schema.SchemaChangeTypeModifyIndex,
					Name: utils.StringPtr("idx_street"),
					SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
						Changes: schema.PartialIndexDefinition{
							Unique: utils.BoolPtr(true),
						},
					},
				},
			},
		},
	}
	converter :=mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "replace", Path: "/fields/profile/schema/indexes/0/unique", Value: true},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertRemoveIndex_IndexNotFound(t *testing.T) {
	baseSchema := createTestSchema("1.0.0") // Does not have "non_existent_idx"
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveIndex,
		Name: utils.StringPtr("non_existent_idx"),
	}
	converter := mg.NewPatchConverter(baseSchema)
	_, err := converter.Convert(change)

	assertSystemErrorCode(t, err, "ERR_CREATE_MIGRATION_PATCH")
}

func TestPatchConverter_convertModifyIndex_UnsetIndexFields(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	// Ensure 'idx_name' has fields defined
	baseSchema.Indexes[0].Index.Fields = []string{"field1", "field2"} // Assuming idx_name is at index 0

	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyIndex,
		Name: utils.StringPtr("idx_name"),
		SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
			Changes: schema.PartialIndexDefinition{
				Unset: []string{"fields"},
			},
		},
	}
	converter := mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "remove", Path: "/indexes/0/fields"},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyConstraint_UnsetField(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	// Assume "min_length_name" constraint has a field defined
	if baseSchema.Constraints[0].Constraint == nil {
		t.Fatalf("Expected constraint at index 0 to be a simple constraint")
	}
	baseSchema.Constraints[0].Constraint.Field = utils.StringPtr("name") // Ensure it has a field

	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyConstraint,
		Name: utils.StringPtr("min_length_name"),
		SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
			Changes: schema.PartialConstraint{
				Unset: []string{"field"},
			},
		},
	}
	converter := mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "remove", Path: "/constraints/0/field"},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyField_AddOptionalProperty(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	// Ensure the 'id' field's description is initially nil
	baseSchema.Fields["id"].Description = nil

	newDescription := "A unique identifier for the entity."
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("id"),
		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
			Changes: schema.PartialFieldDefinition{
				Description: utils.StringPtr(newDescription), // Add a new description
			},
		},
	}
	converter := mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "add", Path: "/fields/id/description", Value: newDescription},
	}
	comparePatches(t, actualPatches, expectedPatches)
}

func TestPatchConverter_convertModifyField_ReplaceOptionalProperty(t *testing.T) {
	baseSchema := createTestSchema("1.0.0")
	// Ensure the 'id' field's description is initially non-nil
	baseSchema.Fields["id"].Description = utils.StringPtr("Original description")

	newDescription := "Updated unique identifier for the entity."
	change := schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   utils.StringPtr("id"),
		SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
			Changes: schema.PartialFieldDefinition{
				Description: utils.StringPtr(newDescription), // Replace existing description
			},
		},
	}
	converter := mg.NewPatchConverter(baseSchema)
	actualPatches, err := converter.Convert(change)

	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedPatches := []scjson.PatchOperation{
		{Op: "replace", Path: "/fields/id/description", Value: newDescription},
	}
	comparePatches(t, actualPatches, expectedPatches)
}


// Helper to get a pointer to schema.IndexType
func indexTypePtr(it schema.IndexType) *schema.IndexType {
	return &it
}
