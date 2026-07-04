package cache

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Benchmarks for managedCache, tested in isolation (no anansi dependencies).
//
// Run with:
//   go test ./utils/... -bench=. -benchmem -run=^$
//
// -benchmem reports allocs/op and B/op alongside ns/op, which is what
// matters for the memory/GC-pressure/throughput questions this suite is
// meant to answer.
// ---------------------------------------------------------------------------

// benchArtifact stands in for a compiled artifact. It's a pointer type so
// that Set/Get benchmarks reflect the realistic case (T = *SomeCompiledRule)
// rather than a bare int, which would hide the pointer-copy cost that real
// usage incurs.
type benchArtifact struct {
	Name    string
	Version int
	Payload [64]byte // gives the artifact a nontrivial, realistic size
}

func benchCloneFn(v *benchArtifact) (*benchArtifact, error) { return v, nil }

func newBenchCache(cfg CacheConfig) *managedCache[*benchArtifact] {
	return newManagedCache[*benchArtifact](cfg, benchCloneFn, nil, nil)
}

// ---------------------------------------------------------------------------
// The headline scenario: many goroutines reading ONE hot key.
//
// Sharding by key cannot help this pattern at all (every goroutine hashes to
// the same shard). This benchmark is what actually demonstrates whether the
// RWMutex+CLOCK redesign delivers on its purpose, as opposed to the earlier
// design's Mutex+MoveToFront-per-read, which serialized every single read of
// a shard — including reads of the identical key — behind one exclusive lock.
func BenchmarkGet_SingleHotKey_Parallel(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newBenchCache(cfg)
	defer c.Close()

	c.Set("authenticated", &benchArtifact{Name: "authenticated", Version: 1})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, ok := c.Get("authenticated"); !ok {
				b.Fatal("unexpected miss on hot key")
			}
		}
	})
}

// BenchmarkGet_SingleHotKey_Serial is the single-goroutine baseline for the
// same key, useful as a reference point: comparing its ns/op against the
// parallel version's per-op ns/op (which testing.B normalizes per
// goroutine-op, not wall-clock) shows how well throughput scales with
// GOMAXPROCS for this exact access pattern.
func BenchmarkGet_SingleHotKey_Serial(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newBenchCache(cfg)
	defer c.Close()

	c.Set("authenticated", &benchArtifact{Name: "authenticated", Version: 1})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := c.Get("authenticated"); !ok {
			b.Fatal("unexpected miss on hot key")
		}
	}
}

// BenchmarkGet_ManyKeys_Parallel is the contrasting case: many goroutines,
// each reading a large, distinct set of keys, so load actually does spread
// across shards. Comparing this against the single-hot-key benchmark
// isolates how much of any throughput ceiling is specifically attributable
// to single-key contention versus general per-operation cost.
func BenchmarkGet_ManyKeys_Parallel(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxEntries = 0 // unbounded: this benchmark measures read cost, not eviction
	c := newBenchCache(cfg)
	defer c.Close()

	const keyCount = 10_000
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("rule-%d", i)
		c.Set(keys[i], &benchArtifact{Name: keys[i], Version: i})
	}

	b.ReportAllocs()
	b.ResetTimer()
	var counter atomic.Uint64
	b.RunParallel(func(pb *testing.PB) {
		i := counter.Add(1)
		for pb.Next() {
			key := keys[i%keyCount]
			i++
			if _, ok := c.Get(key); !ok {
				b.Fatal("unexpected miss")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Write path: allocation cost of new-key inserts vs. refreshing existing keys.
// ---------------------------------------------------------------------------

// BenchmarkSet_NewKeys measures the cost of the cold-insert path (one
// cacheItem + one list.Element allocated per call). MaxEntries is large
// enough relative to b.N that this benchmark stays in "insert", not
// "insert-then-immediately-evict", territory for realistic run lengths.
func BenchmarkSet_NewKeys(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxEntries = 0 // unbounded: isolate insert cost from eviction cost
	c := newBenchCache(cfg)
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("k%d", i)
		c.Set(key, &benchArtifact{Name: key, Version: i})
	}
}

// BenchmarkSet_ExistingKey_Refresh measures the update path: re-Setting a
// key that's already cached should be allocation-free at the cache layer
// (in-place field mutation), unlike a cold insert.
func BenchmarkSet_ExistingKey_Refresh(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newBenchCache(cfg)
	defer c.Close()

	c.Set("authenticated", &benchArtifact{Name: "authenticated", Version: 0})
	artifact := &benchArtifact{Name: "authenticated", Version: 1}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set("authenticated", artifact)
	}
}

// BenchmarkNullify_NewKeys measures negative-cache insert cost, relevant to
// the negative-cache-amplification discussion: every distinct missed key
// costs the same 2 allocations as a positive Set.
func BenchmarkNullify_NewKeys(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxEntries = 0
	c := newBenchCache(cfg)
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Nullify(fmt.Sprintf("missing-%d", i))
	}
}

// BenchmarkGetStatus_NegativeHit measures the cost of repeatedly checking a
// key that's confirmed absent — the path that protects against
// negative-cache-miss amplification by never touching the database.
func BenchmarkGetStatus_NegativeHit(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newBenchCache(cfg)
	defer c.Close()

	c.Nullify("is_niche_rule")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, status := c.GetStatus("is_niche_rule"); status != CacheHitNegative {
			b.Fatal("expected CacheHitNegative")
		}
	}
}

