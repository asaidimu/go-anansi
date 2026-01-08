package data_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) (base.Collection, func()) {
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
			if traceID, ok := ctx.Value(testutils.TraceIDKey).(string); ok {
				return map[string]any{"trace_id": traceID}, nil
			}
			return nil, nil
		},
	}
	data.ResetFactoryForTesting()
	testutils.ConfigureDocumentFactory(contextualProvider)
	return testutils.SetupCollectionTest(t)
}

// TestContextualMetadataIsPersisted verifies that metadata injected from the context
// during document creation is correctly persisted to and retrieved from the database.
func TestContextualMetadataIsPersisted(t *testing.T) {
	// 1. Set up persistence with the special contextual provider
	collection, cleanup := setupTest(t)
	defer cleanup()

	// 2. Create a context with the traceID
	expectedTraceID := "trace-id-for-persistence-test"
	ctxWithTraceID := context.WithValue(context.Background(), testutils.TraceIDKey, expectedTraceID)

	// 3. Create a new document using the context-aware NewDocument function
	doc, err := data.NewDocument(map[string]any{"name": "test_doc", "age": 30}, ctxWithTraceID)
	require.NoError(t, err)

	// 4. Persist the document
	createResult, err := collection.CreateOne(context.Background(), doc)
	require.NoError(t, err)
	docID := createResult.Data.ID()

	// 5. Read the document back from the database
	readQuery := query.NewQueryBuilder().Where("id").Eq(docID).Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	require.Equal(t, 1, len(readResult.Data))
	readDoc := readResult.Data[0]

	// 6. Assert that the contextual metadata was persisted and retrieved
	retrievedTraceID, err := readDoc.GetMetadataString("trace_id")
	require.NoError(t, err)
	assert.Equal(t, expectedTraceID, retrievedTraceID, "The trace_id from the context should be persisted and retrieved from the database.")
}

// TestContextIsPropagatedOnRead verifies that the context used to read documents
// is the same context attached to the documents returned from the read operation.
func TestContextIsPropagatedOnRead(t *testing.T) {
	// 1. Set up persistence
	collection, cleanup := setupTest(t)
	defer cleanup()

	// 2. Create and persist a document
	doc, err := data.NewDocument(map[string]any{"name": "test_doc", "age": 30}, context.Background())
	require.NoError(t, err)
	createResult, err := collection.CreateOne(context.Background(), doc)
	require.NoError(t, err)
	docID := createResult.Data.ID()

	// 3. Create a specific context for the read operation
	type readContextKey string
	const key = readContextKey("read-op")
	readCtx := context.WithValue(context.Background(), key, "my-read-op")

	// 4. Read the document back from the database with the specific context
	readQuery := query.NewQueryBuilder().Where("id").Eq(docID).Build()
	readResult, err := collection.Read(readCtx, &readQuery)
	require.NoError(t, err)
	require.Equal(t, 1, len(readResult.Data))

	// 5. Assert that each document in the result set has the correct context
	for _, readDoc := range readResult.Data {
		assert.Equal(t, readCtx, readDoc.Context(), "The context of the read document should be the same as the context used for the read operation.")
		retrievedValue, ok := readDoc.Context().Value(key).(string)
		require.True(t, ok, "The value from the context should be retrievable from the document's context.")
		assert.Equal(t, "my-read-op", retrievedValue, "The value from the context should be correct.")
	}
}

// TestContextIsPropagatedOnUpdateWithReturning verifies that the context used to update documents
// is the same context attached to the documents returned from the update operation.
func TestContextIsPropagatedOnUpdateWithReturning(t *testing.T) {
	// 1. Set up persistence
	collection, cleanup := setupTest(t)
	defer cleanup()

	// 2. Create and persist a document
	doc, err := data.NewDocument(map[string]any{"name": "original_name", "age": 25}, context.Background())
	require.NoError(t, err)
	createResult, err := collection.CreateOne(context.Background(), doc)
	require.NoError(t, err)
	docID := createResult.Data.ID()

	// 3. Create a specific context for the update operation
	type updateContextKey string
	const key = updateContextKey("update-op")
	updateCtx := context.WithValue(context.Background(), key, "my-update-op")

	// 4. Perform an update with returning
	updateParams := &base.CollectionUpdate{
		Filter: query.NewQueryBuilder().Where("id").Eq(docID).Build().Filters,
		Set:    data.Patch{"name": "updated_name"}.Document(),
		ReturnDocument: true, // Crucial for this test
	}
	updateResult, err := collection.Update(updateCtx, updateParams)
	require.NoError(t, err)
	require.Equal(t, 1, len(updateResult.Data))

	// 5. Assert that each returned document has the correct context
	for _, updatedDoc := range updateResult.Data {
		assert.Equal(t, updateCtx, updatedDoc.Context(), "The context of the updated document should be the same as the context used for the update operation.")
		retrievedValue, ok := updatedDoc.Context().Value(key).(string)
		require.True(t, ok, "The value from the update context should be retrievable from the document's context.")
		assert.Equal(t, "my-update-op", retrievedValue, "The value from the update context should be correct.")
	}

	// 6. Verify that the document in the database was actually updated
	readQuery := query.NewQueryBuilder().Where("id").Eq(docID).Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	require.Equal(t, 1, len(readResult.Data))
	readDoc := readResult.Data[0]
	assert.Equal(t, "updated_name", readDoc.MustGet("name"))
}
