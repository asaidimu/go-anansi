package anansi_test

// Synthetic-shape speed+allocation suite for the codec itself (as opposed to
// erp_bench_test.go's repo-native ERP fixture). Four shapes:
//
//      tiny       — 5 scalar fields, all set                (RPC-message scale)
//      wide64     — 64 scalar/array fields, all set         (dense wide row)
//      wide_10pct — same 64-field schema, ~10% of fields set (sparse-ish row)
//      nested20   — string + 20 array-object child elements (nested packet fan-out)
//
// Each operation is benchmarked with ReportAllocs; decode benches measure
// fresh, reuse (same container, Clear per iteration) and pooled documents;
// batch benches measure 100-record row and columnar packets fresh and pooled.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const benchTinySchemaJSON = `{
  "version": "1.0.0",
  "name": "tiny",
  "fields": {
    "f01": { "name": "count",  "type": "integer" },
    "f02": { "name": "score",  "type": "number" },
    "f03": { "name": "title",  "type": "string" },
    "f04": { "name": "active", "type": "boolean" },
    "f05": { "name": "note",   "type": "string" }
  }
}`

const benchNestedSchemaJSON = `{
  "version": "1.0.0",
  "name": "nested",
  "fields": {
    "f01": { "name": "title", "type": "string" },
    "f02": { "name": "items", "type": "array", "schema": { "id": "item" } }
  },
  "schemas": {
    "item": {
      "name": "item",
      "fields": {
        "i1": { "name": "sku",   "type": "string" },
        "i2": { "name": "qty",   "type": "integer" },
        "i3": { "name": "price", "type": "number" }
      }
    }
  }
}`

// benchWideSchemaJSON builds a 64-field schema cycling integer/number/string/
// boolean/array-of-string/array-of-integer.
func benchWideSchemaJSON() string {
	var b strings.Builder
	b.WriteString(`{"version":"1.0.0","name":"wide","fields":{`)
	kind := []string{"array", "integer", "number", "string", "boolean", "array"}
	for i := 1; i <= 64; i++ {
		k := kind[i%6]
		sub := ``
		switch k {
		case "array":
			if i%6 == 5 {
				sub = `, "schema": {"type": "string"}`
			} else {
				sub = `, "schema": {"type": "integer"}`
			}
		}
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"f%02d": {"name": "f%02d", "type": "%s"%s}`, i, i, k, sub)
	}
	b.WriteString(`}}`)
	return b.String()
}

type benchShape struct {
	name string
	cs   *definition.CompiledSchema
	docs []*container.DataContainer
}

func compileBenchSchema(t testing.TB, json string) *definition.CompiledSchema {
	t.Helper()
	s, err := definition.FromJSON([]byte(json))
	if err != nil {
		t.Fatalf("schema parse: %v", err)
	}
	rs, err := definition.Compile(s)
	if err != nil {
		t.Fatalf("schema compile: %v", err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		t.Fatalf("schema link: %v", err)
	}
	return cs
}

func buildBenchDoc(t testing.TB, cs *definition.CompiledSchema, payload string) *container.DataContainer {
	t.Helper()
	doc := container.NewDataContainer()
	if err := cjson.DecodeJSONInto(cs, []byte(payload), doc, nil); err != nil {
		t.Fatalf("build doc: %v", err)
	}
	return doc
}

// setupShapes materializes the four shapes once per benchmark process.
func setupShapes(b *testing.B) map[string]*benchShape {
	b.Helper()
	shapes := map[string]*benchShape{}

	tiny := compileBenchSchema(b, benchTinySchemaJSON)
	shapes["tiny"] = &benchShape{name: "tiny", cs: tiny, docs: []*container.DataContainer{
		buildBenchDoc(b, tiny, `{"count": 42, "score": 3.25, "title": "hello world", "active": true, "note": "trailing note"}`),
	}}

	wide := compileBenchSchema(b, benchWideSchemaJSON())
	var widePayload, sparsePayload strings.Builder
	widePayload.WriteString("{")
	sparsePayload.WriteString("{")
	for i := 1; i <= 64; i++ {
		if i > 1 {
			widePayload.WriteString(",")
		}
		switch i % 6 {
		case 1:
			fmt.Fprintf(&widePayload, `"f%02d": %d`, i, i*1000)
		case 2:
			fmt.Fprintf(&widePayload, `"f%02d": %d.5`, i, i)
		case 3:
			fmt.Fprintf(&widePayload, `"f%02d": "value-%02d"`, i, i)
		case 4:
			if i%2 == 0 {
				fmt.Fprintf(&widePayload, `"f%02d": true`, i)
			} else {
				fmt.Fprintf(&widePayload, `"f%02d": false`, i)
			}
		case 5:
			fmt.Fprintf(&widePayload, `"f%02d": ["s%02d-1","s%02d-2"]`, i, i, i)
		case 0:
			fmt.Fprintf(&widePayload, `"f%02d": [%d,%d,%d]`, i, i, i+1, i+2)
		}
		if i%6 == 1 && i <= 61 { // every 10th field for the sparse shape (f01,f11,...,f61)
			fmt.Fprintf(&sparsePayload, `"f%02d": %d,`, i, i*1000)
		}
	}
	widePayload.WriteString("}")
	sparsePayload.WriteString(`"f63": "63000"}`)

	shapes["wide64"] = &benchShape{name: "wide64", cs: wide, docs: []*container.DataContainer{
		buildBenchDoc(b, wide, widePayload.String()),
	}}
	shapes["wide_10pct"] = &benchShape{name: "wide_10pct", cs: wide, docs: []*container.DataContainer{
		buildBenchDoc(b, wide, sparsePayload.String()),
	}}

	nested := compileBenchSchema(b, benchNestedSchemaJSON)
	var nestedPayload strings.Builder
	nestedPayload.WriteString(`{"title": "order-42", "items": [`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			nestedPayload.WriteString(",")
		}
		fmt.Fprintf(&nestedPayload, `{"sku": "SKU-%04d", "qty": %d, "price": %d.75}`, i, i%7+1, i)
	}
	nestedPayload.WriteString("]}")
	shapes["nested20"] = &benchShape{name: "nested20", cs: nested, docs: []*container.DataContainer{
		buildBenchDoc(b, nested, nestedPayload.String()),
	}}

	return shapes
}

