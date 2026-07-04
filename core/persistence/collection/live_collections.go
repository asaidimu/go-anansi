package collection

import (
	"context"
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/v7/core/cache"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
)

// LiveCollection provides a clean, local key-value repository for processed artifacts.
// Set and Unset are in-memory only – they do not affect the underlying database.
//
// Artifacts are treated as read-only, shared values. Get returns the exact
// instance held by the cache — not a copy — to every caller that looks up
// that key, including concurrent callers. Callers must never mutate a value
// returned by Get; state changes must go through Set/SetWithTTL, which
// atomically replace the cache entry rather than mutating it in place.
// Mutating a returned value directly corrupts the cache for every other
// reader and is a data race if a concurrent reader exists. You have been
// warned.
type LiveCollection[T any] interface {
	// Get returns the cached artifact for key, or the zero value and false
	// if it is absent (including confirmed-absent via negative caching).
	// See the interface-level doc comment above for the read-only contract
	// on the returned value: it is shared, not copied.
	Get(key string) (T, bool)
	Set(key string, value T)
	// SetWithTTL is like Set but overrides the cache's default positive TTL
	// for this specific key. Use cache.DefaultTTL to explicitly request the
	// configured default, or cache.NoExpiration to request the entry never expire
	// on its own (it remains subject to capacity-based eviction).
	SetWithTTL(key string, value T, ttl time.Duration)
	Unset(key string)
	// NullifyWithTTL marks key as confirmed-absent from the underlying store
	// for the given duration, overriding the cache's default negative TTL.
	// This is cache-only bookkeeping; it does not touch the database.
	NullifyWithTTL(key string, ttl time.Duration)
	// TTL reports the remaining time-to-live for key. ok is false if key is
	// not cached (or has already expired). If the key is cached but has no
	// expiration, the returned duration is cache.NoExpiration.
	TTL(key string) (time.Duration, bool)
	// Persist removes any expiration from key, so it no longer expires by
	// TTL (it remains subject to capacity-based eviction). Returns false if
	// key is not currently cached.
	Persist(key string) bool
	Keys() []string
	Clone() (LiveCollection[T], error)

	// Close stops any background maintenance goroutines owned by this
	// repository's cache (janitor, watermark evictor) and releases
	// associated resources. Idempotent and safe to call from multiple
	// goroutines. Callers MUST call Close when a repository created with a
	// managed cache is no longer needed, or those goroutines will leak for
	// the lifetime of the process.
	Close() error
}

// DocumentProcessor abstracts the conversion from a persistent document to an executable asset.
type DocumentProcessor[T any] interface {
	// Compile converts a persisted document into an executable artifact.
	// Artifacts are treated as read-only once cached (see LiveCollection's
	// doc comment) — Compile should return a value safe to share across
	// concurrent readers indefinitely.
	Compile(ctx context.Context, doc *data.Document) (T, error)

	// CloneState deep-copies an artifact. It is NOT called on
	// LiveCollection.Get's hot path (Get returns the shared cached instance
	// directly). It is only used, if at all, by a cache.RepositoryCache's Clone()
	// method when producing an independent whole-cache snapshot; the
	// default cache wiring used by NewLiveRepository does not invoke it
	// there either, for the same read-only-artifact reasons. It remains
	// part of this interface for callers who construct a RepositoryCache
	// directly (e.g. via cache.NewManagedCache) and want cache.Clone() to
	// genuinely deep-copy entries.
	CloneState(state T) (T, error)
}

// ---------------------------------------------------------------------------
// LiveRepository
// ---------------------------------------------------------------------------

// LiveRepositoryOptions configures the creation of a live repository.
type LiveRepositoryOptions[T any] struct {
	// Collection is the underlying persistence collection (required).
	Collection base.Collection

	// Processor compiles documents into artifacts and clones them (required).
	Processor DocumentProcessor[T]

	// QueryKey is the document field path used as the cache key (required).
	QueryKey string

	// Cache is an optional custom cache implementation. If nil, a managed
	// cache is constructed from CacheConfig (or cache.DefaultCacheConfig if that
	// is also nil). If a Cache is shared across repositories, its lifecycle
	// must be managed by the caller; Close on the repository still
	// delegates to the cache's Close.
	Cache cache.RepositoryCache[T]

	// CacheConfig optionally tunes the managed cache created when Cache is
	// nil. Ignored when Cache is provided.
	CacheConfig *cache.CacheConfig

	// Active determines whether to preload all documents on startup. If the
	// configured MaxEntries is smaller than the collection, a warning is
	// logged and excess entries are silently evicted per the eviction
	// policy.
	Active bool
}

