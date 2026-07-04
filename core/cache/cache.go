// Package cache provides a production-grade, sharded, bounded, TTL-aware
// cache with lock-free reads and CLOCK (second-chance) eviction.
//
// This package has zero dependencies beyond the standard library and is
// designed to be used as a building block for higher-level caching patterns
// (read-through, write-through, etc.).
//
// Basic usage:
//
//	cfg := cache.DefaultCacheConfig()
//	cfg.MaxEntries = 10000
//	cache := cache.NewManagedCache[int](cfg, nil)
//	cache.Set("my-key", 42)
//	val, ok := cache.Get("my-key")
package cache

import (
	"container/list"
	"context"
	"fmt"
	"hash/maphash"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// TTL sentinels
// ---------------------------------------------------------------------------

const (
	// DefaultTTL, passed to SetWithTTL or NullifyWithTTL, requests that the
	// cache's configured default TTL be used for this entry
	// (CacheConfig.PositiveTTL for positive entries, CacheConfig.NegativeTTL
	// for negative/nullified entries). This is the zero value of
	// time.Duration, so a plain Set/Nullify call behaves identically to
	// SetWithTTL(key, value, DefaultTTL).
	DefaultTTL time.Duration = 0

	// NoExpiration, passed to SetWithTTL or NullifyWithTTL, requests that
	// this entry never expire on its own via TTL. It remains subject to
	// capacity-based eviction (LRU / watermark eviction) and to explicit
	// removal via Evict/Unset, but will not be removed by TTL expiration.
	NoExpiration time.Duration = -1
)

// resolveTTL turns a caller-requested TTL (which may be DefaultTTL or
// NoExpiration) into a concrete duration to apply, where a result <= 0 means
// "never expires" and a result > 0 means "expires after this long".
func resolveTTL(requested, configuredDefault time.Duration) time.Duration {
	switch {
	case requested < 0:
		return 0 // NoExpiration (or any negative) => never expires
	case requested == 0:
		if configuredDefault < 0 {
			return 0
		}
		return configuredDefault // DefaultTTL => use the configured default
	default:
		return requested
	}
}

// ---------------------------------------------------------------------------
// CacheStatus: distinguishes a genuine miss from a cached negative result.
// ---------------------------------------------------------------------------

// CacheStatus describes the outcome of a cache lookup.
type CacheStatus int

const (
	// CacheMiss indicates the key is not present (or has expired). Callers
	// should fall back to the underlying data store.
	CacheMiss CacheStatus = iota

	// CacheHitPositive indicates a live, compiled artifact is cached.
	CacheHitPositive

	// CacheHitNegative indicates the key was previously confirmed absent
	// from the data store. Callers MUST NOT fall back to the database;
	// doing so defeats the purpose of negative caching and re-exposes the
	// repository to repeated-miss amplification.
	CacheHitNegative
)

func (s CacheStatus) String() string {
	switch s {
	case CacheHitPositive:
		return "positive"
	case CacheHitNegative:
		return "negative"
	default:
		return "miss"
	}
}

// ---------------------------------------------------------------------------
// CacheStats: observable snapshot of cache health.
// ---------------------------------------------------------------------------

// CacheStats is a point-in-time snapshot suitable for export to metrics systems.
type CacheStats struct {
	Size          int
	PositiveCount int
	NegativeCount int

	Hits         uint64
	Misses       uint64
	NegativeHits uint64

	// Evictions is HardCapEvictions + WatermarkEvictions.
	Evictions uint64
	// HardCapEvictions counts entries removed synchronously, inline with a
	// Set/Nullify call, because a shard's absolute MaxEntries bound was hit.
	// In steady state this should stay near zero; sustained nonzero values
	// mean the watermark evictor cannot keep up with the write rate.
	HardCapEvictions uint64
	// WatermarkEvictions counts entries removed by the background watermark
	// evictor.
	WatermarkEvictions uint64

	Expirations uint64
	Compactions uint64

	// EvictorActive reports whether the background watermark evictor
	// goroutine is currently running (i.e. size is at/above the high
	// watermark and has not yet drained to the low watermark).
	EvictorActive bool
}

// ---------------------------------------------------------------------------
// RepositoryCache interface
// ---------------------------------------------------------------------------

// RepositoryCache provides an abstraction for caching compiled artifacts.
// Implementations must be safe for concurrent use from multiple goroutines.
//
// Get/GetStatus return the stored value directly, not a copy. Callers must
// treat T as read-only; mutating a returned value corrupts the entry for
// every other caller and, under concurrent access, is a data race. Use
// Set/SetWithTTL to atomically replace an entry instead of mutating one
// in place.
type RepositoryCache[T any] interface {
	// Get returns the cached value and true if it is present and positive.
	// Prefer GetStatus when the caller needs to distinguish a genuine miss
	// from a cached negative (e.g. to avoid inadvertent read-through on a
	// key that has been confirmed absent from the database).
	Get(key string) (T, bool)

	// GetStatus is the canonical lookup method. Get is a thin wrapper around it.
	GetStatus(key string) (T, CacheStatus)

	// Set stores a positive value for the key using the configured default
	// positive TTL, clearing any negative marker. Equivalent to
	// SetWithTTL(key, value, DefaultTTL).
	Set(key string, value T)

	// SetWithTTL is like Set but overrides the default positive TTL for
	// this key. See DefaultTTL and NoExpiration.
	SetWithTTL(key string, value T, ttl time.Duration)

	// Nullify stores a negative marker for the key using the configured
	// default negative TTL, to avoid future database lookups. Equivalent to
	// NullifyWithTTL(key, DefaultTTL).
	Nullify(key string)

	// NullifyWithTTL is like Nullify but overrides the default negative TTL
	// for this key.
	NullifyWithTTL(key string, ttl time.Duration)

	// TTL reports the remaining time-to-live for key. ok is false if the
	// key is not cached (or has already expired). If the key is cached but
	// has no expiration, the returned duration is NoExpiration.
	TTL(key string) (time.Duration, bool)

	// Persist removes any expiration from key so it no longer expires by
	// TTL (it remains subject to capacity-based eviction). Returns false if
	// key is not currently cached.
	Persist(key string) bool

	// Evict removes any entry (positive or negative) for the key.
	Evict(key string)

	// Keys returns all keys with live positive entries.
	Keys() []string

	// Clear removes all entries from the cache.
	Clear()

	// Clone returns a deep copy, including an independent background
	// janitor/evictor if the implementation runs them. Callers must Close
	// the clone.
	Clone() (RepositoryCache[T], error)

	// Stats returns a point-in-time snapshot.
	Stats() CacheStats

	// Close stops background goroutines and releases resources. Idempotent.
	Close() error
}

// ---------------------------------------------------------------------------
// tickerHandle: abstracts time.Ticker so tests can inject a manually-driven
// fake instead of a real wall-clock ticker, making janitor/evictor behavior
// deterministic and instant to test.
// ---------------------------------------------------------------------------

type tickerHandle interface {
	C() <-chan time.Time
	Stop()
}

type tickerFactory func(time.Duration) tickerHandle

type realTickerHandle struct{ t *time.Ticker }

func newRealTickerHandle(d time.Duration) tickerHandle {
	return &realTickerHandle{t: time.NewTicker(d)}
}

func (r *realTickerHandle) C() <-chan time.Time { return r.t.C }
func (r *realTickerHandle) Stop()               { r.t.Stop() }

// ---------------------------------------------------------------------------
// cacheItem / cacheShard
// ---------------------------------------------------------------------------

// cacheItem is stored as the Value of each list.Element inside a cacheShard.
//
// accessed is set (via a single atomic store) by every successful read, and
// consulted — never mutated — by readers otherwise. Only the background
// eviction path (which already holds the shard's exclusive lock) clears it.
// This is what lets reads take only a shared lock: recency is recorded as a
// flag instead of by physically reordering the list, which would otherwise
// require exclusive access on every single Get.
type cacheItem[T any] struct {
	key          string
	artifact     T
	notAvailable bool
	expiresAt    time.Time // zero = never expires
	accessed     atomic.Bool
}

func (it *cacheItem[T]) expired(now time.Time) bool {
	return !it.expiresAt.IsZero() && now.After(it.expiresAt)
}

// cacheShard is one independently-locked partition of a managedCache.
//
// Locking model: mu is a sync.RWMutex. Pure reads (getStatus, ttl, Keys/Stats
// enumeration, snapshot) take only RLock, so many goroutines — including many
// goroutines reading the very same hot key — can proceed concurrently. Any
// operation that mutates the map or the list (set, nullify, evict, eviction,
// sweep, compaction, persist) takes the exclusive Lock. Recency tracking for
// eviction purposes does NOT reorder the list on read (see cacheItem.accessed
// above); eviction instead uses a CLOCK/second-chance scan (see
// evictOneCLOCK), which is an approximation of LRU, not exact LRU. For
// read-heavy workloads dominated by a small number of very hot keys, this
// approximation is deliberately traded for eliminating the read-side
// exclusive lock entirely.
//
// One consequence: getStatus/ttl no longer remove an expired entry the
// moment it's observed as expired (that would require the exclusive lock).
// An expired entry may therefore sit in items/order, physically present but
// logically dead, until the janitor's next bounded sweep or until a write
// touches that exact key. Stats()/Keys() filter by expiry so this is not
// observable through those APIs; it does mean the hard MaxEntries cap is on
// physical slots, which can transiently include not-yet-reaped garbage
// during heavy TTL churn combined with a slow JanitorInterval.
type cacheShard[T any] struct {
	mu         sync.RWMutex
	items      map[string]*list.Element // key → list element
	order      *list.List               // traversal order for the CLOCK scan; NOT an LRU/MRU order
	clockHand  *list.Element            // current position of the CLOCK eviction scan
	maxEntries int                      // 0 = unbounded; hard safety-net cap
	compactAt  int                      // rebuild backing map after this many tombstones
	tombstones int

	// globalSize, if non-nil, is kept in sync with len(items) so the owning
	// managedCache can cheaply check total size (for watermark eviction)
	// without locking every shard.
	globalSize *atomic.Int64
}

func newCacheShard[T any](maxEntries, compactAt int) *cacheShard[T] {
	return &cacheShard[T]{
		items:      make(map[string]*list.Element),
		order:      list.New(),
		maxEntries: maxEntries,
		compactAt:  compactAt,
	}
}

// removeElementLocked removes the element and its map key, fixing up the
// CLOCK hand if it currently points at the element being removed. Caller
// holds s.mu exclusively.
func (s *cacheShard[T]) removeElementLocked(el *list.Element) {
	if s.clockHand == el {
		s.clockHand = el.Prev()
		if s.clockHand == nil {
			s.clockHand = el.Next()
		}
		if s.clockHand == el {
			s.clockHand = nil // el was the only element in the list
		}
	}
	it := el.Value.(*cacheItem[T])
	delete(s.items, it.key)
	s.order.Remove(el)
	s.tombstones++
	if s.globalSize != nil {
		s.globalSize.Add(-1)
	}
}

// evictOneCLOCK evicts one entry using a CLOCK (second-chance) approximation
// of LRU. Candidates are scanned starting from the shard's clock hand; an
// item with its accessed bit set is given a second chance (the bit is
// cleared and the scan continues) rather than being evicted outright. This
// lets a read mark recency with a single atomic store under a shared lock,
// instead of requiring the exclusive lock to physically reorder a linked
// list on every access — which would otherwise serialize every read of a
// shard behind one lock, regardless of how many distinct keys live in it.
//
// A nice side effect (shared with real CLOCK implementations, e.g. Linux's
// page reclaim): scan resistance. A burst of one-off inserts that are never
// read again start with accessed=false and are the first things evicted,
// rather than displacing genuinely hot keys the way a naive
// insertion-order/FIFO scheme would.
//
// Caller holds s.mu exclusively.
func (s *cacheShard[T]) evictOneCLOCK() bool {
	if s.order.Len() == 0 {
		return false
	}
	if s.clockHand == nil {
		s.clockHand = s.order.Back()
	}
	maxScan := s.order.Len()
	for scanned := 0; scanned < maxScan; scanned++ {
		candidate := s.clockHand
		it := candidate.Value.(*cacheItem[T])
		if it.accessed.Load() {
			it.accessed.Store(false) // give it a second chance
			next := candidate.Prev()
			if next == nil {
				next = s.order.Back() // wrap around
			}
			s.clockHand = next
			continue
		}
		s.removeElementLocked(candidate) // fixes up clockHand internally
		return true
	}
	// Every entry had its accessed bit set (a full pass gave everyone a
	// second chance). Evict whatever the hand now points to rather than
	// looping forever under sustained all-hot access; this bounds a single
	// eviction call to at most 2x the shard's length of work.
	if s.clockHand != nil {
		s.removeElementLocked(s.clockHand)
		return true
	}
	return false
}

// evictOneCLOCKLocking acquires the shard's exclusive lock and evicts one
// entry via evictOneCLOCK. Used by the background watermark evictor, which
// calls shard methods without already holding the shard's lock.
func (s *cacheShard[T]) evictOneCLOCKLocking() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evictOneCLOCK()
}

