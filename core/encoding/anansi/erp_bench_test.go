package anansi_test

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ERP-for-SME scale benchmarks, mirroring core/encoding/json's ERP fixture
// so anansi and JSON numbers are directly comparable: same schema shape,
// same generated documents.
//
//	small  — 5 line items   (~1.5 KB JSON)   retail/web order
//	medium — 25 line items  (~6 KB JSON)     wholesale order
//	large  — 120 line items (~25 KB JSON)    B2B / bulk order
//
// Batch benchmarks simulate a 100-medium-order sync.

const benchERPSchemaJSON = `{
  "version": "1.0.0",
  "name": "erp_order",
  "fields": {
    "id":              { "name": "id",              "type": "string" },
    "customer":        { "name": "customer",        "type": "object", "schema": { "id": "customer" } },
    "billing_address": { "name": "billing_address", "type": "object", "schema": { "id": "address" } },
    "shipping_address":{"name": "shipping_address", "type": "object", "schema": { "id": "address" } },
    "lines":           { "name": "lines",           "type": "array",  "schema": { "id": "order_line" } },
    "status":          { "name": "status",          "type": "string" },
    "currency":        { "name": "currency",        "type": "string" },
    "subtotal":        { "name": "subtotal",        "type": "number" },
    "tax":             { "name": "tax",             "type": "number" },
    "total":           { "name": "total",           "type": "number" },
    "notes":           { "name": "notes",           "type": "array",  "schema": { "type": "string" } }
  },
  "schemas": {
    "customer": {
      "name": "customer",
      "fields": {
        "id":    { "name": "id",    "type": "string" },
        "name":  { "name": "name",  "type": "string" },
        "email": { "name": "email", "type": "string" },
        "tier":  { "name": "tier",  "type": "string" }
      }
    },
    "address": {
      "name": "address",
      "fields": {
        "street": { "name": "street", "type": "string" },
        "city":   { "name": "city",   "type": "string" },
        "region": { "name": "region", "type": "string" },
        "postal": { "name": "postal", "type": "string" },
        "country":{ "name": "country","type": "string" }
      }
    },
    "order_line": {
      "name": "order_line",
      "fields": {
        "sku":         { "name": "sku",         "type": "string" },
        "description": { "name": "description", "type": "string" },
        "qty":         { "name": "qty",         "type": "integer" },
        "unit_price":  { "name": "unit_price",  "type": "number" },
        "discount":    { "name": "discount",    "type": "number" },
        "tax_rate":    { "name": "tax_rate",    "type": "number" },
        "line_total":  { "name": "line_total",  "type": "number" }
      }
    }
  }
}`

var benchLineCounts = []struct {
	name string
	n    int
}{
	{"small", 5},
	{"medium", 25},
	{"large", 120},
}

func buildBenchOrder(id string, nLines int) string {
	var buf []byte
	w := func(s string) { buf = append(buf, s...) }
	wf := func(format string, args ...any) { buf = append(buf, fmt.Sprintf(format, args...)...) }

	w(`{"id":`)
	wf("%q", id)
	w(`,"customer":{"id":"CUST-1234","name":"Acme Widgets GmbH","email":"billing@acme.example","tier":"gold"}`)
	w(`,"billing_address":{"street":"1 Market St","city":"Berlin","region":"BE","postal":"10115","country":"DE"}`)
	w(`,"shipping_address":{"street":"42 Harbor Way","city":"Hamburg","region":"HH","postal":"20095","country":"DE"}`)
	w(`,"lines":[`)
	var subtotal float64
	for i := 0; i < nLines; i++ {
		if i > 0 {
			w(",")
		}
		qty := i%9 + 1
		unit := 10.0 + float64(i%50) + 0.99
		discount := 0.0
		if i%3 == 0 {
			discount = 0.10
		}
		lineTotal := float64(qty) * unit * (1 - discount)
		subtotal += lineTotal
		w(`{"sku":"SKU-`)
		wf("%05d", i+1)
		w(`","description":"Widget brass `)
		wf("%.0f", 8+float64(i%40))
		w(` mm","qty":`)
		wf("%d", qty)
		w(`,"unit_price":`)
		wf("%.2f", unit)
		w(`,"discount":`)
		wf("%.2f", discount)
		w(`,"tax_rate":0.19,"line_total":`)
		wf("%.2f", lineTotal)
		w(`}`)
	}
	tax := subtotal * 0.19
	w(`],"status":"processing","currency":"EUR","subtotal":`)
	wf("%.2f", subtotal)
	w(`,"tax":`)
	wf("%.2f", tax)
	w(`,"total":`)
	wf("%.2f", subtotal+tax)
	w(`,"notes":["Priority customer","Fragile items"]}`)
	return string(buf)
}

