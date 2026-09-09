package document

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const testSchemaJSON = `{
  "version": "1.0.0",
  "name": "testdoc",
  "fields": {
    "id":        { "name": "id",        "type": "string" },
    "active":    { "name": "active",    "type": "boolean" },
    "age":       { "name": "age",       "type": "integer" },
    "score":     { "name": "score",     "type": "number" },
    "nickname":  { "name": "nickname",  "type": "string", "default": "anon" },
    "tags":      { "name": "tags",      "type": "array",  "schema": { "type": "string" } },
    "intlist":   { "name": "intlist",   "type": "array",  "schema": { "type": "integer" } },
    "address":   { "name": "address",   "type": "object", "schema": { "id": "addr" } },
    "profile":   { "name": "profile",   "type": "record", "schema": { "id": "addr" } },
    "items":     { "name": "items",     "type": "array",  "schema": { "id": "addr" } }
  },
  "schemas": {
    "addr": {
      "name": "addr",
      "fields": {
        "street": { "name": "street", "type": "string" },
        "zip":    { "name": "zip",    "type": "integer" }
      }
    }
  }
}`

func newTestCollection(t *testing.T) *DocumentPool {
	t.Helper()
	s, err := definition.FromJSON([]byte(testSchemaJSON))
	require.NoError(t, err)
	col, err := NewDocumentPool(s)
	require.NoError(t, err)
	return col
}

func testDocument(t *testing.T) *Document {
	t.Helper()
	input := map[string]any{
		"_id_":    "019fc29f444b736986075644a478fb92",
		"id":      "user-1",
		"active":  true,
		"age":     31,
		"score":   9.5,
		"tags":    []string{"go", "schema"},
		"address": map[string]any{"street": "1 Main St", "zip": int64(10001)},
		"profile": map[string]any{"street": "2 Side Rd", "zip": int64(20002)},
		"items": []any{
			map[string]any{"street": "3 Leaf Ct", "zip": int64(30003)},
			map[string]any{"street": "4 Oak Ave", "zip": int64(40004)},
		},
	}
	d, err := newTestCollection(t).FromMap(input)
	require.NoError(t, err)
	return d
}

func TestCompileTimeInterface(t *testing.T) {
	var _ data.Documenter = (*Document)(nil)
}

func TestFromMapAndData(t *testing.T) {
	d := testDocument(t)
	require.Equal(t, "019fc29f444b736986075644a478fb92", d.ID())

	dataMap := d.Data()
	assert.Equal(t, "user-1", dataMap["id"])
	assert.Equal(t, int64(31), dataMap["age"])
	assert.Equal(t, 9.5, dataMap["score"])
	assert.Equal(t, []string{"go", "schema"}, dataMap["tags"])
	assert.Equal(t, map[string]any{"street": "1 Main St", "zip": int64(10001)}, dataMap["address"])

	// System fields are excluded from Data().
	_, hasID := dataMap[data.DocumentIDField]
	assert.False(t, hasID)
	_, hasMeta := dataMap[data.MetadataField]
	assert.False(t, hasMeta)
}

func TestToMapIncludesSystemFields(t *testing.T) {
	d := testDocument(t)
	all := d.ToMap()
	assert.Equal(t, "019fc29f444b736986075644a478fb92", all[data.DocumentIDField])
	meta, ok := all[data.MetadataField].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), meta["version"])
	assert.Equal(t, "user-1", all["id"])
}

func TestGetByPath(t *testing.T) {
	d := testDocument(t)

	v, err := d.Get("id")
	require.NoError(t, err)
	assert.Equal(t, "user-1", v)

	v, err = d.Get("address.zip")
	require.NoError(t, err)
	assert.Equal(t, int64(10001), v)

	v, err = d.Get("address.street")
	require.NoError(t, err)
	assert.Equal(t, "1 Main St", v)

	// Nested object materializes as a map.
	v, err = d.Get("address")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"street": "1 Main St", "zip": int64(10001)}, v)

	// Record field materializes as a map.
	v, err = d.Get("profile")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"street": "2 Side Rd", "zip": int64(20002)}, v)

	// Array of objects materializes as []any of maps.
	v, err = d.Get("items")
	require.NoError(t, err)
	arr, ok := v.([]any)
	require.True(t, ok)
	require.Len(t, arr, 2)
	assert.Equal(t, "3 Leaf Ct", arr[0].(map[string]any)["street"])
}

