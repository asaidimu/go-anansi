package data

import (
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// DefaultMetadataSchema returns the base schema definition for the _metadata block.
// This schema includes the fields managed by the framework for versioning and security.
// Consumers can extend the returned object by adding their own custom field definitions
// to its Fields map before passing it into the EnrichmentOptions.
func DefaultMetadataSchema() *schema.NestedSchemaDefinition {
	IsStructured := true
	return &schema.NestedSchemaDefinition{
		Name: MetadataFieldName,
		IsStructured: &IsStructured,
		StructuredFieldsMap: map[string]*schema.FieldDefinition{
			"version": {
				Name:     "version",
				Type:     schema.FieldTypeInteger,
				Required: utils.BoolPtr(true),
			},
			"created": {
				Name:     "created",
				Type:     schema.FieldTypeString,
				Required: utils.BoolPtr(true),
			},
			"updated": {
				Name:     "updated",
				Type:     schema.FieldTypeString,
				Required: utils.BoolPtr(true),
			},
			"hash": {
				Name:     "hash",
				Type:     schema.FieldTypeString,
				Required: utils.BoolPtr(true),
			},
		},
	}
}
