package bits

import (
	"errors"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/cache"
)

type CachedStats struct {
	Registry RegistryStats
	Cache    cache.CacheStats
}

type CachedRegistry[K comparable, T any] struct {
	registry    *Registry[K, T]
	cache       cache.RepositoryCache[T]
	keyToString func(K) string
	posTTL      time.Duration
	negTTL      time.Duration
}

func NewCachedRegistry[K comparable, T any](
	registry *Registry[K, T],
	cache cache.RepositoryCache[T],
	keyToString func(K) string,
	positiveTTL time.Duration,
	negativeTTL time.Duration,
) *CachedRegistry[K, T] {
	return &CachedRegistry[K, T]{
		registry:    registry,
		cache:       cache,
		keyToString: keyToString,
		posTTL:      positiveTTL,
		negTTL:      negativeTTL,
	}
}

func (c *CachedRegistry[K, T]) Get(key K) (T, error) {
	kstr := c.keyToString(key)
	val, status := c.cache.GetStatus(kstr)
	switch status {
	case cache.CacheHitPositive:
		return val, nil
	case cache.CacheHitNegative:
		var zero T
		return zero, ErrNotFound
	case cache.CacheMiss:
		val, err := c.registry.Get(key)
		if err == nil {
			c.cache.SetWithTTL(kstr, val, c.posTTL)
			return val, nil
		}
		var zero T
		if errors.Is(err, ErrNotFound) {
			c.cache.NullifyWithTTL(kstr, c.negTTL)
			return zero, err
		}
		return zero, err
	}
	var zero T
	return zero, ErrNotFound
}

func (c *CachedRegistry[K, T]) Set(key K, value []byte) (T, error) {
	val, err := c.registry.Set(key, value)
	kstr := c.keyToString(key)
	if err == nil {
		c.cache.SetWithTTL(kstr, val, c.posTTL)
	} else {
		c.cache.Evict(kstr)
	}
	return val, err
}

func (c *CachedRegistry[K, T]) Delete(key K) bool {
	kstr := c.keyToString(key)
	c.cache.Evict(kstr)
	return c.registry.Delete(key)
}

func (c *CachedRegistry[K, T]) Export() []byte {
	return c.registry.Export()
}

func (c *CachedRegistry[K, T]) Import(data []byte) error {
	c.cache.Clear()
	return c.registry.Import(data)
}

func (c *CachedRegistry[K, T]) Stats() CachedStats {
	var cs CachedStats
	if c.registry != nil {
		cs.Registry = c.registry.Stats()
	}
	if c.cache != nil {
		cs.Cache = c.cache.Stats()
	}
	return cs
}

func (c *CachedRegistry[K, T]) Close() error {
	var err1, err2 error
	if c.registry != nil {
		err1 = c.registry.Close()
	}
	if c.cache != nil {
		err2 = c.cache.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}
