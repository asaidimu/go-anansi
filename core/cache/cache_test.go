package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test doubles: fake clock + fake ticker, giving fully deterministic control
// over TTL expiry and janitor/evictor tick cadence without real sleeps.
// ---------------------------------------------------------------------------

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fakeTicker is a manually-fired tickerHandle. Tests call fire() to push a
// tick synchronously; the consuming goroutine's select picks it up on its
// own schedule, so tests must synchronize (e.g. via a follow-up channel or
// a brief poll) when asserting on the effect of a fired tick.
type fakeTicker struct {
	ch      chan time.Time
	stopped chan struct{}
}

func newFakeTickerFactory(registry *tickerRegistry) tickerFactory {
	return func(_ time.Duration) tickerHandle {
		ft := &fakeTicker{ch: make(chan time.Time, 1), stopped: make(chan struct{})}
		registry.register(ft)
		return ft
	}
}

func (f *fakeTicker) C() <-chan time.Time { return f.ch }
func (f *fakeTicker) Stop() {
	select {
	case <-f.stopped:
	default:
		close(f.stopped)
	}
}

// fire pushes a tick. It is a no-op (does not block) if the ticker has been
// stopped or if a tick is already pending.
func (f *fakeTicker) fire(t time.Time) {
	select {
	case <-f.stopped:
		return
	default:
	}
	select {
	case f.ch <- t:
	default:
	}
}

// tickerRegistry lets a test grab a handle to whichever fakeTicker(s) get
// constructed by the cache under test (janitor ticker, evictor ticker, and
// any created by Clone), keyed by creation order.
type tickerRegistry struct {
	mu      sync.Mutex
	tickers []*fakeTicker
}

func (r *tickerRegistry) register(t *fakeTicker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tickers = append(r.tickers, t)
}

func (r *tickerRegistry) get(i int) *fakeTicker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < 0 || i >= len(r.tickers) {
		return nil
	}
	return r.tickers[i]
}

func (r *tickerRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tickers)
}

// waitFor polls cond until it returns true or the deadline elapses, failing
// the test otherwise. Used to synchronize with background-goroutine effects
// (e.g. after firing a fake tick) without relying on fixed sleeps.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if !cond() {
		t.Fatalf("timed out waiting for condition: %s", msg)
	}
}

// identityClone is a no-op clone function for a comparable value type,
// suitable for most tests where deep-copy semantics aren't under test.
func identityClone(v int) (int, error) { return v, nil }

// ---------------------------------------------------------------------------
// Basic correctness
// ---------------------------------------------------------------------------

func TestManagedCache_SetGetRoundTrip(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0 // no background goroutines needed for this test
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	c.Set("a", 42)
	v, ok := c.Get("a")
	if !ok || v != 42 {
		t.Fatalf("Get(a) = (%v, %v), want (42, true)", v, ok)
	}

	if _, ok := c.Get("missing"); ok {
		t.Fatalf("Get(missing) reported a hit for an unset key")
	}
}

func TestManagedCache_NegativeCacheStatusDistinguishesFromMiss(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	// A genuine miss must report CacheMiss.
	if _, status := c.GetStatus("ghost"); status != CacheMiss {
		t.Fatalf("GetStatus(ghost) = %v, want CacheMiss", status)
	}

	// After Nullify, the SAME key must report CacheHitNegative, not
	// CacheMiss and not CacheHitPositive. This is the exact bug class that
	// motivated GetStatus: liveRepository.Get must be able to distinguish
	// "confirmed absent" from "never looked up" in order to avoid a
	// database round-trip on every repeated lookup of a nonexistent key.
	c.Nullify("ghost")
	v, status := c.GetStatus("ghost")
	if status != CacheHitNegative {
		t.Fatalf("GetStatus(ghost) after Nullify = %v, want CacheHitNegative", status)
	}
	if v != 0 {
		t.Fatalf("GetStatus(ghost) after Nullify returned non-zero value %v", v)
	}

	// The plain bool-returning Get must also report false for a negative
	// hit (it must never be conflated with a positive hit).
	if _, ok := c.Get("ghost"); ok {
		t.Fatalf("Get(ghost) reported true after Nullify")
	}
}

func TestManagedCache_SetClearsNegativeMarker(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	c.Nullify("k")
	if _, status := c.GetStatus("k"); status != CacheHitNegative {
		t.Fatalf("expected CacheHitNegative after Nullify")
	}

	c.Set("k", 7)
	v, status := c.GetStatus("k")
	if status != CacheHitPositive || v != 7 {
		t.Fatalf("GetStatus(k) after Set = (%v, %v), want (7, CacheHitPositive)", v, status)
	}
}

