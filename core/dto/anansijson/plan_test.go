package anansijson

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// DIFFERENTIAL PARITY TESTS
// ============================================================================
//
// Every test drives BOTH the compiled-plan path (Marshal/Unmarshal) and the
// legacy container path (marshalLegacy/unmarshalLegacy) over the same input
// and requires identical observable behavior: same bytes on success, both
// erroring on failure. These tests are what licenses the fast path to run by
// default.

// --- shapes ---

type Address struct {
	Street string `anansi:"street" json:"street"`
	City   string `anansi:"city" json:"city"`
	Zip    string `anansi:"zip" json:"zip"`
}

type OrderLine struct {
	SKU       string  `anansi:"sku" json:"sku"`
	Quantity  int     `anansi:"qty" json:"qty"`
	UnitPrice float64 `anansi:"unit_price" json:"unit_price"`
}

type Order struct {
	ID       string      `anansi:"id" json:"id"`
	Customer string      `anansi:"customer" json:"customer"`
	Status   string      `anansi:"status" json:"status"`
	Priority int         `anansi:"priority" json:"priority"`
	Total    float64     `anansi:"total" json:"total"`
	Paid     bool        `anansi:"paid" json:"paid"`
	Tags     []string    `anansi:"tags" json:"tags"`
	Billing  Address     `anansi:"billing" json:"billing"`
	Lines    []OrderLine `anansi:"lines" json:"lines"`
}

type AllScalars struct {
	I32  int32     `anansi:"i32"`
	I64  int64     `anansi:"i64"`
	I    int       `anansi:"i"`
	U64  uint64    `anansi:"u64"`
	F64  float64   `anansi:"f64"`
	B    bool      `anansi:"b"`
	S    string    `anansi:"s"`
	IAry []int     `anansi:"iary"`
	UAry []uint64  `anansi:"uary"`
	FAry []float64 `anansi:"fary"`
	SAry []string  `anansi:"sary"`
	BAry []bool    `anansi:"bary"`
}

// UnsupportedScalars is a fallback shape: the legacy coercion layer
// (AsInt64/AsFloat64) only accepts int, int32, int64, uint64, float64 — every
// field here makes the legacy path error, and the plan must refuse so the
// fallback reproduces that.
type UnsupportedScalars struct {
	I8  int8    `anansi:"i8"`
	I16 int16   `anansi:"i16"`
	U   uint    `anansi:"u"`
	U8  uint8   `anansi:"u8"`
	U32 uint32  `anansi:"u32"`
	F32 float32 `anansi:"f32"`
}

type AllOmit struct {
	Name   string  `anansi:"name,omitempty"`
	Count  int     `anansi:"count,omitempty"`
	Ratio  float64 `anansi:"ratio,omitempty"`
	Flag   bool    `anansi:"flag,omitempty"`
	Items  []string `anansi:"items,omitempty"`
	Child  Inner   `anansi:"child,omitempty"`
}

type Inner struct {
	A string `anansi:"a"`
	B int    `anansi:"b,omitempty"`
}

type Nested struct {
	Level1 struct {
		Level2 struct {
			Leaf string `anansi:"leaf"`
			Num  int    `anansi:"num,omitempty"`
		} `anansi:"level2"`
		Other int `anansi:"other"`
	} `anansi:"level1"`
	Top string `anansi:"top"`
}

type Embedded struct {
	Address `anansi:"addr"` // hmm: embed flattening — tags on the embed name it
	Extra   string `anansi:"extra"`
}

type Wide struct {
	Aa string `anansi:"aa"`
	Ab string `anansi:"ab"`
	Ac string `anansi:"ac"`
	Ad string `anansi:"ad"`
	Ae string `anansi:"ae"`
	Af string `anansi:"af"`
	Ag string `anansi:"ag"`
	Ah string `anansi:"ah"`
	Ai string `anansi:"ai"`
	Aj string `anansi:"aj"`
	Ak string `anansi:"ak"`
	Al string `anansi:"al"`
	Am string `anansi:"am"`
	An string `anansi:"an"`
	Ao string `anansi:"ao"`
	Ap string `anansi:"ap"`
	Aq string `anansi:"aq"`
	Ar string `anansi:"ar"`
	As string `anansi:"as"`
	At string `anansi:"at"`
	Au string `anansi:"au"`
	Av string `anansi:"av"`
	Aw string `anansi:"aw"`
	Ax string `anansi:"ax"`
	Ay string `anansi:"ay"`
	Az string `anansi:"az"`
}

