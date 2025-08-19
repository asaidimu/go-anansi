package testutils

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// ConfigureDocumentFactory sets up the document factory with a default secret for tests.
func ConfigureDocumentFactory() {
	config := data.DocumentFactoryConfig{
		HmacSecret: []byte("a-very-secret-key-for-testing-purposes-123"),
		Providers: []data.MetadataProviderConfig{
			{
				Name: "custom",
				Schema: &schema.NestedSchemaDefinition{
					Name: "custom_meta",
					StructuredFieldsMap: map[string]*schema.FieldDefinition{
						"custom_field": {
							Name: "custom_field", Type: "string", Required: utils.BoolPtr(true),
						},
					},
				},
				Provider: func(_ context.Context, _ data.Document) (map[string]any, error) {
					return map[string]any{"custom_field": "custom_value"}, nil
				},
			},
		},
	}
	// This might be called by multiple test packages, but the factory is a singleton
	// and is designed to be configured only once.
	_ = data.ConfigureDocumentFactory(config)
}