func TestManagedCache_MaxKeyLengthBypassed(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxKeyLength = 4
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	longKey := "this-key-is-too-long"
	c.Set(longKey, 1)
	if _, ok := c.Get(longKey); ok {
		t.Fatalf("overlength key was stored despite MaxKeyLength=%d", cfg.MaxKeyLength)
	}
	if stats := c.Stats(); stats.Size != 0 {
		t.Fatalf("cache size = %d after only an overlength Set, want 0", stats.Size)
	}

	shortKey := "ok"
	c.Set(shortKey, 2)
	if v, ok := c.Get(shortKey); !ok || v != 2 {
		t.Fatalf("Get(shortKey) = (%v, %v), want (2, true)", v, ok)
	}
}

// ---------------------------------------------------------------------------
// TTL: lazy expiry, per-key overrides, TTL()/Persist() introspection
// ---------------------------------------------------------------------------

func TestManagedCache_LazyExpiry(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0 // rely purely on lazy (access-time) expiry
	cfg.PositiveTTL = 10 * time.Second
	c := newManagedCache[int](cfg, identityClone, clock.Now, nil)
	defer c.Close()

	c.Set("k", 1)
	if v, ok := c.Get("k"); !ok || v != 1 {
		t.Fatalf("Get(k) before expiry = (%v, %v), want (1, true)", v, ok)
	}

	clock.Advance(11 * time.Second)
	if _, ok := c.Get("k"); ok {
		t.Fatalf("Get(k) after TTL elapsed still reports a hit")
	}
	if stats := c.Stats(); stats.Size != 0 {
		t.Fatalf("expired entry was not removed on lazy access; size = %d", stats.Size)
	}
}

func TestManagedCache_PerKeyTTLOverride(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.PositiveTTL = 1 * time.Minute
	c := newManagedCache[int](cfg, identityClone, clock.Now, nil)
	defer c.Close()

	// authenticated: long custom TTL, far outliving the configured default.
	c.SetWithTTL("authenticated", 1, 1*time.Hour)
	// is_niche_rule: short custom TTL, much shorter than the default.
	c.SetWithTTL("is_niche_rule", 1, 5*time.Second)
	// third key: DefaultTTL, should use cfg.PositiveTTL (1 minute).
	c.SetWithTTL("plain", 1, DefaultTTL)

	clock.Advance(10 * time.Second)
	if _, ok := c.Get("is_niche_rule"); ok {
		t.Fatalf("is_niche_rule should have expired after 10s (TTL=5s)")
	}
	if _, ok := c.Get("authenticated"); !ok {
		t.Fatalf("authenticated should still be live after 10s (TTL=1h)")
	}
	if _, ok := c.Get("plain"); !ok {
		t.Fatalf("plain should still be live after 10s (TTL=1m default)")
	}

	clock.Advance(55 * time.Second) // total elapsed: 65s
	if _, ok := c.Get("plain"); ok {
		t.Fatalf("plain should have expired after 65s (TTL=1m default)")
	}
	if _, ok := c.Get("authenticated"); !ok {
		t.Fatalf("authenticated should still be live after 65s (TTL=1h)")
	}
}

func TestManagedCache_NoExpirationNeverExpiresByTTL(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, clock.Now, nil)
	defer c.Close()

	c.SetWithTTL("pinned", 1, NoExpiration)
	clock.Advance(365 * 24 * time.Hour)
	if _, ok := c.Get("pinned"); !ok {
		t.Fatalf("NoExpiration entry expired after a year of simulated time")
	}
	ttl, ok := c.TTL("pinned")
	if !ok || ttl != NoExpiration {
		t.Fatalf("TTL(pinned) = (%v, %v), want (NoExpiration, true)", ttl, ok)
	}
}

func TestManagedCache_TTLIntrospection(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, clock.Now, nil)
	defer c.Close()

	if _, ok := c.TTL("absent"); ok {
		t.Fatalf("TTL(absent) reported ok=true for a key that was never set")
	}

	c.SetWithTTL("k", 1, 30*time.Second)
	ttl, ok := c.TTL("k")
	if !ok {
		t.Fatalf("TTL(k) reported ok=false immediately after Set")
	}
	if ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("TTL(k) = %v, want a value in (0, 30s]", ttl)
	}

	clock.Advance(20 * time.Second)
	ttl, ok = c.TTL("k")
	if !ok {
		t.Fatalf("TTL(k) reported ok=false after 20s (TTL=30s)")
	}
	if ttl <= 0 || ttl > 10*time.Second {
		t.Fatalf("TTL(k) after 20 of 30s elapsed = %v, want a value in (0, 10s]", ttl)
	}
}

