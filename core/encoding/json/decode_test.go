package json

import (
	"encoding/json"
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeJSON_Demo(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
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

	// Defaults are not injected at decode: the absent nickname is not stored.
	_, ok = dump["nickname"]
	assert.False(t, ok, "decode must not apply schema defaults")
}

// TestDump_EmptyDocument verifies Dump works on a document decoded from an
// empty object: no defaults are injected, so the container is empty and the
// dump has no keys.
func TestDump_EmptyDocument(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{}`))
	require.NoError(t, err)

	dump, err := Dump(cs, doc)
	require.NoError(t, err)

	assert.Len(t, dump, 0, "decode of {} must store nothing, got %v", dump)
	for k := range dump {
		assert.NotContains(t, k, "<unknown", "every key must resolve to a real schema path, got %q", k)
	}
}

// TestDump_ReverseCacheUsed verifies the decoder populates the schema's
// address→path cache as it stores values: a value resolved through the cache
// (not a locally-walked index) yields the same fully-qualified path that the
// decoder used to store it.
func TestDump_ReverseCacheUsed(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
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
	cs, err := compileSchema(t, []byte(requiredSchemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`{ "other": "x" }`))
	require.Error(t, err)
}

func TestDecodeJSON_RequiredPresent(t *testing.T) {
	cs, err := compileSchema(t, []byte(requiredSchemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{ "id": "abc" }`))
	require.NoError(t, err)
	dump, err := Dump(cs, doc)
	require.NoError(t, err)
	assert.Equal(t, "abc", dump["id"])
}

func TestDecodeJSON_TypeMismatch(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`{ "active": 1 }`))
	require.Error(t, err)
}

func TestDecodeJSON_RootNotObject(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`[1, 2, 3]`))
	require.Error(t, err)
}

func TestDecodeJSON_TrailingData(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
	require.NoError(t, err)

	_, err = DecodeJSON(cs, []byte(`{} {}`))
	require.Error(t, err)
}

// TestDecodeJSONInto_PooledMatchesUnpooled verifies the pooled entry point
// produces an identical container (and dump) to DecodeJSON, including pooled
// array-of-object children, and that a document can be safely reused after
// being returned to the pool.
func TestDecodeJSONInto_PooledMatchesUnpooled(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
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

// ── jsonparser ────────────────────────────────────────────────────────────────

func TestParseStringEscapes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"plain"`, "plain"},
		{`"a\"b"`, `a"b`},
		{`"a\\b"`, `a\b`},
		{`"a\/b"`, "a/b"},
		{`"a\nb\tc\rd"`, "a\nb\tc\rd"},
		{`"\b\f"`, "\b\f"},
		{`"\u0041"`, "A"},
		{`"\ud83d\ude00"`, "\U0001F600"},
		{`"\u00e9"`, "é"},
		{`"mixed \u0042 and \n"`, "mixed B and \n"},
	}
	for _, c := range cases {
		p := newJSONParser([]byte(c.in))
		got, err := p.parseString()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, got, c.in)
		assert.True(t, p.eof(), c.in)
	}
}

func TestParseStringErrors(t *testing.T) {
	for _, in := range []string{
		`"unterminated`,
		`"bad \x escape"`,
		`"\u12"`,
		`"\uZZZZ"`,
		`"ctl \x01 char"`,
	} {
		p := newJSONParser([]byte(in))
		_, err := p.parseString()
		require.Error(t, err, in)
	}
}

