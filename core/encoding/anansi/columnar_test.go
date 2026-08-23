package anansi_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// columnarPayloads mixes full-density, sparse, null-heavy and empty records
// so every state-map code (not-set / null / has-value) appears in the
// columns, across every leaf type of the test schema.
var columnarPayloads = []string{
	fullPayload,
	`{"count": 1, "title": "one"}`,
	`{}`,
	`{"count": 99, "tags": ["only", "two"]}`,
	`{"title": null, "score": 2.5, "address": {"street": "s", "zip": null}}`,
	`{"items": [{"sku": "X1", "qty": 3, "price": 1.5}], "nickname": null}`,
	fullPayload,
}

func columnarDocs(t *testing.T, cs *definition.CompiledSchema, payloads []string) []*container.DataContainer {
	t.Helper()
	docs := make([]*container.DataContainer, 0, len(payloads))
	for _, p := range payloads {
		doc := container.NewDataContainer()
		if err := cjson.DecodeJSONInto(cs, []byte(p), doc, nil); err != nil {
			t.Fatalf("DecodeJSONInto(%s): %v", p, err)
		}
		docs = append(docs, doc)
	}
	return docs
}

func dumpAll(t *testing.T, cs *definition.CompiledSchema, docs []*container.DataContainer) []map[string]any {
	t.Helper()
	out := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		dump, err := cjson.Dump(cs, d)
		if err != nil {
			t.Fatalf("Dump: %v", err)
		}
		out = append(out, dump)
	}
	return out
}

func TestBatchColumnar_RoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	docs := columnarDocs(t, cs, columnarPayloads)
	want := dumpAll(t, cs, docs)

	wire, err := anansi.EncodeBatchColumnar(cs, docs, 0)
	if err != nil {
		t.Fatalf("EncodeBatchColumnar: %v", err)
	}
	if len(wire) == 0 {
		t.Fatal("empty wire payload")
	}
	if got := wire[0] & 0x0B; got != 0x0A { // Batch (10) + columnar bit (bit 3)
		t.Fatalf("flags byte %#x does not mark a columnar batch packet", wire[0])
	}

	decoded, version, err := anansi.DecodeBatchRows(cs, wire, nil)
	if err != nil {
		t.Fatalf("DecodeBatchRows: %v", err)
	}
	if version != 0 {
		t.Fatalf("unexpected version %d", version)
	}
	got := dumpAll(t, cs, decoded)
	if len(got) != len(want) {
		t.Fatalf("record count mismatch: want %d got %d", len(want), len(got))
	}
	for i := range want {
		if !reflect.DeepEqual(want[i], got[i]) {
			t.Fatalf("record %d mismatch:\n want: %v\n got:  %v", i, want[i], got[i])
		}
	}
}

func TestBatchColumnar_ParityWithRowOriented(t *testing.T) {
	cs := loadCompiledSchema(t)
	docs := columnarDocs(t, cs, columnarPayloads)

	rowWire, err := anansi.EncodeBatchRows(cs, docs, 7)
	if err != nil {
		t.Fatalf("EncodeBatchRows: %v", err)
	}
	colWire, err := anansi.EncodeBatchColumnar(cs, docs, 7)
	if err != nil {
		t.Fatalf("EncodeBatchColumnar: %v", err)
	}
	if bytes.Equal(rowWire, colWire) {
		t.Fatal("row-oriented and columnar packets must differ")
	}

	want := dumpAll(t, cs, docs)
	for name, wire := range map[string][]byte{"row": rowWire, "columnar": colWire} {
		decoded, version, err := anansi.DecodeBatchRows(cs, wire, nil)
		if err != nil {
			t.Fatalf("decode %s batch: %v", name, err)
		}
		if version != 7 {
			t.Fatalf("%s batch version = %d, want 7", name, version)
		}
		got := dumpAll(t, cs, decoded)
		for i := range want {
			if !reflect.DeepEqual(want[i], got[i]) {
				t.Fatalf("%s batch record %d mismatch:\n want: %v\n got:  %v", name, i, want[i], got[i])
			}
		}
	}
}

func TestBatchColumnar_EmptyAndSingle(t *testing.T) {
	cs := loadCompiledSchema(t)

	empty := []*container.DataContainer{}
	wire, err := anansi.EncodeBatchColumnar(cs, empty, 0)
	if err != nil {
		t.Fatalf("encode empty batch: %v", err)
	}
	decoded, _, err := anansi.DecodeBatchRows(cs, wire, nil)
	if err != nil {
		t.Fatalf("decode empty batch: %v", err)
	}
	if len(decoded) != 0 {
		t.Fatalf("expected 0 records, got %d", len(decoded))
	}

	single := []*container.DataContainer{columnarDocs(t, cs, []string{`{"count": 5}`})[0]}
	wire, err = anansi.EncodeBatchColumnar(cs, single, 0)
	if err != nil {
		t.Fatalf("encode single-record batch: %v", err)
	}
	decoded, _, err = anansi.DecodeBatchRows(cs, wire, nil)
	if err != nil {
		t.Fatalf("decode single-record batch: %v", err)
	}
	dump, err := cjson.Dump(cs, decoded[0])
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	if dump["count"] != int64(5) {
		t.Fatalf("count = %v, want 5", dump["count"])
	}
}