// getStatus is the hot read path. It takes only a shared (read) lock: it
// never mutates the map or the list, and the only per-item state it touches
// is a single atomic store of the accessed flag. This is what allows many
// goroutines — including many goroutines reading the exact same key — to
// proceed concurrently instead of serializing behind an exclusive lock.
//
// An expired entry is reported as CacheMiss but is NOT removed here (removal
// is a write and would require the exclusive lock); it is reaped by the
// janitor's next sweep or overwritten by the next write to that key. See the
// cacheShard doc comment for the full trade-off.
func (s *cacheShard[T]) getStatus(key string, now time.Time) (T, CacheStatus) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	el, ok := s.items[key]
	if !ok {
		var zero T
		return zero, CacheMiss
	}
	it := el.Value.(*cacheItem[T])
	if it.expired(now) {
		var zero T
		return zero, CacheMiss
	}
	it.accessed.Store(true)
	if it.notAvailable {
		var zero T
		return zero, CacheHitNegative
	}
	return it.artifact, CacheHitPositive
}

// upsertLocked creates or updates an entry and applies the hard capacity
// cap via CLOCK eviction if exceeded. Returns true if a hard-cap eviction
// occurred. Caller holds s.mu exclusively.
func (s *cacheShard[T]) upsertLocked(key string, artifact T, notAvailable bool, expiresAt time.Time) bool {
	if el, ok := s.items[key]; ok {
		it := el.Value.(*cacheItem[T])
		it.artifact = artifact
		it.notAvailable = notAvailable
		it.expiresAt = expiresAt
		// An explicit write is not itself "read access": reset the accessed
		// bit so a key isn't protected from eviction merely because it was
		// just refreshed rather than actually read.
		it.accessed.Store(false)
		s.order.MoveToFront(el)
		return false
	}
	item := &cacheItem[T]{key: key, artifact: artifact, notAvailable: notAvailable, expiresAt: expiresAt}
	el := s.order.PushFront(item)
	s.items[key] = el
	if s.globalSize != nil {
		s.globalSize.Add(1)
	}
	if s.maxEntries > 0 && len(s.items) > s.maxEntries {
		s.evictOneCLOCK()
		return true
	}
	return false
}

