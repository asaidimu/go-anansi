package registry

import (
	"context"
	"maps"
	"sync/atomic"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"go.uber.org/zap"
)

// registrySnapshot is an immutable point-in-time view of the entire registry.
// All read operations use snapshots, eliminating locks on the hot path.
type registrySnapshot struct {
	entries   map[string]*RegistryEntry // Immutable map (never modified after creation)
	loadedAt  time.Time
	entryList []*RegistryEntry // Pre-computed list for List() operations
}

// snapshotCache manages immutable registry snapshots with atomic swapping.
type snapshotCache struct {
	// Atomic pointer to current snapshot - reads are lock-free
	current atomic.Pointer[registrySnapshot]

	// Configuration
	refreshInterval time.Duration // How often to reload entire registry

	// Dependencies
	loader snapshotLoader
	logger *zap.Logger

	// Lifecycle
	stopCh chan struct{}
	doneCh chan struct{}

	// Metrics (atomic counters for lock-free updates)
	stats cacheMetrics
}

type cacheMetrics struct {
	hits           atomic.Int64
	misses         atomic.Int64
	refreshes      atomic.Int64
	refreshErrors  atomic.Int64
	lastRefreshDur atomic.Int64 // Duration in nanoseconds
}

// snapshotLoader abstracts database operations for testing
type snapshotLoader interface {
	LoadAll(ctx context.Context) ([]*RegistryEntry, error)
}

// SnapshotCacheConfig defines cache behavior
type SnapshotCacheConfig struct {
	// RefreshInterval: How often to reload the entire registry
	// Default: 30 seconds (balances freshness vs DB load)
	//
	// Tuning guidance:
	// - 10s: Fresh data, higher DB load (10 reads/min per instance)
	// - 30s: Good balance (2 reads/min per instance)
	// - 60s: Lower DB load, slower change propagation
	// - 5m: Minimal DB load, only for rarely-changing registries
	RefreshInterval time.Duration

	// EnableBackgroundRefresh: Whether to auto-refresh in background
	// Default: true (essential for multi-instance deployments)
	EnableBackgroundRefresh bool
}

func DefaultSnapshotCacheConfig() SnapshotCacheConfig {
	return SnapshotCacheConfig{
		RefreshInterval:         30 * time.Second,
		EnableBackgroundRefresh: true,
	}
}

func newSnapshotCache(config SnapshotCacheConfig, loader snapshotLoader, logger *zap.Logger) *snapshotCache {
	cache := &snapshotCache{
		refreshInterval: config.RefreshInterval,
		loader:          loader,
		logger:          logger,
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}

	// Initialize with empty snapshot
	cache.current.Store(&registrySnapshot{
		entries:   make(map[string]*RegistryEntry),
		loadedAt:  time.Now(),
		entryList: []*RegistryEntry{},
	})

	// Start background refresh if enabled
	if config.EnableBackgroundRefresh {
		go cache.backgroundRefresh()
	}

	return cache
}

// get retrieves an entry by name (lock-free read)
func (c *snapshotCache) get(name string) *RegistryEntry {
	snapshot := c.current.Load()

	entry, exists := snapshot.entries[name]
	if !exists {
		c.stats.misses.Add(1)
		return nil
	}

	c.stats.hits.Add(1)

	// Return a deep copy to prevent external mutation
	// Cost: ~500ns per entry, negligible vs 1-10ms DB query
	return copyEntry(entry)
}

// list returns all registry entries (lock-free read)
func (c *snapshotCache) list() []*RegistryEntry {
	snapshot := c.current.Load()
	c.stats.hits.Add(1)

	// Return deep copy of pre-computed list
	result := make([]*RegistryEntry, len(snapshot.entryList))
	for i, entry := range snapshot.entryList {
		result[i] = copyEntry(entry)
	}
	return result
}

