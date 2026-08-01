package main

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeJSON_Demo(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(documentJSON))
	require.NoError(t, err)

	dump, err := Dump(cs, doc)
	require.NoError(t, err)

	assert.Equal(t, "user-1", dump["id"])
	assert.Equal(t, true, dump["active"])
	assert.Equal(t, int64(31), dump["age"])
	assert.Equal(t, 9.5, dump["score"])
	assert.Equal(t, []string{"go", "schema"}, dump["tags"])

	// Named object children flatten into the key space.
	assert.Equal(t, "1 Main St", dump["address.street"])
	assert.Equal(t, int64(10001), dump["address.zip"])

	// Record holds the raw map value.
	profile, ok := dump["profile"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2 Side Rd", profile["street"])
	assert.Equal(t, int64(20002), profile["zip"])

	// Array of objects: one dumped map per element.
	items, ok := dump["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)
	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "3 Leaf Ct", first["items.street"])
	assert.Equal(t, int64(30003), first["items.zip"])
	second, ok := items[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "4 Oak Ave", second["items.street"])

	// Unknown field keeps its raw value.
	assert.Equal(t, map[string]any{"source": "demo"}, dump["meta"])

	// Default was applied for the absent field.
	assert.Equal(t, "anon", dump["nickname"])
}

// TestDump_EmptyDocument verifies Dump works on a document that only carries
// defaulted fields, with no locally-built path index: every flattened child is
// named through the compiled schema's address→path cache, which the decoder
// populated as it stored each value.
func TestDump_EmptyDocument(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{}`))
	require.NoError(t, err)

	dump, err := Dump(cs, doc)
	require.NoError(t, err)

	assert.Equal(t, "anon", dump["nickname"])
	for k := range dump {
		assert.NotContains(t, k, "<unknown", "every key must resolve to a real schema path, got %q", k)
	}
}

// TestDump_ReverseCacheUsed verifies the decoder populates the schema's
// address→path cache as it stores values: a value resolved through the cache
// (not a locally-walked index) yields the same fully-qualified path that the
// decoder used to store it.
func TestDump_ReverseCacheUsed(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(documentJSON))
	require.NoError(t, err)

	dump, err := Dump(cs, doc)
	require.NoError(t, err)
	require.Equal(t, int64(10001), dump["address.zip"])

	// Every addressable value stored by the decoder must be recoverable via
	// PathForAddress, so Dump never needs to re-walk the schema.
	var leaves int
	_, err = doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		for k, idx := range positions {
			key := container.DataContainerKey(k)
			if idx < 0 {
				continue
			}
			switch key.Type() {
			case container.TypeRecord, container.TypeArrayObject, container.TypeUnknown,
				container.TypeArrayUnknown:
				continue
			}
			leaves++
			rp, ok := cs.PathForAddress(uint32(key.DataPoint().ID()))
			require.True(t, ok, "decoded leaf address must have a reverse path entry")
			assert.NotEmpty(t, cs.PathString(rp))
		}
		return nil, nil
	})
	require.NoError(t, err)
	require.Greater(t, leaves, 0)
}

func TestDecodeJSON_MissingRequired(t *testing.T) {
	cs, err := compileSchema([]byte(requiredSchemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`{ "other": "x" }`))
	require.Error(t, err)
}

func TestDecodeJSON_RequiredPresent(t *testing.T) {
	cs, err := compileSchema([]byte(requiredSchemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{ "id": "abc" }`))
	require.NoError(t, err)
	dump, err := Dump(cs, doc)
	require.NoError(t, err)
	assert.Equal(t, "abc", dump["id"])
}

func TestDecodeJSON_TypeMismatch(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`{ "active": 1 }`))
	require.Error(t, err)
}

func TestDecodeJSON_RootNotObject(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`[1, 2, 3]`))
	require.Error(t, err)
}

func TestDecodeJSON_TrailingData(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`{} {}`))
	require.Error(t, err)
}

// TestDecodeJSONInto_PooledMatchesUnpooled verifies the pooled entry point
// produces an identical container (and dump) to DecodeJSON, including pooled
// array-of-object children, and that a document can be safely reused after
// being returned to the pool.
func TestDecodeJSONInto_PooledMatchesUnpooled(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	ref, err := DecodeJSON(cs, benchDocJSON)
	require.NoError(t, err)
	refDump, err := Dump(cs, ref)
	require.NoError(t, err)

	pool := container.NewPool()
	for i := 0; i < 3; i++ {
		doc := pool.Get()
		require.NoError(t, DecodeJSONInto(cs, benchDocJSON, doc, pool))
		got, err := Dump(cs, doc)
		require.NoError(t, err)
		assert.Equal(t, refDump, got, "pooled decode must match unpooled decode (cycle %d)", i)
		pool.Put(doc)
	}
}

const requiredSchemaJSON = `{
  "version": "1.0.0",
  "name": "required_demo",
  "fields": {
    "id": { "name": "id", "type": "string", "required": true }
  }
}`