func (s *cacheShard[T]) set(key string, artifact T, expiresAt time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertLocked(key, artifact, false, expiresAt)
}

func (s *cacheShard[T]) nullify(key string, expiresAt time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var zero T
	return s.upsertLocked(key, zero, true, expiresAt)
}

func (s *cacheShard[T]) evict(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.items[key]; ok {
		s.removeElementLocked(el)
	}
}

func (s *cacheShard[T]) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.globalSize != nil {
		s.globalSize.Add(-int64(len(s.items)))
	}
	s.items = make(map[string]*list.Element)
	s.order = list.New()
	s.clockHand = nil
	s.tombstones = 0
}

// appendPositiveKeys is read-only and takes only a shared lock. Entries that
// are logically expired (even if not yet physically reaped) are excluded so
// Keys() never reports stale keys regardless of janitor timing.
func (s *cacheShard[T]) appendPositiveKeys(out []string, now time.Time) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for e := s.order.Front(); e != nil; e = e.Next() {
		it := e.Value.(*cacheItem[T])
		if !it.notAvailable && !it.expired(now) {
			out = append(out, it.key)
		}
	}
	return out
}

// sweepExpiredBounded removes expired entries, examining at most `limit`
// entries starting from the tail of the traversal order (limit <= 0 means
// unbounded). This is a write operation (removal) and takes the exclusive
// lock. Any expired entries beyond the budget are caught on the next tick,
// or reported as logically absent in the meantime via getStatus/Stats/Keys,
// which all check expiry independently of physical presence.
func (s *cacheShard[T]) sweepExpiredBounded(now time.Time, limit int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := 0
	examined := 0
	var next *list.Element
	for e := s.order.Back(); e != nil && (limit <= 0 || examined < limit); e = next {
		next = e.Prev()
		examined++
		if e.Value.(*cacheItem[T]).expired(now) {
			s.removeElementLocked(e)
			expired++
		}
	}
	return expired
}

