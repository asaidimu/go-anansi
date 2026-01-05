package data

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/google/uuid"
)

const (
	DocumentIDField        = "id"
	MetadataField     = "_metadata_"
	MetadataChecksum  = "checksum"
	MetadataSignature = "signature"
	MetadataVersion   = "version"
	MetadataCreated   = "created"
	MetadataUpdated   = "updated"
)

func MetadataFieldPath(field string) string {
	return fmt.Sprintf("%s.%s", MetadataField, field)
}

// DefaultMetadataSchema returns the base schema definition for the _metadata_ block.
// This schema includes the fields managed by the framework for versioning and security.
// Consumers can extend the returned object by adding their own custom field definitions
// to its Fields map before passing it into the EnrichmentOptions.
func DefaultMetadataSchema() *schema.NestedSchemaDefinition {
	return &schema.NestedSchemaDefinition{
		ID: utils.StringPtr(uuid.Must(uuid.NewV7()).String()),
		Name: MetadataField,
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
				"version": {
					Name:     MetadataVersion,
					Type:     schema.FieldTypeNumber,
					Required: utils.BoolPtr(true),
				},
				"created": {
					Name:     MetadataCreated,
					Type:     schema.FieldTypeString,
					Required: utils.BoolPtr(true),
				},
				"updated": {
					Name:     MetadataUpdated,
					Type:     schema.FieldTypeString,
					Required: utils.BoolPtr(true),
				},
				"checksum": {
					Name:     MetadataChecksum,
					Type:     schema.FieldTypeString,
					Required: utils.BoolPtr(true),
				},
				"signature": {
					Name:     MetadataSignature,
					Type:     schema.FieldTypeString,
					Required: utils.BoolPtr(false),
				},
			},
		},
	}
}
