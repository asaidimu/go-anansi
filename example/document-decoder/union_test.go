package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const unionSchemaJSON = `{
  "version": "1.0.0",
  "name": "union_demo",
  "fields": {
    "payload": { "name": "payload", "type": "union", "schema": [ { "id": "obj_a" }, { "id": "obj_b" } ] },
    "extras":  { "name": "extras",  "type": "array",  "schema": { "id": "holder" } }
  },
  "schemas": {
    "obj_a": { "name": "obj_a", "fields": { "a_name": { "name": "a_name", "type": "string" } } },
    "obj_b": { "name": "obj_b", "fields": { "b_name": { "name": "b_name", "type": "string" } } },
    "holder": { "name": "holder", "fields": {
      "choice": { "name": "choice", "type": "union", "schema": [ { "id": "obj_a" }, { "id": "obj_b" } ] }
    } }
  }
}`

func unionFieldDescriptor(t *testing.T, cs *definition.CompiledSchema, path string) definition.FieldDescriptor {
	t.Helper()
	key, fd, err := keyForPath(cs, path)
	require.NoError(t, err)
	require.NotZero(t, key)
	return fd
}

func TestUnion_FieldClassification(t *testing.T) {
	cs, err := compileSchema([]byte(unionSchemaJSON))
	require.NoError(t, err)

	// Union fields are TypeUnknown / KindComplex and carry no single child
	// schema, so paths cannot descend through them.
	fd := unionFieldDescriptor(t, cs, "payload")
	assert.Equal(t, container.TypeUnknown, fd.DataType())
	assert.Equal(t, definition.KindComplex, fd.Kind())
	assert.False(t, fd.Terminal())
	assert.Equal(t, definition.FdNoChild, fd.ChildSchemaIdx())

	_, err = cs.ResolvePath("payload.a_name")
	require.Error(t, err)
}

func TestUnion_DecodeAndLookup(t *testing.T) {
	cs, err := compileSchema([]byte(unionSchemaJSON))
	require.NoError(t, err)

	// Variant a.
	doc, err := DecodeJSON(cs, []byte(`{ "payload": { "a_name": "x" } }`))
	require.NoError(t, err)
	v, err := Lookup(cs, doc, "payload")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"a_name": "x"}, v)

	// Variant b.
	doc, err = DecodeJSON(cs, []byte(`{ "payload": { "b_name": "y" } }`))
	require.NoError(t, err)
	v, err = Lookup(cs, doc, "payload")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"b_name": "y"}, v)
}

func TestUnion_InsideArrayElement(t *testing.T) {
	cs, err := compileSchema([]byte(unionSchemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{ "extras": [ { "choice": { "a_name": "c" } }, { "choice": { "b_name": "d" } } ] }`))
	require.NoError(t, err)

	extrasRaw, err := Lookup(cs, doc, "extras")
	require.NoError(t, err)
	extras, ok := extrasRaw.([]*container.DataContainer)
	require.True(t, ok)
	require.Len(t, extras, 2)

	first, err := Lookup(cs, extras[0], "extras.choice")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"a_name": "c"}, first)

	second, err := Lookup(cs, extras[1], "extras.choice")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"b_name": "d"}, second)
}

func TestUnion_AbsentAndNull(t *testing.T) {
	cs, err := compileSchema([]byte(unionSchemaJSON))
	require.NoError(t, err)

	// Absent union field: skipped (not required, no default).
	doc, err := DecodeJSON(cs, []byte(`{}`))
	require.NoError(t, err)
	_, err = Lookup(cs, doc, "payload")
	require.Error(t, err)
}

func TestUnion_Dump(t *testing.T) {
	cs, err := compileSchema([]byte(unionSchemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{ "payload": { "b_name": "y" } }`))
	require.NoError(t, err)

	dump, err := Dump(cs, doc)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"b_name": "y"}, dump["payload"])
}