// compactIfNeeded rebuilds the backing map to reclaim memory from deleted
// slots (Go maps do not release bucket memory after deletions). Write
// operation; takes the exclusive lock.
func (s *cacheShard[T]) compactIfNeeded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tombstones < s.compactAt {
		return false
	}
	rebuilt := make(map[string]*list.Element, len(s.items))
	for k, v := range s.items {
		rebuilt[k] = v
	}
	s.items = rebuilt
	s.tombstones = 0
	return true
}

// snapshot returns a deep copy of the shard's live entries for cache-level
// Clone(). Read-only over the map/list; takes only a shared lock.
func (s *cacheShard[T]) snapshot(cloneFn func(T) (T, error)) (map[string]*cacheItem[T], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*cacheItem[T], len(s.items))
	for k, el := range s.items {
		src := el.Value.(*cacheItem[T])
		dst := &cacheItem[T]{key: src.key, notAvailable: src.notAvailable, expiresAt: src.expiresAt}
		dst.accessed.Store(src.accessed.Load())
		if !src.notAvailable && cloneFn != nil {
			cloned, err := cloneFn(src.artifact)
			if err != nil {
				return nil, fmt.Errorf("cache snapshot: key %q: %w", k, err)
			}
			dst.artifact = cloned
		} else {
			dst.artifact = src.artifact
		}
		out[k] = dst
	}
	return out, nil
}

// counts is read-only and takes only a shared lock. Logically expired
// entries are excluded so Stats() never over-reports due to janitor timing.
func (s *cacheShard[T]) counts(now time.Time) (positive, negative int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, el := range s.items {
		it := el.Value.(*cacheItem[T])
		if it.expired(now) {
			continue
		}
		if it.notAvailable {
			negative++
		} else {
			positive++
		}
	}
	return
}

// ttl reports the remaining time-to-live for key. ok is false if key is
// absent or has already expired. If the key exists with no expiration, the
// returned duration is NoExpiration. Like getStatus, ttl takes only a
// shared lock and does not remove an expired entry (that's a write); it
// mirrors Redis's TTL command, which is a read-only introspection and also
// does not affect recency/eviction ordering.
func (s *cacheShard[T]) ttl(key string, now time.Time) (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	el, ok := s.items[key]
	if !ok {
		return 0, false
	}
	it := el.Value.(*cacheItem[T])
	if it.expired(now) {
		return 0, false
	}
	if it.expiresAt.IsZero() {
		return NoExpiration, true
	}
	remaining := it.expiresAt.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	return remaining, true
}

// persist removes any expiration from key. Returns false if key is absent
// or already expired (in which case, unlike ttl/getStatus, it IS removed
// here — persist already needs the exclusive lock to mutate expiresAt, so
// eagerly reaping a found-to-be-expired entry costs nothing extra). Mirrors
// Redis's PERSIST, which does not affect recency/eviction ordering.
func (s *cacheShard[T]) persist(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[key]
	if !ok {
		return false
	}
	it := el.Value.(*cacheItem[T])
	if it.expired(now) {
		s.removeElementLocked(el)
		return false
	}
	it.expiresAt = time.Time{}
	return true
}

// ---------------------------------------------------------------------------
// managedCache
// ---------------------------------------------------------------------------

