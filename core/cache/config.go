package cache

import (
	"log/slog"
	"time"
)

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


