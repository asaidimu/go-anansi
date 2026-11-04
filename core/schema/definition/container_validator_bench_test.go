package definition_test

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/container"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Benchmark schema: a realistic mid-size document — flat scalars with enums,
// a required nested object, and a two-item array of objects. Fast path.
const cvOrderSchemaJSON = `{
  "version": "1.0.0",
  "name": "order",
  "fields": {
    "id":       { "name": "id",       "type": "string",  "required": true },
    "status":   { "name": "status",   "type": "enum",    "schema": { "id": "statusVals" }, "required": true },
    "priority": { "name": "priority", "type": "enum",    "schema": { "id": "prioVals" } },
    "total":    { "name": "total",    "type": "integer", "required": true },
    "weight":   { "name": "weight",   "type": "number" },
    "fragile":  { "name": "fragile",  "type": "boolean" },
    "address":  { "name": "address",  "type": "object",  "schema": { "id": "addr" }, "required": true },
    "lines":    { "name": "lines",    "type": "array",   "schema": { "id": "line" }, "required": true }
  },
  "schemas": {
    "statusVals": { "name": "statusVals", "type": "string",  "values": ["draft", "placed", "shipped"] },
    "prioVals":   { "name": "prioVals",   "type": "integer", "values": [1, 2, 3] },
    "addr": {
      "name": "addr",
      "fields": {
        "street": { "name": "street", "type": "string", "required": true },
        "zip":    { "name": "zip",    "type": "integer" }
      }
    },
    "line": {
      "name": "line",
      "fields": {
        "sku":  { "name": "sku",  "type": "string",  "required": true },
        "qty":  { "name": "qty",  "type": "integer", "required": true },
        "kind": { "name": "kind", "type": "enum",    "schema": { "id": "lineKindVals" } }
      }
    },
    "lineKindVals": { "name": "lineKindVals", "type": "string", "values": ["new", "used"] }
  }
}`

const cvOrderPayloadJSON = `{
  "id": "ord-1",
  "status": "placed",
  "priority": 2,
  "total": 4200,
  "weight": 1.75,
  "fragile": true,
  "address": { "street": "1 Main St", "zip": 10001 },
  "lines": [
    { "sku": "sku-1", "qty": 2, "kind": "new" },
    { "sku": "sku-2", "qty": 1, "kind": "used" }
  ]
}`

func cvBenchFixtures(tb testing.TB) (*definition.CompiledSchema, *container.DataContainer, map[string]any, *definition.ContainerValidator, *definition.DocumentValidator) {
	tb.Helper()
	var sc definition.Schema
	if err := json.Unmarshal([]byte(cvOrderSchemaJSON), &sc); err != nil {
		tb.Fatalf("schema: %v", err)
	}
	rs, err := definition.Compile(&sc)
	if err != nil {
		tb.Fatalf("compile: %v", err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		tb.Fatalf("link: %v", err)
	}

	c := container.NewDataContainer()
	if err := cjson.DecodeJSONInto(cs, []byte(cvOrderPayloadJSON), c, nil); err != nil {
		tb.Fatalf("decode: %v", err)
	}
	m := definition.Materialize(cs, c)

	cv, err := definition.NewContainerValidator(&sc, nil)
	if err != nil {
		tb.Fatalf("container validator: %v", err)
	}
	if !cv.FastPath() {
		tb.Fatalf("benchmark schema must take the fast path, reason=%q", cv.UnsupportedReason())
	}
	dv, err := definition.NewDocumentValidator(&sc, nil)
	if err != nil {
		tb.Fatalf("document validator: %v", err)
	}
	if issues, ok := cv.Validate(c); !ok {
		tb.Fatalf("fixture must validate clean, got %v", issues)
	}
	return cs, c, m, cv, dv
}

// BenchmarkContainerValidator validates the container natively: one
// positions-map probe per field, no materialization, no boxing.
func BenchmarkContainerValidator(b *testing.B) {
	_, c, _, cv, _ := cvBenchFixtures(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if issues, ok := cv.Validate(c); !ok {
			b.Fatalf("unexpected issues: %v", issues)
		}
	}
}

// BenchmarkMapPipelineGraph is the old map lane with the map already in hand:
// the graph walk itself.
func BenchmarkMapPipelineGraph(b *testing.B) {
	_, _, m, _, dv := cvBenchFixtures(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if issues, ok := dv.Validate(m); !ok {
			b.Fatalf("unexpected issues: %v", issues)
		}
	}
}

// BenchmarkMapPipelineEndToEnd is what a container-backed caller does today:
// materialize the document into a map, then run the graph.
func BenchmarkMapPipelineEndToEnd(b *testing.B) {
	cs, c, _, _, dv := cvBenchFixtures(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := definition.Materialize(cs, c)
		if issues, ok := dv.Validate(m); !ok {
			b.Fatalf("unexpected issues: %v", issues)
		}
	}
}

// BenchmarkMaterialize isolates the materialization cost the native lane
// avoids.
func BenchmarkMaterialize(b *testing.B) {
	cs, c, _, _, _ := cvBenchFixtures(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = definition.Materialize(cs, c)
	}
}

// TestChurnProbeValidator quantifies GC churn for the two lanes over fixed
// batches: garbage bytes/op, mallocs/op and GC cycles. Run with -v; the
// numbers feed the investigation report, not CI assertions.
func TestChurnProbeValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("churn probe skipped in -short")
	}
	cs, c, _, cv, dv := cvBenchFixtures(t)

	const ops = 100_000
	snapshot := func() (uint32, runtime.MemStats) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ms.NumGC, ms
	}

	rows := []struct {
		name string
		fn   func()
	}{
		{"map end-to-end (materialize+graph)", func() {
			m := definition.Materialize(cs, c)
			_, _ = dv.Validate(m)
		}},
		{"map graph only", func() { _, _ = dv.Validate(mFrozen(cs, c)) }},
		{"container native", func() { _, _ = cv.Validate(c) }},
	}

	t.Logf("%-32s %9s %13s %10s %12s %10s",
		"variant", "ops", "garbage MB", "KB/op", "mallocs/op", "GC cycles")
	for _, r := range rows {
		// warmup
		for i := 0; i < 1000; i++ {
			r.fn()
		}
		gcBefore, msBefore := snapshot()
		for i := 0; i < ops; i++ {
			r.fn()
		}
		gcAfter, msAfter := snapshot()
		garbage := msAfter.TotalAlloc - msBefore.TotalAlloc
		mallocs := msAfter.Mallocs - msBefore.Mallocs
		t.Logf("%-32s %9d %13.2f %10.2f %12.2f %10d",
			r.name, ops,
			float64(garbage)/(1<<20), float64(garbage)/float64(ops)/1024,
			float64(mallocs)/float64(ops), gcAfter-gcBefore)
	}
}

// mFrozen materializes once outside the loop for the graph-only row.
var mCache map[string]any

func mFrozen(cs *definition.CompiledSchema, c *container.DataContainer) map[string]any {
	if mCache == nil {
		mCache = definition.Materialize(cs, c)
	}
	return mCache
}