// managedCache is the production-grade RepositoryCache implementation.
type managedCache[T any] struct {
	cfg       CacheConfig
	shards    []*cacheShard[T]
	seed      maphash.Seed
	cloneFn   func(T) (T, error)
	clock     func() time.Time
	newTicker tickerFactory

	size atomic.Int64

	hits               atomic.Uint64
	misses             atomic.Uint64
	negHits            atomic.Uint64
	hardCapEvictions   atomic.Uint64
	watermarkEvictions atomic.Uint64
	expirations        atomic.Uint64
	compactions        atomic.Uint64
	evictorRunning     atomic.Bool

	baseCtx   context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewManagedCache constructs a production-grade RepositoryCache.
//
// Key properties:
//   - Sharded: cfg.ShardCount independent read-write locks reduce contention.
//   - Read-optimized: Get/GetStatus/TTL take only a shared (read) lock and
//     never mutate the shard's list; recency is tracked via a single atomic
//     flag per entry instead of by reordering a linked list. This means
//     concurrent reads of the exact same hot key never serialize behind an
//     exclusive lock, which sharding by key alone cannot help with.
//   - Bounded: a hard per-shard cap (derived from cfg.MaxEntries) evicts an
//     entry via a CLOCK (second-chance) approximation of LRU if ever
//     exceeded, as a burst safety net. CLOCK is an approximation, not exact
//     LRU — see cacheShard's doc comment for the trade-off this buys.
//   - Proactively bounded: a background watermark evictor starts once total
//     size crosses cfg.EvictionHighWatermark and runs until it drains to
//     cfg.EvictionLowWatermark, keeping the synchronous hard cap rarely
//     exercised in steady state (Redis maxmemory-eviction-inspired).
//   - TTL-aware per key: SetWithTTL/NullifyWithTTL allow overriding the
//     default TTL per entry; TTL/Persist mirror Redis's TTL/PERSIST.
//   - Self-compacting: the janitor rebuilds shard maps after
//     cfg.CompactionThreshold deletions to reclaim memory.
//   - Hash-randomised: shard selection uses a process-unique random seed so
//     attacker-controlled keys cannot be crafted to target a single shard.
//   - Read-only artifacts: Get returns the cached instance directly with no
//     defensive copy. Callers must not mutate it in place — see
//     LiveCollection.Get's doc comment for the full contract.
//
// The caller MUST call Close on the returned value to stop background
// goroutines.
func NewManagedCache[T any](cfg CacheConfig, cloneFn func(T) (T, error)) RepositoryCache[T] {
	return newManagedCache[T](cfg, cloneFn, nil, nil)
}

// newManagedCache is the unexported constructor used internally and by
// tests to inject a deterministic clock and/or ticker factory. Passing nil
// for either uses the real wall clock / real time.Ticker, identical to
// NewManagedCache.
func newManagedCache[T any](cfg CacheConfig, cloneFn func(T) (T, error), clock func() time.Time, tf tickerFactory) *managedCache[T] {
	cfg = cfg.normalize()
	if cloneFn == nil {
		cloneFn = func(v T) (T, error) { return v, nil }
	}
	if clock == nil {
		clock = time.Now
	}
	if tf == nil {
		tf = newRealTickerHandle
	}

	perShard := 0
	if cfg.MaxEntries > 0 {
		perShard = cfg.MaxEntries / cfg.ShardCount
		if perShard < 1 {
			perShard = 1
		}
	}

	c := &managedCache[T]{
		cfg:       cfg,
		seed:      maphash.MakeSeed(),
		cloneFn:   cloneFn,
		clock:     clock,
		newTicker: tf,
	}
	c.baseCtx, c.cancel = context.WithCancel(context.Background())

	c.shards = make([]*cacheShard[T], cfg.ShardCount)
	for i := range c.shards {
		c.shards[i] = newCacheShard[T](perShard, cfg.CompactionThreshold)
		c.shards[i].globalSize = &c.size
	}

	if cfg.JanitorInterval > 0 {
		c.wg.Add(1)
		go c.janitor()
	}

	return c
}

// shardFor selects a shard via a process-unique random hash seed so that
// attacker-controlled key values cannot deliberately concentrate load on a
// single shard across process restarts.
func (c *managedCache[T]) shardFor(key string) *cacheShard[T] {
	h := maphash.String(c.seed, key)
	return c.shards[h&uint64(len(c.shards)-1)]
}

func (c *managedCache[T]) GetStatus(key string) (T, CacheStatus) {
	v, status := c.shardFor(key).getStatus(key, c.clock())
	switch status {
	case CacheHitPositive:
		c.hits.Add(1)
	case CacheHitNegative:
		c.negHits.Add(1)
	default:
		c.misses.Add(1)
	}
	return v, status
}

func (c *managedCache[T]) Get(key string) (T, bool) {
	v, s := c.GetStatus(key)
	return v, s == CacheHitPositive
}

func (c *managedCache[T]) SetWithTTL(key string, value T, ttl time.Duration) {
	if len(key) > c.cfg.MaxKeyLength {
		return
	}
	resolved := resolveTTL(ttl, c.cfg.PositiveTTL)
	var expiresAt time.Time
	if resolved > 0 {
		expiresAt = c.clock().Add(resolved)
	}
	if c.shardFor(key).set(key, value, expiresAt) {
		c.hardCapEvictions.Add(1)
	}
	c.maybeStartEvictor()
}

func (c *managedCache[T]) Set(key string, value T) {
	c.SetWithTTL(key, value, DefaultTTL)
}

func (c *managedCache[T]) NullifyWithTTL(key string, ttl time.Duration) {
	if len(key) > c.cfg.MaxKeyLength {
		return
	}
	resolved := resolveTTL(ttl, c.cfg.NegativeTTL)
	var expiresAt time.Time
	if resolved > 0 {
		expiresAt = c.clock().Add(resolved)
	}
	if c.shardFor(key).nullify(key, expiresAt) {
		c.hardCapEvictions.Add(1)
	}
	c.maybeStartEvictor()
}

func (c *managedCache[T]) Nullify(key string) {
	c.NullifyWithTTL(key, DefaultTTL)
}

func (c *managedCache[T]) TTL(key string) (time.Duration, bool) {
	return c.shardFor(key).ttl(key, c.clock())
}

func (c *managedCache[T]) Persist(key string) bool {
	return c.shardFor(key).persist(key, c.clock())
}

func (c *managedCache[T]) Evict(key string) {
	c.shardFor(key).evict(key)
}

func (c *managedCache[T]) Keys() []string {
	now := c.clock()
	out := make([]string, 0)
	for _, s := range c.shards {
		out = s.appendPositiveKeys(out, now)
	}
	return out
}

func (c *managedCache[T]) Clear() {
	for _, s := range c.shards {
		s.clear()
	}
}

// Clone returns a deep copy of the cache with independent background
// goroutines. Relative LRU order between entries is not preserved; per-entry
// TTLs are. The caller must Close the clone independently.
func (c *managedCache[T]) Clone() (RepositoryCache[T], error) {
	clone := &managedCache[T]{
		cfg:       c.cfg,
		seed:      c.seed,
		cloneFn:   c.cloneFn,
		clock:     c.clock,
		newTicker: c.newTicker,
	}
	clone.baseCtx, clone.cancel = context.WithCancel(context.Background())
	clone.shards = make([]*cacheShard[T], len(c.shards))

	var total int64
	for i, s := range c.shards {
		snap, err := s.snapshot(c.cloneFn)
		if err != nil {
			clone.cancel()
			return nil, err
		}
		ns := newCacheShard[T](s.maxEntries, c.cfg.CompactionThreshold)
		ns.globalSize = &clone.size
		for _, item := range snap {
			el := ns.order.PushFront(item)
			ns.items[item.key] = el
		}
		total += int64(len(snap))
		clone.shards[i] = ns
	}
	clone.size.Store(total)

	if c.cfg.JanitorInterval > 0 {
		clone.wg.Add(1)
		go clone.janitor()
	}
	return clone, nil
}

func (c *managedCache[T]) Stats() CacheStats {
	var pos, neg int
	for _, s := range c.shards {
		p, n := s.counts(c.clock())
		pos += p
		neg += n
	}
	hardCap := c.hardCapEvictions.Load()
	watermark := c.watermarkEvictions.Load()
	return CacheStats{
		Size:               pos + neg,
		PositiveCount:      pos,
		NegativeCount:      neg,
		Hits:               c.hits.Load(),
		Misses:             c.misses.Load(),
		NegativeHits:       c.negHits.Load(),
		Evictions:          hardCap + watermark,
		HardCapEvictions:   hardCap,
		WatermarkEvictions: watermark,
		Expirations:        c.expirations.Load(),
		Compactions:        c.compactions.Load(),
		EvictorActive:      c.evictorRunning.Load(),
	}
}

// Close stops the background janitor and watermark evictor (if running)
// and waits for them to exit. Idempotent.
func (c *managedCache[T]) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		c.wg.Wait()
	})
	return nil
}

