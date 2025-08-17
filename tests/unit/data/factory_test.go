package data_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/require"
)

func TestGetMetadataSchema_MergesCustomSchemas(t *testing.T) {
	// Register a custom metadata schema
	err := data.RegisterMetadata("custom", &schema.NestedSchemaDefinition{
		StructuredFieldsMap: map[string]*schema.FieldDefinition{
			"custom_field": {
				Name: "custom_field",
				Type: schema.FieldTypeString,
			},
		},
	}, func(_ data.Document) (map[string]any, error) {
		return map[string]any{"custom_field": "custom_value"}, nil
	})
	require.NoError(t, err)

	// Get the merged metadata schema
	mergedSchema := data.GetMetadataSchema()

	// Check that the default fields are present
	require.Contains(t, mergedSchema.StructuredFieldsMap, "version")
	require.Contains(t, mergedSchema.StructuredFieldsMap, "created")
	require.Contains(t, mergedSchema.StructuredFieldsMap, "updated")
	require.Contains(t, mergedSchema.StructuredFieldsMap, "hash")

	// Check that the custom field is present
	require.Contains(t, mergedSchema.StructuredFieldsMap, "custom_field")
}

func TestRegisterMetadataSchema_FailsOnConflict(t *testing.T) {
	// Try to register a schema with a conflicting field
	err := data.RegisterMetadata("conflict", &schema.NestedSchemaDefinition{
		StructuredFieldsMap: map[string]*schema.FieldDefinition{
			"version": {
				Name: "version",
				Type: schema.FieldTypeString,
			},
		},
	}, func(_ data.Document) (map[string]any, error) {
		return map[string]any{"version": "custom_version"}, nil
	})

	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrConflictingMetadataField)
}
