package data_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/stretchr/testify/require"
)

func TestSchemaFrom_InlineArraySchemaHasNoID(t *testing.T) {
	type Document struct {
		Tags []string `anansi:"tags"`
	}

	schemaJSON, err := data.SchemaFrom[Document]()
	require.NoError(t, err)

	var schema map[string]any
	err = json.Unmarshal(schemaJSON, &schema)
	require.NoError(t, err)

	fields, ok := schema["fields"].(map[string]any)
	require.True(t, ok)
	require.Len(t, fields, 1)

	var tagsField map[string]any
	for _, f := range fields {
		fm := f.(map[string]any)
		if fm["name"] == "tags" {
			tagsField = fm
			break
		}
	}
	require.NotNil(t, tagsField)
	require.Equal(t, "array", tagsField["type"])

	// Rule 20 (Form 3): an inline element type must not carry an id.
	schemaRef, ok := tagsField["schema"].(map[string]any)
	require.True(t, ok)
	_, hasID := schemaRef["id"]
	require.False(t, hasID, "inline array schema must not emit an id, got: %s", schemaJSON)
	require.Equal(t, "string", schemaRef["type"])
}