func (c *managedCache[T]) janitor() {
	defer c.wg.Done()
	t := c.newTicker(c.cfg.JanitorInterval)
	defer t.Stop()
	for {
		select {
		case <-c.baseCtx.Done():
			return
		case now := <-t.C():
			c.sweep(now)
		}
	}
}

func (c *managedCache[T]) sweep(now time.Time) {
	var totalExpired, totalCompacted int
	for _, s := range c.shards {
		totalExpired += s.sweepExpiredBounded(now, c.cfg.JanitorBatchSize)
		if s.compactIfNeeded() {
			totalCompacted++
		}
	}
	if totalExpired > 0 {
		c.expirations.Add(uint64(totalExpired))
	}
	if totalCompacted > 0 {
		c.compactions.Add(uint64(totalCompacted))
	}
	if (totalExpired > 0 || totalCompacted > 0) && c.cfg.Logger != nil {
		stats := c.Stats()
		c.cfg.Logger.Debug("cache janitor sweep",
			slog.Int("expired", totalExpired),
			slog.Int("shardsCompacted", totalCompacted),
			slog.Int("size", stats.Size),
			slog.Int("positive", stats.PositiveCount),
			slog.Int("negative", stats.NegativeCount),
		)
	}
}

// maybeStartEvictor spawns the background watermark evictor if total size
// has reached the configured high watermark and it is not already running.
// Safe to call unconditionally on every write; the check is a single atomic
// load plus a CAS in the (rare) triggering case.
func (c *managedCache[T]) maybeStartEvictor() {
	if c.cfg.MaxEntries <= 0 {
		return
	}
	highCount := int64(float64(c.cfg.MaxEntries) * c.cfg.EvictionHighWatermark)
	if c.size.Load() < highCount {
		return
	}
	if !c.evictorRunning.CompareAndSwap(false, true) {
		return // already running
	}
	c.wg.Add(1)
	go c.runEvictor()
}

