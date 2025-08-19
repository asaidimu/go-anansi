package data_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/require"
)


func TestGetMetadataSchema_MergesCustomSchemas(t *testing.T) {
	// Get the merged metadata schema
	mergedSchema, _ := data.GetMetadataSchema()

	// Check that the default fields are present
	require.Contains(t, mergedSchema.StructuredFieldsMap, "version")
	require.Contains(t, mergedSchema.StructuredFieldsMap, "created")
	require.Contains(t, mergedSchema.StructuredFieldsMap, "updated")
	require.Contains(t, mergedSchema.StructuredFieldsMap, "hash")
	require.Contains(t, mergedSchema.StructuredFieldsMap, "custom_field")
}

func TestNewDocument_WithUserProvidedMetadata(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{"field": "value"})
	require.NoError(t, err)

	meta, ok := doc.Metadata()
	require.True(t, ok)
	require.Contains(t, meta, "custom_field")
	require.Equal(t, "custom_value", meta["custom_field"])
}