// batchDocs builds n tiny documents for the batch benchmarks.
func batchDocs(b *testing.B, cs *definition.CompiledSchema, n int) []*container.DataContainer {
	b.Helper()
	docs := make([]*container.DataContainer, n)
	for i := 0; i < n; i++ {
		docs[i] = buildBenchDoc(b, cs, fmt.Sprintf(`{"count": %d, "score": %d.5, "title": "row-%03d", "active": %t, "note": "note-%03d"}`, i, i, i, i%2 == 0, i))
	}
	return docs
}

func benchEncode(b *testing.B, name string, encode func() ([]byte, error)) {
	b.Helper()
	wire, err := encode()
	if err != nil {
		b.Fatalf("%s: %v", name, err)
	}
	b.SetBytes(int64(len(wire)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := encode(); err != nil {
			b.Fatalf("%s: %v", name, err)
		}
	}
}

func benchDecodeInto(b *testing.B, wire []byte, decode func(wire []byte, doc *container.DataContainer, pool *container.Pool) error) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := container.NewDataContainer()
		if err := decode(wire, doc, nil); err != nil {
			b.Fatalf("decode: %v", err)
		}
	}
}

func benchDecodeReuse(b *testing.B, wire []byte, decode func(wire []byte, doc *container.DataContainer, pool *container.Pool) error) {
	doc := container.NewDataContainer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.Clear()
		if err := decode(wire, doc, nil); err != nil {
			b.Fatalf("decode: %v", err)
		}
	}
}

func benchDecodePooled(b *testing.B, wire []byte, decode func(wire []byte, doc *container.DataContainer, pool *container.Pool) error) {
	pool := container.NewPool()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := pool.Get()
		if err := decode(wire, doc, pool); err != nil {
			b.Fatalf("decode: %v", err)
		}
		pool.Put(doc)
	}
}

func runDecodeVariants(b *testing.B, decode func(wire []byte, doc *container.DataContainer, pool *container.Pool) error, wire []byte) {
	b.Run("fresh", func(b *testing.B) { benchDecodeInto(b, wire, decode) })
	b.Run("reuse", func(b *testing.B) { benchDecodeReuse(b, wire, decode) })
	b.Run("pooled", func(b *testing.B) { benchDecodePooled(b, wire, decode) })
}

// BenchmarkEncodeDense measures forced-Dense encoding of each shape.
func BenchmarkEncodeDense(b *testing.B) {
	for _, shape := range setupShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			benchEncode(b, "dense", func() ([]byte, error) {
				return anansi.EncodeDense(shape.cs, shape.docs[0], 1)
			})
		})
	}
}

// BenchmarkEncodeSparse measures forced-Sparse encoding of each shape.
func BenchmarkEncodeSparse(b *testing.B) {
	for _, shape := range setupShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			benchEncode(b, "sparse", func() ([]byte, error) {
				return anansi.EncodeSparse(shape.cs, shape.docs[0], 1)
			})
		})
	}
}

// BenchmarkEncodeAuto measures SerializeAnansi's auto-selected encoding.
func BenchmarkEncodeAuto(b *testing.B) {
	for _, shape := range setupShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			benchEncode(b, "auto", func() ([]byte, error) {
				return anansi.SerializeAnansi(shape.cs, shape.docs[0], 1)
			})
		})
	}
}

