package definition_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/container"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Differential parity suite: for every schema family and payload, the
// ContainerValidator's issues (code, message, path — order-insensitive) must
// equal DocumentValidator's issues on the same logical data expressed as a
// map. The map is produced by definition.Materialize from the same container,
// so both lanes see exactly the data the container holds (int64s stay int64,
// as in the document layer's ToMap).
//
// Valid payloads reach the container through the production JSON decoder.
// Payloads the decoder itself rejects (it enforces required fields) are built
// directly with typed writes at binding-resolved keys, so the validator —
// not the decoder — is what observes the violation.

// mustSchema unmarshals a schema JSON document.
func cvSchema(t *testing.T, raw string) *definition.Schema {
	t.Helper()
	var sc definition.Schema
	if err := json.Unmarshal([]byte(raw), &sc); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	return &sc
}

// mustCS compiles and links a schema into its flat CompiledSchema.
func cvCS(t *testing.T, sc *definition.Schema) *definition.CompiledSchema {
	t.Helper()
	rs, err := definition.Compile(sc)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	return cs
}

// cvKey resolves a dotted path to its flat container key.
func cvKey(t *testing.T, cs *definition.CompiledSchema, path string) container.DataContainerKey {
	t.Helper()
	rp, fd, err := binding.ResolveViewPath(cs, nil, 0, path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	k, err := binding.ComputeLeafKey(cs, fd, rp)
	if err != nil {
		t.Fatalf("leaf key %s: %v", path, err)
	}
	return k
}

// cvSetAny writes a Go value into the matching typed slot.
func cvSetAny(c *container.DataContainer, k container.DataContainerKey, v any) error {
	switch tv := v.(type) {
	case string:
		return c.SetString(k, tv)
	case int:
		return c.SetInt(k, int64(tv))
	case int64:
		return c.SetInt(k, tv)
	case float64:
		return c.SetFloat(k, tv)
	case bool:
		return c.SetBool(k, tv)
	case []string:
		return c.SetArrayString(k, tv)
	case []int:
		vals := make([]int64, len(tv))
		for i, n := range tv {
			vals[i] = int64(n)
		}
		return c.SetArrayInt(k, vals)
	case []any:
		for _, el := range tv {
			if err := c.AppendString(k, el.(string)); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("cvSetAny: unsupported %T", v)
}

// cvSpec describes a container to build directly.
type cvSpec struct {
	fields map[string]any              // dotted path → value
	nulls  []string                    // dotted paths to mark explicitly null
	items  map[string][]map[string]any // array-of-object path → item fields (item-local names)
}

// cvBuild decodes through the production decoder when possible (valid
// payloads), otherwise assembles the container with typed writes.
func cvBuild(t *testing.T, cs *definition.CompiledSchema, sc *definition.Schema, payload string, spec *cvSpec) *container.DataContainer {
	t.Helper()
	if spec == nil {
		c := container.NewDataContainer()
		if err := cjson.DecodeJSONInto(cs, []byte(payload), c, nil); err != nil {
			t.Fatalf("decode %s: %v", payload, err)
		}
		return c
	}

	c := container.NewDataContainer()
	for path, v := range spec.fields {
		if err := cvSetAny(c, cvKey(t, cs, path), v); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	for arrPath, itemMaps := range spec.items {
		_, fdArr, err := binding.ResolveViewPath(cs, nil, 0, arrPath)
		if err != nil {
			t.Fatalf("resolve %s: %v", arrPath, err)
		}
		children := make([]*container.DataContainer, 0, len(itemMaps))
		for _, im := range itemMaps {
			child := container.NewDataContainer()
			for k, v := range im {
				if err := cvSetAny(child, cvKey(t, cs, arrPath+"."+k), v); err != nil {
					t.Fatalf("set %s.%s: %v", arrPath, k, err)
				}
			}
			children = append(children, child)
		}
		if err := c.SetArrayObject(binding.InternalKey(fdArr), children); err != nil {
			t.Fatalf("set %s: %v", arrPath, err)
		}
	}
	for _, path := range spec.nulls {
		c.SetNull(cvKey(t, cs, path))
	}
	return c
}

// cvNormalize renders issues as a sorted, comparable form.
func cvNormalize(issues []common.Issue) []string {
	out := make([]string, 0, len(issues))
	for _, is := range issues {
		out = append(out, fmt.Sprintf("%s|%s|%s", is.Path, is.Code, is.Message))
	}
	sort.Strings(out)
	return out
}

// cvCompare validates c through both lanes in all three modes and fails on
// any divergence.
func cvCompare(t *testing.T, sc *definition.Schema, fmap definition.PredicateMap, c *container.DataContainer) {
	t.Helper()
	cs := cvCS(t, sc)
	m := definition.Materialize(cs, c)

	cv, err := definition.NewContainerValidator(sc, fmap)
	if err != nil {
		t.Fatalf("container validator: %v", err)
	}
	dv, err := definition.NewDocumentValidator(sc, fmap)
	if err != nil {
		t.Fatalf("document validator: %v", err)
	}

	lanes := []struct {
		name    string
		call    func() ([]common.Issue, bool)
		mapCall func() ([]common.Issue, bool)
	}{
		{"strict", func() ([]common.Issue, bool) { return cv.Validate(c) }, func() ([]common.Issue, bool) { return dv.Validate(m) }},
		{"partial", func() ([]common.Issue, bool) { return cv.ValidatePartial(c) }, func() ([]common.Issue, bool) { return dv.ValidatePartial(m) }},
		{"loose", func() ([]common.Issue, bool) { return cv.ValidateLoose(c) }, func() ([]common.Issue, bool) { return dv.ValidateLoose(m) }},
	}
	for _, ln := range lanes {
		gotIssues, gotOK := ln.call()
		wantIssues, wantOK := ln.mapCall()
		got, want := cvNormalize(gotIssues), cvNormalize(wantIssues)
		if len(got) != len(want) {
			t.Errorf("[%s] issue count differs:\n container: %v\n map:       %v", ln.name, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%s] issue differs:\n container: %s\n map:       %s", ln.name, got[i], want[i])
			}
		}
		if gotOK != wantOK {
			t.Errorf("[%s] ok flag differs: container=%v map=%v", ln.name, gotOK, wantOK)
		}
	}
}

// cvDecoded runs the differential for a decode-legal JSON payload.
func cvDecoded(t *testing.T, sc *definition.Schema, payload string) {
	t.Helper()
	cvCompare(t, sc, nil, cvBuild(t, cvCS(t, sc), sc, payload, nil))
}

// cvDirect runs the differential for a directly built container (decoder-
// rejected payloads: required violations, non-nullable nulls).
func cvDirect(t *testing.T, sc *definition.Schema, spec *cvSpec) {
	t.Helper()
	cvCompare(t, sc, nil, cvBuild(t, cvCS(t, sc), sc, "", spec))
}

// requireCodes asserts the container lane produced the given codes (sanity
// guard: both lanes empty would make a differential vacuous).
func cvRequireCodes(t *testing.T, sc *definition.Schema, spec *cvSpec, wantCodes ...string) {
	t.Helper()
	if spec == nil {
		spec = &cvSpec{fields: map[string]any{}}
	}
	c := cvBuild(t, cvCS(t, sc), sc, "", spec)
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("container validator: %v", err)
	}
	issues, _ := cv.Validate(c)
	got := map[string]bool{}
	for _, is := range issues {
		got[is.Code] = true
	}
	for _, code := range wantCodes {
		if !got[code] {
			t.Errorf("expected issue %s, got %v", code, cvNormalize(issues))
		}
	}
}

// =============================================================================
// Schemas
// =============================================================================

const cvFastSchemaJSON = `{
  "version": "1.0.0",
  "name": "fast",
  "fields": {
    "id":     { "name": "id",     "type": "string",  "required": true },
    "qty":    { "name": "qty",    "type": "integer", "required": true },
    "price":  { "name": "price",  "type": "number" },
    "active": { "name": "active", "type": "boolean" },
    "blob":   { "name": "blob",   "type": "bytes" },
    "grade":  { "name": "grade",  "type": "enum", "schema": { "id": "gradeVals" } },
    "level":  { "name": "level",  "type": "enum", "schema": { "id": "levelVals" } },
    "note":   { "name": "note",   "type": "string",  "nullable": true, "required": true },
    "strict": { "name": "strict", "type": "string",  "nullable": false },
    "tags":   { "name": "tags",   "type": "array",   "schema": { "type": "string" } },
    "scores": { "name": "scores", "type": "array",   "schema": { "type": "integer" } }
  },
  "schemas": {
    "gradeVals": { "name": "gradeVals", "type": "string",  "values": ["a", "b", "c"] },
    "levelVals": { "name": "levelVals", "type": "integer", "values": [1, 2, 3] }
  }
}`

const cvNestedSchemaJSON = `{
  "version": "1.0.0",
  "name": "nested",
  "fields": {
    "id":      { "name": "id",      "type": "string",  "required": true },
    "address": { "name": "address", "type": "object",  "schema": { "id": "addr" }, "required": true },
    "optaddr": { "name": "optaddr", "type": "object", "schema": { "id": "addr" } }
  },
  "schemas": {
    "addr": {
      "name": "addr",
      "fields": {
        "street": { "name": "street", "type": "string", "required": true },
        "zip":    { "name": "zip",    "type": "integer" },
        "kind":   { "name": "kind",   "type": "enum", "schema": { "id": "kindVals" } }
      }
    },
    "kindVals": { "name": "kindVals", "type": "string", "values": ["home", "work"] }
  }
}`

const cvArrObjSchemaJSON = `{
  "version": "1.0.0",
  "name": "arr_obj",
  "fields": {
    "id":    { "name": "id",    "type": "string", "required": true },
    "items": { "name": "items", "type": "array",  "schema": { "id": "item" }, "required": true }
  },
  "schemas": {
    "item": {
      "name": "item",
      "fields": {
        "sku":   { "name": "sku",   "type": "string",  "required": true },
        "count": { "name": "count", "type": "integer", "required": true },
        "kind":  { "name": "kind",  "type": "enum", "schema": { "id": "itemKindVals" } }
      }
    },
    "itemKindVals": { "name": "itemKindVals", "type": "string", "values": ["new", "used"] }
  }
}`

const cvRecordSchemaJSON = `{
  "version": "1.0.0",
  "name": "with_record",
  "fields": {
    "id":      { "name": "id",      "type": "string", "required": true },
    "profile": { "name": "profile", "type": "record", "schema": { "id": "addr" } }
  },
  "schemas": {
    "addr": {
      "name": "addr",
      "fields": {
        "street": { "name": "street", "type": "string" }
      }
    }
  }
}`

// cvConstrainedSchemaJSON attaches a global constraint on field "id",
// evaluated through a caller-supplied predicate.
const cvConstrainedSchemaJSON = `{
  "version": "1.0.0",
  "name": "constrained",
  "fields": {
    "id": { "name": "id", "type": "string", "required": true }
  },
  "constraints": {
    "id_len": {
      "name": "id_len",
      "predicate": "minLen3",
      "fields": ["id"]
    }
  }
}`

// cvMinLen3 is the predicate referenced by the constrained schema: the value
// under keys[0] must be at least three characters.
func cvMinLen3(params definition.PredicateParams) []common.Issue {
	m, ok := params.Data.(map[string]any)
	if !ok {
		return []common.Issue{{Code: "INVALID_ROOT_TYPE"}}
	}
	for _, k := range params.Keys {
		v, ok := m[string(k)].(string)
		if !ok || len(v) < 3 {
			return []common.Issue{{Code: "ID_TOO_SHORT", Message: "id shorter than 3"}}
		}
	}
	return nil
}

// =============================================================================
// Differential matrices
// =============================================================================

func TestContainerValidator_Differential_Fast(t *testing.T) {
	sc := cvSchema(t, cvFastSchemaJSON)
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	if !cv.FastPath() {
		t.Fatalf("expected fast path, reason=%q", cv.UnsupportedReason())
	}

	for _, payload := range []string{
		`{"id":"x","qty":1,"price":9.5,"active":true,"grade":"a","level":2,"note":"n","strict":"s","tags":["t"],"scores":[1,2]}`,
		`{"id":"x","qty":1,"note":null}`,
		`{"id":"x","qty":1,"note":"n","grade":"z"}`,
		`{"id":"x","qty":1,"note":"n","level":9}`,
		`{"id":"x","qty":1,"note":"n","grade":"z","level":9}`,
		`{"id":"x","qty":1,"note":"n","tags":[],"scores":[7]}`,
	} {
		payload := payload
		t.Run("decode:"+payload, func(t *testing.T) { cvDecoded(t, sc, payload) })
	}

	for _, spec := range []*cvSpec{
		{fields: map[string]any{}},
		{fields: map[string]any{"id": "x"}},
		{fields: map[string]any{"id": "x", "qty": 1}, nulls: []string{"strict"}},
		{fields: map[string]any{"id": "x", "qty": 1, "note": "n"}, nulls: []string{"price", "active"}},
	} {
		t.Run("direct", func(t *testing.T) { cvDirect(t, sc, spec) })
	}

	cvRequireCodes(t, sc, &cvSpec{fields: map[string]any{"id": "x"}}, "REQUIRED_FIELD_MISSING")
	cvRequireCodes(t, sc, &cvSpec{fields: map[string]any{"id": "x", "qty": 1}, nulls: []string{"strict"}}, "TYPE_MISMATCH")
}

func TestContainerValidator_Differential_Nested(t *testing.T) {
	sc := cvSchema(t, cvNestedSchemaJSON)
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	if !cv.FastPath() {
		t.Fatalf("expected fast path, reason=%q", cv.UnsupportedReason())
	}

	for _, payload := range []string{
		`{"id":"x","address":{"street":"main"}}`,
		`{"id":"x","address":{"street":"main","zip":1,"kind":"home"}}`,
		`{"id":"x","address":{"street":"main","kind":"other"}}`,
		`{"id":"x","address":{"street":"main"},"optaddr":{"street":"side"}}`,
	} {
		payload := payload
		t.Run("decode:"+payload, func(t *testing.T) { cvDecoded(t, sc, payload) })
	}

	for _, spec := range []*cvSpec{
		{fields: map[string]any{"id": "x"}},
		{fields: map[string]any{"id": "x", "address.zip": 1}},
		{fields: map[string]any{"id": "x", "address.street": "main"}, nulls: []string{"address.kind"}},
	} {
		t.Run("direct", func(t *testing.T) { cvDirect(t, sc, spec) })
	}
}

func TestContainerValidator_Differential_ArrayOfObjects(t *testing.T) {
	sc := cvSchema(t, cvArrObjSchemaJSON)
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	if !cv.FastPath() {
		t.Fatalf("expected fast path, reason=%q", cv.UnsupportedReason())
	}

	for _, payload := range []string{
		`{"id":"x","items":[]}`,
		`{"id":"x","items":[{"sku":"s","count":1}]}`,
		`{"id":"x","items":[{"sku":"s","count":1,"kind":"new"},{"sku":"t","count":2,"kind":"used"}]}`,
		`{"id":"x","items":[{"sku":"s","count":1,"kind":"broken"}]}`,
	} {
		payload := payload
		t.Run("decode:"+payload, func(t *testing.T) { cvDecoded(t, sc, payload) })
	}

	for _, spec := range []*cvSpec{
		{fields: map[string]any{"id": "x"}},
		{fields: map[string]any{"id": "x"}, items: map[string][]map[string]any{"items": {{"sku": "s"}}}},
		{fields: map[string]any{"id": "x"}, items: map[string][]map[string]any{"items": {{"count": 1}}}},
		{fields: map[string]any{"id": "x"}, items: map[string][]map[string]any{"items": {{"sku": "s", "count": 1}, {"sku": "t"}}}},
	} {
		t.Run("direct", func(t *testing.T) { cvDirect(t, sc, spec) })
	}
}

func TestContainerValidator_Differential_RecordFallsBack(t *testing.T) {
	sc := cvSchema(t, cvRecordSchemaJSON)
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	if cv.FastPath() {
		t.Fatalf("record payloads must force the map lane, reason=%q", cv.UnsupportedReason())
	}
	for _, payload := range []string{
		`{"id":"x"}`,
		`{"id":"x","profile":{"street":"side"}}`,
	} {
		payload := payload
		t.Run(payload, func(t *testing.T) { cvDecoded(t, sc, payload) })
	}
}

func TestContainerValidator_Differential_ConstraintFallsBack(t *testing.T) {
	sc := cvSchema(t, cvConstrainedSchemaJSON)
	fmap := definition.PredicateMap{"minLen3": cvMinLen3}
	cv, err := definition.NewContainerValidator(sc, fmap)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	if cv.FastPath() {
		t.Fatalf("cross-field constraints must force the map lane, reason=%q", cv.UnsupportedReason())
	}

	cs := cvCS(t, sc)
	for _, payload := range []string{
		`{"id":"abc"}`,
		`{"id":"ab"}`,
	} {
		t.Run(payload, func(t *testing.T) {
			c := cvBuild(t, cs, sc, payload, nil)
			cvCompare(t, sc, fmap, c)
		})
	}
}

// TestMaterializeNullArrayObjectMatchesToMap pins Materialize to the document
// layer's map semantics (binding.MaterializeSlot, which backs Document.ToMap):
// an explicitly-null array-of-object field materializes as an empty array,
// because GetArrayObject reports it as a present, empty slice. The fallback
// lane validates this map, so the two must agree.
func TestMaterializeNullArrayObjectMatchesToMap(t *testing.T) {
	sc := cvSchema(t, cvArrObjSchemaJSON)
	cs := cvCS(t, sc)

	c := container.NewDataContainer()
	if err := c.SetString(cvKey(t, cs, "id"), "x"); err != nil {
		t.Fatal(err)
	}
	c.SetNull(cvKey(t, cs, "items"))

	got := definition.Materialize(cs, c)
	want, err := binding.MaterializeSlot(cs, c, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Materialize differs from the document layer's ToMap:\n got=%#v\nwant=%#v", got, want)
	}
	if arr, ok := got["items"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("null array-of-object should materialize as an empty array, got %#v", got["items"])
	}
}

// =============================================================================
// Targeted behavior checks (beyond the differential)
// =============================================================================

func TestContainerValidator_RequiredAndEnumCodes(t *testing.T) {
	sc := cvSchema(t, cvFastSchemaJSON)
	cvRequireCodes(t, sc, &cvSpec{fields: map[string]any{"id": "x"}}, "REQUIRED_FIELD_MISSING")
	cvRequireCodes(t, sc, nil, "REQUIRED_FIELD_MISSING")

	// Bad enums survive the decoder (it does not enforce them) and surface
	// here as ENUM_VIOLATION.
	cs := cvCS(t, sc)
	c := cvBuild(t, cs, sc, `{"id":"x","qty":1,"note":"n","grade":"z","level":9}`, nil)
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	issues, ok := cv.Validate(c)
	if ok {
		t.Fatalf("bad enums must fail validation, got ok")
	}
	codes := map[string]bool{}
	for _, is := range issues {
		codes[is.Code] = true
	}
	if !codes["ENUM_VIOLATION"] {
		t.Fatalf("expected ENUM_VIOLATION, got %v", cvNormalize(issues))
	}
}

func TestContainerValidator_NullableNullSatisfiesRequired(t *testing.T) {
	sc := cvSchema(t, cvFastSchemaJSON)
	cs := cvCS(t, sc)
	c := cvBuild(t, cs, sc, "", &cvSpec{
		fields: map[string]any{"id": "x", "qty": 1},
		nulls:  []string{"note"},
	})
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	issues, ok := cv.Validate(c)
	if !ok || len(issues) != 0 {
		t.Fatalf("nullable null must pass, got %v", cvNormalize(issues))
	}
}

func TestContainerValidator_PartialSuppressesRequired(t *testing.T) {
	sc := cvSchema(t, cvFastSchemaJSON)
	c := container.NewDataContainer() // nothing set
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	issues, ok := cv.ValidatePartial(c)
	if !ok || len(issues) != 0 {
		t.Fatalf("partial mode must suppress required-field issues, got %v", cvNormalize(issues))
	}
	issues, ok = cv.ValidateLoose(c)
	if !ok || len(issues) != 0 {
		t.Fatalf("loose mode must suppress required-field issues, got %v", cvNormalize(issues))
	}
	issues, ok = cv.Validate(c)
	if ok || len(issues) == 0 {
		t.Fatalf("strict mode must report the missing required fields, got %v", cvNormalize(issues))
	}
}

func TestContainerValidator_FastPathIsAllocationFree(t *testing.T) {
	sc := cvSchema(t, cvFastSchemaJSON)
	cs := cvCS(t, sc)
	c := cvBuild(t, cs, sc, `{"id":"x","qty":1,"price":9.5,"grade":"a","level":2,"note":"n","strict":"s","tags":["t"],"scores":[1,2]}`, nil)
	cv, err := definition.NewContainerValidator(sc, nil)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	cv.Validate(c)
	if testing.Short() {
		return
	}
	if avg := testing.AllocsPerRun(200, func() {
		cv.Validate(c)
	}); avg > 0.001 {
		t.Fatalf("fast-path Validate allocated %v times per run", avg)
	}
}
