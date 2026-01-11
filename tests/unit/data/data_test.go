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

	meta := doc.Metadata()
	require.Contains(t, meta, "created")
	require.Contains(t, meta, "updated")
	require.Contains(t, meta, "version")
	require.Contains(t, meta, "checksum")
}

func TestMustNewDocument_FromMap_HasMetadata(t *testing.T) {
	input := map[string]any{"foo": "baz"}
	doc := data.MustNewDocument(input)
	require.NotNil(t, doc)

	meta := doc.Metadata()
	require.NotZero(t, meta["created"])
	require.NotZero(t, meta["checksum"])
}
