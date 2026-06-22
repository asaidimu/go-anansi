package meta_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/core/schema/meta"
	"github.com/stretchr/testify/require"
)

func TestMetaSchema(t *testing.T) {
	// 1. AsMap should match unmarshalled data
	t.Run("AsMap matches unmarshalled ToJSON", func(t *testing.T) {
		marshaledJSON := meta.MetaSchema.ToJSON()
		asMap := meta.MetaSchema.AsMap()

		var unmarshaled map[string]any
		err := json.Unmarshal(marshaledJSON, &unmarshaled)

		require.NoError(t, err, "ToJSON should produce valid JSON")
		require.Equal(t, unmarshaled, asMap, "The map representation should match the unmarshalled JSON")
	})

	// 2. AsMap when marshalled should match ToJSON
	t.Run("Marshalled AsMap matches ToJSON output", func(t *testing.T) {
		asMap := meta.MetaSchema.AsMap()
		marshaledAsMap, err := json.Marshal(asMap)
		require.NoError(t, err)

		// Note: We unmarshal both into maps for comparison to avoid
		// false negatives due to JSON key ordering.
		var expected, actual any
		require.NoError(t, json.Unmarshal(meta.MetaSchema.ToJSON(), &expected))
		require.NoError(t, json.Unmarshal(marshaledAsMap, &actual))

		require.Equal(t, expected, actual, "Marshaling the map should result in the same JSON structure")
	})

	// 3. The meta schema should be valid against itself
	t.Run("Self-Validation", func(t *testing.T) {
		vd, err := definition.NewDocumentValidator(&meta.MetaSchema, meta.MetaSchemaPredicates)
		require.NoError(t, err, "Should be able to create a validator from MetaSchema")

		asMap := meta.MetaSchema.AsMap()
		issues, ok := vd.Validate(asMap)

		require.Empty(t, issues, "Schema should have no validation issues against itself")
		require.True(t, ok, "Schema should be valid against itself")
	})
}

func TestMetaSchema_EnforcesEnumSchema(t *testing.T) {
	// A schema where a field of type enum is missing its required schema reference.
	// This should be caught by the newly enabled MetaSchema constraints.
	invalidSchema := definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "InvalidEnumSchema",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {
					Name: "policy",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeEnum,
						// Schema reference is missing!
					},
				},
			},
		},
	}

	vd, err := definition.NewDocumentValidator(&meta.MetaSchema, meta.MetaSchemaPredicates)
	require.NoError(t, err)

	issues, ok := vd.Validate(invalidSchema.AsMap())
	require.False(t, ok, "MetaSchema should reject enum without schema reference")

	found := false
	for _, issue := range issues {
		if issue.Code == "ENUM_MISSING_SCHEMA" {
			found = true
			t.Logf("Caught expected validation issue: %s", issue.Message)
		}
	}
	require.True(t, found, "Should have found a constraint violation for the missing enum schema")
}