func TestManagedCache_Persist(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, clock.Now, nil)
	defer c.Close()

	if c.Persist("absent") {
		t.Fatalf("Persist(absent) returned true for a key that was never set")
	}

	c.SetWithTTL("k", 1, 5*time.Second)
	if !c.Persist("k") {
		t.Fatalf("Persist(k) returned false for a live entry")
	}

	clock.Advance(time.Hour)
	if _, ok := c.Get("k"); !ok {
		t.Fatalf("k expired despite Persist being called before its original TTL elapsed")
	}
	ttl, ok := c.TTL("k")
	if !ok || ttl != NoExpiration {
		t.Fatalf("TTL(k) after Persist = (%v, %v), want (NoExpiration, true)", ttl, ok)
	}
}

func TestManagedCache_NullifyWithTTLOverride(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.NegativeTTL = 1 * time.Minute
	c := newManagedCache[int](cfg, identityClone, clock.Now, nil)
	defer c.Close()

	c.NullifyWithTTL("short-lived-miss", 2*time.Second)
	if _, status := c.GetStatus("short-lived-miss"); status != CacheHitNegative {
		t.Fatalf("expected CacheHitNegative immediately after NullifyWithTTL")
	}

	clock.Advance(3 * time.Second)
	if _, status := c.GetStatus("short-lived-miss"); status != CacheMiss {
		t.Fatalf("expected the negative marker to expire and report CacheMiss, got status after 3s (TTL=2s)")
	}
}

// ---------------------------------------------------------------------------
// LRU (hard-cap) eviction
// ---------------------------------------------------------------------------

// TestManagedCache_HardCapEvictionFavorsRecentlyAccessedEntries exercises the
// CLOCK (second-chance) approximation of LRU that eviction now uses instead
// of exact list-reordering-on-read. For this specific access pattern
// (several cold inserts, one entry read once, then one more insert pushing
// the shard over its cap), CLOCK produces the identical outcome to exact
// LRU: the touched entry survives via its second chance, and the scan moves
// on to evict the first genuinely untouched candidate it finds. This is not
// a general guarantee of exact-LRU ordering (CLOCK is an approximation), but
// it is the expected, and desirable, behavior for "one entry is much hotter
// than the rest" — the pattern this design is optimized for.
func TestManagedCache_HardCapEvictionFavorsRecentlyAccessedEntries(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.ShardCount = 1 // force everything into one shard for a deterministic scan
	cfg.MaxEntries = 3
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	// Touch "a" so its accessed bit is set; the CLOCK scan will give it a
	// second chance rather than evicting it outright.
	if _, ok := c.Get("a"); !ok {
		t.Fatalf("Get(a) unexpectedly missed before eviction")
	}

	c.Set("d", 4) // exceeds MaxEntries=3, triggers one CLOCK eviction

	if _, ok := c.Get("b"); ok {
		t.Fatalf("expected b to be evicted (untouched since insert, unlike a), but it is still present")
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.Get(k); !ok {
			t.Fatalf("expected %q to remain cached after eviction of b", k)
		}
	}

	stats := c.Stats()
	if stats.HardCapEvictions != 1 {
		t.Fatalf("HardCapEvictions = %d, want 1", stats.HardCapEvictions)
	}
}

// TestManagedCache_CLOCKGivesAccessedEntriesASecondChance isolates the
// mechanism that TestManagedCache_HardCapEvictionFavorsRecentlyAccessedEntries
// exploits: an entry that is read repeatedly keeps surviving eviction even
// though — unlike exact LRU — it is never physically moved in the
// underlying list on read. Only its atomic accessed flag changes.
func TestManagedCache_CLOCKGivesAccessedEntriesASecondChance(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.ShardCount = 1
	cfg.MaxEntries = 2
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	c.Set("cold", 1)
	c.Set("hot", 2)

	for i := 0; i < 5; i++ {
		if _, ok := c.Get("hot"); !ok {
			t.Fatalf("Get(hot) unexpectedly missed during warm-up")
		}
	}

	// Insert beyond MaxEntries=2: CLOCK must give "hot" a second chance
	// (its accessed bit is set) and evict "cold" instead, even though
	// "cold" was inserted more recently than "hot" was last read.
	c.Set("new", 3)

	if _, ok := c.Get("cold"); ok {
		t.Fatalf("expected cold to be evicted (never read after insert), but it is still present")
	}
	if _, ok := c.Get("hot"); !ok {
		t.Fatalf("expected hot to survive eviction due to its accessed bit (CLOCK second chance)")
	}
	if _, ok := c.Get("new"); !ok {
		t.Fatalf("expected the newly inserted key to remain present")
	}
}

