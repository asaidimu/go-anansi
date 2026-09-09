package anansi_test

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// stringDataPtr returns the address of a string's underlying bytes (0 for
// the empty string).
func stringDataPtr(s string) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(unsafe.StringData(s)))
}

func mustDump(t *testing.T, cs *definition.CompiledSchema, doc *container.DataContainer) map[string]any {
	t.Helper()
	m, err := cjson.Dump(cs, doc)
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	return m
}

func dumpStr(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	s, ok := m[key].(string)
	if !ok {
		t.Fatalf("dump[%q] = %T, want string", key, m[key])
	}
	return s
}

func itemMapAt(t *testing.T, m map[string]any, i int) map[string]any {
	t.Helper()
	raw, ok := m["items"].([]any)
	if !ok || len(raw) <= i {
		t.Fatalf("dump[items][%d] missing", i)
	}
	e, ok := raw[i].(map[string]any)
	if !ok {
		t.Fatalf("dump[items][%d] = %T, want map", i, raw[i])
	}
	return e
}

func TestStringAliasing_RoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	wire, err := anansi.SerializeAnansi(cs, doc, 300)
	if err != nil {
		t.Fatal(err)
	}

	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil); err != nil {
		t.Fatalf("aliased decode: %v", err)
	}
	if got := containerJSON(t, cs, out); got != want {
		t.Fatalf("aliased round-trip mismatch:\n want: %s\n got:  %s", want, got)
	}
	if len(out.Backing()) == 0 {
		t.Fatal("aliased decode must attach a backing buffer")
	}
}

// TestStringAliasing_ZeroCopy proves strings view the backing in place:
// decoded string data pointers lie within the backing buffer's range — for
// root-level, flattened-object, and nested-element strings alike.
func TestStringAliasing_ZeroCopy(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil); err != nil {
		t.Fatal(err)
	}

	backing := out.Backing()
	lo := uintptr(unsafe.Pointer(&backing[0]))
	hi := uintptr(unsafe.Pointer(&backing[len(backing)-1]))

	m := mustDump(t, cs, out)
	for name, s := range map[string]string{
		"title":    dumpStr(t, m, "title"),
		"address":  dumpStr(t, m, "address.city"),
		"item.sku": dumpStr(t, itemMapAt(t, m, 0), "items.sku"),
	} {
		p := stringDataPtr(s)
		if p < lo || p > hi {
			t.Fatalf("%s data ptr %#x outside backing [%#x,%#x]: not zero-copy", name, p, lo, hi)
		}
	}
}

// TestStringAliasing_InputNotRetained pins the safety contract: the caller
// may reuse its input buffer after an aliased decode, because the decoder
// snapshots into private backing rather than borrowing the input.
func TestStringAliasing_InputNotRetained(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatal(err)
	}

	input := append([]byte(nil), wire...) // a buffer the caller owns
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, input, out, nil); err != nil {
		t.Fatal(err)
	}

	// Trash the caller's buffer post-decode: aliased strings must be
	// unaffected because they point into private backing.
	for i := range input {
		input[i] = 'x'
	}
	title := dumpStr(t, mustDump(t, cs, out), "title")
	if !strings.Contains(title, "hello world") {
		t.Fatalf("aliased value corrupted by input mutation: %q", title)
	}
}

// TestStringAliasing_PoolRetention verifies amortization semantics: Clear
// keeps backing capacity for reuse and a pooled Get→decode cycle produces
// correct results on reused buffers.
func TestStringAliasing_PoolRetention(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatal(err)
	}

	pool := container.NewPool()
	first := pool.Get()
	if _, err := anansi.DecodeAnansiInto(cs, wire, first, nil); err != nil {
		t.Fatal(err)
	}
	capAfterFirst := cap(first.Backing())
	if capAfterFirst == 0 {
		t.Fatal("expected retained backing capacity")
	}
	pool.Put(first)

	second := pool.Get()
	if cap(second.Backing()) != capAfterFirst {
		t.Fatal("Clear must retain backing capacity for amortized reuse")
	}
	defer pool.Put(second)
	if _, err := anansi.DecodeAnansiInto(cs, wire, second, nil); err != nil {
		t.Fatal(err)
	}
	if got := containerJSON(t, cs, second); got != want {
		t.Fatal("reused-buffer aliased decode mismatch")
	}
}

// TestStringAliasing_BatchSharesBacking checks that all batch records alias
// one shared backing buffer.
func TestStringAliasing_BatchSharesBacking(t *testing.T) {
	cs := loadCompiledSchema(t)
	docs := columnarDocs(t, cs, columnarPayloads)

	wire, err := anansi.EncodeBatchColumnar(cs, docs, 0)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := anansi.DecodeBatchRows(cs, wire, nil)
	if err != nil {
		t.Fatal(err)
	}
	var shared []byte
	for i, d := range decoded {
		b := d.Backing()
		if len(b) == 0 {
			t.Fatalf("record %d has no backing", i)
		}
		if shared == nil {
			shared = b
			continue
		}
		if &shared[0] != &b[0] {
			t.Fatalf("record %d does not share the batch backing", i)
		}
	}
}

// TestStringAliasing_CompressedRoundTrip covers aliasing through the
// transform stack, where the backing is the already-private decompressed
// buffer (no second copy).
func TestStringAliasing_CompressedRoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	wire, err := anansi.SerializeAnansi(cs, doc, 0,
		anansi.WithCompression(), anansi.WithIntegrity())
	if err != nil {
		t.Fatal(err)
	}
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil); err != nil {
		t.Fatal(err)
	}
	if got := containerJSON(t, cs, out); got != want {
		t.Fatal("compressed aliased round-trip mismatch")
	}
}

// TestCopyStrings_OptOut verifies the escape hatch: no backing is attached
// and every string materializes as its own allocation.
func TestCopyStrings_OptOut(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil, anansi.WithCopyStrings()); err != nil {
		t.Fatal(err)
	}
	if got := containerJSON(t, cs, out); got != want {
		t.Fatal("copy-strings round-trip mismatch")
	}
	if len(out.Backing()) != 0 {
		t.Fatal("WithCopyStrings must not attach a backing buffer")
	}
}