func TestGetMissingReturnsError(t *testing.T) {
	d := testDocument(t)
	_, err := d.Get("nope")
	require.Error(t, err)
	_, err = d.Get("address.missing")
	require.Error(t, err)
}

func TestDefaultNotInjectedAtRead(t *testing.T) {
	d := testDocument(t)
	// nickname has a schema default ("anon") but is absent from the input;
	// defaults are applied only by the persistence layer, not at read time.
	_, err := d.Get("nickname")
	require.Error(t, err)
}

func TestSetAndUpdate(t *testing.T) {
	d := testDocument(t)

	require.NoError(t, d.Set("id", "user-2"))
	v, err := d.Get("id")
	require.NoError(t, err)
	assert.Equal(t, "user-2", v)

	require.NoError(t, d.Set("address.zip", int64(99999)))
	v, err = d.Get("address.zip")
	require.NoError(t, err)
	assert.Equal(t, int64(99999), v)

	// Setting a top-level object via map writes all leaves.
	require.NoError(t, d.Set("address", map[string]any{"street": "9 Maple Dr", "zip": int64(111)}))
	v, err = d.Get("address.street")
	require.NoError(t, err)
	assert.Equal(t, "9 Maple Dr", v)
	v, err = d.Get("address.zip")
	require.NoError(t, err)
	assert.Equal(t, int64(111), v)
}

func TestSetOnReservedFieldFails(t *testing.T) {
	d := testDocument(t)
	err := d.Set(data.DocumentIDField, "x")
	require.Error(t, err)
	err = d.Set(data.MetadataField, map[string]any{})
	require.Error(t, err)
}

func TestSetUnknownFieldFails(t *testing.T) {
	d := testDocument(t)
	err := d.Set("bogus", 1)
	require.Error(t, err)
}

func TestSetIfNotExists(t *testing.T) {
	d := testDocument(t)
	assert.False(t, d.SetIfNotExists("id", "nope"))
	// nickname has a schema default but is not stored; without read-time
	// default injection it is absent, so SetIfNotExists writes it.
	assert.True(t, d.SetIfNotExists("nickname", "set-nick"))

	fresh := newTestCollection(t).MustFromMap(map[string]any{})
	assert.True(t, fresh.SetIfNotExists("id", "set-me"))
	v, err := fresh.Get("id")
	require.NoError(t, err)
	assert.Equal(t, "set-me", v)
}

func TestUnsetAndHasPath(t *testing.T) {
	d := testDocument(t)
	require.True(t, d.HasPath("address.zip"))
	d.Unset("address.zip")
	require.False(t, d.HasPath("address.zip"))
	_, err := d.Get("address.zip")
	require.Error(t, err)

	require.True(t, d.HasKey("age"))
	d.Unset("age")
	require.False(t, d.HasKey("age"))
}

func TestKeysValuesLen(t *testing.T) {
	d := testDocument(t)
	keys := d.Keys()
	assert.Contains(t, keys, "id")
	assert.Contains(t, keys, "address")
	assert.Contains(t, keys, "items")
	assert.NotContains(t, keys, data.DocumentIDField)

	values := d.Values()
	require.Equal(t, len(keys), len(values))
	assert.Greater(t, d.Len(), 0)
	assert.False(t, d.IsEmpty())
}