// ---------------------------------------------------------------------------
// Bounded/incremental janitor sweep (active expiry)
// ---------------------------------------------------------------------------

func TestManagedCache_JanitorActiveExpiryViaRealGoroutine(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	registry := &tickerRegistry{}
	cfg := DefaultCacheConfig()
	cfg.ShardCount = 1
	cfg.JanitorInterval = time.Millisecond // irrelevant to cadence; we fire manually
	cfg.JanitorBatchSize = 1000
	cfg.PositiveTTL = 5 * time.Second

	c := newManagedCache[int](cfg, identityClone, clock.Now, newFakeTickerFactory(registry))
	defer c.Close()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	clock.Advance(6 * time.Second) // all three now expired, but not yet accessed

	waitFor(t, time.Second, func() bool { return registry.count() >= 1 }, "janitor ticker to be created")
	janitorTicker := registry.get(0)

	janitorTicker.fire(clock.Now())

	waitFor(t, time.Second, func() bool {
		return c.Stats().Expirations >= 3
	}, "janitor to actively expire all 3 stale entries")

	if stats := c.Stats(); stats.Size != 0 {
		t.Fatalf("cache size after active expiry sweep = %d, want 0", stats.Size)
	}
}

func TestManagedCache_JanitorSweepIsBoundedPerTick(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	cfg := DefaultCacheConfig()
	cfg.ShardCount = 1
	cfg.JanitorInterval = 0 // drive sweep() directly, no goroutine needed
	cfg.PositiveTTL = 1 * time.Second

	c := newManagedCache[int](cfg, identityClone, clock.Now, nil)
	defer c.Close()

	for i := 0; i < 10; i++ {
		c.Set(fmt.Sprintf("k%d", i), i)
	}
	clock.Advance(2 * time.Second) // all 10 now expired

	// Directly bound one shard's sweep to 4 examined entries and confirm it
	// does not remove more than 4 in a single call, proving the sweep is
	// bounded rather than an unconditional full scan.
	//
	// Note: Stats().Size/Keys() filter by logical expiry independently of
	// physical reaping (see the cacheShard doc comment — getStatus/TTL no
	// longer eagerly remove expired entries under the read lock), so they
	// can't be used here to observe *partial* sweep progress: with all 10
	// entries already past their TTL, Stats().Size would already read 0
	// even before any physical removal happens. To verify the sweep itself
	// is bounded, we check the shard's physical entry count directly.
	physicalSize := func() int {
		c.shards[0].mu.RLock()
		defer c.shards[0].mu.RUnlock()
		return len(c.shards[0].items)
	}

	if physicalSize() != 10 {
		t.Fatalf("physical shard size before any sweep = %d, want 10", physicalSize())
	}

	expired := c.shards[0].sweepExpiredBounded(clock.Now(), 4)
	if expired > 4 {
		t.Fatalf("sweepExpiredBounded(limit=4) expired %d entries, want <= 4", expired)
	}
	if expired == 0 {
		t.Fatalf("sweepExpiredBounded(limit=4) expired 0 entries, want > 0 (all entries were expired)")
	}

	wantRemaining := 10 - expired
	if got := physicalSize(); got != wantRemaining {
		t.Fatalf("physical shard size after bounded sweep = %d, want %d", got, wantRemaining)
	}

	// Regardless of physical reaping progress, the logical view must
	// already report the (still-physically-present) remainder as gone,
	// since every entry is past its TTL.
	if stats := c.Stats(); stats.Size != 0 {
		t.Fatalf("Stats().Size = %d after partial sweep, want 0 (all entries are logically expired)", stats.Size)
	}

	// A second bounded sweep should make further progress reaping the rest.
	expired2 := c.shards[0].sweepExpiredBounded(clock.Now(), 4)
	if expired+expired2 == 0 {
		t.Fatalf("second bounded sweep made no progress")
	}
}

// ---------------------------------------------------------------------------
// Watermark-triggered background evictor: start/stop hysteresis
// ---------------------------------------------------------------------------

