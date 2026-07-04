package meta_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/core/schema/meta"
	"github.com/stretchr/testify/require"
)

var cartSchemaJSON = `
{
  "name": "Carts",
  "version": "1.0.0",
  "fields": {
    "user_id": {
      "name": "user_id",
      "type": "string",
      "required": true
    },
    "product_ids": {
      "name": "product_ids",
      "type": "array",
      "schema": {
        "type": "string"
      },
      "required": true
    },
    "quantity": {
      "name": "quantity",
      "type": "integer",
      "required": true
    }
  }
}
`

func TestCartSchema(t *testing.T) {

	t.Run("Round-trip map matches original JSON", func(t *testing.T) {
		sc, err := definition.FromJSON([]byte(cartSchemaJSON))
		require.NoError(t, err, "Should be able to parse definition")

		asMap := sc.AsMap()

		var expected map[string]any
		err = json.Unmarshal([]byte(cartSchemaJSON), &expected)
		require.NoError(t, err)

		require.Equal(t, expected, asMap, "AsMap() should produce the same structure as the original JSON")
	})

	t.Run("Can validate schemas", func(t *testing.T) {
		vd := meta.DevelopmentSchemaValidator()
		sc, err := definition.FromJSON([]byte(cartSchemaJSON))
		require.NoError(t, err, "Should be able to parse definition")

		issues, ok := vd.Validate(sc.AsMap())

		require.Empty(t, issues, "Schema should have no validation issues against itself")
		require.True(t, ok, "Schema should be valid against itself")
	})

	t.Run("Schema deep copy preserves inline type descriptors", func(t *testing.T) {
		sc, err := definition.FromJSON([]byte(cartSchemaJSON))
		require.NoError(t, err)

		// DeepCopy is used by WithFieldEnsured during EnrichSchema
		copied := sc.DeepCopy()

		// Check that the inline type descriptor is preserved in the copy
		for _, field := range copied.Fields {
			if field.Name == "product_ids" {
				sr, err := definition.FieldSchemaAs[definition.SchemaReference](field.Schema)
				require.NoError(t, err)
				require.True(t, sr.IsInline(), "Inline type descriptor should be preserved after deep copy")
				require.Equal(t, definition.FieldTypeString, sr.Type, "Type should be preserved after deep copy")
			}
		}
	})

}
