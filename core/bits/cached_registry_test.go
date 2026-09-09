package bits

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/cache"
)

func TestCachedRegistry_GetSetDelete(t *testing.T) {
	spec := mustSpec(t, 16)
	reg := NewRegistry(testConfig(spec))
	defer reg.Close()

	cacheCfg := cache.DefaultCacheConfig()
	cacheCfg.MaxEntries = 100
	cImpl := cache.NewManagedCache[testView](cacheCfg, nil)
	defer cImpl.Close()

	cr := NewCachedRegistry[string, testView](
		reg,
		cImpl,
		func(k string) string { return k },
		5*time.Minute,
		1*time.Minute,
	)
	defer cr.Close()

	// 1. Initial Get - miss and absent from registry -> negative cache
	_, err := cr.Get("user:1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(user:1) = %v, want ErrNotFound", err)
	}

	// Stats check - 1 miss
	stats := cr.Stats()
	if stats.Cache.Misses != 1 {
		t.Fatalf("Cache Misses = %d, want 1", stats.Cache.Misses)
	}

	// 2. Second Get - negative cache hit
	_, err = cr.Get("user:1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(user:1) second time = %v, want ErrNotFound", err)
	}
	stats = cr.Stats()
	if stats.Cache.NegativeHits != 1 {
		t.Fatalf("Cache NegativeHits = %d, want 1", stats.Cache.NegativeHits)
	}

	// 3. Set - write-through
	v1, err := cr.Set("user:1", []byte("alice"))
	if err != nil {
		t.Fatalf("Set(user:1) = %v", err)
	}
	if !bytes.Equal(v1.Data, []byte("alice")) {
		t.Fatalf("v1.Data = %q, want %q", v1.Data, "alice")
	}

	// 4. Get after Set - positive cache hit
	got, err := cr.Get("user:1")
	if err != nil || !bytes.Equal(got.Data, []byte("alice")) {
		t.Fatalf("Get(user:1) after Set = (%q, %v), want %q", got.Data, err, "alice")
	}
	stats = cr.Stats()
	if stats.Cache.Hits != 1 {
		t.Fatalf("Cache Hits = %d, want 1", stats.Cache.Hits)
	}

	// 5. Delete - evicts cache and deletes from registry
	if !cr.Delete("user:1") {
		t.Fatalf("Delete(user:1) = false, want true")
	}

	// 6. Get after Delete - miss from cache, absent in registry -> negative marker backfilled
	_, err = cr.Get("user:1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(user:1) after Delete = %v, want ErrNotFound", err)
	}
}

func TestCachedRegistry_WriteThroughAndEviction(t *testing.T) {
	spec := mustSpec(t, 16)
	reg := NewRegistry(testConfig(spec))
	defer reg.Close()

	cacheCfg := cache.DefaultCacheConfig()
	cImpl := cache.NewManagedCache[testView](cacheCfg, nil)
	defer cImpl.Close()

	cr := NewCachedRegistry[string, testView](
		reg,
		cImpl,
		func(k string) string { return k },
		10*time.Minute,
		2*time.Minute,
	)
	defer cr.Close()

	// Set initial value
	_, err := cr.Set("k1", []byte("val1"))
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify positive cache hit
	got, err := cr.Get("k1")
	if err != nil || !bytes.Equal(got.Data, []byte("val1")) {
		t.Fatalf("Get(k1) = (%q, %v), want %q", got.Data, err, "val1")
	}

	// Overwrite value (Write-Through)
	_, err = cr.Set("k1", []byte("val1-updated"))
	if err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}

	// Verify immediate cache update
	got, err = cr.Get("k1")
	if err != nil || !bytes.Equal(got.Data, []byte("val1-updated")) {
		t.Fatalf("Get(k1) after overwrite = (%q, %v), want %q", got.Data, err, "val1-updated")
	}
}

