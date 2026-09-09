package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

type modelTestProduct struct {
	DocumentModel
	Name   string `anansi:"name"`
	Status string `anansi:"status"`
}

func TestDocumentModel_NewInitializesIdentity(t *testing.T) {
	p := New(&modelTestProduct{Name: "widget"})
	require.NotNil(t, p)
	assert.NotEmpty(t, p.GetID())
	assert.NotEmpty(t, p.Metadata[data.MetadataCreated])
	assert.NotEmpty(t, p.Metadata[data.MetadataUpdated])
	assert.Equal(t, 1, p.Metadata[data.MetadataVersion])
}

func TestDocumentModel_DocumentBuildsContainerBackedDocument(t *testing.T) {
	p := New(&modelTestProduct{Name: "widget", Status: "active"})
	doc, err := p.Document()
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, p.GetID(), doc.ID())
	assert.Equal(t, "widget", doc.GetOr("name", nil))
	assert.Equal(t, "active", doc.GetOr("status", nil))
}

func TestDocumentModel_PatchSkipsSystemAndZeroFields(t *testing.T) {
	p := New(&modelTestProduct{Name: "widget"})
	patch, err := p.Patch()
	require.NoError(t, err)
	require.NotNil(t, patch)
	assert.Empty(t, patch.ID())
	assert.Equal(t, "widget", patch.GetOr("name", nil))
}

func TestDocumentModel_MustDocumentPanicsWhenUninitialized(t *testing.T) {
	require.Panics(t, func() {
		(&modelTestProduct{Name: "x"}).MustDocument()
	})
}

func TestDocumentModel_DocumentFailsWhenUninitialized(t *testing.T) {
	_, err := (&modelTestProduct{Name: "x"}).Document()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModelNotInitialized)
}

func TestDocumentModel_LenCountsNonZeroUserFields(t *testing.T) {
	p := New(&modelTestProduct{Name: "widget"})
	assert.Equal(t, 1, p.Len())
	empty := New(&modelTestProduct{})
	assert.Equal(t, 0, empty.Len())
}

func TestDocumentPoolFromType_BuildsSchemaBoundDocumentPool(t *testing.T) {
	col, err := DocumentPoolFromType[*modelTestProduct]()
	require.NoError(t, err)
	require.NotNil(t, col)

	p := New(&modelTestProduct{Name: "widget", Status: "active"})
	doc, err := col.FromStruct(p)
	require.NoError(t, err)
	assert.Equal(t, p.GetID(), doc.ID())
	assert.Equal(t, "widget", doc.GetOr("name", nil))
}

func TestDocumentModel_BindToRoundTrip(t *testing.T) {
	data.ResetFactoryForTesting()
	require.NoError(t, data.ConfigureDocumentFactory(data.DocumentFactoryConfig{}, nil))

	p := New(&modelTestProduct{Name: "widget", Status: "active"})
	doc, err := p.Document()
	require.NoError(t, err)

	back := new(modelTestProduct)
	require.NoError(t, doc.BindTo(back))
	assert.Equal(t, "widget", back.Name)
	assert.Equal(t, "active", back.Status)
	assert.Equal(t, p.GetID(), back.GetID())

	// The binder restores the parent reference, so promoted methods work on
	// the materialized result.
	roundTrip, err := back.Document()
	require.NoError(t, err)
	assert.Equal(t, p.GetID(), roundTrip.ID())
}
