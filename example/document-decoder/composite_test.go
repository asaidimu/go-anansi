package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const compositeSchemaJSON = `{
  "version": "1.0.0",
  "name": "composite_demo",
  "fields": {
    "identity": { "name": "identity", "type": "composite", "schema": [ { "id": "contact" }, { "id": "geo" } ] }
  },
  "schemas": {
    "contact": { "name": "contact", "fields": {
      "email": { "name": "email", "type": "string" }
    } },
    "geo": { "name": "geo", "fields": {
      "lat": { "name": "lat", "type": "number" },
      "lng": { "name": "lng", "type": "number" }
    } }
  }
}`

func compositeDescriptor(t *testing.T, cs *definition.CompiledSchema) definition.FieldDescriptor {
	t.Helper()
	for i, meta := range cs.FieldsMeta {
		if meta.Name == "identity" && cs.Descriptors[i].SchemaIdx() == 0 {
			return cs.Descriptors[i]
		}
	}
	t.Fatalf("identity field not found")
	return 0
}

func TestComposite_ClassifiesAsObject(t *testing.T) {
	cs, err := compileSchema([]byte(compositeSchemaJSON))
	require.NoError(t, err)

	fd := compositeDescriptor(t, cs)
	assert.Equal(t, container.TypeUnknown, fd.DataType())
	assert.Equal(t, definition.KindObject, fd.Kind())
	assert.False(t, fd.Terminal())
	// Collapsed into a single child schema, like an object.
	assert.NotEqual(t, definition.FdNoChild, fd.ChildSchemaIdx())
}

func TestComposite_FlattensChildren(t *testing.T) {
	cs, err := compileSchema([]byte(compositeSchemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{ "identity": { "email": "a@b.c", "lat": 1.5, "lng": 2.5 } }`))
	require.NoError(t, err)

	// Part fields share the document key space with the object, addressable by
	// dotted path.
	email, err := Lookup(cs, doc, "identity.email")
	require.NoError(t, err)
	assert.Equal(t, "a@b.c", email)

	lat, err := Lookup(cs, doc, "identity.lat")
	require.NoError(t, err)
	assert.Equal(t, 1.5, lat)

	lng, err := Lookup(cs, doc, "identity.lng")
	require.NoError(t, err)
	assert.Equal(t, 2.5, lng)
}

func TestComposite_PathResolvesThroughField(t *testing.T) {
	cs, err := compileSchema([]byte(compositeSchemaJSON))
	require.NoError(t, err)

	rp, err := cs.ResolvePath("identity.email")
	require.NoError(t, err)
	require.Len(t, rp, 2)
	assert.GreaterOrEqual(t, cs.Address(rp), uint32(definition.MultiStepBase))

	_, err = cs.ResolvePath("identity.nope")
	require.Error(t, err)
}

func TestComposite_PartialAndAbsentFields(t *testing.T) {
	cs, err := compileSchema([]byte(compositeSchemaJSON))
	require.NoError(t, err)

	// Only one part's fields present; the other part is just absent (skipped,
	// not required, no default).
	doc, err := DecodeJSON(cs, []byte(`{ "identity": { "email": "x@y.z" } }`))
	require.NoError(t, err)

	email, err := Lookup(cs, doc, "identity.email")
	require.NoError(t, err)
	assert.Equal(t, "x@y.z", email)

	_, err = Lookup(cs, doc, "identity.lat")
	require.Error(t, err)
}

func TestComposite_Dump(t *testing.T) {
	cs, err := compileSchema([]byte(compositeSchemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{ "identity": { "email": "a@b.c", "lat": 1.5 } }`))
	require.NoError(t, err)

	dump, err := Dump(cs, doc)
	require.NoError(t, err)
	assert.Equal(t, "a@b.c", dump["identity.email"])
	assert.Equal(t, 1.5, dump["identity.lat"])
}

func TestComposite_RejectsDuplicatePartFields(t *testing.T) {
	schema := `{
	  "version": "1.0.0",
	  "name": "dup_composite",
	  "fields": {
	    "c": { "name": "c", "type": "composite", "schema": [ { "id": "s1" }, { "id": "s2" } ] }
	  },
	  "schemas": {
	    "s1": { "name": "s1", "fields": { "name": { "name": "name", "type": "string" } } },
	    "s2": { "name": "s2", "fields": { "name": { "name": "name", "type": "integer" } } }
	  }
	}`
	_, err := compileSchema([]byte(schema))
	require.Error(t, err)
}