// ---------------------------------------------------------------------------
// Mixed workloads
// ---------------------------------------------------------------------------

// BenchmarkMixedReadWrite_Parallel simulates a realistic mix: mostly reads
// of a modest working set, with occasional refreshes (e.g. a rule getting
// recompiled after a DB update) and occasional new keys, all from many
// concurrent goroutines. 90% reads / 8% refresh-existing / 2% new key.
func BenchmarkMixedReadWrite_Parallel(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxEntries = 0
	c := newBenchCache(cfg)
	defer c.Close()

	const keyCount = 1000
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("rule-%d", i)
		c.Set(keys[i], &benchArtifact{Name: keys[i], Version: 0})
	}

	b.ReportAllocs()
	b.ResetTimer()
	var counter atomic.Uint64
	b.RunParallel(func(pb *testing.PB) {
		i := counter.Add(1000000) // spread starting points across goroutines
		for pb.Next() {
			i++
			roll := i % 100
			key := keys[i%keyCount]
			switch {
			case roll < 90:
				c.Get(key)
			case roll < 98:
				c.Set(key, &benchArtifact{Name: key, Version: int(i)})
			default:
				c.Set(fmt.Sprintf("new-%d", i), &benchArtifact{Name: "new", Version: int(i)})
			}
		}
	})
}

// ---------------------------------------------------------------------------
// TTL / introspection path cost
// ---------------------------------------------------------------------------

func BenchmarkTTL_Introspection(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newBenchCache(cfg)
	defer c.Close()

	c.SetWithTTL("authenticated", &benchArtifact{Name: "authenticated"}, 1*time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := c.TTL("authenticated"); !ok {
			b.Fatal("expected TTL to report the key as present")
		}
	}
}

// ---------------------------------------------------------------------------
// Watermark eviction overhead under sustained churn
// ---------------------------------------------------------------------------

// BenchmarkSet_UnderSustainedEvictionPressure measures Set throughput/allocs
// when the cache is kept permanently above its high watermark by a fast
// producer, so the background evictor is continuously active and competing
// for shard locks with the foreground Set calls.
func BenchmarkSet_UnderSustainedEvictionPressure(b *testing.B) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxEntries = 1000
	cfg.EvictionHighWatermark = 0.90
	cfg.EvictionLowWatermark = 0.80
	cfg.EvictionInterval = time.Millisecond
	cfg.EvictionBatchSize = 50
	c := newBenchCache(cfg)
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("k%d", i)
		c.Set(key, &benchArtifact{Name: key, Version: i})
	}
}