func TestTypedGetters(t *testing.T) {
	d := testDocument(t)

	s, err := d.GetString("id")
	require.NoError(t, err)
	assert.Equal(t, "user-1", s)

	n, err := d.GetInt("age")
	require.NoError(t, err)
	assert.Equal(t, 31, n)

	f, err := d.GetFloat64("score")
	require.NoError(t, err)
	assert.Equal(t, 9.5, f)

	b, err := d.GetBool("active")
	require.NoError(t, err)
	assert.True(t, b)

	strs, err := d.GetStringArray("tags")
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "schema"}, strs)

	_, err = d.GetInt("id")
	require.Error(t, err)
}

func TestGetDocumentAndArray(t *testing.T) {
	d := testDocument(t)

	sub, err := d.GetDocument("address")
	require.NoError(t, err)
	assert.Equal(t, "1 Main St", sub.Data()["street"])

	rec, err := d.GetDocument("profile")
	require.NoError(t, err)
	assert.Equal(t, int64(20002), rec.Data()["zip"])

	children, err := d.GetDocumentArray("items")
	require.NoError(t, err)
	require.Len(t, children, 2)
	assert.Equal(t, "3 Leaf Ct", children[0].Data()["street"])
	assert.Equal(t, int64(40004), children[1].Data()["zip"])
}

func TestMetadataDefaultsAndCustom(t *testing.T) {
	d := testDocument(t)
	meta := d.Metadata()
	assert.Equal(t, float64(1), meta["version"]) // schema-declared as a number
	assert.NotNil(t, meta["created"])
	assert.NotNil(t, meta["updated"])

	// created/updated are nanosecond timestamps stored as strings.
	createdStr, ok := meta["created"].(string)
	require.True(t, ok)
	assert.Len(t, createdStr, 19)

	version, err := d.Version()
	require.NoError(t, err)
	assert.Equal(t, 1, version)

	created, err := d.GetMetadataTime("created")
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), created, time.Minute)
}

func TestSetCustomMetadataValue(t *testing.T) {
	d := testDocument(t)
	err := d.SetMetadataValue("customKey", "customValue")
	require.Error(t, err) // undeclared metadata keys are rejected

	err = d.SetMetadataValue("version", 99)
	require.Error(t, err) // reserved key is read-only
}

func TestHashVerifyHash(t *testing.T) {
	d := testDocument(t)
	require.NoError(t, d.Hash())

	ok, err := d.VerifyHash()
	require.NoError(t, err)
	assert.True(t, ok)

	// Tamper with data; verification must fail.
	require.NoError(t, d.Set("age", int64(99)))
	ok, err = d.VerifyHash()
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSignVerify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	d := testDocument(t)
	require.NoError(t, d.Sign(key))
	require.NoError(t, d.Verify(&key.PublicKey))

	// Tamper; verification must fail.
	require.NoError(t, d.Set("age", int64(42)))
	require.Error(t, d.Verify(&key.PublicKey))

	// Wrong key must fail.
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	require.Error(t, d.Verify(&other.PublicKey))
}

func TestCloneIsDeep(t *testing.T) {
	d := testDocument(t)
	clone := d.Clone().(*Document)
	assert.True(t, d.Equals(clone))
	assert.Equal(t, d.Data(), clone.Data())

	require.NoError(t, clone.Set("id", "mutated"))
	assert.Equal(t, "user-1", d.Data()["id"])
	assert.Equal(t, "mutated", clone.Data()["id"])
}

func TestStripMetadata(t *testing.T) {
	d := testDocument(t)
	stripped := d.StripMetadata().(*Document)
	_, err := stripped.GetMetadataValue(data.MetadataVersion)
	require.Error(t, err)
}

func TestDiffAndApply(t *testing.T) {
	d := testDocument(t)
	other := d.Clone().(*Document)
	require.NoError(t, other.Set("age", int64(50)))
	other.Unset("score")

	diff := d.Diff(other)
	assert.NotEmpty(t, diff.Modified["age"])
	assert.NotEmpty(t, diff.Removed["score"])

	applied := d.Apply(diff).(*Document)
	assert.Equal(t, int64(50), applied.Data()["age"])
	_, err := applied.Get("score")
	require.Error(t, err)

	// Apply must not mutate the original.
	assert.Equal(t, int64(31), d.Data()["age"])
}

