package base

import "github.com/asaidimu/go-anansi/v6/core/schema"

// DefaultMetadataProvider is a function that generates custom metadata fields
// with their default values. It is called only during a Create operation.
// It should return a map of custom field names to their initial values.
type DefaultMetadataProvider func() (map[string]any, error)

// MetadataOptions allows consumers to define and extend the _metadata field.
// It provides a mechanism to inject custom, signed metadata into documents.
type MetadataOptions struct {
	// MetadataSchema is the complete schema for the `_metadata` block.
	// Consumers should get the default schema via DefaultMetadataSchema()
	// and add their custom fields to it before providing it here.
	MetadataSchema *schema.NestedSchemaDefinition

	// DependentSchemas is a slice of any other nested schemas that MetadataSchema
	// might reference. This is necessary for the validator to resolve the full
	// dependency tree.
	DependentSchemas []*schema.NestedSchemaDefinition

	// DefaultProvider is a function that supplies default values for custom
	// metadata fields during a Create operation.
	DefaultProvider DefaultMetadataProvider

	// HmacSecretKey is the secret key used to sign and verify the `_metadata` hash.
	// It must be a secure, private key and should not be hardcoded.
	HmacSecretKey []byte
}
