package data_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContextualMetadataProvider verifies that a metadata provider can inject
// data from the context into a document's metadata upon its creation.
func TestContextualMetadataProvider(t *testing.T) {
	// 1. Define a context-aware metadata provider
	contextualProvider := data.MetadataProviderConfig{
		Name: "contextual",
		Schema: &definition.NestedSchema{
			BaseSchema: definition.BaseSchema{
				Name: "contextual_meta",
				Fields: map[definition.FieldId]definition.Field{
					"trace_id": {Name: "trace_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				},
			},
		},
		Provider: func(ctx context.Context, _ *data.Document) (map[string]any, error) {
			if traceID, ok := ctx.Value(testutils.TraceIDKey).(string); ok {
				return map[string]any{"trace_id": traceID}, nil
			}
			return nil, nil
		},
	}

	// 2. Configure the DocumentFactory with this provider
	testutils.ConfigureDocumentFactory(contextualProvider)

	// 3. Create a context with a traceID
	expectedTraceID := "trace-id-12345"
	ctx := context.WithValue(context.Background(), testutils.TraceIDKey, expectedTraceID)

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
