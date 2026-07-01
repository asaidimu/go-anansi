package utils

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
)

// LiveCollection provides a clean, local key-value repository for processed artifacts.
// Set and Unset are in‑memory only – they do not affect the underlying database.
type LiveCollection[T any] interface {
	Get(key string) (T, bool)
	Set(key string, value T)
	Unset(key string)
	Keys() []string
	Clone() (LiveCollection[T], error)
}

// DocumentProcessor abstracts the conversion from a persistent document to an executable asset.
type DocumentProcessor[T any] interface {
	Compile(ctx context.Context, doc *data.Document) (T, error)
	CloneState(state T) (T, error)
}

// RepositoryCache provides an abstraction for caching compiled artifacts.
// It supports positive caching, negative caching (unavailable markers), and eviction.
type RepositoryCache[T any] interface {
	// Get returns the cached value and true if it is present and positive.
	// Returns false if the key is missing or marked as unavailable.
	Get(key string) (T, bool)

	// Set stores a positive value for the key and clears any unavailable marker.
	Set(key string, value T)

	// Nullify stores a negative marker for the key to avoid future database lookups.
	Nullify(key string)

	// Evict removes any entry (positive or negative) for the key.
	Evict(key string)

	// Keys returns all keys that have positive entries.
	Keys() []string

	// Clear removes all entries from the cache.
	Clear()

	// Clone returns a deep copy of the cache.
	Clone() (RepositoryCache[T], error)
}

// cacheEntry holds either a positive artifact or a negative marker.
type cacheEntry[T any] struct {
	artifact     T
	notAvailable bool // true means negative cache
}

// inMemoryCache is a thread-safe implementation of RepositoryCache.
type inMemoryCache[T any] struct {
	mu    sync.RWMutex
	data  map[string]*cacheEntry[T]
	clone func(T) (T, error) // used for deep cloning during Clone()
}

// NewInMemoryCache creates a new in-memory cache with a clone function.
// The clone function is used to deep-copy artifacts when cloning the cache.
func NewInMemoryCache[T any](cloneFunc func(T) (T, error)) RepositoryCache[T] {
	return &inMemoryCache[T]{
		data:  make(map[string]*cacheEntry[T]),
		clone: cloneFunc,
	}
}

func (c *inMemoryCache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.data[key]
	if !ok || entry.notAvailable {
		var zero T
		return zero, false
	}
	return entry.artifact, true
}

func (c *inMemoryCache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = &cacheEntry[T]{artifact: value}
}

func (c *inMemoryCache[T]) Nullify(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = &cacheEntry[T]{notAvailable: true}
}

func (c *inMemoryCache[T]) Evict(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

func (c *inMemoryCache[T]) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.data))
	for k, entry := range c.data {
		if !entry.notAvailable {
			keys = append(keys, k)
		}
	}
	return keys
}

func (c *inMemoryCache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]*cacheEntry[T])
}

func (c *inMemoryCache[T]) Clone() (RepositoryCache[T], error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	clonedData := make(map[string]*cacheEntry[T], len(c.data))
	for k, entry := range c.data {
		if entry.notAvailable {
			clonedData[k] = &cacheEntry[T]{notAvailable: true}
			continue
		}
		// Use the clone function if provided, otherwise just shallow copy.
		if c.clone != nil {
			clonedArtifact, err := c.clone(entry.artifact)
			if err != nil {
				// If cloning fails, fall back to a shallow copy (or return error).
				clonedArtifact = entry.artifact
			}
			clonedData[k] = &cacheEntry[T]{artifact: clonedArtifact}
		} else {
			clonedData[k] = &cacheEntry[T]{artifact: entry.artifact}
		}
	}
	return &inMemoryCache[T]{
		data:  clonedData,
		clone: c.clone,
	}, nil
}

// LiveRepositoryOptions configures the creation of a live repository.
type LiveRepositoryOptions[T any] struct {
	// Collection is the underlying persistence collection (required).
	Collection base.Collection

	// Processor compiles documents into artifacts and clones them (required).
	Processor DocumentProcessor[T]

	// QueryKey is the document field path used as the cache key (required).
	QueryKey string

	// Cache is an optional custom cache implementation. If nil, a default in-memory cache is used.
	Cache RepositoryCache[T]

	// Active determines whether to preload all documents on startup.
	Active bool
}

// liveRepository implements LiveCollection by wrapping a Collection and maintaining a cache.
type liveRepository[T any] struct {
	base.Collection
	processor DocumentProcessor[T]
	queryKey  string
	cache     RepositoryCache[T]
	ctx       context.Context
}

// NewLiveRepository creates a new live repository from the provided options.
func NewLiveRepository[T any](ctx context.Context, opts LiveRepositoryOptions[T]) (LiveCollection[T], error) {
	if opts.Collection == nil || opts.Processor == nil {
		return nil, fmt.Errorf("collection and processor dependencies are required")
	}
	if opts.QueryKey == "" {
		return nil, fmt.Errorf("queryKey is required")
	}

	// Use provided cache or create a default one.
	cache := opts.Cache
	if cache == nil {
		// The clone function uses the processor's CloneState.
		cache = NewInMemoryCache[T](func(v T) (T, error) {
			return opts.Processor.CloneState(v)
		})
	}

	repo := &liveRepository[T]{
		Collection: opts.Collection,
		processor:  opts.Processor,
		queryKey:   opts.QueryKey,
		cache:      cache,
		ctx:        ctx,
	}

	if opts.Active {
		if err := repo.prime(ctx); err != nil {
			return nil, fmt.Errorf("failed to prime live repository: %w", err)
		}
	}

	return repo, nil
}

