package data_test

import (
	"os"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutils.ConfigureDocumentFactory()
	os.Exit(m.Run())
}

func TestNewDocument_HasSystemMetadata(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{"foo": "bar"})
	require.NoError(t, err)

	meta, ok := doc.Metadata()
	require.True(t, ok, "document should always have metadata")
	require.Contains(t, meta, "created")
	require.Contains(t, meta, "updated")
	require.Contains(t, meta, "version")
	require.Contains(t, meta, "hash")
}

func TestMustNewDocument_FromMap_HasMetadata(t *testing.T) {
	input := map[string]any{"foo": "baz"}
	doc := data.MustNewDocument(input)

	meta, ok := doc.Metadata()
	require.True(t, ok)
	require.NotZero(t, meta["created"])
	require.NotZero(t, meta["hash"])
}

func TestFromJSON_HasMetadata(t *testing.T) {
	input := []byte(`{"hello": "world"}`)
	doc, err := data.FromJSON(input)
	require.NoError(t, err)

	meta, ok := doc.Metadata()
	require.True(t, ok)
	require.NotZero(t, meta["created"])
}

func TestDocument_TouchMetadata_UpdatesTimestampAndHash(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{"foo": "bar"})
	require.NoError(t, err)

	meta1, _ := doc.Metadata()
	created := meta1["created"]
	originalHash := meta1["hash"]

	// Wait to ensure timestamp change is visible
	time.Sleep(time.Millisecond * 5)

	err = doc.TouchMetadata()
	require.NoError(t, err)

	meta2, _ := doc.Metadata()
	require.Equal(t, created, meta2["created"], "created timestamp should not change")
	require.NotEqual(t, originalHash, meta2["hash"], "hash should be recalculated")
	createdT, _ := data.CoerceToFloat64(meta2["created"])
	updatedT, _ := data.CoerceToFloat64(meta2["updated"])
	require.True(t, updatedT > createdT, "updated should move forward")
}

func TestNormalize_RemovesNestedMetadata(t *testing.T) {
	nested, err := data.NewDocument(map[string]any{"a": 1})
	require.NoError(t, err)
	doc, err := data.NewDocument(map[string]any{
		"nested": nested,
	})
	require.NoError(t, err)

	// Force nested to have metadata
	_, ok := nested.Metadata()
	require.True(t, ok)

	normalized := doc.Normalize()

	// Root metadata remains
	_, ok = normalized.Metadata()
	require.True(t, ok)

	// Nested metadata stripped
	child := normalized["nested"].(data.Document)
	_, ok = child.Metadata()
	require.False(t, ok)
}

func TestDocument_MetadataHashingAndVerification(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{"key": "value"})
	require.NoError(t, err)

	// Get initial hash
	meta, ok := doc.Metadata()
	require.True(t, ok)
	initialHash := meta["hash"].(string)
	require.NotEmpty(t, initialHash)

	// Verify initial hash
	ok = doc.VerifyHash()
	require.True(t, ok, "initial hash should be valid")

	// Modify document data - hash should change
	doc["key"] = "new-value"
	err = doc.TouchMetadata()
	require.NoError(t, err)

	meta, ok = doc.Metadata()
	require.True(t, ok)
	newHash := meta["hash"].(string)
	require.NotEmpty(t, newHash)
	require.NotEqual(t, initialHash, newHash, "hash should change after data modification")

	ok = doc.VerifyHash()
	require.True(t, ok, "new hash should be valid")

	// Tamper with the hash and verify it fails
	meta, _ = doc.Metadata()
	meta["hash"] = "tampered-hash"
	doc.SetMetadata(meta)

	// Tamper with the version and verify it fails
	meta, _ = doc.Metadata()
	meta["version"] = 9
	doc.SetMetadata(meta)

	ok = doc.VerifyHash()
	require.False(t, ok, "tampered hash should be invalid")

	// Restore correct hash and verify it passes
	meta["version"] = 2
	meta["hash"] = newHash
	doc.SetMetadata(meta)

	ok = doc.VerifyHash()
	require.True(t, ok, "restored hash should be valid")
}
