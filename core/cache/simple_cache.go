package cache

import (
	"sync"
	"time"
)

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