// --- fallback shapes (plan must refuse; behavior must match legacy) ---

type WithTime struct {
	When time.Time `anansi:"when"`
	Name string    `anansi:"name"`
}

type WithMap struct {
	Meta map[string]any `anansi:"meta"`
	Name string         `anansi:"name"`
}

type WithPtrScalar struct {
	Note *string `anansi:"note"`
	Name string  `anansi:"name"`
}

type WithPtrStruct struct {
	Billing *Address `anansi:"billing"`
	Name    string   `anansi:"name"`
}

type WithDottedTag struct {
	Leaf string `anansi:"meta.title"`
	Name string `anansi:"name"`
}

type WithBytes struct {
	Blob []byte `anansi:"blob"`
	Name string `anansi:"name"`
}

type WithGeometry struct {
	Area [][]float64 `anansi:"area"`
	Name string      `anansi:"name"`
}

type WithSliceOfPtr struct {
	Lines []*OrderLine `anansi:"lines"`
	Name  string       `anansi:"name"`
}

type EmptyChild struct {
	Child struct{} `anansi:"child"`
	Name  string   `anansi:"name"`
}

// --- harness ---

func assertMarshalParity(t *testing.T, name string, v any) {
	t.Helper()
	legacy, legacyErr := marshalLegacy(v)
	got, gotErr := Marshal(v)

	if (legacyErr == nil) != (gotErr == nil) {
		t.Fatalf("%s: error mismatch: legacy err=%v, fast err=%v", name, legacyErr, gotErr)
	}
	if legacyErr != nil {
		return
	}
	if !bytes.Equal(legacy, got) {
		t.Fatalf("%s: output mismatch\nlegacy: %s\nfast:   %s", name, legacy, got)
	}
}

func assertUnmarshalParity(t *testing.T, name string, data []byte, target any) {
	t.Helper()
	targetType := reflect.TypeOf(target)

	legacyPtr := reflect.New(targetType.Elem())
	legacyErr := unmarshalLegacy(data, legacyPtr.Interface())

	fastPtr := reflect.New(targetType.Elem())
	fastErr := Unmarshal(data, fastPtr.Interface())

	if (legacyErr == nil) != (fastErr == nil) {
		t.Fatalf("%s: error mismatch: legacy err=%v, fast err=%v", name, legacyErr, fastErr)
	}
	if legacyErr != nil {
		return
	}
	if !reflect.DeepEqual(legacyPtr.Elem().Interface(), fastPtr.Elem().Interface()) {
		t.Fatalf("%s: decode mismatch\nlegacy: %+v\nfast:   %+v",
			name, legacyPtr.Elem().Interface(), fastPtr.Elem().Interface())
	}
}

// requireFast reports whether the type actually took the compiled-plan path;
// fallback shapes are still parity-checked, but labeled.
func requireFast(t *testing.T, v any, want bool) bool {
	t.Helper()
	tt := reflect.TypeOf(v)
	for tt.Kind() == reflect.Pointer {
		tt = tt.Elem()
	}
	e := planFor(tt)
	if want && e.err != nil {
		t.Logf("note: %s refused by plan (fallback): %v", tt, e.err)
	}
	return e.err == nil
}

// --- marshal parity over representative values ---

func sampleOrder() Order {
	return Order{
		ID:       "ord-000123",
		Customer: "Acme <Corporation> & Co",
		Status:   "shipped",
		Priority: 2,
		Total:    199.99,
		Paid:     true,
		Tags:     []string{"priority", "gift", "export"},
		Billing: Address{
			Street: "1 Billing \"Way\"",
			City:   "Springfield",
			Zip:    "10001",
		},
		Lines: []OrderLine{
			{SKU: "SKU-001", Quantity: 3, UnitPrice: 19.99},
			{SKU: "SKU-002", Quantity: 1, UnitPrice: 139.99},
			{SKU: "SKU-003", Quantity: 2, UnitPrice: 9.99},
		},
	}
}