// liveRepository implements LiveCollection by wrapping a base.Collection
// with a cache.RepositoryCache that is kept consistent with every write operation.
type liveRepository[T any] struct {
	base.Collection
	processor DocumentProcessor[T]
	queryKey  string
	cache     cache.RepositoryCache[T]
	ctx       context.Context
}

// NewLiveRepository creates a new live repository from the provided options.
// The caller MUST call Close on the returned LiveCollection to stop
// background goroutines when a managed cache is used (the default).
func NewLiveRepository[T any](ctx context.Context, opts LiveRepositoryOptions[T]) (LiveCollection[T], error) {
	if opts.Collection == nil || opts.Processor == nil {
		return nil, fmt.Errorf("collection and processor dependencies are required")
	}
	if opts.QueryKey == "" {
		return nil, fmt.Errorf("queryKey is required")
	}

	cacheImpl := opts.Cache
	if cacheImpl == nil {
		cfg := cache.DefaultCacheConfig()
		if opts.CacheConfig != nil {
			cfg = *opts.CacheConfig
		}
		cacheImpl = cache.NewManagedCache[T](cfg, func(v T) (T, error) {
			return opts.Processor.CloneState(v)
		})
	}

	repo := &liveRepository[T]{
		Collection: opts.Collection,
		processor:  opts.Processor,
		queryKey:   opts.QueryKey,
		cache:      cacheImpl,
		ctx:        ctx,
	}

	if opts.Active {
		if err := repo.prime(ctx); err != nil {
			_ = cacheImpl.Close()
			return nil, fmt.Errorf("failed to prime live repository: %w", err)
		}
	}

	return repo, nil
}

// prime fetches all documents from the collection and populates the cache.
func (r *liveRepository[T]) prime(ctx context.Context) error {
	docs, err := r.Read(ctx, &query.Query{})
	if err != nil {
		return err
	}
	attempted := 0
	for _, doc := range docs.Data {
		if doc == nil || doc.ID() == "" {
			continue
		}
		key, err := r.extractKey(doc)
		if err != nil {
			continue
		}
		compiled, err := r.processor.Compile(ctx, doc)
		if err != nil {
			continue
		}
		r.cache.Set(key, compiled)
		attempted++
	}
	if attempted > 0 {
		if retained := len(r.cache.Keys()); retained < attempted {
			// This is best-effort; cache bound smaller than document count
		}
	}
	return nil
}

// extractKey retrieves the queryKey field value from a document as a string.
func (r *liveRepository[T]) extractKey(doc *data.Document) (string, error) {
	val, err := doc.Get(r.queryKey)
	if err != nil {
		return "", err
	}
	if str, ok := val.(string); ok {
		return str, nil
	}
	return fmt.Sprintf("%v", val), nil
}

// --- LiveCollection API ---

// Get returns the cached artifact directly, performing a read-through to the
// database on a genuine miss. A cached negative result (cache.CacheHitNegative) is
// honored and short-circuits the database call; callers cannot override this
// without explicitly calling Unset to evict the marker first.
//
// IMPORTANT — artifacts are shared, read-only values. The T returned here is
// the SAME instance held by the cache and handed to every other concurrent
// caller looking up this key; Get does NOT make a defensive copy. Callers
// MUST treat the returned value as read-only:
//
//   - Do not mutate fields on it directly. Doing so corrupts the cached
//     instance for every other reader and, if a concurrent reader is
//     present, is a data race.
//   - To change an artifact's state, construct a new value and call
//     Set/SetWithTTL, which atomically replaces the cache entry rather than
//     mutating the existing one in place.
//
// This is a deliberate trade-off: cloning on every read was previously the
// dominant source of allocation and GC pressure for hot keys checked on
// every request. If T is not safe to treat as immutable after Compile
// produces it, do not use this cache as-is — wrap Compile so it returns a
// value that genuinely is immutable (e.g. by construction, or via an
// unexported type with no exported mutating methods). You have been warned:
// mutating a value returned by Get is unsupported and will produce
// confusing, hard-to-debug behavior shared across unrelated callers.
func (r *liveRepository[T]) Get(key string) (T, bool) {
	val, status := r.cache.GetStatus(key)
	switch status {
	case cache.CacheHitPositive:
		return val, true

	case cache.CacheHitNegative:
		var zero T
		return zero, false
	}

	// CacheMiss: read-through to the database.
	q := query.NewQueryBuilder().Where(r.queryKey).Eq(key).Build()
	docs, err := r.Read(context.Background(), &q)
	if err != nil || docs == nil || docs.Count == 0 {
		r.cache.Nullify(key)
		var zero T
		return zero, false
	}

	compiled, err := r.processor.Compile(r.ctx, docs.Data[0])
	if err != nil {
		r.cache.Nullify(key)
		var zero T
		return zero, false
	}

	r.cache.Set(key, compiled)
	return compiled, true
}

