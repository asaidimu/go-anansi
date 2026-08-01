package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func lookupDemo(t *testing.T) (*definition.CompiledSchema, *container.DataContainer) {
	t.Helper()
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)
	doc, err := DecodeJSON(cs, []byte(documentJSON))
	require.NoError(t, err)
	return cs, doc
}

func TestLookup_PathAccess(t *testing.T) {
	cs, doc := lookupDemo(t)

	id, err := Lookup(cs, doc, "id")
	require.NoError(t, err)
	assert.Equal(t, "user-1", id)

	zipVal, err := Lookup(cs, doc, "address.zip")
	require.NoError(t, err)
	assert.Equal(t, int64(10001), zipVal)

	street, err := Lookup(cs, doc, "address.street")
	require.NoError(t, err)
	assert.Equal(t, "1 Main St", street)

	score, err := Lookup(cs, doc, "score")
	require.NoError(t, err)
	assert.Equal(t, 9.5, score)
}

func TestLookup_ContainerValues(t *testing.T) {
	cs, doc := lookupDemo(t)

	itemsRaw, err := Lookup(cs, doc, "items")
	require.NoError(t, err)
	items, ok := itemsRaw.([]*container.DataContainer)
	require.True(t, ok)
	require.Len(t, items, 2)

	s, err := Lookup(cs, items[0], "items.street")
	require.NoError(t, err)
	assert.Equal(t, "3 Leaf Ct", s)

	profileRaw, err := Lookup(cs, doc, "profile")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"street": "2 Side Rd", "zip": int64(20002)}, profileRaw)

	metaRaw, err := Lookup(cs, doc, "meta")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"source": "demo"}, metaRaw)
}

func TestLookup_Errors(t *testing.T) {
	cs, doc := lookupDemo(t)

	// Flattened objects store their children, not a value of their own.
	_, err := Lookup(cs, doc, "address")
	require.Error(t, err)

	// Path through a terminal field.
	_, err = Lookup(cs, doc, "id.foo")
	require.Error(t, err)

	// Unknown path segment.
	_, err = Lookup(cs, doc, "nope")
	require.Error(t, err)

	// Valid path, but the value was never stored.
	_, err = Lookup(cs, doc, "nickname")
	require.NoError(t, err) // default was applied
}

func TestLookup_MutateAndReread(t *testing.T) {
	cs, doc := lookupDemo(t)

	key, fd, err := keyForPath(cs, "address.zip")
	require.NoError(t, err)
	require.Equal(t, container.TypeInt, fd.DataType())

	require.NoError(t, doc.SetInt(key, 12345))

	v, err := Lookup(cs, doc, "address.zip")
	require.NoError(t, err)
	assert.Equal(t, int64(12345), v)
}
