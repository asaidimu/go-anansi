package query

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

// LRUCache is a thread-safe, in-memory implementation of the Cache interface
// using a size-limited LRU (Least Recently Used) policy.
type LRUCache struct {
	lru *lru.Cache[uint64, *PartitionedQuery]
}

// NewLRUCache creates a new LRUCache with the specified size.
// If size is 0, a non-caching cache is returned, which is useful for development or testing.
func NewLRUCache(size int) (*LRUCache, error) {
	if size <= 0 {
		// Return a no-op cache if size is not positive.
		return &LRUCache{lru: nil}, nil
	}

	cache, err := lru.New[uint64, *PartitionedQuery](size)
	if err != nil {
		return nil, err
	}
	return &LRUCache{lru: cache}, nil
}

// Get retrieves a partitioned query from the cache.
func (c *LRUCache) Get(key uint64) (*PartitionedQuery, bool) {
	if c.lru == nil {
		return nil, false
	}
	return c.lru.Get(key)
}

// Set adds a partitioned query to the cache.
func (c *LRUCache) Set(key uint64, value *PartitionedQuery) {
	if c.lru == nil {
		return
	}
	c.lru.Add(key, value)
}