// refresh loads a new snapshot from the database and swaps it in atomically
func (c *snapshotCache) refresh(ctx context.Context) error {
	start := time.Now()
	defer func() {
		c.stats.lastRefreshDur.Store(time.Since(start).Nanoseconds())
	}()

	// Load all entries from database
	entries, err := c.loader.LoadAll(ctx)
	if err != nil {
		c.stats.refreshErrors.Add(1)
		return err
	}

	// Build new immutable snapshot
	entryMap := make(map[string]*RegistryEntry, len(entries))
	for _, entry := range entries {
		entryMap[entry.Name] = entry
	}

	newSnapshot := &registrySnapshot{
		entries:   entryMap,
		loadedAt:  time.Now(),
		entryList: entries, // Already a slice from DB
	}

	// Atomic swap - all subsequent reads see new data
	oldSnapshot := c.current.Swap(newSnapshot)
	c.stats.refreshes.Add(1)

	c.logger.Debug("cache refreshed",
		zap.Int("entries", len(entries)),
		zap.Duration("duration", time.Since(start)),
		zap.Time("previous_load", oldSnapshot.loadedAt),
	)

	return nil
}

// invalidate forces an immediate refresh
func (c *snapshotCache) invalidate(ctx context.Context) error {
	return c.refresh(ctx)
}

// backgroundRefresh periodically reloads the entire registry
func (c *snapshotCache) backgroundRefresh() {
	defer close(c.doneCh)

	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return

		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := c.refresh(ctx); err != nil {
				c.logger.Error("background refresh failed", zap.Error(err))
			}
			cancel()
		}
	}
}

// stop gracefully shuts down background refresh
func (c *snapshotCache) stop() {
	close(c.stopCh)
	<-c.doneCh
}

// getMetrics returns current cache statistics
func (c *snapshotCache) getMetrics() map[string]any {
	snapshot := c.current.Load()

	hits := c.stats.hits.Load()
	misses := c.stats.misses.Load()
	total := hits + misses

	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return map[string]any{
		"entries":          len(snapshot.entries),
		"loaded_at":        snapshot.loadedAt,
		"age_seconds":      time.Since(snapshot.loadedAt).Seconds(),
		"hits":             hits,
		"misses":           misses,
		"hit_rate":         hitRate,
		"refreshes":        c.stats.refreshes.Load(),
		"refresh_errors":   c.stats.refreshErrors.Load(),
		"last_refresh_ms":  c.stats.lastRefreshDur.Load() / 1_000_000,
		"refresh_interval": c.refreshInterval.String(),
	}
}

// copyEntry creates a deep copy of a registry entry to prevent external mutation
func copyEntry(src *RegistryEntry) *RegistryEntry {
	if src == nil {
		return nil
	}

	// Copy versions map
	versions := make(map[string]SchemaVersionRecord, len(src.Versions))
	// SchemaVersionRecord is value type, gets copied
	maps.Copy(versions, src.Versions)

	return &RegistryEntry{
		Name:          src.Name,
		Description:   src.Description,
		ActiveVersion: src.ActiveVersion,
		Versions:      versions,
	}
}

// registryLoader implements snapshotLoader for production use
type registryLoader struct {
	executor RegistryExecutor
}

func newRegistryLoader(executor RegistryExecutor) *registryLoader {
	return &registryLoader{executor: executor}
}

// LoadAll retrieves all registry entries from the database
func (l *registryLoader) LoadAll(ctx context.Context) ([]*RegistryEntry, error) {
	result, err := execute(ctx, l.executor, false, func(tctx context.Context, collection base.Collection, manager query.SchemaManager) ([]*RegistryEntry, error) {
		q := query.NewQueryBuilder().Build()

		readResult, err := collection.Read(tctx, &q)
		if err != nil {
			return nil, err
		}

		if readResult.Count == 0 {
			return []*RegistryEntry{}, nil
		}

		entries := make([]*RegistryEntry, 0, readResult.Count)

		rows := readResult.Data
		for _, row := range rows {
			entry, err := unmarshalEntry(row)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}

		return entries, nil
	})

	return result, err
}