func TestMergeAndDeepMerge(t *testing.T) {
	d := testDocument(t)
	other := newTestCollection(t).MustFromMap(map[string]any{"id": "merged", "age": int64(99)})

	d.Merge(other)
	assert.Equal(t, "merged", d.Data()["id"])
	assert.Equal(t, int64(99), d.Data()["age"])
}

func TestNormalize(t *testing.T) {
	d := testDocument(t)
	normalized := d.Normalize().(*Document)
	all := normalized.ToMap()
	// Normalize strips nested system fields but preserves top-level _id_/_metadata_.
	assert.Equal(t, d.ID(), all[data.DocumentIDField])
	assert.NotNil(t, all[data.MetadataField])
	assert.Equal(t, "1 Main St", all["address"].(map[string]any)["street"])
}

func TestJSONRoundTrip(t *testing.T) {
	d := testDocument(t)
	b, err := json.Marshal(d)
	require.NoError(t, err)

	re, err := newTestCollection(t).FromJSON(b)
	require.NoError(t, err)
	assert.Equal(t, d.ID(), re.ID())
	assert.Equal(t, d.Data(), re.Data())
	assert.Equal(t, d.Metadata(), re.Metadata())

	b2, err := json.Marshal(re)
	require.NoError(t, err)
	assert.Equal(t, string(b), string(b2))
}

func TestJSONPathQuery(t *testing.T) {
	d := testDocument(t)
	results, err := d.JSONPathQuery("$.address.zip")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, int64(10001), results[0])
}

func TestMustHelper(t *testing.T) {
	d := testDocument(t)
	assert.Equal(t, "user-1", d.Must().Get("id"))
}

func TestFromMapMissingRequiredIDIsGenerated(t *testing.T) {
	d := newTestCollection(t).MustFromMap(map[string]any{"id": "no-uuid"})
	assert.Len(t, d.ID(), 32)
}

func TestWithIDAndContext(t *testing.T) {
	type ctxKey string
	ctx := context.WithValue(context.Background(), ctxKey("k"), "v")
	d, err := newTestCollection(t).New(WithID("019fc29f444b736986075644a478fb92"), WithContext(ctx))
	require.NoError(t, err)
	assert.Equal(t, "019fc29f444b736986075644a478fb92", d.ID())
	assert.Equal(t, "v", d.Context().Value(ctxKey("k")))
}

func TestRecordView(t *testing.T) {
	rec := newRecordView(map[string]any{"street": "x", "zip": int64(1)}, nil)
	assert.Equal(t, "x", rec.Data()["street"])
	require.NoError(t, rec.Set("zip", int64(2)))
	assert.Equal(t, int64(2), rec.Data()["zip"])
}

func TestStringRendersJSON(t *testing.T) {
	d := testDocument(t)
	s := d.String()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	assert.Equal(t, "user-1", m["id"])
}

func TestIsAndEquals(t *testing.T) {
	d := testDocument(t)
	assert.True(t, d.Equals(d))
	assert.False(t, d.Is(nil))
	clone := d.Clone().(*Document)
	assert.True(t, d.Is(clone))
	require.NoError(t, clone.Set("id", "other"))
	assert.False(t, d.Is(clone))
}

func TestToStructBind(t *testing.T) {
	require.NoError(t, data.ConfigureDocumentFactory(data.DocumentFactoryConfig{}, nil))
	t.Cleanup(data.ResetFactoryForTesting)

	type addr struct {
		Street string `json:"street"`
		Zip    int64  `json:"zip"`
	}
	type doc struct {
		ID      string `json:"id"`
		Age     int    `json:"age"`
		Address addr   `json:"address"`
	}
	d := testDocument(t)
	var out doc
	err := d.BindToTag(&out, "json")
	require.NoError(t, err)
	assert.Equal(t, "user-1", out.ID)
	assert.Equal(t, 31, out.Age)
	assert.Equal(t, "1 Main St", out.Address.Street)
}

