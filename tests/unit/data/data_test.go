package data_test

import (
	"os"
	"testing"

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
	require.NotNil(t, doc)

	meta, ok := doc.Metadata()
	require.True(t, ok, "document should always have metadata")
	require.Contains(t, meta, "created")
	require.Contains(t, meta, "updated")
	require.Contains(t, meta, "version")
	require.Contains(t, meta, "checksum")
}

func TestMustNewDocument_FromMap_HasMetadata(t *testing.T) {
	input := map[string]any{"foo": "baz"}
	doc := data.MustNewDocument(input)
	require.NotNil(t, doc)

	meta, ok := doc.Metadata()
	require.True(t, ok)
	require.NotZero(t, meta["created"])
	require.NotZero(t, meta["checksum"])
}

func TestFromJSON_HasMetadata(t *testing.T) {
	input := []byte(`{"hello": "world"}`)
	doc, err := data.FromJSON(input)
	require.NoError(t, err)
	require.NotNil(t, doc)

	meta, ok := doc.Metadata()
	require.True(t, ok)
	require.NotZero(t, meta["created"])
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
	require.NotNil(t, normalized)

	// Root metadata remains
	_, ok = normalized.Metadata()
	require.True(t, ok)

	// Nested metadata stripped
	childVal, err := normalized.Get("nested")
	require.NoError(t, err)
	child, ok := childVal.(*data.Document)
	require.True(t, ok)

	_, ok = child.Metadata()
	require.False(t, ok)
}
