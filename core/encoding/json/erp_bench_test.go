package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ERP-for-SME scale benchmarks.
//
// The document is a realistic ERP order: a header, a flattened customer object,
// two mounts of a shared address schema (billing/shipping — distinct flat
// addresses), an array of line items (each a child DataContainer), and a few
// scalar/array fields. Three sizes map to SME order volumes:
//
//	small  — 5 line items  (~1.5 KB)   retail/web order
//	medium — 25 line items (~6 KB)    wholesale order
//	large  — 120 line items (~25 KB)  B2B / bulk order
//
// "Batch" simulates an end-of-day sync or report: 100 medium orders processed
// per operation, all documents pooled and recycled.

const erpSchemaJSON = `{
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

var erpLineCounts = map[string]int{
	"small": 5, "medium": 25, "large": 120,
}

// buildERPOrder generates a deterministic ERP order document with nLines line
// items. Generation happens once, outside the timed loop; benchmarks then serve
// the same payload the way a long-lived API serves a fixed document set.
func buildERPOrder(id string, nLines int) []byte {
	var buf bytes.Buffer
	buf.WriteString(`{"id":`)
	buf.WriteString(fmt.Sprintf("%q", id))
	buf.WriteString(`,"customer":{"id":"CUST-1234","name":"Acme Widgets GmbH","email":"billing@acme.example","tier":"gold"}`)
	buf.WriteString(`,"billing_address":{"street":"1 Market St","city":"Berlin","region":"BE","postal":"10115","country":"DE"}`)
	buf.WriteString(`,"shipping_address":{"street":"42 Harbor Way","city":"Hamburg","region":"HH","postal":"20095","country":"DE"}`)
	buf.WriteString(`,"lines":[`)
	var subtotal float64
	for i := 0; i < nLines; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		qty := i%9 + 1
		unit := 10.0 + float64(i%50) + 0.99
		discount := 0.0
		if i%3 == 0 {
			discount = 0.10
		}
		lineTotal := float64(qty) * unit * (1 - discount)
		subtotal += lineTotal
		buf.WriteString(`{"sku":"SKU-`)
		buf.WriteString(fmt.Sprintf("%05d", i+1))
		buf.WriteString(`","description":"Widget brass `)
		buf.WriteString(fmt.Sprintf("%.0f", 8+float64(i%40)))
		buf.WriteString(` mm","qty":`)
		buf.WriteString(fmt.Sprintf("%d", qty))
		buf.WriteString(`,"unit_price":`)
		buf.WriteString(fmt.Sprintf("%.2f", unit))
		buf.WriteString(`,"discount":`)
		buf.WriteString(fmt.Sprintf("%.2f", discount))
		buf.WriteString(`,"tax_rate":0.19,"line_total":`)
		buf.WriteString(fmt.Sprintf("%.2f", lineTotal))
		buf.WriteString(`}`)
	}
	tax := subtotal * 0.19
	buf.WriteString(`],"status":"processing","currency":"EUR","subtotal":`)
	buf.WriteString(fmt.Sprintf("%.2f", subtotal))
	buf.WriteString(`,"tax":`)
	buf.WriteString(fmt.Sprintf("%.2f", tax))
	buf.WriteString(`,"total":`)
	buf.WriteString(fmt.Sprintf("%.2f", subtotal+tax))
	buf.WriteString(`,"notes":["Priority customer","Fragile items"]}`)
	return buf.Bytes()
}

func benchERPCS(b *testing.B) *definition.CompiledSchema {
	b.Helper()
	cs, err := compileSchema(b, []byte(erpSchemaJSON))
	if err != nil {
		b.Fatalf("compile ERP schema: %v", err)
	}
	return cs
}

func erpPayloads(b *testing.B) map[string][]byte {
	b.Helper()
	out := make(map[string][]byte, len(erpLineCounts))
	for name, n := range erpLineCounts {
		out[name] = buildERPOrder("ORD-2026-000042", n)
	}
	return out
}

// BenchmarkERP_Decode_Container measures decode cost into a pooled document per
// order size.
func BenchmarkERP_Decode_Container(b *testing.B) {
	cs := benchERPCS(b)
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			pool := container.NewPool()
			primePoolWith(b, cs, pool, payload)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc := pool.Get()
				if err := DecodeJSONInto(cs, payload, doc, pool); err != nil {
					pool.Put(doc)
					b.Fatal(err)
				}
				pool.Put(doc)
			}
		})
	}
}

// BenchmarkERP_Dump_Map isolates materialization (container -> map[string]any)
// per order size: the decoded document is reused, the response map is rebuilt
// each iteration.
func BenchmarkERP_Dump_Map(b *testing.B) {
	cs := benchERPCS(b)
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			pool := container.NewPool()
			doc := pool.Get()
			if err := DecodeJSONInto(cs, payload, doc, pool); err != nil {
				b.Fatal(err)
			}
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
		})
	}
}

