package registry

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// CacheConfig holds configuration for the in-memory cache
type CacheConfig struct {
	// RefreshInterval defines how often to check for external changes
	RefreshInterval time.Duration
	// MaxCacheSize limits the number of entries kept in memory (0 = unlimited)
	MaxCacheSize int
	// EnableAutoRefresh determines if cache should auto-refresh in background
	EnableAutoRefresh bool
}

// DefaultCacheConfig provides sensible defaults for cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		RefreshInterval:   5 * time.Minute,
		MaxCacheSize:      0, // unlimited
		EnableAutoRefresh: true,
	}
}

// cacheEntry wraps a registry entry with metadata for cache management
type cacheEntry struct {
	entry     *RegistryEntry
	lastRead  time.Time
	loadedAt  time.Time
	dirty     bool // true if cache might be stale
}

// collectionCache manages the in-memory cache of registry entries
type collectionCache struct {
	entries map[string]*cacheEntry
	mu      sync.RWMutex
	config  CacheConfig
	logger  *zap.Logger

	// LRU tracking for cache eviction
	accessOrder []string
	maxSize     int
}

// newCollectionCache creates a new cache instance
func newCollectionCache(config CacheConfig, logger *zap.Logger) *collectionCache {
	return &collectionCache{
		entries:     make(map[string]*cacheEntry),
		config:      config,
		logger:      logger,
		accessOrder: make([]string, 0),
		maxSize:     config.MaxCacheSize,
	}
}

// get retrieves an entry from cache, returns nil if not found or stale
func (c *collectionCache) get(name string) *RegistryEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[name]
	if !exists || entry.dirty {
		return nil
	}

	// Update access time and order for LRU
	entry.lastRead = time.Now()
	c.updateAccessOrder(name)

	return entry.entry
}

// set stores an entry in cache
func (c *collectionCache) set(name string, registryEntry *RegistryEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.entries[name] = &cacheEntry{
		entry:    registryEntry,
		lastRead: now,
		loadedAt: now,
		dirty:    false,
	}

	c.updateAccessOrder(name)
	c.evictIfNeeded()
}

// delete removes an entry from cache
func (c *collectionCache) delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, name)
	c.removeFromAccessOrder(name)
}

// clear removes all entries from cache
func (c *collectionCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
	c.accessOrder = make([]string, 0)
}

// list returns all cached entry names
func (c *collectionCache) list() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		if !c.entries[name].dirty {
			names = append(names, name)
		}
	}
	return names
}

// updateAccessOrder maintains LRU order (must be called with lock held)
func (c *collectionCache) updateAccessOrder(name string) {
	// Remove from current position
	for i, n := range c.accessOrder {
		if n == name {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
	// Add to end (most recent)
	c.accessOrder = append(c.accessOrder, name)
}

// removeFromAccessOrder removes name from access tracking (must be called with lock held)
func (c *collectionCache) removeFromAccessOrder(name string) {
	for i, n := range c.accessOrder {
		if n == name {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
}

// evictIfNeeded removes least recently used entries if cache is over limit
func (c *collectionCache) evictIfNeeded() {
	if c.maxSize <= 0 || len(c.entries) <= c.maxSize {
		return
	}

	// Remove least recently used entries
	toRemove := len(c.entries) - c.maxSize
	for i := 0; i < toRemove && len(c.accessOrder) > 0; i++ {
		oldestName := c.accessOrder[0]
		delete(c.entries, oldestName)
		c.accessOrder = c.accessOrder[1:]
		c.logger.Debug("evicted cache entry", zap.String("collection", oldestName))
	}
}
