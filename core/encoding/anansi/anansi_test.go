package anansi_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/document"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func loadCompiledSchema(t *testing.T) *definition.CompiledSchema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	s, err := definition.FromJSON(data)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	resolved, err := definition.Compile(s)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	cs, err := definition.Link(resolved)
	if err != nil {
		t.Fatalf("link schema: %v", err)
	}
	return cs
}

func containerFromJSON(t *testing.T, cs *definition.CompiledSchema, payload string) *container.DataContainer {
	t.Helper()
	doc := container.NewDataContainer()
	if err := cjson.DecodeJSONInto(cs, []byte(payload), doc, nil); err != nil {
		t.Fatalf("DecodeJSONInto: %v\npayload: %s", err, payload)
	}
	return doc
}

func containerJSON(t *testing.T, cs *definition.CompiledSchema, doc *container.DataContainer) string {
	t.Helper()
	b, err := cjson.SerializeJSON(cs, doc)
	if err != nil {
		t.Fatalf("SerializeJSON: %v", err)
	}
	return string(b)
}

// fullPayload exercises every declarable leaf type, a nested flattened
// object, and a nested array-of-object, with a mix of present, explicitly
// null, and entirely absent fields.
const fullPayload = `{
	"count": 42,
	"price": 3.5,
	"title": "hello world",
	"active": true,
	"blob": "aGVsbG8=",
	"shape": [[0,0],[1,0],[1,1],[0,0]],
	"meta": {"nested": {"a": 1, "b": [1,2,3]}, "flag": true, "note": "x"},
	"tags": ["a","b","c"],
	"scores": [1.5, 2.5, -3.25],
	"flags": [true, false, true],
	"ids": [1,2,3,4,5],
	"blobs": ["aGk=", "eW8="],
	"items": [
		{"sku": "A1", "qty": 2, "price": 9.99},
		{"sku": "B2", "qty": 0, "price": 0}
	],
	"address": {"street": "1 Main St", "city": "Nairobi"}
}`

func TestRoundTrip_AutoSelect(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	if len(wire) == 0 {
		t.Fatalf("SerializeAnansi produced empty payload")
	}

	decoded, version, err := anansi.DecodeAnansi(cs, wire)
	if err != nil {
		t.Fatalf("DecodeAnansi: %v", err)
	}
	if version != 0 {
		t.Fatalf("unexpected version: %d", version)
	}

	got := containerJSON(t, cs, decoded)
	if want != got {
		t.Fatalf("round-trip mismatch:\n want: %s\n got:  %s", want, got)
	}
}

func TestRoundTrip_ForcedDenseAndSparse(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	cases := []struct {
		name   string
		encode func() ([]byte, error)
	}{
		{"dense", func() ([]byte, error) { return anansi.EncodeDense(cs, doc, 0) }},
		{"sparse", func() ([]byte, error) { return anansi.EncodeSparse(cs, doc, 0) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wire, err := c.encode()
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, _, err := anansi.DecodeAnansi(cs, wire)
			if err != nil {
				t.Fatalf("DecodeAnansi: %v", err)
			}
			got := containerJSON(t, cs, decoded)
			if want != got {
				t.Fatalf("%s round-trip mismatch:\n want: %s\n got:  %s", c.name, want, got)
			}
		})
	}
}

func TestEmptyDocument(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, `{}`)
	want := containerJSON(t, cs, doc)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	decoded, _, err := anansi.DecodeAnansi(cs, wire)
	if err != nil {
		t.Fatalf("DecodeAnansi: %v", err)
	}
	got := containerJSON(t, cs, decoded)
	if want != got {
		t.Fatalf("empty document round-trip mismatch:\n want: %s\n got:  %s", want, got)
	}
}

func TestTruncatedPacket_ReturnsError(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}

	for cut := 0; cut < len(wire); cut += 7 {
		truncated := wire[:cut]
		if _, _, err := anansi.DecodeAnansi(cs, truncated); err == nil {
			t.Fatalf("expected error decoding truncated packet at cut=%d, got nil", cut)
		}
	}
}

func TestVersionHeaderRoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	for _, v := range []uint16{0, 1, 255, 256, 511, 512, 1023} {
		wire, err := anansi.SerializeAnansi(cs, doc, v)
		if err != nil {
			t.Fatalf("SerializeAnansi(%d): %v", v, err)
		}
		_, gotVersion, err := anansi.DecodeAnansi(cs, wire)
		if err != nil {
			t.Fatalf("DecodeAnansi(%d): %v", v, err)
		}
		if gotVersion != v {
			t.Fatalf("version mismatch: want %d got %d", v, gotVersion)
		}
	}

	if _, err := anansi.SerializeAnansi(cs, doc, 1024); err == nil {
		t.Fatalf("expected error for out-of-range schema version 1024")
	}
}

func TestBatchRoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)

	payloads := []string{
		fullPayload,
		`{"count": 1, "title": "one"}`,
		`{}`,
		`{"count": 99, "tags": ["only","two"]}`,
	}

	var docs []*container.DataContainer
	var wantJSON []string
	for _, p := range payloads {
		d := containerFromJSON(t, cs, p)
		docs = append(docs, d)
		wantJSON = append(wantJSON, containerJSON(t, cs, d))
	}

	batch, err := anansi.EncodeBatchRows(cs, docs, 0)
	if err != nil {
		t.Fatalf("EncodeBatchRows: %v", err)
	}

	decoded, version, err := anansi.DecodeBatchRows(cs, batch, nil)
	if err != nil {
		t.Fatalf("DecodeBatchRows: %v", err)
	}
	if version != 0 {
		t.Fatalf("unexpected version: %d", version)
	}
	if len(decoded) != len(payloads) {
		t.Fatalf("record count mismatch: want %d got %d", len(payloads), len(decoded))
	}
	for i, c := range decoded {
		got := containerJSON(t, cs, c)
		if got != wantJSON[i] {
			t.Fatalf("record %d mismatch:\n want: %s\n got:  %s", i, wantJSON[i], got)
		}
	}
}

func TestGeometryRoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, `{"shape": [[0,0],[1,1],[2,0.5],[0,0]]}`)
	want := containerJSON(t, cs, doc)

	wire, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	decoded, _, err := anansi.DecodeAnansi(cs, wire)
	if err != nil {
		t.Fatalf("DecodeAnansi: %v", err)
	}
	got := containerJSON(t, cs, decoded)
	if want != got {
		t.Fatalf("geometry round-trip mismatch:\n want: %s\n got: %s", want, got)
	}
}

func TestEncodingIsDeterministic(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	w1, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	w2, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	if !bytes.Equal(w1, w2) {
		t.Fatalf("encoding the same document twice produced different bytes")
	}
}

// TestDocumentIntegration exercises the Document/DocumentPool-level wiring
// (Document.ToAnansi / DocumentPool.FromAnansi), proving the codec composes
// correctly with the rest of the document package, not just at the raw
// container level tested above.
func TestDocumentIntegration(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	pool, err := document.NewDocumentPoolFromJSON(data)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	doc, err := pool.FromJSON([]byte(fullPayload))
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	defer pool.Release(doc)

	wire, err := doc.ToAnansi(0)
	if err != nil {
		t.Fatalf("ToAnansi: %v", err)
	}

	doc2, err := pool.FromAnansi(wire)
	if err != nil {
		t.Fatalf("FromAnansi: %v", err)
	}
	defer pool.Release(doc2)

	want, err := doc.StripMetadata().(interface {
		MarshalJSON() ([]byte, error)
	}).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	got, err := doc2.StripMetadata().(interface {
		MarshalJSON() ([]byte, error)
	}).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	if string(want) != string(got) {
		t.Fatalf("document round-trip mismatch:\n want: %s\n got:  %s", want, got)
	}
}
