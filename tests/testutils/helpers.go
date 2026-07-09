package testutils

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ConfigureDocumentFactory sets up the document factory with a default secret for tests.
func ConfigureDocumentFactory(providers ...data.MetadataProviderConfig) {
	if providers == nil {
		providers = []data.MetadataProviderConfig{
			{
				Name: "custom",
				Schema: &definition.NestedSchema{
					BaseSchema: definition.BaseSchema{
						Name: "custom_meta",
						Fields: map[definition.FieldId]definition.Field{
							"019f3d5c-847c-7618-a2ea-ac43462b96f7": {
								Name: "custom_field", Required: true,
								FieldProperties: definition.FieldProperties{
									Type: definition.FieldTypeString,
								},
							},
						},
					},
				},
				Provider: func(_ context.Context, _ *data.Document) (map[string]any, error) {
					return map[string]any{"custom_field": "custom_value"}, nil
				},
			},
		}
	}
	config := data.DocumentFactoryConfig{
		Providers: providers,
	}
	// This might be called by multiple test packages, but the factory is a singleton
	// and is designed to be configured only once.
	_ = data.ConfigureDocumentFactory(config, nil)
}