func benchCompiled(b *testing.B) *definition.CompiledSchema {
	b.Helper()
	s, err := definition.FromJSON([]byte(benchERPSchemaJSON))
	if err != nil {
		b.Fatalf("parse schema: %v", err)
	}
	rs, err := definition.Compile(s)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		b.Fatalf("link: %v", err)
	}
	return cs
}

func benchDoc(b *testing.B, cs *definition.CompiledSchema, nLines int) *container.DataContainer {
	b.Helper()
	doc := container.NewDataContainer()
	payload := buildBenchOrder("ORD-2026-000042", nLines)
	if err := cjson.DecodeJSONInto(cs, []byte(payload), doc, nil); err != nil {
		b.Fatalf("decode fixture: %v", err)
	}
	return doc
}

func reportBytes(b *testing.B, wire []byte) {
	b.SetBytes(int64(len(wire)))
	b.ReportMetric(float64(len(wire)), "wire-bytes")
}

// ── single document encode ────────────────────────────────────────────────────

func BenchmarkAnansiEncode(b *testing.B) {
	cs := benchCompiled(b)
	for _, size := range benchLineCounts {
		doc := benchDoc(b, cs, size.n)

		b.Run("dense/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			var wire []byte
			var err error
			for i := 0; i < b.N; i++ {
				wire, err = anansi.EncodeDense(cs, doc, 0)
				if err != nil {
					b.Fatal(err)
				}
			}
			reportBytes(b, wire)
		})
		b.Run("sparse/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			var wire []byte
			var err error
			for i := 0; i < b.N; i++ {
				wire, err = anansi.EncodeSparse(cs, doc, 0)
				if err != nil {
					b.Fatal(err)
				}
			}
			reportBytes(b, wire)
		})
		b.Run("auto/comp_hash/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			var wire []byte
			var err error
			for i := 0; i < b.N; i++ {
				wire, err = anansi.SerializeAnansi(cs, doc, 0, anansi.WithCompression(), anansi.WithIntegrity())
				if err != nil {
					b.Fatal(err)
				}
			}
			reportBytes(b, wire)
		})
	}
}

// ── single document decode ────────────────────────────────────────────────────

