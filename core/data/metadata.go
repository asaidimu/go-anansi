package data

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

const (
	DocumentIDField   = "_id_"
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
func DefaultMetadataSchema() *definition.NestedSchema {
	return &definition.NestedSchema{
		BaseSchema: definition.BaseSchema{
			Name: MetadataField,
			Fields: map[definition.FieldId]definition.Field{
				"v1": {
					Name:     definition.FieldName(MetadataVersion),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeNumber,
					},
				},
				"c1": {
					Name:     definition.FieldName(MetadataCreated),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"u1": {
					Name:     definition.FieldName(MetadataUpdated),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"cs1": {
					Name:     definition.FieldName(MetadataChecksum),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"s1": {
					Name:     definition.FieldName(MetadataSignature),
					Required: false,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
			},
		},
	}
}

func isReservedMetadataField(key string) bool {
	switch key {
	case MetadataCreated, MetadataUpdated, MetadataVersion, MetadataChecksum, MetadataSignature:
		return true
	default:
		return false
	}
}
