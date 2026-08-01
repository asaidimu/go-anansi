package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// These benchmarks measure the materialization cost of serializing a
// DataContainer to JSON versus serializing an equivalent map[string]any, under
// a simulated read-heavy API request load. The pooled variants decode into a
// pooled document and return it to the pool after the response is built — the
// container.Pool recurses into array-of-object children, so the whole subtree
// (including column capacity) is recycled between requests.

var benchDocJSON = []byte(documentJSON)

func benchCompileSchema(b *testing.B) *definition.CompiledSchema {
	b.Helper()
	cs, err := compileSchema([]byte(schemaJSON))
	if err != nil {
		b.Fatalf("compile schema: %v", err)
	}
	return cs
}

// BenchmarkSerialize_MapToJSON is the baseline: a prebuilt map[string]any is
// marshaled with encoding/json. It isolates what the container path pays on top
// of plain JSON serialization.
func BenchmarkSerialize_MapToJSON(b *testing.B) {
	cs := benchCompileSchema(b)
	doc, err := DecodeJSON(cs, benchDocJSON)
	if err != nil {
		b.Fatal(err)
	}
	m, err := Dump(cs, doc)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(m); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSerialize_DecodeOnly_Pooled isolates the decode stage of a pooled
// request: get a document, decode into it, release it.
func BenchmarkSerialize_DecodeOnly_Pooled(b *testing.B) {
	cs := benchCompileSchema(b)
	pool := container.NewPool()
	primePool(b, cs, pool)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := pool.Get()
		if err := DecodeJSONInto(cs, benchDocJSON, doc, pool); err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		pool.Put(doc)
	}
}

// BenchmarkSerialize_DumpOnly_Pooled isolates the materialization stage:
// container -> map[string]any, with no json.Marshal and no decode (the decode
// result is reused across iterations, as a query layer would serve cached
// documents).
func BenchmarkSerialize_DumpOnly_Pooled(b *testing.B) {
	cs := benchCompileSchema(b)
	pool := container.NewPool()
	doc := pool.Get()
	if err := DecodeJSONInto(cs, benchDocJSON, doc, pool); err != nil {
		b.Fatal(err)
	}
	// Warm the reverse cache exactly as a long-lived server would.
	if _, err := Dump(cs, doc); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := Dump(cs, doc)
		if err != nil {
			b.Fatal(err)
		}
		if len(m) == 0 {
			b.Fatal("empty dump")
		}
	}
}

// BenchmarkSerialize_ContainerToJSON_Pooled is the full pooled request cycle:
// decode into a pooled document, materialize the response map, marshal it, then
// return the document to the pool.
func BenchmarkSerialize_ContainerToJSON_Pooled(b *testing.B) {
	cs := benchCompileSchema(b)
	pool := container.NewPool()
	primePool(b, cs, pool)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := pool.Get()
		if err := DecodeJSONInto(cs, benchDocJSON, doc, pool); err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		m, err := Dump(cs, doc)
		if err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		if _, err := json.Marshal(m); err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		pool.Put(doc)
	}
}

// BenchmarkSerialize_ContainerToJSON_Pooled_BufReuse is the same pooled cycle
// but reuses the response byte buffer (json.Encoder) across requests, the way a
// long-lived HTTP handler typically does.
func BenchmarkSerialize_ContainerToJSON_Pooled_BufReuse(b *testing.B) {
	cs := benchCompileSchema(b)
	pool := container.NewPool()
	primePool(b, cs, pool)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := pool.Get()
		if err := DecodeJSONInto(cs, benchDocJSON, doc, pool); err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		m, err := Dump(cs, doc)
		if err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		buf.Reset()
		if err := enc.Encode(m); err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		pool.Put(doc)
	}
}

// BenchmarkSerialize_ContainerToJSON_NoPool is the full request cycle without
// pooling: each request allocates fresh root and child documents.
func BenchmarkSerialize_ContainerToJSON_NoPool(b *testing.B) {
	cs := benchCompileSchema(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc, err := DecodeJSON(cs, benchDocJSON)
		if err != nil {
			b.Fatal(err)
		}
		m, err := Dump(cs, doc)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(m); err != nil {
			b.Fatal(err)
		}
	}
}

// primePool runs a few full decode cycles so the pool starts warm with
// documents (including array-object children) before timing begins.
func primePool(b *testing.B, cs *definition.CompiledSchema, pool *container.Pool) {
	b.Helper()
	for i := 0; i < 8; i++ {
		doc := pool.Get()
		if err := DecodeJSONInto(cs, benchDocJSON, doc, pool); err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		pool.Put(doc)
	}
}