// runEvictor is the background watermark evictor goroutine body. It ticks
// at cfg.EvictionInterval, each tick evicting LRU entries round-robin across
// shards (bounded by cfg.EvictionBatchSize), until total size drains to the
// low watermark, at which point it exits (rather than idling), matching the
// start/stop-on-threshold behavior of a hysteresis band. It also exits
// promptly on cache Close.
func (c *managedCache[T]) runEvictor() {
	defer c.wg.Done()
	defer c.evictorRunning.Store(false)

	lowCount := int64(float64(c.cfg.MaxEntries) * c.cfg.EvictionLowWatermark)
	t := c.newTicker(c.cfg.EvictionInterval)
	defer t.Stop()

	shardIdx := 0
	for {
		select {
		case <-c.baseCtx.Done():
			return
		case <-t.C():
			if c.size.Load() <= lowCount {
				return
			}
			evictedThisTick := 0
			noProgress := 0
			reachedLowWatermark := false
			for evictedThisTick < c.cfg.EvictionBatchSize {
				if c.size.Load() <= lowCount {
					reachedLowWatermark = true
					break
				}
				shard := c.shards[shardIdx%len(c.shards)]
				shardIdx++
				if shard.evictOneCLOCKLocking() {
					evictedThisTick++
					noProgress = 0
				} else {
					noProgress++
					if noProgress >= len(c.shards) {
						break // a full round found nothing to evict anywhere
					}
				}
			}
			if evictedThisTick > 0 {
				c.watermarkEvictions.Add(uint64(evictedThisTick))
				if c.cfg.Logger != nil {
					c.cfg.Logger.Debug("watermark evictor tick",
						slog.Int("evicted", evictedThisTick),
						slog.Int64("size", c.size.Load()))
				}
			}
			if reachedLowWatermark {
				if c.cfg.Logger != nil {
					c.cfg.Logger.Debug("watermark evictor reached low watermark, stopping",
						slog.Int64("size", c.size.Load()),
						slog.Int64("lowWatermark", lowCount))
				}
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// inMemoryCache – simple, unbounded, no-expiration implementation.
// Suitable for tests and short-lived processes only.
// ---------------------------------------------------------------------------

type simpleCacheEntry[T any] struct {
	artifact     T
	notAvailable bool
}

type inMemoryCache[T any] struct {
	mu      sync.RWMutex
	entries map[string]*simpleCacheEntry[T]
	cloneFn func(T) (T, error)
}

// NewInMemoryCache creates an unbounded in-memory cache with no expiration
// or eviction policy. TTL parameters accepted by SetWithTTL/NullifyWithTTL
// are ignored (present only for interface compatibility). For production
// use, prefer NewManagedCache, which enforces size bounds and TTL
// expiration.
func NewInMemoryCache[T any](cloneFn func(T) (T, error)) RepositoryCache[T] {
	return &inMemoryCache[T]{
		entries: make(map[string]*simpleCacheEntry[T]),
		cloneFn: cloneFn,
	}
}

func (c *inMemoryCache[T]) GetStatus(key string) (T, CacheStatus) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok {
		var zero T
		return zero, CacheMiss
	}
	if e.notAvailable {
		var zero T
		return zero, CacheHitNegative
	}
	return e.artifact, CacheHitPositive
}

func (c *inMemoryCache[T]) Get(key string) (T, bool) {
	v, s := c.GetStatus(key)
	return v, s == CacheHitPositive
}

func (c *inMemoryCache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &simpleCacheEntry[T]{artifact: value}
}

// SetWithTTL ignores ttl: inMemoryCache has no expiration concept.
func (c *inMemoryCache[T]) SetWithTTL(key string, value T, _ time.Duration) {
	c.Set(key, value)
}

func (c *inMemoryCache[T]) Nullify(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &simpleCacheEntry[T]{notAvailable: true}
}

// NullifyWithTTL ignores ttl: inMemoryCache has no expiration concept.
func (c *inMemoryCache[T]) NullifyWithTTL(key string, _ time.Duration) {
	c.Nullify(key)
}

func (c *inMemoryCache[T]) TTL(key string) (time.Duration, bool) {
	if _, status := c.GetStatus(key); status != CacheMiss {
		return NoExpiration, true
	}
	return 0, false
}

func (c *inMemoryCache[T]) Persist(key string) bool {
	_, status := c.GetStatus(key)
	return status != CacheMiss
}

func (c *inMemoryCache[T]) Evict(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *inMemoryCache[T]) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.entries))
	for k, e := range c.entries {
		if !e.notAvailable {
			out = append(out, k)
		}
	}
	return out
}

func (c *inMemoryCache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*simpleCacheEntry[T])
}

func (c *inMemoryCache[T]) Clone() (RepositoryCache[T], error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copied := make(map[string]*simpleCacheEntry[T], len(c.entries))
	for k, e := range c.entries {
		if e.notAvailable {
			copied[k] = &simpleCacheEntry[T]{notAvailable: true}
			continue
		}
		var artifact T
		if c.cloneFn != nil {
			cloned, err := c.cloneFn(e.artifact)
			if err != nil {
				cloned = e.artifact // best-effort shallow copy on failure
			}
			artifact = cloned
		} else {
			artifact = e.artifact
		}
		copied[k] = &simpleCacheEntry[T]{artifact: artifact}
	}
	return &inMemoryCache[T]{entries: copied, cloneFn: c.cloneFn}, nil
}

func (c *inMemoryCache[T]) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var pos, neg int
	for _, e := range c.entries {
		if e.notAvailable {
			neg++
		} else {
			pos++
		}
	}
	return CacheStats{Size: pos + neg, PositiveCount: pos, NegativeCount: neg}
}

func (c *inMemoryCache[T]) Close() error { return nil }