func TestTypedGettersByConstruction(t *testing.T) {
	d := testDocument(t)

	// Matching getter succeeds.
	v, err := d.GetString("id")
	require.NoError(t, err)
	assert.Equal(t, "user-1", v)

	// Mismatched getter is a call-site error, not a coercion.
	_, err = d.GetInt("id")
	require.Error(t, err)
	_, err = d.GetString("age")
	require.Error(t, err)
	_, err = d.GetString("active")
	require.Error(t, err)
	_, err = d.GetFloat64("age")
	require.Error(t, err)

	// Nested leaves resolve relative to the view.
	addr, err := d.GetDocument("address")
	require.NoError(t, err)
	street, err := addr.GetString("street")
	require.NoError(t, err)
	assert.Equal(t, "1 Main St", street)
	zip, err := addr.GetInt("zip")
	require.NoError(t, err)
	assert.Equal(t, 10001, zip)
}

func TestTypedGetterNoDefaults(t *testing.T) {
	d := testDocument(t)

	// nickname has a schema default ("anon") but is absent from the input;
	// defaults are applied only by the persistence layer, so reads error.
	_, err := d.GetString("nickname")
	require.Error(t, err)

	// Unset field without a default errors.
	_, err = d.GetString("nope")
	require.Error(t, err)
}

func TestTypedSetters(t *testing.T) {
	d := testDocument(t)

	require.NoError(t, d.SetString("id", "user-2"))
	require.NoError(t, d.SetInt("age", 42))
	require.NoError(t, d.SetFloat64("score", 8.75))
	require.NoError(t, d.SetBool("active", false))
	require.NoError(t, d.SetStringArray("tags", []string{"a", "b", "c"}))
	require.NoError(t, d.SetNested("address.zip", 99999))

	got, err := d.GetString("id")
	require.NoError(t, err)
	assert.Equal(t, "user-2", got)
	n, err := d.GetInt("age")
	require.NoError(t, err)
	assert.Equal(t, 42, n)
	f, err := d.GetFloat64("score")
	require.NoError(t, err)
	assert.Equal(t, 8.75, f)
	b, err := d.GetBool("active")
	require.NoError(t, err)
	assert.False(t, b)
	strs, err := d.GetStringArray("tags")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, strs)
	sub, err := d.GetDocument("address")
	require.NoError(t, err)
	zip, err := sub.GetInt("zip")
	require.NoError(t, err)
	assert.Equal(t, 99999, zip)
}

func TestTypedSettersByConstruction(t *testing.T) {
	d := testDocument(t)

	// Mismatched typed setter is rejected before touching the slot.
	require.Error(t, d.SetString("age", "oops"))             // age is an integer
	require.Error(t, d.SetInt("id", 1))                      // id is a string
	require.Error(t, d.SetBool("score", true))               // score is a number
	require.Error(t, d.SetStringArray("age", []string{"x"})) // age is not an array

	// Failed setters leave the stored value untouched.
	n, err := d.GetInt("age")
	require.NoError(t, err)
	assert.Equal(t, 31, n)
}

func TestIntArraySetterGetter(t *testing.T) {
	d := testDocument(t)

	// intlist is schema-declared as an array of integers.
	require.NoError(t, d.SetIntArray("intlist", []int{1, 2, 3}))
	arr, err := d.GetIntArray("intlist")
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, arr)

	// A string-array getter on an integer-array field is a mismatch.
	_, err = d.GetStringArray("intlist")
	require.Error(t, err)

	// tags is schema-declared as an array of strings — integer access rejects.
	require.Error(t, d.SetIntArray("tags", []int{9}))
	_, err = d.GetIntArray("tags")
	require.Error(t, err)
}
