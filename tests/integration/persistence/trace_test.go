package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/stretchr/testify/require"
)

func TestTraceDocumentInsertion(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	schema := registry.MustEnrichSchema(newTestSchema("trace_collection"))

	// We need to create the collection schema in the database for the interactor to work
	err := interactor.CreateCollection(context.Background(), *schema)
	require.NoError(t, err)


		docToCreate, err := data.NewDocument(map[string]any{"id": "1", "name": "trace-doc", "age": 30, "is_active": true, "price": 99.99})


		require.NoError(t, err)





		// Directly insert using the interactor


		insertedDocs, err := interactor.InsertDocuments(context.Background(), schema, []map[string]any{docToCreate.ToMap()})
	require.NoError(t, err)
	require.Len(t, insertedDocs, 1)

	// Log 2: Document After interactor.InsertDocuments
	insertedDoc := insertedDocs[0]

	// Verify the hash of the returned document
	ok, err := data.MustNewDocument(insertedDoc).VerifyHash()
	require.NoError(t, err)
	require.True(t, ok, "Hash of document returned by interactor should be valid")

	// Optionally, read it back from the database to see if it changes again
	readQuery := query.NewQueryBuilder().Alias(schema.Name).From(schema.Name).Schema(schema).Where("name").Eq("trace-doc").Build()
	readDocs, _, err := interactor.SelectDocuments(context.Background(), schema, &readQuery)
	require.NoError(t, err)
	require.Len(t, readDocs, 1)

	readDoc := readDocs[0]

	ok, err = data.MustNewDocument(readDoc).VerifyHash()
	require.NoError(t, err)
	require.NoError(t, err)
	require.True(t, ok, "Hash of document read back from DB should be valid")

}