// ---------------------------------------------------------------------------
// CacheConfig – configuration for managedCache
// ---------------------------------------------------------------------------

// CacheConfig tunes the managed cache. See DefaultCacheConfig for sane defaults.
//
// Security note: keys in a read-through repository frequently originate
// from caller-supplied values. Without bounds, an adversary can exhaust
// memory via repeated lookups for distinct nonexistent keys (negative-cache
// amplification). MaxEntries, MaxKeyLength, and NegativeTTL together close
// that vector.
type CacheConfig struct {
	// MaxEntries bounds total entries across all shards. <= 0 disables the
	// bound entirely (no eviction of any kind); not recommended in
	// production. When > 0, it is enforced two ways: (1) proactively, by
	// the background watermark evictor once size crosses
	// EvictionHighWatermark, and (2) as an absolute, synchronous safety net
	// per shard if writes outrun the evictor.
	MaxEntries int

	// PositiveTTL is the default lifetime of compiled artifacts, used when
	// Set (or SetWithTTL with DefaultTTL) is called. Zero means entries
	// never expire by TTL (only by eviction). Overridable per key via
	// SetWithTTL.
	PositiveTTL time.Duration

	// NegativeTTL is the default lifetime of "not found" markers, used when
	// Nullify (or NullifyWithTTL with DefaultTTL) is called. Keeping this
	// short limits both negative-cache amplification and shadow duration
	// for keys that become available after an earlier miss. Overridable
	// per key via NullifyWithTTL.
	NegativeTTL time.Duration

	// JanitorInterval controls how often the background goroutine sweeps
	// expired entries and compacts shard maps. <= 0 disables the janitor
	// (expiry is still enforced lazily on access).
	JanitorInterval time.Duration

	// JanitorBatchSize bounds how many entries (starting from each shard's
	// LRU tail, where cold entries naturally accumulate) are examined for
	// expiry per shard per janitor tick. This keeps a single sweep from
	// holding a shard's lock for an unbounded time on a large cache; any
	// expired entries beyond the budget are caught on the next tick or
	// lazily on next access. Defaults to 1000.
	JanitorBatchSize int

	// ShardCount is rounded up to the next power of two. Defaults to 16.
	ShardCount int

	// MaxKeyLength bounds key size; overlength keys are silently bypassed
	// (never stored). Protects against memory exhaustion from huge keys.
	// Defaults to 512.
	MaxKeyLength int

	// CompactionThreshold is the number of delete-tombstones a shard map
	// may accumulate before the janitor rebuilds it to reclaim memory Go's
	// map implementation holds after deletions. Defaults to 256.
	CompactionThreshold int

	// EvictionHighWatermark is the fraction of MaxEntries (0,1] at which
	// the background watermark evictor goroutine is started. Defaults to
	// 0.90. Ignored if MaxEntries <= 0.
	EvictionHighWatermark float64

	// EvictionLowWatermark is the fraction of MaxEntries (0, EvictionHighWatermark)
	// at which the watermark evictor stops (exits) after having been
	// started. Must be strictly less than EvictionHighWatermark; if it
	// isn't, it is reset to 80% of the high watermark. Defaults to 0.75.
	EvictionLowWatermark float64

	// EvictionInterval is the tick interval at which the watermark evictor,
	// while running, evicts a bounded batch of entries. Defaults to 2s.
	EvictionInterval time.Duration

	// EvictionBatchSize bounds how many entries the watermark evictor
	// removes per tick, spread round-robin across shards. Defaults to 200.
	EvictionBatchSize int

	// Logger receives structured diagnostics. Defaults to slog.Default().
	Logger *slog.Logger
}

// DefaultCacheConfig returns sane production defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxEntries:            10_000,
		PositiveTTL:           30 * time.Minute,
		NegativeTTL:           1 * time.Minute,
		JanitorInterval:       1 * time.Minute,
		JanitorBatchSize:      1000,
		ShardCount:            16,
		MaxKeyLength:          512,
		CompactionThreshold:   256,
		EvictionHighWatermark: 0.90,
		EvictionLowWatermark:  0.75,
		EvictionInterval:      2 * time.Second,
		EvictionBatchSize:     200,
	}
}

func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func (cfg CacheConfig) normalize() CacheConfig {
	if cfg.ShardCount <= 0 {
		cfg.ShardCount = 16
	} else {
		cfg.ShardCount = nextPowerOfTwo(cfg.ShardCount)
	}
	if cfg.MaxKeyLength <= 0 {
		cfg.MaxKeyLength = 512
	}
	if cfg.CompactionThreshold <= 0 {
		cfg.CompactionThreshold = 256
	}
	if cfg.JanitorBatchSize <= 0 {
		cfg.JanitorBatchSize = 1000
	}
	if cfg.EvictionHighWatermark <= 0 || cfg.EvictionHighWatermark > 1 {
		cfg.EvictionHighWatermark = 0.90
	}
	if cfg.EvictionLowWatermark <= 0 || cfg.EvictionLowWatermark >= cfg.EvictionHighWatermark {
		cfg.EvictionLowWatermark = cfg.EvictionHighWatermark * 0.8
	}
	if cfg.EvictionInterval <= 0 {
		cfg.EvictionInterval = 2 * time.Second
	}
	if cfg.EvictionBatchSize <= 0 {
		cfg.EvictionBatchSize = 200
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}
