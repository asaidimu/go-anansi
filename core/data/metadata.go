package data

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
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

// Static UUIDv7 IDs for system entities injected by EnrichSchema.
// Using constants ensures EnrichSchema is idempotent — the same schema
// enriched twice produces identical output, which is critical for
// migration diff computation.
const (
	SystemFieldIDDocumentID = "019f4065-0a3d-7ea1-bc46-bbaeed4bfd6d"
	SystemFieldIDMetadata   = "019f4065-0a3d-7ecf-a2eb-7af6e1fdd6f0"
	SystemSchemaIDMetadata  = "019f4065-0a3d-7ed7-8ab1-417acc881135"
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
				"019f32a2-1eb3-7c39-885e-c3d545f981ac": {
					Name:     definition.FieldName(MetadataVersion),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeNumber,
					},
				},
				"019f32a2-1eb5-78b8-971d-ac164c938f2f": {
					Name:     definition.FieldName(MetadataCreated),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"019f32a2-1eb5-72a9-a0d6-086140f78a85": {
					Name:     definition.FieldName(MetadataUpdated),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"019f32a2-1eb5-7440-b104-8d774438853a": {
					Name:     definition.FieldName(MetadataChecksum),
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"019f32a2-1eb5-774f-abf3-09d64b4dbdd7": {
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