// BenchmarkERP_ContainerToJSON is the full pooled request cycle (decode +
// materialize + marshal + release) per order size.
func BenchmarkERP_ContainerToJSON(b *testing.B) {
	cs := benchERPCS(b)
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			pool := container.NewPool()
			primePoolWith(b, cs, pool, payload)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc := pool.Get()
				if err := DecodeJSONInto(cs, payload, doc, pool); err != nil {
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
		})
	}
}

// BenchmarkERP_ContainerToJSON_NoPool is the same cycle without pooling.
func BenchmarkERP_ContainerToJSON_NoPool(b *testing.B) {
	cs := benchERPCS(b)
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc, err := DecodeJSON(cs, payload)
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
		})
	}
}

// BenchmarkERP_MapToJSON is the baseline: a prebuilt materialized map is
// marshaled per iteration (no decode, no materialization).
func BenchmarkERP_MapToJSON(b *testing.B) {
	cs := benchERPCS(b)
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			doc, err := DecodeJSON(cs, payload)
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
		})
	}
}

// BenchmarkERP_SerializeJSON isolates the direct serializer: a pooled document
// is decoded once and serialized repeatedly, the way a query layer serves a
// cached document. It walks the typed columns straight into a buffer — no
// intermediate map[string]any.
func BenchmarkERP_SerializeJSON(b *testing.B) {
	cs := benchERPCS(b)
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			pool := container.NewPool()
			doc := pool.Get()
			if err := DecodeJSONInto(cs, payload, doc, pool); err != nil {
				b.Fatal(err)
			}
			if _, err := SerializeJSON(cs, doc); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := SerializeJSON(cs, doc)
				if err != nil {
					b.Fatal(err)
				}
				if len(out) == 0 {
					b.Fatal("empty output")
				}
			}
		})
	}
}

// BenchmarkERP_SerializeJSON_FullCycle is the proper full pipeline: decode into
// a pooled document, serialize directly to JSON bytes, release. The document
// key packs the type/kind/address, so no flattening into a map happens.
func BenchmarkERP_SerializeJSON_FullCycle(b *testing.B) {
	cs := benchERPCS(b)
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			pool := container.NewPool()
			primePoolWith(b, cs, pool, payload)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc := pool.Get()
				if err := DecodeJSONInto(cs, payload, doc, pool); err != nil {
					pool.Put(doc)
					b.Fatal(err)
				}
				if _, err := SerializeJSON(cs, doc); err != nil {
					pool.Put(doc)
					b.Fatal(err)
				}
				pool.Put(doc)
			}
		})
	}
}

// BenchmarkERP_Map_Unmarshal measures decoding JSON into a map[string]any via
// encoding/json — the idiomatic alternative to the DataContainer.
func BenchmarkERP_Map_Unmarshal(b *testing.B) {
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			var m map[string]any
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := json.Unmarshal(payload, &m); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkERP_Map_UnmarshalMarshal is the full map[string]any round-trip:
// Unmarshal into a map, then Marshal it back to JSON. This is directly
// comparable to BenchmarkERP_ContainerToJSON (decode + materialize + marshal):
// both produce JSON bytes from the same input document.
func BenchmarkERP_Map_UnmarshalMarshal(b *testing.B) {
	payloads := erpPayloads(b)
	for _, name := range []string{"small", "medium", "large"} {
		payload := payloads[name]
		b.Run(name, func(b *testing.B) {
			var m map[string]any
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := json.Unmarshal(payload, &m); err != nil {
					b.Fatal(err)
				}
				if _, err := json.Marshal(m); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkERP_Batch100 simulates an end-of-day sync / report: 100 medium
// orders decoded, materialized, and marshaled per operation. All documents are
// pooled and recycled between operations.
func BenchmarkERP_Batch100(b *testing.B) {
	cs := benchERPCS(b)
	nOrders := 100
	payloads := make([][]byte, nOrders)
	for i := range payloads {
		payloads[i] = buildERPOrder(fmt.Sprintf("ORD-2026-%06d", i), erpLineCounts["medium"])
	}
	pool := container.NewPool()
	for _, p := range payloads {
		primePoolWith(b, cs, pool, p)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, payload := range payloads {
			doc := pool.Get()
			if err := DecodeJSONInto(cs, payload, doc, pool); err != nil {
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
}

// primePoolWith runs a few full decode cycles for the given payload so the
// pool starts warm with documents (including array-of-object children).
func primePoolWith(b *testing.B, cs *definition.CompiledSchema, pool *container.Pool, payload []byte) {
	b.Helper()
	for i := 0; i < 8; i++ {
		doc := pool.Get()
		if err := DecodeJSONInto(cs, payload, doc, pool); err != nil {
			pool.Put(doc)
			b.Fatal(err)
		}
		pool.Put(doc)
	}
}
