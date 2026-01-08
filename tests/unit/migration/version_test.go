package migration_test

import (
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	. "github.com/asaidimu/go-anansi/v6/core/schema/migration"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/stretchr/testify/assert"
)

func TestVersioningUtil_CalculateNextVersion_NoChangesProvided(t *testing.T) {
	vu := NewVersioningUtil()
	_, err := vu.CalculateNextVersion("1.0.0", []schema.SchemaChange{}, nil)

	assertSystemErrorCode(t, err, "ERR_NO_CHANGES")
}

func TestVersioningUtil_CalculateNextVersion_InvalidCurrentVersion(t *testing.T) {
	vu := NewVersioningUtil()
	_, err := vu.CalculateNextVersion("invalid-version", []schema.SchemaChange{
		{Type: schema.SchemaChangeTypeModifyProperty, ID: utils.StringPtr("description")},
	}, nil)

	if err == nil || !strings.Contains(err.Error(), "invalid-version") {
		t.Errorf("Expected an error containing 'invalid-version', got %v", err)
	}
}

func TestVersioningUtil_CalculateNextVersion_HighestImpactIsPATCH(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("description"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{Value: "new description"},
		},
		{
			Type: schema.SchemaChangeTypeAddIndex,
			SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
				Definition: schema.IndexDefinition{Name: "idx_patch", Fields: []string{"id"}, Type: schema.IndexTypeNormal},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)

	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.0.1" {
		t.Errorf("Expected 1.0.1, got %s", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_HighestImpactIsMINOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("description"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{Value: "new description"},
		},
		{
			Type: schema.SchemaChangeTypeAddField,
			ID:   utils.StringPtr("new_optional"),
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "new_optional", Type: schema.FieldTypeString, Required: utils.BoolPtr(false)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)

	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.1.0" {
		t.Errorf("Expected 1.1.0, got %s", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_HighestImpactIsMAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("description"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{Value: "new description"},
		},
		{
			Type: schema.SchemaChangeTypeRemoveField,
			ID:   utils.StringPtr("name"),
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)

	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected 2.0.0, got %s", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_RemoveField_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0") // Has 'name' field
	changes := []schema.SchemaChange{
		{Type: schema.SchemaChangeTypeRemoveField, ID: utils.StringPtr("name")},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for remove field, got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_AddField_RequiredWithoutDefault_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddField,
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "new_req_field", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for AddField (required without default), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_AddField_Optional_MINOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddField,
			SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
				Definition: schema.FieldDefinition{Name: "new_opt_field", Type: schema.FieldTypeString, Required: utils.BoolPtr(false)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.1.0" {
		t.Errorf("Expected '1.1.0' for AddField (optional), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_AddConstraint_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddConstraint,
			SchemaChangeAddConstraintPayload: &schema.SchemaChangeAddConstraintPayload{
				Constraint: schema.ConstraintRule{
					Constraint: &schema.Constraint{Name: "new_constraint", Predicate: "unique", Field: utils.StringPtr("id")},
				},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for AddConstraint, got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_RemoveConstraint_MINOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	// Add a constraint to oldSchema first for removal test
	oldSchema.Constraints = append(oldSchema.Constraints, schema.ConstraintRule{
		Constraint: &schema.Constraint{Name: "to_be_removed", Predicate: "some_pred"},
	})
	changes := []schema.SchemaChange{
		{Type: schema.SchemaChangeTypeRemoveConstraint, Name: utils.StringPtr("to_be_removed")},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.1.0" {
		t.Errorf("Expected '1.1.0' for RemoveConstraint, got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_AddIndex_Unique_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddIndex,
			SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
				Definition: schema.IndexDefinition{Name: "unique_idx", Fields: []string{"id"}, Type: schema.IndexTypeUnique, Unique: utils.BoolPtr(true)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for AddIndex (unique), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_AddIndex_NonUnique_PATCH(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeAddIndex,
			SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
				Definition: schema.IndexDefinition{Name: "non_unique_idx", Fields: []string{"name"}, Type: schema.IndexTypeNormal, Unique: utils.BoolPtr(false)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.0.1" {
		t.Errorf("Expected '1.0.1' for AddIndex (non-unique), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyField_TypeChange_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0") // 'name' is string
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyField,
			ID:   utils.StringPtr("name"),
			SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
				Changes: schema.PartialFieldDefinition{Type: fieldTypePtr(schema.FieldTypeInteger)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for ModifyField (type change), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyField_RequiredAdded_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	oldSchema.Fields["new_opt"] = &schema.FieldDefinition{Name: "new_opt", Type: schema.FieldTypeString, Required: utils.BoolPtr(false)}
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyField,
			ID:   utils.StringPtr("new_opt"),
			SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
				Changes: schema.PartialFieldDefinition{Required: utils.BoolPtr(true)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for ModifyField (required added), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyField_RequiredRemoved_MINOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0") // 'name' is required: true
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyField,
			ID:   utils.StringPtr("name"),
			SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
				Changes: schema.PartialFieldDefinition{Required: utils.BoolPtr(false)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.1.0" {
		t.Errorf("Expected '1.1.0' for ModifyField (required removed), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyIndex_UniqueAdded_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0") // 'idx_name' is not unique
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyIndex,
			Name: utils.StringPtr("idx_name"),
			SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
				Changes: schema.PartialIndexDefinition{Unique: utils.BoolPtr(true)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for ModifyIndex (unique added), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyIndex_UniqueRemoved_MINOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	oldSchema.Indexes[0].Index.Unique = utils.BoolPtr(true) // Make idx_name unique first
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyIndex,
			Name: utils.StringPtr("idx_name"),
			SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
				Changes: schema.PartialIndexDefinition{Unique: utils.BoolPtr(false)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.1.0" {
		t.Errorf("Expected '1.1.0' for ModifyIndex (unique removed), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyConstraint_PredicateChange_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0") // 'min_length_name' has 'min_length' predicate
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyConstraint,
			Name: utils.StringPtr("min_length_name"),
			SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
				Changes: schema.PartialConstraint{Predicate: utils.StringPtr("max_length")},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for ModifyConstraint (predicate change), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyConstraint_GroupOperatorChange_ANDtoOR_MINOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	// Add an AND group
	oldSchema.Constraints = append(oldSchema.Constraints, schema.ConstraintRule{
		ConstraintGroup: &schema.ConstraintGroup{Name: "group_op", Operator: common.LogicalAnd, Rules: []schema.ConstraintRule{}},
	})
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyConstraint,
			Name: utils.StringPtr("group_op"),
			SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
				Changes: schema.PartialConstraint{Operator: utils.PrimitivePtr(common.LogicalOr)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "1.1.0" {
		t.Errorf("Expected '1.1.0' for ModifyConstraint (AND to OR), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifyConstraint_GroupOperatorChange_ORtoAND_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	// Add an OR group
	oldSchema.Constraints = append(oldSchema.Constraints, schema.ConstraintRule{
		ConstraintGroup: &schema.ConstraintGroup{Name: "group_op", Operator: common.LogicalOr, Rules: []schema.ConstraintRule{}},
	})
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifyConstraint,
			Name: utils.StringPtr("group_op"),
			SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
				Changes: schema.PartialConstraint{Operator: utils.PrimitivePtr(common.LogicalAnd)},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for ModifyConstraint (OR to AND), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifySchema_NestedMajor_MAJOR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	// Add a nested schema with a field
	oldSchema.NestedSchemas["profile_schema"] = &schema.NestedSchemaDefinition{
		Name: "profile_schema",
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
				"email": {Name: "email", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			},
		},
	}
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifySchema,
			ID:   utils.StringPtr("profile_schema"),
			SchemaChangeModifySchemaPayload: &schema.SchemaChangeModifySchemaPayload{
				Changes: []schema.SchemaChange{
					{
						Type: schema.SchemaChangeTypeRemoveField,
						ID:   utils.StringPtr("email"), // Major change within nested schema
					},
				},
			},
		},
	}
	nextVersion, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	if err != nil {
		t.Fatalf("CalculateNextVersion failed: %v", err)
	}
	if nextVersion != "2.0.0" {
		t.Errorf("Expected '2.0.0' for ModifySchema (nested major), got '%s'", nextVersion)
	}
}

func TestVersioningUtil_CalculateNextVersion_ModifySchemaReference_UnsupportedNestedChange_ERROR(t *testing.T) {
	vu := NewVersioningUtil()
	oldSchema := createTestSchema("1.0.0")
	// Add a field that references a nested schema
	oldSchema.Fields["user_profile"] = &schema.FieldDefinition{
		Name: "user_profile", Type: schema.FieldTypeObject,
		Schema: schema.NestedSchemaReference{
			ID: "profile_schema_ref",
			Indexes: []schema.IndexOrReference{
				{Index: &schema.IndexDefinition{Name: "idx_test", Fields: []string{"some_field"}, Type: schema.IndexTypeNormal}},
			},
		},
	}
	changes := []schema.SchemaChange{
		{
			Type: schema.SchemaChangeTypeModifySchemaReference,
			SchemaChangeModifySchemaReferencePayload: &schema.SchemaChangeModifySchemaReferencePayload{
				Field: "user_profile",
				ID:    utils.StringPtr("profile_schema_ref"),
				Changes: []schema.SchemaChange{
					{
						Type: schema.SchemaChangeTypeModifyIndex, // This is the unsupported nested change type
						Name: utils.StringPtr("idx_test"),
						SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
							Changes: schema.PartialIndexDefinition{
								Unique: utils.BoolPtr(true),
							},
						},
					},
				},
			},
		},
	}
	_, err := vu.CalculateNextVersion("1.0.0", changes, oldSchema)
	assert.Error(t, err)
}
