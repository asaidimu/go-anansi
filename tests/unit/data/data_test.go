package data_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

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
	require.True(t, meta2["updated"].(int64) > created.(int64), "updated should move forward")
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

func TestRegisterUserMetadataSchema_AndProvider(t *testing.T) {
	// Register custom provider
	data.RegisterMetadata("custom", &schema.NestedSchemaDefinition{
		Name: "custom_meta",
	}, func(_ data.Document) (map[string]any, error) {
		return map[string]any{"foo": "bar"}, nil
	})

	doc, err := data.NewDocument(map[string]any{"field": "value"})
	require.NoError(t, err)

	meta, ok := doc.Metadata()
	require.True(t, ok)
	require.Equal(t, "bar", meta["foo"], "custom metadata should be merged in")
}
