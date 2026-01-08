package data_test
import (
	"context"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	"github.com/stretchr/testify/require"
)

func TestDocumentHashingIntegrity(t *testing.T) {
	col, cleanup := testutils.SetupCollectionTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Create a document
	initialDoc, err := data.NewDocument(map[string]any{"name": "test_doc", "age": 30})
	require.NoError(t, err)
	require.NotNil(t, initialDoc)

	// 2. Insert the document
	results, err := col.CreateMany(ctx, []*data.Document{initialDoc})
	require.NoError(t, err)

	resultSet := base.CreateResultSet(results)
	require.Len(t, resultSet.Documents(), 1)
	createdDoc := resultSet.Documents()[0]

	// 3. Retrieve the document from the database
	q := query.NewQueryBuilder().Where("id").Eq(createdDoc.ID()).Build()
	readResultSet, err := col.Read(ctx, &q)
	require.NoError(t, err)
	require.Len(t, readResultSet.Data, 1)
	retrievedDoc := readResultSet.Data[0]

	// 4. Verify the hash of the retrieved document
	ok, err := retrievedDoc.VerifyHash()
	require.NoError(t, err, "Error verifying hash for retrieved document")
	require.True(t, ok, "Retrieved document hash should be valid")

	// 5. Ensure the content and metadata are consistent
	require.Equal(t, createdDoc.ID(), retrievedDoc.ID())
	require.Equal(t, createdDoc.MustGet("name"), retrievedDoc.MustGet("name"))
	require.Equal(t, createdDoc.MustGet("age"), retrievedDoc.MustGet("age"))

	// Verify metadata fields that should be preserved and updated
	initialMeta, _ := createdDoc.Metadata()
	retrievedMeta, _ := retrievedDoc.Metadata()

	require.Equal(t, initialMeta[data.MetadataChecksum], retrievedMeta[data.MetadataChecksum])
	require.Equal(t, initialMeta[data.MetadataVersion], retrievedMeta[data.MetadataVersion])
	require.Equal(t, initialMeta[data.MetadataCreated], retrievedMeta[data.MetadataCreated])
}