// BenchmarkEncodeIntegrity measures encoding with the BLAKE3 integrity
// transform enabled (digest over the plaintext body).
func BenchmarkEncodeIntegrity(b *testing.B) {
	for _, shape := range setupShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			benchEncode(b, "integrity", func() ([]byte, error) {
				return anansi.EncodeDense(shape.cs, shape.docs[0], 1, anansi.WithIntegrity())
			})
		})
	}
}

// BenchmarkDecodeDense measures Dense decode into fresh, reused and pooled
// containers.
func BenchmarkDecodeDense(b *testing.B) {
	for _, shape := range setupShapes(b) {
		wire, err := anansi.EncodeDense(shape.cs, shape.docs[0], 1)
		if err != nil {
			b.Fatalf("dense encode: %v", err)
		}
		b.Run(shape.name, func(b *testing.B) {
			runDecodeVariants(b, func(wire []byte, doc *container.DataContainer, pool *container.Pool) error {
				_, err := anansi.DecodeAnansiInto(shape.cs, wire, doc, pool)
				return err
			}, wire)
		})
	}
}

// BenchmarkDecodeSparse measures Sparse decode into fresh, reused and pooled
// containers.
func BenchmarkDecodeSparse(b *testing.B) {
	for _, shape := range setupShapes(b) {
		wire, err := anansi.EncodeSparse(shape.cs, shape.docs[0], 1)
		if err != nil {
			b.Fatalf("sparse encode: %v", err)
		}
		b.Run(shape.name, func(b *testing.B) {
			runDecodeVariants(b, func(wire []byte, doc *container.DataContainer, pool *container.Pool) error {
				_, err := anansi.DecodeAnansiInto(shape.cs, wire, doc, pool)
				return err
			}, wire)
		})
	}
}

// BenchmarkRoundTrip measures encode+decode round trips per shape (Dense).
func BenchmarkRoundTrip(b *testing.B) {
	for _, shape := range setupShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			wire, err := anansi.EncodeDense(shape.cs, shape.docs[0], 1)
			if err != nil {
				b.Fatalf("encode: %v", err)
			}
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := anansi.EncodeDense(shape.cs, shape.docs[0], 1); err != nil {
					b.Fatalf("encode: %v", err)
				}
				if _, _, err := anansi.DecodeAnansi(shape.cs, wire); err != nil {
					b.Fatalf("decode: %v", err)
				}
			}
		})
	}
}

// BenchmarkBatchRows measures 100-record row-oriented batch encoding and
// decoding, fresh and pooled.
func BenchmarkBatchRows(b *testing.B) {
	cs := compileBenchSchema(b, benchTinySchemaJSON)
	docs := batchDocs(b, cs, 100)
	wire, err := anansi.EncodeBatchRows(cs, docs, 1)
	if err != nil {
		b.Fatalf("batch encode: %v", err)
	}
	b.SetBytes(int64(len(wire)))

	b.Run("encode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := anansi.EncodeBatchRows(cs, docs, 1); err != nil {
				b.Fatalf("batch encode: %v", err)
			}
		}
	})
	b.Run("decode/fresh", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := anansi.DecodeBatchRows(cs, wire, nil); err != nil {
				b.Fatalf("batch decode: %v", err)
			}
		}
	})
	b.Run("decode/pooled", func(b *testing.B) {
		pool := container.NewPool()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			got, _, err := anansi.DecodeBatchRows(cs, wire, pool)
			if err != nil {
				b.Fatalf("batch decode: %v", err)
			}
			for _, d := range got {
				pool.Put(d)
			}
		}
	})
}

// BenchmarkBatchColumnar measures 100-record columnar batch encoding and
// decoding, fresh and pooled.
func BenchmarkBatchColumnar(b *testing.B) {
	cs := compileBenchSchema(b, benchTinySchemaJSON)
	docs := batchDocs(b, cs, 100)
	wire, err := anansi.EncodeBatchColumnar(cs, docs, 1)
	if err != nil {
		b.Fatalf("columnar encode: %v", err)
	}
	b.SetBytes(int64(len(wire)))

	b.Run("encode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := anansi.EncodeBatchColumnar(cs, docs, 1); err != nil {
				b.Fatalf("columnar encode: %v", err)
			}
		}
	})
	b.Run("decode/fresh", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := anansi.DecodeBatchRows(cs, wire, nil); err != nil {
				b.Fatalf("columnar decode: %v", err)
			}
		}
	})
	b.Run("decode/pooled", func(b *testing.B) {
		pool := container.NewPool()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			got, _, err := anansi.DecodeBatchRows(cs, wire, pool)
			if err != nil {
				b.Fatalf("columnar decode: %v", err)
			}
			for _, d := range got {
				pool.Put(d)
			}
		}
	})
}
