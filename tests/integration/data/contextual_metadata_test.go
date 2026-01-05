package data_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type contextKey string

const traceIDKey contextKey = "traceID"

// TestContextualMetadataProvider verifies that a metadata provider can inject
// data from the context into a document's metadata upon its creation.
func TestContextualMetadataProvider(t *testing.T) {
	// 1. Define a context-aware metadata provider
	contextualProvider := data.MetadataProviderConfig{
		Name: "contextual",
		Schema: &schema.NestedSchemaDefinition{
			Name: "contextual_meta",
			Fields: &schema.NestedSchemaFields{
				FieldsMap: map[string]*schema.FieldDefinition{
					"trace_id": {Name: "trace_id", Type: "string"},
				},
			},
		},
		Provider: func(ctx context.Context, _ *data.Document) (map[string]any, error) {
			if traceID, ok := ctx.Value(traceIDKey).(string); ok {
				return map[string]any{"trace_id": traceID}, nil
			}
			return nil, nil
		},
	}

	// 2. Configure the DocumentFactory with this provider
	// We do this specifically for this test to ensure isolation.
	config := data.DocumentFactoryConfig{
		Providers: []data.MetadataProviderConfig{contextualProvider},
	}
	logger, _ := zap.NewDevelopment()
	err := data.ConfigureDocumentFactory(config, logger)
	// We expect an error if the factory is already configured by another test package's TestMain.
	// This is acceptable, as the factory is a singleton and we can't re-configure it.
	// The key is that a provider is configured. We assume the test runner runs this test
	// in a context where a compatible configuration is active.
	if err != nil && err != data.ErrFactoryAlreadyConfigured {
		require.NoError(t, err)
	}

	// 3. Create a context with a traceID
	expectedTraceID := "trace-id-12345"
	ctx := context.WithValue(context.Background(), traceIDKey, expectedTraceID)

	// 4. Create a new document, passing the context directly.
	// The document factory will now use this context to run the providers.
	doc, err := data.NewDocument(map[string]any{"field": "value"}, ctx)
	require.NoError(t, err)

	// 5. Assert that the contextual metadata is present
	retrievedTraceID, err := doc.GetMetadataString("trace_id")
	require.NoError(t, err)
	assert.Equal(t, expectedTraceID, retrievedTraceID, "The trace_id from the context should be injected into the document's metadata.")

	// 6. Test with a context that does not have the value
	ctxWithoutTraceID := context.Background()
	doc2, err := data.NewDocument(map[string]any{"field": "value2"}, ctxWithoutTraceID)
	require.NoError(t, err)

	_, err = doc2.GetMetadataString("trace_id")
	assert.Error(t, err, "Should return an error as trace_id is not expected to be in the metadata")
}