func TestCachedRegistry_NegativeCachingSafety(t *testing.T) {
	spec := mustSpec(t, 16)
	reg := NewRegistry(testConfig(spec))
	defer reg.Close()

	cacheCfg := cache.DefaultCacheConfig()
	cImpl := cache.NewManagedCache[testView](cacheCfg, nil)
	defer cImpl.Close()

	cr := NewCachedRegistry[string, testView](
		reg,
		cImpl,
		func(k string) string { return k },
		5*time.Minute,
		1*time.Minute,
	)
	defer cr.Close()

	// Miss on non-existent key
	_, err := cr.Get("missing-key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}

	// Verify it's cached as negative in cache
	val, status := cImpl.GetStatus("missing-key")
	if status != cache.CacheHitNegative {
		t.Fatalf("cache status = %v, want CacheHitNegative", status)
	}
	_ = val

	// Subsequent Get returns ErrNotFound immediately without hitting registry
	regStatsBefore := reg.Stats()
	_, err = cr.Get("missing-key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
	regStatsAfter := reg.Stats()
	if regStatsBefore != regStatsAfter {
		t.Fatalf("registry should not be touched on negative hit")
	}
}

func TestCachedRegistry_StatsAndClose(t *testing.T) {
	spec := mustSpec(t, 16)
	reg := NewRegistry(testConfig(spec))
	cacheCfg := cache.DefaultCacheConfig()
	cImpl := cache.NewManagedCache[testView](cacheCfg, nil)

	cr := NewCachedRegistry[string, testView](
		reg,
		cImpl,
		func(k string) string { return k },
		5*time.Minute,
		1*time.Minute,
	)

	_, _ = cr.Set("item1", []byte("data1"))
	stats := cr.Stats()

	if stats.Registry.Entries != 1 {
		t.Fatalf("Registry Entries = %d, want 1", stats.Registry.Entries)
	}
	if stats.Cache.PositiveCount != 1 {
		t.Fatalf("Cache PositiveCount = %d, want 1", stats.Cache.PositiveCount)
	}

	if err := cr.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
}

func TestCachedRegistry_ConcurrentReadWrite(t *testing.T) {
	spec := mustSpec(t, 16)
	reg := NewRegistry(testConfig(spec))
	defer reg.Close()

	cacheCfg := cache.DefaultCacheConfig()
	cacheCfg.ShardCount = 16
	cImpl := cache.NewManagedCache[testView](cacheCfg, nil)
	defer cImpl.Close()

	cr := NewCachedRegistry[string, testView](
		reg,
		cImpl,
		func(k string) string { return k },
		50*time.Millisecond,
		20*time.Millisecond,
	)
	defer cr.Close()

	const workers = 8
	const duration = 200 * time.Millisecond
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id%4)
			for {
				select {
				case <-stop:
					return
				default:
				}
				val := []byte(fmt.Sprintf("value-%d", id))
				_, _ = cr.Set(key, val)
				got, err := cr.Get(key)
				if err == nil && len(got.Data) == 0 {
					t.Errorf("Get(%s) returned empty view data", key)
				}
				cr.Delete(key)
			}
		}(i)
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()
}

func TestCachedRegistry_ExportImport(t *testing.T) {
	spec := mustSpec(t, 16)
	reg1 := NewRegistry(testConfig(spec))
	defer reg1.Close()

	cImpl1 := cache.NewManagedCache[testView](cache.DefaultCacheConfig(), nil)
	defer cImpl1.Close()

	cr1 := NewCachedRegistry[string, testView](
		reg1,
		cImpl1,
		func(k string) string { return k },
		5*time.Minute,
		1*time.Minute,
	)
	defer cr1.Close()

	_, _ = cr1.Set("k1", []byte("v1"))
	exported := cr1.Export()

	reg2 := NewRegistry(testConfig(spec))
	defer reg2.Close()

	cImpl2 := cache.NewManagedCache[testView](cache.DefaultCacheConfig(), nil)
	defer cImpl2.Close()

	cr2 := NewCachedRegistry[string, testView](
		reg2,
		cImpl2,
		func(k string) string { return k },
		5*time.Minute,
		1*time.Minute,
	)
	defer cr2.Close()

	err := cr2.Import(exported)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	got, err := cr2.Get("k1")
	if err != nil || !bytes.Equal(got.Data, []byte("v1")) {
		t.Fatalf("cr2.Get(k1) = (%q, %v), want %q", got.Data, err, "v1")
	}
}