func TestManagedCache_WatermarkEvictorStartsAndStopsWithHysteresis(t *testing.T) {
	registry := &tickerRegistry{}
	cfg := DefaultCacheConfig()
	cfg.ShardCount = 1
	cfg.JanitorInterval = 0 // isolate the evictor from janitor activity
	cfg.MaxEntries = 100
	cfg.EvictionHighWatermark = 0.80 // starts at 80 entries
	cfg.EvictionLowWatermark = 0.50  // stops at 50 entries
	cfg.EvictionBatchSize = 1000     // allow a full drain in one tick

	c := newManagedCache[int](cfg, identityClone, nil, newFakeTickerFactory(registry))
	defer c.Close()

	// Below the high watermark: evictor must not be running.
	for i := 0; i < 79; i++ {
		c.Set(fmt.Sprintf("k%d", i), i)
	}
	time.Sleep(10 * time.Millisecond) // allow any (incorrect) spawn to happen
	if c.Stats().EvictorActive {
		t.Fatalf("evictor is active at size=79 (high watermark=80); should not have started yet")
	}
	if registry.count() != 0 {
		t.Fatalf("a ticker was created before the high watermark was reached")
	}

	// Cross the high watermark (80).
	c.Set("k79", 79)
	waitFor(t, time.Second, func() bool { return c.Stats().EvictorActive }, "evictor to start after crossing high watermark")
	waitFor(t, time.Second, func() bool { return registry.count() >= 1 }, "evictor ticker to be created")

	evictorTicker := registry.get(0)
	evictorTicker.fire(time.Now())

	// Should drain from 80 down to the low watermark (50) and then stop.
	waitFor(t, time.Second, func() bool { return !c.Stats().EvictorActive }, "evictor to stop after draining to the low watermark")

	stats := c.Stats()
	if stats.Size > 50 {
		t.Fatalf("cache size after evictor stopped = %d, want <= 50 (low watermark)", stats.Size)
	}
	if stats.WatermarkEvictions == 0 {
		t.Fatalf("WatermarkEvictions = 0, want > 0 after the evictor ran")
	}
}

func TestManagedCache_WatermarkEvictorDisabledWhenMaxEntriesUnbounded(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxEntries = 0 // unbounded

	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	for i := 0; i < 10_000; i++ {
		c.Set(fmt.Sprintf("k%d", i), i)
	}
	time.Sleep(10 * time.Millisecond)
	if c.Stats().EvictorActive {
		t.Fatalf("evictor started despite MaxEntries being unbounded (<=0)")
	}
}

// ---------------------------------------------------------------------------
// Stats counters
// ---------------------------------------------------------------------------

func TestManagedCache_StatsCounters(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	c.Set("a", 1)
	c.Get("a")          // hit
	c.Get("a")          // hit
	c.Get("nope")       // miss
	c.Nullify("gone")   // negative
	c.Get("gone")       // negative hit
	c.Get("gone")       // negative hit

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Fatalf("Hits = %d, want 2", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", stats.Misses)
	}
	if stats.NegativeHits != 2 {
		t.Fatalf("NegativeHits = %d, want 2", stats.NegativeHits)
	}
	if stats.PositiveCount != 1 || stats.NegativeCount != 1 {
		t.Fatalf("PositiveCount/NegativeCount = %d/%d, want 1/1", stats.PositiveCount, stats.NegativeCount)
	}
}

// ---------------------------------------------------------------------------
// Clone independence
// ---------------------------------------------------------------------------

func TestManagedCache_CloneIsIndependent(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	c.Set("a", 1)

	cloneIface, err := c.Clone()
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}
	clone := cloneIface.(*managedCache[int])
	defer clone.Close()

	// Mutating the original after cloning must not affect the clone.
	c.Set("a", 999)
	c.Set("b", 2)

	if v, ok := clone.Get("a"); !ok || v != 1 {
		t.Fatalf("clone.Get(a) = (%v, %v), want (1, true) — clone was affected by post-clone mutation of original", v, ok)
	}
	if _, ok := clone.Get("b"); ok {
		t.Fatalf("clone.Get(b) unexpectedly hit — clone saw a key added to the original after cloning")
	}

	// Mutating the clone after cloning must not affect the original.
	clone.Set("only-in-clone", 42)
	if _, ok := c.Get("only-in-clone"); ok {
		t.Fatalf("original saw a key added only to the clone")
	}
}

