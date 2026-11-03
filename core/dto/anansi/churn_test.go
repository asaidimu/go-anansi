package anansi_test

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/dto/anansijson"
)

// TestGCChurnProbe quantifies GC churn — garbage bytes, mallocs and GC cycles
// over a fixed batch of operations — for the serialization paths. ns/op hides
// this cost; in a long-running process every B/op becomes a GC cycle and GC
// cycles are where tail latency comes from. Run with -v; numbers are meant
// for the investigation report, not CI assertions.
func TestGCChurnProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("churn probe skipped in -short")
	}
	small := sampleAddress(1)
	large := sampleOrder(1)
	smallIn := benchPayloadJSON(&small)
	largeIn := benchPayloadJSON(&large)
	bufS := make([]byte, 0, 1024)
	bufL := make([]byte, 0, 1024)

	const (
		marshalOps   = 200_000
		marshalBig   = 100_000
		unmarshalOps = 200_000
		unmarshalBig = 50_000
	)

	rows := []struct {
		name string
		ops  int
		fn   func()
	}{
		{"marshal stdlib/small", marshalOps, func() { _, _ = json.Marshal(&small) }},
		{"marshal anansijson/small", marshalOps, func() { _, _ = anansijson.Marshal(&small) }},
		{"marshal anansijson-append/small", marshalOps, func() {
			bufS, _ = anansijson.MarshalAppend(bufS[:0], &small)
		}},
		{"marshal stdlib/large", marshalBig, func() { _, _ = json.Marshal(&large) }},
		{"marshal anansijson/large", marshalBig, func() { _, _ = anansijson.Marshal(&large) }},
		{"marshal anansijson-append/large", marshalBig, func() {
			bufL, _ = anansijson.MarshalAppend(bufL[:0], &large)
		}},
		{"unmarshal stdlib/small", unmarshalOps, func() {
			var v Address
			_ = json.Unmarshal(smallIn, &v)
		}},
		{"unmarshal anansijson/small", unmarshalOps, func() {
			var v Address
			_ = anansijson.Unmarshal(smallIn, &v)
		}},
		{"unmarshal stdlib/large", unmarshalBig, func() {
			var v Order
			_ = json.Unmarshal(largeIn, &v)
		}},
		{"unmarshal anansijson/large", unmarshalBig, func() {
			var v Order
			_ = anansijson.Unmarshal(largeIn, &v)
		}},
	}

	t.Logf("%-30s %9s %13s %10s %12s %10s",
		"variant", "ops", "garbage MB", "KB/op", "mallocs/op", "GC cycles")
	for _, r := range rows {
		gcBefore, msBefore := churnSnapshot()
		for i := 0; i < r.ops; i++ {
			r.fn()
		}
		gcAfter, msAfter := churnSnapshot()
		garbage := msAfter.TotalAlloc - msBefore.TotalAlloc
		mallocs := msAfter.Mallocs - msBefore.Mallocs
		t.Logf("%-30s %9d %13.2f %10.2f %12.2f %10d",
			r.name, r.ops,
			float64(garbage)/(1<<20), float64(garbage)/float64(r.ops)/1024,
			float64(mallocs)/float64(r.ops), gcAfter-gcBefore)
	}
}

// churnSnapshot returns cumulative GC cycles and the matching MemStats;
// TotalAlloc/Mallocs/NumGC are exact monotonic counters, so deltas over the
// batch are precise regardless of background activity.
func churnSnapshot() (uint32, runtime.MemStats) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.NumGC, ms
}