// TestBatchColumnar_VersionEpochs pins full-version round-trips across every
// epoch boundary (spec 2.2): the epoch bits must survive encoding, not just
// the low version byte. Regression: EncodeBatchColumnar originally dropped
// the epoch, so versions > 255 decoded as version & 0xFF.
func TestBatchColumnar_VersionEpochs(t *testing.T) {
	cs := loadCompiledSchema(t)
	docs := columnarDocs(t, cs, []string{`{"count": 5}`, `{}`})

	for _, v := range []uint16{0, 7, 255, 256, 300, 511, 512, 767, 768, 1023} {
		wire, err := anansi.EncodeBatchColumnar(cs, docs, v)
		if err != nil {
			t.Fatalf("EncodeBatchColumnar(%d): %v", v, err)
		}
		_, got, err := anansi.DecodeBatchRows(cs, wire, nil)
		if err != nil {
			t.Fatalf("DecodeBatchRows(%d): %v", v, err)
		}
		if got != v {
			t.Fatalf("fullVersion = %d, want %d", got, v)
		}
	}

	if _, err := anansi.EncodeBatchColumnar(cs, docs, 1024); err == nil {
		t.Fatal("expected error for out-of-range schema version 1024")
	}
}

func TestBatchColumnar_PooledDecode(t *testing.T) {
	cs := loadCompiledSchema(t)
	docs := columnarDocs(t, cs, columnarPayloads)
	want := dumpAll(t, cs, docs)

	wire, err := anansi.EncodeBatchColumnar(cs, docs, 0)
	if err != nil {
		t.Fatalf("EncodeBatchColumnar: %v", err)
	}

	pool := container.NewPool()
	decoded, _, err := anansi.DecodeBatchRows(cs, wire, pool)
	if err != nil {
		t.Fatalf("pooled DecodeBatchRows: %v", err)
	}
	got := dumpAll(t, cs, decoded)
	for i := range want {
		if !reflect.DeepEqual(want[i], got[i]) {
			t.Fatalf("record %d mismatch:\n want: %v\n got:  %v", i, want[i], got[i])
		}
	}
	for _, d := range decoded {
		pool.Put(d)
	}
}

func TestBatchColumnar_Deterministic(t *testing.T) {
	cs := loadCompiledSchema(t)
	docs := columnarDocs(t, cs, columnarPayloads)

	first, err := anansi.EncodeBatchColumnar(cs, docs, 0)
	if err != nil {
		t.Fatalf("first encode: %v", err)
	}
	second, err := anansi.EncodeBatchColumnar(cs, docs, 0)
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("encoding the same batch twice produced different bytes")
	}
}

func TestBatchColumnar_TruncatedReturnsError(t *testing.T) {
	cs := loadCompiledSchema(t)
	docs := columnarDocs(t, cs, columnarPayloads)

	wire, err := anansi.EncodeBatchColumnar(cs, docs, 0)
	if err != nil {
		t.Fatalf("EncodeBatchColumnar: %v", err)
	}

	for cut := 4; cut < len(wire); cut++ {
		if _, _, err := anansi.DecodeBatchRows(cs, wire[:cut], nil); err == nil {
			t.Fatalf("expected error decoding truncated columnar packet at cut=%d", cut)
		}
	}
}

// TestBoolPacking_DenseWireSize pins the spec 2.5 bool packing end to end:
// 10 all-set bool fields must yield header(2) + state map ceil(20/8)=3 +
// bool block ceil(10/8)=2 = exactly 7 bytes. Before packing, the block
// alone cost 10 bytes.
func TestBoolPacking_DenseWireSize(t *testing.T) {
	const schema = `{
		"version": "1.0.0",
		"name": "flags",
		"fields": {
			"b0": {"name": "b0", "type": "boolean"}, "b1": {"name": "b1", "type": "boolean"},
			"b2": {"name": "b2", "type": "boolean"}, "b3": {"name": "b3", "type": "boolean"},
			"b4": {"name": "b4", "type": "boolean"}, "b5": {"name": "b5", "type": "boolean"},
			"b6": {"name": "b6", "type": "boolean"}, "b7": {"name": "b7", "type": "boolean"},
			"b8": {"name": "b8", "type": "boolean"}, "b9": {"name": "b9", "type": "boolean"}
		}
	}`
	cs, err := compileSchemaBytes(t, schema)
	if err != nil {
		t.Fatalf("compile+link: %v", err)
	}

	payload := `{"b0": true, "b1": false, "b2": true, "b3": true, "b4": false, "b5": true, "b6": false, "b7": false, "b8": true, "b9": true}`
	d := containerFromJSON(t, cs, payload)

	wire, err := anansi.EncodeDense(cs, d, 0)
	if err != nil {
		t.Fatalf("EncodeDense: %v", err)
	}
	if len(wire) != 7 {
		t.Fatalf("dense packet = %d bytes, want exactly 7 (2 hdr + 3 state map + 2 packed bools)", len(wire))
	}

	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := containerJSON(t, cs, out); got != containerJSON(t, cs, d) {
		t.Fatal("bool round-trip mismatch")
	}
}

// compileSchemaBytes parses, compiles and links a schema JSON string.
func compileSchemaBytes(t *testing.T, schema string) (*definition.CompiledSchema, error) {
	t.Helper()
	s, err := definition.FromJSON([]byte(schema))
	if err != nil {
		return nil, err
	}
	rs, err := definition.Compile(s)
	if err != nil {
		return nil, err
	}
	return definition.Link(rs)
}