func TestMarshalParity(t *testing.T) {
	o := sampleOrder()
	if !requireFast(t, &o, true) {
		t.Fatalf("Order should take the fast path")
	}

	values := []struct {
		name string
		v    any
		fast bool
	}{
		{"order/populated", sampleOrder(), true},
		{"order/zero", Order{}, true},
		{"order/nil-lines", func() Order { v := sampleOrder(); v.Lines = nil; return v }(), true},
		{"order/empty-lines", func() Order { v := sampleOrder(); v.Lines = []OrderLine{}; return v }(), true},
		{"order/empty-line-elem", func() Order { v := sampleOrder(); v.Lines = []OrderLine{{}}; return v }(), true},
		{"order/nil-tags", func() Order { v := sampleOrder(); v.Tags = nil; return v }(), true},
		{"order/empty-tags", func() Order { v := sampleOrder(); v.Tags = []string{}; return v }(), true},
		{"order/empty-strings", func() Order { v := sampleOrder(); v.ID = ""; v.Customer = ""; return v }(), true},
		{"address/populated", Address{Street: "1 Market St", City: "Berlin", Zip: "10115"}, true},
		{"address/zero", Address{}, true},
		{"scalars/extremes", AllScalars{
			I32: math.MinInt32, I64: math.MinInt64, I: math.MinInt,
			U64: math.MaxUint64,
			F64: 1e308,
			B:   true, S: "hello\tworld\n\"quoted\"<html>&amp;",
			IAry: []int{1, -2, 3}, UAry: []uint64{1, 2, math.MaxUint64},
			FAry: []float64{1e-7, 1e21, 0.1, -0.0, 123456789.123456789},
			SAry: []string{"a", "", "<b>&"}, BAry: []bool{true, false},
		}, true},
		{"scalars/zero", AllScalars{}, true},
		{"scalars/nil-arrays", AllScalars{IAry: nil, SAry: nil}, true},
		{"scalars/empty-arrays", AllScalars{
			IAry: []int{}, UAry: []uint64{}, FAry: []float64{}, SAry: []string{}, BAry: []bool{},
		}, true},
		{"omit/zero", AllOmit{}, true},
		{"omit/partial", AllOmit{Name: "x", Flag: true}, true},
		{"omit/full", AllOmit{Name: "x", Count: 1, Ratio: 0.5, Flag: true, Items: []string{"a"},
			Child: Inner{A: "a", B: 1}}, true},
		{"omit/empty-items", AllOmit{Name: "x", Items: []string{}}, true},
		{"omit/empty-child-nonzero", AllOmit{Name: "x", Child: Inner{B: 3}}, true},
		{"nested/populated", nestedPopulated(), true},
		{"nested/zero-leaf", nestedZeroLeaf(), true},
		{"wide/populated", widePopulated(), true},
		{"wide/zero", Wide{}, true},

		// Fallback shapes: plan refuses, legacy owns them. Parity is still
		// asserted — refusal must never change behavior.
		{"fallback/time", WithTime{When: time.Unix(1700000000, 0).UTC(), Name: "x"}, false},
		{"fallback/map", WithMap{Meta: map[string]any{"k": "v"}, Name: "x"}, false},
		{"fallback/ptr-scalar", func() any { s := "hi"; return WithPtrScalar{Note: &s, Name: "x"} }(), false},
		{"fallback/ptr-struct", func() any {
			return WithPtrStruct{Billing: &Address{City: "SF"}, Name: "x"}
		}(), false},
		{"fallback/dotted-tag", WithDottedTag{Leaf: "x", Name: "y"}, false},
		{"fallback/bytes", WithBytes{Blob: []byte{1, 2, 3}, Name: "x"}, false},
		{"fallback/geometry", WithGeometry{Area: [][]float64{{1, 2}, {3, 4}}, Name: "x"}, false},
		{"fallback/slice-of-ptr", WithSliceOfPtr{Lines: []*OrderLine{{SKU: "a"}}, Name: "x"}, false},
		{"fallback/empty-child", EmptyChild{Name: "x"}, false},
		{"fallback/unsupported-scalars", UnsupportedScalars{I8: 1, U: 2, F32: 1.5}, false},
	}

	for _, tc := range values {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := requireFast(t, tc.v, tc.fast); got != tc.fast {
				t.Fatalf("plan support mismatch: want fast=%v got %v", tc.fast, got)
			}
			assertMarshalParity(t, tc.name, tc.v)
		})
	}
}

func nestedPopulated() Nested {
	var n Nested
	n.Level1.Level2.Leaf = "deep"
	n.Level1.Level2.Num = 7
	n.Level1.Other = 3
	n.Top = "top"
	return n
}