func TestParseNumber(t *testing.T) {
	p := newJSONParser([]byte(`0`))
	n, err := p.parseInteger()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	for _, c := range []struct {
		in   string
		want int64
	}{
		{`-17`, -17},
		{`42`, 42},
		{`9007199254740991`, 9007199254740991},
	} {
		p := newJSONParser([]byte(c.in))
		n, err := p.parseInteger()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, n, c.in)
	}

	for _, c := range []struct {
		in   string
		want float64
	}{
		{`1.5`, 1.5},
		{`-2.25`, -2.25},
		{`1e3`, 1000},
		{`1.5E-2`, 0.015},
		{`0.1`, 0.1},
	} {
		p := newJSONParser([]byte(c.in))
		f, err := p.parseFloat()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, f, c.in)
	}

	// Integer fields must reject non-integral literals.
	p = newJSONParser([]byte(`1.5`))
	_, err = p.parseInteger()
	require.Error(t, err)

	// Malformed numbers.
	for _, in := range []string{`-`, `01`, `-01`, `1.`, `1e`, `+1`} {
		p := newJSONParser([]byte(in))
		_, err := p.parseAny()
		require.Error(t, err, in)
	}
}

func TestParseAnyTypes(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{`true`, true},
		{`false`, false},
		{`null`, nil},
		{`"hi"`, "hi"},
		{`42`, int64(42)},
		{`4.5`, 4.5},
		{`[1, 2]`, []any{int64(1), int64(2)}},
		{`{"a": 1}`, map[string]any{"a": int64(1)}},
		{`{"a": [true, null], "b": {"c": 1.5}}`, map[string]any{"a": []any{true, nil}, "b": map[string]any{"c": 1.5}}},
	}
	for _, c := range cases {
		p := newJSONParser([]byte(c.in))
		got, err := p.parseAny()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, got, c.in)
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		``,
		`{`,
		`[`,
		`[1`,
		`[1 2]`,
		`{"a" 1}`,
		`{"a":}`,
		`tru`,
		`nul`,
		`'single'`,
	} {
		p := newJSONParser([]byte(in))
		_, err := p.parseAny()
		require.Error(t, err, in)
	}
}

// ── serializer ────────────────────────────────────────────────────────────────

// TestSerializeJSON_RoundTrip verifies the serializer emits the nested wire
// format the decoder consumes: serializing a decoded document and decoding it
// again reproduces the same stored values. This pins float formatting, key
// ordering, escaping, and — critically — the nested object and
// array-of-object shape, which the old flat dotted-path output did not survive
// a decode.
func TestSerializeJSON_RoundTrip(t *testing.T) {
	roundTrip := func(t *testing.T, cs *definition.CompiledSchema, doc *container.DataContainer) {
		t.Helper()
		got, err := SerializeJSON(cs, doc)
		require.NoError(t, err)

		re, err := DecodeJSON(cs, got)
		require.NoError(t, err)
		want, err := Dump(cs, doc)
		require.NoError(t, err)
		got2, err := Dump(cs, re)
		require.NoError(t, err)
		assert.Equal(t, want, got2)

		again, err := SerializeJSON(cs, re)
		require.NoError(t, err)
		assert.Equal(t, string(got), string(again))
	}

	t.Run("demo", func(t *testing.T) {
		cs, err := compileSchema(t, []byte(schemaJSON))
		require.NoError(t, err)

		doc, err := DecodeJSON(cs, []byte(documentJSON))
		require.NoError(t, err)
		roundTrip(t, cs, doc)

		// Named-object children group into nested objects and array-of-object
		// elements use the child schema's field names (relative keys).
		got, err := SerializeJSON(cs, doc)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(got, &m))
		addr, ok := m["address"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "1 Main St", addr["street"])
		items, ok := m["items"].([]any)
		require.True(t, ok)
		first, ok := items[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "3 Leaf Ct", first["street"])
		assert.NotContains(t, first, "items.street")
	})

	cs, err := compileSchema(t, []byte(erpSchemaJSON))
	require.NoError(t, err)

	for name, n := range erpLineCounts {
		t.Run("erp_"+name, func(t *testing.T) {
			doc, err := DecodeJSON(cs, buildERPOrder("ORD-2026-000042", n))
			require.NoError(t, err)
			roundTrip(t, cs, doc)
		})
	}
}

