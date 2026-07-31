package data_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
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

// TestSchemaFrom_RoundTripInlineAndNullable verifies that a DTO schema
// survives a full round trip (SchemaFrom -> definition.FromJSON -> json
// marshal) without synthesizing an id on inline schemas and without dropping
// the nullable flag.
func TestSchemaFrom_RoundTripInlineAndNullable(t *testing.T) {
	type Document struct {
		Tags []string          `anansi:"tags"`
		Meta map[string]string `anansi:"meta"`
		Note *string           `anansi:"note"`
	}

	schemaJSON, err := data.SchemaFrom[Document]()
	require.NoError(t, err)

	s, err := definition.FromJSON(schemaJSON)
	require.NoError(t, err)

	out, err := json.Marshal(s)
	require.NoError(t, err)

	var schema map[string]any
	err = json.Unmarshal(out, &schema)
	require.NoError(t, err)

	fields, ok := schema["fields"].(map[string]any)
	require.True(t, ok)

	var tagsField, metaField, noteField map[string]any
	for _, f := range fields {
		fm := f.(map[string]any)
		switch fm["name"] {
		case "tags":
			tagsField = fm
		case "meta":
			metaField = fm
		case "note":
			noteField = fm
		}
	}

	for name, field := range map[string]map[string]any{
		"tags": tagsField,
		"meta": metaField,
	} {
		require.NotNil(t, field, "field %q should exist", name)
		ref, ok := field["schema"].(map[string]any)
		require.True(t, ok, "field %q should have a schema", name)
		_, hasID := ref["id"]
		require.False(t, hasID, "inline schema for field %q must not carry an id, got: %s", name, out)
		require.Equal(t, "string", ref["type"], "field %q inline schema should keep its type", name)
	}

	require.NotNil(t, noteField, "note field should exist")
	require.Equal(t, "string", noteField["type"])
	require.Equal(t, true, noteField["nullable"], "nullable must survive the round trip, got: %s", out)
}