// Set stores the value in the cache only – no database operation.
func (r *liveRepository[T]) Set(key string, value T) {
	r.cache.Set(key, value)
}

// SetWithTTL is like Set but overrides the default positive TTL for this key.
func (r *liveRepository[T]) SetWithTTL(key string, value T, ttl time.Duration) {
	r.cache.SetWithTTL(key, value, ttl)
}

// Unset removes the key from the cache only – no database operation.
func (r *liveRepository[T]) Unset(key string) {
	r.cache.Evict(key)
}

// NullifyWithTTL marks key as confirmed-absent for the given duration.
func (r *liveRepository[T]) NullifyWithTTL(key string, ttl time.Duration) {
	r.cache.NullifyWithTTL(key, ttl)
}

// TTL reports the remaining time-to-live for key.
func (r *liveRepository[T]) TTL(key string) (time.Duration, bool) {
	return r.cache.TTL(key)
}

// Persist removes any expiration from key.
func (r *liveRepository[T]) Persist(key string) bool {
	return r.cache.Persist(key)
}

func (r *liveRepository[T]) Keys() []string {
	return r.cache.Keys()
}

func (r *liveRepository[T]) Clone() (LiveCollection[T], error) {
	clonedCache, err := r.cache.Clone()
	if err != nil {
		return nil, err
	}
	return &liveRepository[T]{
		Collection: r.Collection,
		processor:  r.processor,
		queryKey:   r.queryKey,
		cache:      clonedCache,
		ctx:        r.ctx,
	}, nil
}

// Close stops the underlying cache's background goroutines and releases its
// resources. Does not affect the embedded base.Collection lifecycle.
func (r *liveRepository[T]) Close() error {
	return r.cache.Close()
}

// --- Intercepted database write operations ---
// Each override keeps the cache consistent with the underlying store.

func (r *liveRepository[T]) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	result, err := r.Collection.CreateOne(ctx, doc)
	if err == nil && result.Status == base.StatusCreated && result.Data != nil {
		if key, keyErr := r.extractKey(result.Data); keyErr == nil {
			if compiled, compErr := r.processor.Compile(ctx, result.Data); compErr == nil {
				r.cache.Set(key, compiled)
			}
		}
	}
	return result, err
}

func (r *liveRepository[T]) CreateMany(ctx context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	results, err := r.Collection.CreateMany(ctx, docs)
	if err == nil {
		for _, res := range results {
			if res.Status == base.StatusCreated && res.Data != nil {
				if key, keyErr := r.extractKey(res.Data); keyErr == nil {
					if compiled, compErr := r.processor.Compile(ctx, res.Data); compErr == nil {
						r.cache.Set(key, compiled)
					}
				}
			}
		}
	}
	return results, err
}

func (r *liveRepository[T]) Update(ctx context.Context, params *base.CollectionUpdate) (*base.ReadResult, error) {
	// Force ReturnDocument=true so we can refresh specific cache entries.
	updateParams := *params
	updateParams.ReturnDocument = true

	result, err := r.Collection.Update(ctx, &updateParams)
	if err != nil {
		return result, err
	}

	if result != nil && len(result.Data) > 0 {
		for _, doc := range result.Data {
			if doc == nil || doc.ID() == "" {
				continue
			}
			key, keyErr := r.extractKey(doc)
			if keyErr != nil {
				r.cache.Clear()
				break
			}
			if compiled, compErr := r.processor.Compile(ctx, doc); compErr == nil {
				r.cache.Set(key, compiled)
			} else {
				r.cache.Evict(key)
			}
		}
	} else if result != nil && result.Count > 0 {
		r.cache.Clear()
	}

	if !params.ReturnDocument && result != nil {
		result.Data = nil
	}

	return result, err
}

// Delete removes documents from the database and marks their cache keys as
// unavailable so that subsequent Gets for those keys are short-circuited
// without hitting the database until the negative TTL elapses.
func (r *liveRepository[T]) Delete(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	var keysToNullify []string
	if filter != nil {
		if readResult, readErr := r.Read(ctx, &query.Query{Filters: filter}); readErr == nil {
			for _, doc := range readResult.Data {
				if doc == nil {
					continue
				}
				if key, keyErr := r.extractKey(doc); keyErr == nil {
					keysToNullify = append(keysToNullify, key)
				}
			}
		}
	}

	count, err := r.Collection.Delete(ctx, filter, unsafe)
	if err == nil && count > 0 {
		if len(keysToNullify) > 0 {
			for _, key := range keysToNullify {
				r.cache.Nullify(key)
			}
		} else {
			r.cache.Clear()
		}
	}
	return count, err
}
