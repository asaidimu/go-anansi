package data_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetMetadataSchema_MergesCustomSchemas(t *testing.T) {
	// Get the merged metadata schema
	mergedSchema, _ := data.GetMetadataSchema()

	// Check that the default fields are present
	require.Contains(t, mergedSchema.Fields.FieldsMap, "version")
	require.Contains(t, mergedSchema.Fields.FieldsMap, "created")
	require.Contains(t, mergedSchema.Fields.FieldsMap, "updated")
	require.Contains(t, mergedSchema.Fields.FieldsMap, "checksum")
	require.Contains(t, mergedSchema.Fields.FieldsMap, "custom_field")
}

func TestNewDocument_WithUserProvidedMetadata(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{"field": "value"})
	require.NoError(t, err)

	meta, ok := doc.Metadata()
	require.True(t, ok)
	require.Contains(t, meta, "custom_field")
	require.Equal(t, "custom_value", meta["custom_field"])
}

func TestNewDocument_IDEnforcement(t *testing.T) {
	t.Run("it should add an id to a new document", func(t *testing.T) {
		doc, err := data.NewDocument(map[string]any{"name": "test"})
		require.NoError(t, err)

		id, ok := doc["id"].(string)
		require.True(t, ok)
		require.NotEmpty(t, id)

		_, err = uuid.Parse(id)
		require.NoError(t, err)
	})

	t.Run("it should overwrite an existing id", func(t *testing.T) {
		originalID := "user-provided-id"
		doc, err := data.NewDocument(map[string]any{"id": originalID, "name": "test"})
		require.NoError(t, err)

		id, ok := doc["id"].(string)
		require.True(t, ok)
		require.NotEqual(t, originalID, id)

		_, err = uuid.Parse(id)
		require.NoError(t, err)
	})
}