// prime loads all documents and populates the cache.
func (r *liveRepository[T]) prime(ctx context.Context) error {
	docs, err := r.Read(ctx, &query.Query{})
	if err != nil {
		return err
	}

	for _, doc := range docs.Data {
		if doc == nil || doc.ID() == "" {
			continue
		}
		key, err := r.extractKey(doc)
		if err != nil {
			continue // skip documents that don't have the queryKey field
		}
		compiled, err := r.processor.Compile(ctx, doc)
		if err != nil {
			continue
		}
		r.cache.Set(key, compiled)
	}
	return nil
}

// extractKey retrieves the value of the queryKey field from a document and returns it as a string.
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

// --- LiveCollection API (cache only) ---

func (r *liveRepository[T]) Get(key string) (T, bool) {
	// Check cache first.
	if val, ok := r.cache.Get(key); ok {
		// Clone the artifact before returning.
		cloned, err := r.processor.CloneState(val)
		if err != nil {
			// If cloning fails, return the cached value anyway (best effort).
			return val, true
		}
		return cloned, true
	}

	// Read-through: query the database using the queryKey field.
	q := query.NewQueryBuilder().Where(r.queryKey).Eq(key).Build()
	docs, err := r.Read(context.Background(), &q)
	if err != nil || docs == nil || docs.Count == 0 {
		// Mark as unavailable to avoid repeated DB calls.
		r.cache.Nullify(key)
		var zero T
		return zero, false
	}

	doc := docs.Data[0]
	compiled, err := r.processor.Compile(r.ctx, doc)
	if err != nil {
		r.cache.Nullify(key)
		var zero T
		return zero, false
	}

	r.cache.Set(key, compiled)
	cloned, err := r.processor.CloneState(compiled)
	if err != nil {
		return compiled, true
	}
	return cloned, true
}

// Set stores the value in the cache only – no database operation.
// It also clears any negative-cache marker.
func (r *liveRepository[T]) Set(key string, value T) {
	r.cache.Set(key, value)
}

// Unset removes the key from the cache only – no database operation.
func (r *liveRepository[T]) Unset(key string) {
	r.cache.Evict(key)
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

// --- Intercepted database write operations ---
// These override the embedded base.Collection methods to keep the cache consistent.

func (r *liveRepository[T]) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	result, err := r.Collection.CreateOne(ctx, doc)
	if err == nil && result.Status == base.StatusCreated && result.Data != nil {
		key, keyErr := r.extractKey(result.Data)
		if keyErr == nil {
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
				key, keyErr := r.extractKey(res.Data)
				if keyErr == nil {
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
	// We need to force ReturnDocument = true to get updated documents for cache refresh.
	updateParams := *params
	updateParams.ReturnDocument = true

	result, err := r.Collection.Update(ctx, &updateParams)
	if err != nil {
		return result, err
	}

	// If we got updated documents, refresh the cache.
	if result != nil && len(result.Data) > 0 {
		for _, doc := range result.Data {
			if doc == nil || doc.ID() == "" {
				continue
			}
			key, keyErr := r.extractKey(doc)
			if keyErr != nil {
				// Cannot determine the key – clear whole cache to be safe.
				r.cache.Clear()
				break
			}
			if compiled, compErr := r.processor.Compile(ctx, doc); compErr == nil {
				r.cache.Set(key, compiled)
			} else {
				// Compilation failed – remove entry to force future read-through.
				r.cache.Evict(key)
			}
		}
	} else if result != nil && result.Count > 0 {
		// Update succeeded but no documents returned – we don't know which keys changed.
		// Invalidate the entire cache.
		r.cache.Clear()
	}

	// Honor the caller's ReturnDocument preference: if they wanted no documents, discard them.
	if !params.ReturnDocument && result != nil {
		result.Data = nil
	}

	return result, err
}

// Delete removes documents matching the filter from the database and marks their cache keys as unavailable.
func (r *liveRepository[T]) Delete(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	// Fetch the queryKey values of documents to be deleted so we can mark them as unavailable.
	keysToEvict := []string{}
	if filter != nil {
		readQuery := &query.Query{
			Filters: filter,
		}
		readResult, readErr := r.Read(ctx, readQuery)
		if readErr == nil {
			for _, doc := range readResult.Data {
				if doc == nil {
					continue
				}
				key, keyErr := r.extractKey(doc)
				if keyErr == nil {
					keysToEvict = append(keysToEvict, key)
				}
			}
		}
	}

	count, err := r.Collection.Delete(ctx, filter, unsafe)
	if err == nil && count > 0 {
		if len(keysToEvict) > 0 {
			for _, key := range keysToEvict {
				// Mark as unavailable to avoid future DB lookups.
				r.cache.Nullify(key)
			}
		} else {
			// No keys obtained – clear the whole cache to be safe.
			r.cache.Clear()
		}
	}
	return count, err
}