func benchmarkAnansiDecode(b *testing.B, encode func(*container.DataContainer) ([]byte, error)) {
	cs := benchCompiled(b)
	for _, size := range benchLineCounts {
		doc := benchDoc(b, cs, size.n)
		wire, err := encode(doc)
		if err != nil {
			b.Fatalf("setup encode: %v", err)
		}
		b.Run(size.name, func(b *testing.B) {
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			out := container.NewDataContainer()
			for i := 0; i < b.N; i++ {
				if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAnansiDecode_Dense(b *testing.B) {
	cs := benchCompiled(b)
	benchmarkAnansiDecode(b, func(d *container.DataContainer) ([]byte, error) {
		return anansi.EncodeDense(cs, d, 0)
	})
}

func BenchmarkAnansiDecode_Sparse(b *testing.B) {
	cs := benchCompiled(b)
	benchmarkAnansiDecode(b, func(d *container.DataContainer) ([]byte, error) {
		return anansi.EncodeSparse(cs, d, 0)
	})
}

// ── JSON reference points (same fixture, exported API) ────────────────────────

func BenchmarkJSONRef(b *testing.B) {
	cs := benchCompiled(b)
	for _, size := range benchLineCounts {
		doc := benchDoc(b, cs, size.n)

		b.Run("encode/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			var out []byte
			var err error
			for i := 0; i < b.N; i++ {
				out, err = cjson.SerializeJSON(cs, doc)
				if err != nil {
					b.Fatal(err)
				}
			}
			reportBytes(b, out)
		})

		payload := buildBenchOrder("ORD-2026-000042", size.n)
		b.Run("decode/"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			out := container.NewDataContainer()
			for i := 0; i < b.N; i++ {
				if err := cjson.DecodeJSONInto(cs, []byte(payload), out, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ── batch: row-oriented vs columnar, 100 medium orders ────────────────────────

func benchBatchDocs(b *testing.B, cs *definition.CompiledSchema, n int) []*container.DataContainer {
	b.Helper()
	docs := make([]*container.DataContainer, n)
	for i := range docs {
		docs[i] = benchDoc(b, cs, 25)
	}
	return docs
}

func BenchmarkAnansiBatch100(b *testing.B) {
	cs := benchCompiled(b)
	docs := benchBatchDocs(b, cs, 100)

	b.Run("encode/row", func(b *testing.B) {
		b.ReportAllocs()
		var wire []byte
		var err error
		for i := 0; i < b.N; i++ {
			wire, err = anansi.EncodeBatchRows(cs, docs, 0)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.SetBytes(int64(len(wire)))
		b.ReportMetric(float64(len(wire))/float64(100), "wire-bytes/record")
	})

	b.Run("encode/columnar", func(b *testing.B) {
		b.ReportAllocs()
		var wire []byte
		var err error
		for i := 0; i < b.N; i++ {
			wire, err = anansi.EncodeBatchColumnar(cs, docs, 0)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.SetBytes(int64(len(wire)))
		b.ReportMetric(float64(len(wire))/float64(100), "wire-bytes/record")
	})

	rowWire, err := anansi.EncodeBatchRows(cs, docs, 0)
	if err != nil {
		b.Fatal(err)
	}
	colWire, err := anansi.EncodeBatchColumnar(cs, docs, 0)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("decode/row", func(b *testing.B) {
		b.SetBytes(int64(len(rowWire)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := anansi.DecodeBatchRows(cs, rowWire, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("decode/columnar", func(b *testing.B) {
		b.SetBytes(int64(len(colWire)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := anansi.DecodeBatchRows(cs, colWire, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("decode/columnar_pooled", func(b *testing.B) {
		pool := container.NewPool()
		b.SetBytes(int64(len(colWire)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			decoded, _, err := anansi.DecodeBatchRows(cs, colWire, pool)
			if err != nil {
				b.Fatal(err)
			}
			for _, d := range decoded {
				pool.Put(d)
			}
		}
	})
}

// ── transform overhead (medium document) ──────────────────────────────────────

func BenchmarkAnansiTransforms(b *testing.B) {
	cs := benchCompiled(b)
	doc := benchDoc(b, cs, 25)

	sets := []struct {
		name string
		opts []anansi.EncodeOption
	}{
		{"plain", nil},
		{"comp", []anansi.EncodeOption{anansi.WithCompression()}},
		{"hash", []anansi.EncodeOption{anansi.WithIntegrity()}},
		{"comp_hash", []anansi.EncodeOption{anansi.WithCompression(), anansi.WithIntegrity()}},
		{"enc_comp_hash", []anansi.EncodeOption{anansi.WithEncryption(bytes32Key()), anansi.WithCompression(), anansi.WithIntegrity()}},
	}

	for _, s := range sets {
		b.Run("encode/"+s.name, func(b *testing.B) {
			b.ReportAllocs()
			var wire []byte
			var err error
			for i := 0; i < b.N; i++ {
				wire, err = anansi.SerializeAnansi(cs, doc, 0, s.opts...)
				if err != nil {
					b.Fatal(err)
				}
			}
			reportBytes(b, wire)
		})
	}

	// Decode with all transforms on: decrypt + decompress + verify path.
	full, err := anansi.SerializeAnansi(cs, doc, 0,
		anansi.WithEncryption(bytes32Key()), anansi.WithCompression(), anansi.WithIntegrity())
	if err != nil {
		b.Fatal(err)
	}
	b.Run("decode/enc_comp_hash", func(b *testing.B) {
		b.SetBytes(int64(len(full)))
		b.ReportAllocs()
		out := container.NewDataContainer()
		keyOpt := anansi.WithDecryptionKey(bytes32Key())
		for i := 0; i < b.N; i++ {
			if _, err := anansi.DecodeAnansiInto(cs, full, out, nil, keyOpt); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func bytes32Key() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

// BenchmarkAnansiDecode_StringStrategy contrasts the default zero-copy
// string decoding against the WithCopyStrings opt-out on identical packets.
func BenchmarkAnansiDecode_StringStrategy(b *testing.B) {
	cs := benchCompiled(b)
	for _, size := range benchLineCounts {
		doc := benchDoc(b, cs, size.n)
		wire, err := anansi.EncodeDense(cs, doc, 0)
		if err != nil {
			b.Fatal(err)
		}
		b.Run("default_zero-copy/"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			out := container.NewDataContainer()
			for i := 0; i < b.N; i++ {
				if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("copy_strings/"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			out := container.NewDataContainer()
			opt := anansi.WithCopyStrings()
			for i := 0; i < b.N; i++ {
				if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil, opt); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkHeadToHead compares JSON and Anansi on identical fixtures across
// document sizes: encode throughput, decode throughput (both anansi string
// strategies), and payload size.
func BenchmarkHeadToHead(b *testing.B) {
	cs := benchCompiled(b)

	for _, size := range benchLineCounts {
		doc := benchDoc(b, cs, size.n)
		jsonPayload := buildBenchOrder("ORD-2026-000042", size.n)
		jsonBytes := []byte(jsonPayload)

		b.Run("encode/json/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := cjson.SerializeJSON(cs, doc); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("encode/anansi/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := anansi.SerializeAnansi(cs, doc, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("decode/json/"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(jsonBytes)))
			b.ReportAllocs()
			out := container.NewDataContainer()
			for i := 0; i < b.N; i++ {
				if err := cjson.DecodeJSONInto(cs, jsonBytes, out, nil); err != nil {
					b.Fatal(err)
				}
			}
		})

		wire, err := anansi.EncodeDense(cs, doc, 0)
		if err != nil {
			b.Fatal(err)
		}

		b.Run("decode/anansi/"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			out := container.NewDataContainer()
			for i := 0; i < b.N; i++ {
				if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("decode/anansi_copystrings/"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			out := container.NewDataContainer()
			opt := anansi.WithCopyStrings()
			for i := 0; i < b.N; i++ {
				if _, err := anansi.DecodeAnansiInto(cs, wire, out, nil, opt); err != nil {
					b.Fatal(err)
				}
			}
		})

		// Size comparison once per size via a throwaway run.
		b.Run("sizes/"+size.name, func(b *testing.B) {
			b.ReportMetric(float64(len(jsonBytes)), "json-bytes")
			b.ReportMetric(float64(len(wire)), "anansi-bytes")
			zipped, _ := anansi.SerializeAnansi(cs, doc, 0, anansi.WithCompression())
			b.ReportMetric(float64(len(zipped)), "anansi+zstd-bytes")
		})
	}
}