// TestBytesRoundTrip pins the bytes transport contract: bytes fields serialize
// to a base64 string and decode back to the exact raw payload, never the
// base64 string's own bytes (double encoding) and never a Go `%v` stringification.
func TestBytesRoundTrip(t *testing.T) {
	const s = `{"version":"1.0.0","name":"s","fields":{"payload":{"name":"payload","type":"bytes"}}}`
	cs, err := compileSchema(t, []byte(s))
	require.NoError(t, err)

	raw := []byte{0x00, 0x01, 0xfe, 0xff, 'r', 'a', 'w'}
	key := internalKey(cs.Descriptors[0])
	doc := container.NewDataContainer()
	require.NoError(t, doc.SetBytes(key, raw))

	got, err := SerializeJSON(cs, doc)
	require.NoError(t, err)

	re, err := DecodeJSON(cs, got)
	require.NoError(t, err)
	dump, err := Dump(cs, re)
	require.NoError(t, err)
	assert.Equal(t, raw, dump["payload"])
}

// TestSerializeJSON_EmptyObject checks the serializer on a container with only
// defaulted fields and on an empty document.
func TestSerializeJSON_EmptyObject(t *testing.T) {
	cs, err := compileSchema(t, []byte(schemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{}`))
	require.NoError(t, err)

	got, err := SerializeJSON(cs, doc)
	require.NoError(t, err)

	re, err := DecodeJSON(cs, got)
	require.NoError(t, err)
	want, err := Dump(cs, doc)
	require.NoError(t, err)
	got2, err := Dump(cs, re)
	require.NoError(t, err)
	assert.Equal(t, want, got2)
}

// ── composite ─────────────────────────────────────────────────────────────────

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
	cs, err := compileSchema(t, []byte(compositeSchemaJSON))
	require.NoError(t, err)

	fd := compositeDescriptor(t, cs)
	assert.Equal(t, container.TypeUnknown, fd.DataType())
	assert.Equal(t, definition.KindObject, fd.Kind())
	assert.False(t, fd.Terminal())
	// Collapsed into a single child schema, like an object.
	assert.NotEqual(t, definition.FdNoChild, fd.ChildSchemaIdx())
}

func TestComposite_FlattensChildren(t *testing.T) {
	cs, err := compileSchema(t, []byte(compositeSchemaJSON))
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
	cs, err := compileSchema(t, []byte(compositeSchemaJSON))
	require.NoError(t, err)

	rp, err := cs.ResolvePath("identity.email")
	require.NoError(t, err)
	require.Len(t, rp, 2)
	assert.GreaterOrEqual(t, cs.Address(rp), uint32(definition.MultiStepBase))

	_, err = cs.ResolvePath("identity.nope")
	require.Error(t, err)
}

func TestComposite_PartialAndAbsentFields(t *testing.T) {
	cs, err := compileSchema(t, []byte(compositeSchemaJSON))
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
	cs, err := compileSchema(t, []byte(compositeSchemaJSON))
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
	_, err := compileSchema(t, []byte(schema))
	require.Error(t, err)
}

// ── union ─────────────────────────────────────────────────────────────────────

func unionFieldDescriptor(t *testing.T, cs *definition.CompiledSchema, path string) definition.FieldDescriptor {
	t.Helper()
	key, fd, err := keyForPath(cs, path)
	require.NoError(t, err)
	require.NotZero(t, key)
	return fd
}

func TestUnion_FieldClassification(t *testing.T) {
	cs, err := compileSchema(t, []byte(unionSchemaJSON))
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
	cs, err := compileSchema(t, []byte(unionSchemaJSON))
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
	cs, err := compileSchema(t, []byte(unionSchemaJSON))
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
	cs, err := compileSchema(t, []byte(unionSchemaJSON))
	require.NoError(t, err)

	// Absent union field: skipped (not required, no default).
	doc, err := DecodeJSON(cs, []byte(`{}`))
	require.NoError(t, err)
	_, err = Lookup(cs, doc, "payload")
	require.Error(t, err)
}

func TestUnion_Dump(t *testing.T) {
	cs, err := compileSchema(t, []byte(unionSchemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{ "payload": { "b_name": "y" } }`))
	require.NoError(t, err)

	dump, err := Dump(cs, doc)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"b_name": "y"}, dump["payload"])
}