func nestedZeroLeaf() Nested {
	var n Nested
	n.Level1.Other = 1
	n.Top = "t"
	return n
}

func widePopulated() Wide {
	var w Wide
	// Fill every field with a distinct value to exercise ordering.
	names := []string{"aa", "ab", "ac", "ad", "ae", "af", "ag", "ah", "ai", "aj",
		"ak", "al", "am", "an", "ao", "ap", "aq", "ar", "as", "at", "au", "av", "aw", "ax", "ay", "az"}
	rv := reflect.ValueOf(&w).Elem()
	for i, n := range names {
		rv.FieldByName(strings.ToUpper(n[:1]) + n[1:]).SetString("v" + strings.Repeat("x", i))
	}
	return w
}

func TestMarshalNilPointer(t *testing.T) {
	var p *Order
	assertMarshalParity(t, "nil-ptr", p)
	var pv any
	assertMarshalParity(t, "nil-interface", pv)
	assertMarshalParity(t, "non-struct", 42)
}

// --- unmarshal parity ---

func TestUnmarshalParity(t *testing.T) {
	// Valid payloads: legacy-produced bytes must decode identically.
	payloads := []struct {
		name   string
		data   string
		target any
	}{
		{"order/full", string(mustMarshalLegacy(t, sampleOrder())), &Order{}},
		{"order/zero", string(mustMarshalLegacy(t, Order{})), &Order{}},
		{"order/missing-keys", `{"id":"x"}`, &Order{}},
		{"order/unknown-keys", `{"id":"x","nope":123,"also":{"deep":[1,2]}}`, &Order{}},
		{"order/nulls", `{"id":null,"total":null,"lines":null,"billing":null,"tags":null,"paid":null,"priority":null}`, &Order{}},
		{"order/empty-object", `{}`, &Order{}},
		{"order/null-document", `null`, &Order{}},
		{"scalars/full", string(mustMarshalLegacy(t, AllScalars{
			I32: -32, I64: -64, I: 1, U64: math.MaxUint64, F64: 1.25, B: true, S: "s",
			IAry: []int{1, 2}, UAry: []uint64{3}, FAry: []float64{0.5}, SAry: []string{"x"}, BAry: []bool{true},
		})), &AllScalars{}},
		{"scalars/zero", string(mustMarshalLegacy(t, AllScalars{})), &AllScalars{}},
		{"nested/full", string(mustMarshalLegacy(t, nestedPopulated())), &Nested{}},
		{"nested/partial", `{"level1":{"level2":{"leaf":"l"}},"top":"t"}`, &Nested{}},
		{"lines/empty-elem", `{"lines":[{}]}`, &Order{}},
		{"lines/whitespace", "  \t\n { \"id\" : \"x\" , \"priority\" : 4 } \n ", &Order{}},
		{"strings/escaped", `{"id":"a\"b\\c\nd\ne<>&"}`, &Order{}},
		{"strings/unicode", `{"id":"héllo \u2028 \U0001F600"}`, &Order{}},
		{"strings/surrogates", `{"id":"😀"}`, &Order{}},
		{"numbers/floats", `{"total":3.5,"unit":0}`, &struct {
			Total float64 `anansi:"total"`
			Unit  float64 `anansi:"unit"`
		}{}},
		{"numbers/ints", `{"priority":9223372036854775807}`, &Order{}},
		{"arrays/empty", `{"tags":[],"lines":[]}`, &Order{}},
	}

	for _, tc := range payloads {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertUnmarshalParity(t, tc.name, []byte(tc.data), tc.target)
		})
	}
}

