package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromMap(t *testing.T) {
	testCases := []struct {
		name     string
		input    map[string]any
		expected *schema.SchemaDefinition
		err      bool // true if error is expected, false otherwise
	}{
		{
			name: "valid schema map",
			input: map[string]any{
				"name":    "TestSchema",
				"version": "1", // Corrected: Version is string
				"fields": map[string]any{
					"field1": map[string]any{
						"name": "field1",
						"type": "string",
					},
				},
			},
			expected: &schema.SchemaDefinition{
				Name:    "TestSchema",
				Version: "1", // Corrected: Version is string
				Fields: map[string]*schema.FieldDefinition{ // Corrected: Map key is string
					"field1": {
						Name: "field1",
						Type: "string",
					},
				},
			},
			err: false,
		},
		{
			name: "nil input map",
			input: nil,
			expected: nil,
			err:      true, // Expecting an error
		},
		{
			name: "invalid schema map (missing name and version are empty string)",
			input: map[string]any{
				// name and version are missing, which means they will be empty strings after unmarshalling
			},
			expected: &schema.SchemaDefinition{
				Name:    "",      // Expected to be empty string
				Version: "",      // Expected to be empty string
				Fields:  nil,
			},
			err: false, // No error expected from json.Unmarshal for missing string fields
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := schema.FromMap(tc.input)

			if tc.err {
				require.Error(t, err)
				assert.Nil(t, s)
			} else {
				require.NoError(t, err)
				require.NotNil(t, s)
				// Deep equality check for SchemaDefinition might be complex due to unexported fields or specific comparison logic.
				// For now, we'll check key fields.
				assert.Equal(t, tc.expected.Name, s.Name)
				assert.Equal(t, tc.expected.Version, s.Version)
				assert.Len(t, s.Fields, len(tc.expected.Fields))
				if len(tc.expected.Fields) > 0 {
					assert.Equal(t, tc.expected.Fields["field1"].Name, s.Fields["field1"].Name)
					assert.Equal(t, tc.expected.Fields["field1"].Type, s.Fields["field1"].Type)
				}
			}
		})
	}
}

// Helper function to convert a map to SchemaDefinition for expected value comparison
// This assumes the schema.FromMap is working correctly, which is what we are testing.
// A more robust test would manually construct the expected SchemaDefinition.
func newSchemaDefinitionFromMap(t *testing.T, data map[string]any) *schema.SchemaDefinition {
	s, err := schema.FromMap(data)
	require.NoError(t, err)
	return s
}