func TestManagedCache_CloneInvokesCloneFunction(t *testing.T) {
	calls := 0
	cloneFn := func(v int) (int, error) {
		calls++
		return v, nil
	}

	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, cloneFn, nil, nil)
	defer c.Close()

	c.Set("a", 1)
	c.Set("b", 2)

	cloneIface, err := c.Clone()
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}
	defer cloneIface.Close()

	if calls != 2 {
		t.Fatalf("clone function was called %d times during Clone(), want 2 (one per positive entry)", calls)
	}
}

// ---------------------------------------------------------------------------
// Close: idempotency and goroutine shutdown
// ---------------------------------------------------------------------------

func TestManagedCache_CloseIsIdempotentAndBounded(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = time.Millisecond // real ticker, short interval

	c := newManagedCache[int](cfg, identityClone, nil, nil)

	done := make(chan struct{})
	go func() {
		c.Close()
		c.Close() // second call must not block or panic
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Close() (called twice) did not return within 2s")
	}
}

func TestManagedCache_CloseStopsJanitorFromFurtherWork(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	registry := &tickerRegistry{}
	cfg := DefaultCacheConfig()
	cfg.ShardCount = 1
	cfg.JanitorInterval = time.Millisecond
	cfg.PositiveTTL = 1 * time.Second

	c := newManagedCache[int](cfg, identityClone, clock.Now, newFakeTickerFactory(registry))

	c.Set("a", 1)
	clock.Advance(2 * time.Second)

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	waitFor(t, time.Second, func() bool { return registry.count() >= 1 }, "janitor ticker to have been created before Close")
	janitorTicker := registry.get(0)

	// Firing the ticker after Close must not cause a sweep: the janitor
	// goroutine has already exited (Close waits on the WaitGroup), so
	// nothing is left to receive from the channel. This also proves Close
	// actually waited for goroutine exit rather than merely canceling a
	// context asynchronously.
	janitorTicker.fire(clock.Now())
	time.Sleep(20 * time.Millisecond)

	if stats := c.Stats(); stats.Expirations != 0 {
		t.Fatalf("Expirations = %d after Close, want 0 (janitor should not run post-Close)", stats.Expirations)
	}
}

// ---------------------------------------------------------------------------
// Single hot-key concurrent reads (the motivating scenario for RWMutex+CLOCK)
// ---------------------------------------------------------------------------

// TestManagedCache_ConcurrentReadsOfSingleHotKey does not assert on timing
// (unit tests should not make throughput claims), but it does prove
// correctness under exactly the access pattern this design targets: many
// goroutines concurrently reading ONE key, which sharding by key cannot
// help distribute, and which the previous exact-LRU design would have fully
// serialized behind one exclusive mutex on every single read. Correctness
// here (no race, consistent value, no deadlock) is verified in combination
// with `go test -race`; relative throughput is measured separately by the
// benchmarks in managed_cache_bench_test.go.
func TestManagedCache_ConcurrentReadsOfSingleHotKey(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	c.Set("authenticated", 1)

	const goroutines = 64
	const readsPerGoroutine = 2000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < readsPerGoroutine; i++ {
				v, ok := c.Get("authenticated")
				if !ok || v != 1 {
					errs <- fmt.Errorf("Get(authenticated) = (%v, %v), want (1, true)", v, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	stats := c.Stats()
	if stats.Hits != uint64(goroutines*readsPerGoroutine) {
		t.Fatalf("Hits = %d, want %d", stats.Hits, goroutines*readsPerGoroutine)
	}
}

// ---------------------------------------------------------------------------
// Concurrency (run with -race)
// ---------------------------------------------------------------------------

func TestManagedCache_ConcurrentAccess(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.JanitorInterval = 0
	cfg.MaxEntries = 500
	cfg.ShardCount = 8

	c := newManagedCache[int](cfg, identityClone, nil, nil)
	defer c.Close()

	const goroutines = 32
	const opsPerGoroutine = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("k%d", (g*opsPerGoroutine+i)%200)
				switch i % 5 {
				case 0:
					c.Set(key, i)
				case 1:
					c.Get(key)
				case 2:
					c.Nullify(key)
				case 3:
					c.Evict(key)
				case 4:
					c.TTL(key)
				}
			}
		}(g)
	}
	wg.Wait()

	// No assertion beyond "did not race/deadlock/panic" (verified via -race);
	// sanity-check Stats() is still callable and internally consistent.
	stats := c.Stats()
	if stats.PositiveCount+stats.NegativeCount != stats.Size {
		t.Fatalf("Stats() internally inconsistent: positive=%d negative=%d size=%d",
			stats.PositiveCount, stats.NegativeCount, stats.Size)
	}
}