func TestUnmarshalParityErrors(t *testing.T) {
	// Both paths must reject these; only error-vs-nil parity is asserted —
	// message text legitimately differs.
	bad := []struct {
		name   string
		data   string
		target any
	}{
		{"int/fractional", `{"priority":3.5}`, &Order{}},
		{"int/float-form", `{"priority":3.0}`, &Order{}},
		{"int/exponent", `{"priority":3e2}`, &Order{}},
		{"int/overflow", `{"priority":9223372036854775808}`, &Order{}},
		{"int/string", `{"priority":"3"}`, &Order{}},
		{"int/bool", `{"priority":true}`, &Order{}},
		{"int/object", `{"priority":{}}`, &Order{}},
		{"int/array", `{"priority":[]}`, &Order{}},
		{"float/string", `{"total":"3.5"}`, &Order{}},
		{"bool/string", `{"paid":"true"}`, &Order{}},
		{"bool/number", `{"paid":1}`, &Order{}},
		{"string/number", `{"id":5}`, &Order{}},
		{"string/object", `{"id":{}}`, &Order{}},
		{"object/scalar", `{"billing":5}`, &Order{}},
		{"object/array", `{"billing":[1]}`, &Order{}},
		{"array/scalar", `{"lines":5}`, &Order{}},
		{"array/object-elem", `{"lines":[5]}`, &Order{}},
		{"array/null-elem", `{"lines":[null]}`, &Order{}},
		{"array/string-elem", `{"tags":[1]}`, &Order{}},
		{"array/bool-elem", `{"tags":[true]}`, &Order{}},
		{"nested/mismatch", `{"level1":5}`, &Nested{}},
		{"trailing/garbage", `{"id":"x"} trailing`, &Order{}},
		{"trailing/second-doc", `{"id":"x"}{"id":"y"}`, &Order{}},
		{"empty/input", ``, &Order{}},
		{"empty/only-ws", `   `, &Order{}},
		{"malformed/truncated", `{"id":`, &Order{}},
		{"malformed/no-colon", `{"id" "x"}`, &Order{}},
		{"malformed/bad-literal", `{"paid":tru}`, &Order{}},
		{"malformed/bad-number", `{"priority":01}`, &Order{}},
		{"malformed/leading-plus", `{"priority":+1}`, &Order{}},
		{"malformed/lone-comma", `{"id":"x",}`, &Order{}},
		{"malformed/bad-escape", `{"id":"\x"}`, &Order{}},
		{"malformed/control-in-string", "{\"id\":\"a\x01b\"}", &Order{}},
		{"malformed/unterminated-string", `{"id":"abc}`, &Order{}},
		{"malformed/unterminated-object", `{"id":"x"`, &Order{}},
		{"malformed/array-no-close", `{"tags":["a"`, &Order{}},
		{"malformed/top-level-array", `[1]`, &Order{}},
		{"malformed/top-level-scalar", `5`, &Order{}},
		{"scalars/type-swap", string(mustMarshalLegacy(t, AllScalars{I32: 1})), &Order{}},
	}

	for _, tc := range bad {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertUnmarshalParity(t, tc.name, []byte(tc.data), tc.target)
		})
	}
}

func TestUnmarshalNilPointer(t *testing.T) {
	var p *Order
	legacyErr := unmarshalLegacy([]byte(`{}`), p)
	fastErr := Unmarshal([]byte(`{}`), p)
	if (legacyErr == nil) != (fastErr == nil) {
		t.Fatalf("nil target: legacy err=%v fast err=%v", legacyErr, fastErr)
	}
}

// --- round trips through the fast path ---

func TestFastRoundTrip(t *testing.T) {
	shapes := []any{sampleOrder(), Address{City: "x"}, AllScalars{
		I32: -1, I64: 9, U64: 7, F64: 1.5, B: true, S: "s",
		IAry: []int{1, 2}, UAry: []uint64{3}, FAry: []float64{2.5}, SAry: []string{"a"}, BAry: []bool{false},
	}, nestedPopulated(), AllOmit{Name: "n", Child: Inner{A: "a"}}}
	for _, in := range shapes {
		b, err := Marshal(in)
		if err != nil {
			t.Fatalf("Marshal(%T): %v", in, err)
		}
		out := reflect.New(reflect.TypeOf(in)).Interface()
		if err := Unmarshal(b, out); err != nil {
			t.Fatalf("Unmarshal(%T): %v (payload %s)", in, err, b)
		}
		if got := reflect.ValueOf(out).Elem().Interface(); !reflect.DeepEqual(in, got) {
			t.Fatalf("%T round trip mismatch:\n in=%+v\nout=%+v\npayload=%s", in, in, got, b)
		}
	}
}

// Determinism: repeated marshals through the fast path are byte-identical.
func TestFastDeterministic(t *testing.T) {
	first, err := Marshal(sampleOrder())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := Marshal(sampleOrder())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, again) {
			t.Fatal("fast marshal is nondeterministic")
		}
	}
}

func mustMarshalLegacy(t *testing.T, v any) []byte {
	t.Helper()
	b, err := marshalLegacy(v)
	if err != nil {
		t.Fatalf("legacy marshal(%T): %v", v, err)
	}
	return b
}
