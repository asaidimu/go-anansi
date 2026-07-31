package collection

import (
	"context"
	"fmt"
	"reflect"

	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v8/core/cache"
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
)

// ============================================================================
// Model Collection Implementation
// ============================================================================

// ModelCollectionOptions configures the creation of a model collection.
type ModelCollectionOptions[P data.DocumentModelProvider] struct {
	// Cache is an optional pre-built cache for model instances. If nil, a
	// managed cache is constructed from CacheConfig. If both are nil, the
	// model collection operates without caching.
	Cache cache.RepositoryCache[P]

	// CacheConfig optionally tunes the managed cache created when Cache is
	// nil and CacheConfig is non-nil. Ignored when Cache is provided.
	CacheConfig *cache.CacheConfig

	// AutoLoad determines whether to preload all documents into the cache
	// on startup. Requires Cache or CacheConfig to be set. Useful for small
	// collections such as application settings.
	AutoLoad bool
}

// ModelCollection is a type-safe wrapper over base.Collection bound to a
// single model P — a pointer to a struct embedding data.DocumentModel. All
// standard CRUD operations are fixed to P. Documents can additionally be
// read or written through alternative shapes (projections) R that also embed
// data.DocumentModel via the generic ReadAs/CreateAs/UpdateAs/FindByIDAs
// methods, so one collection instance serves any subset of the underlying
// schema.
type ModelCollection[P data.DocumentModelProvider] struct {
	base.Collection
	collectionName string
	logger         *zap.Logger
	cache          cache.RepositoryCache[P]
}

func NewModelCollection[P data.DocumentModelProvider](raw base.Collection, logger *zap.Logger, opts ...ModelCollectionOptions[P]) (*ModelCollection[P], error) {
	metadata := raw.Metadata(context.Background(), nil, false)

	var opt ModelCollectionOptions[P]
	if len(opts) > 0 {
		opt = opts[0]
	}

	var cacheImpl cache.RepositoryCache[P]
	if opt.Cache != nil {
		cacheImpl = opt.Cache
	} else if opt.CacheConfig != nil {
		cfg := *opt.CacheConfig
		cacheImpl = cache.NewManagedCache(cfg,
			func(v P) (P, error) { return v, nil },
			func(key string, value P) {},
		)
	}

	mc := &ModelCollection[P]{
		Collection:     raw,
		collectionName: metadata.Name,
		logger:         logger,
		cache:          cacheImpl,
	}

	if opt.AutoLoad {
		if err := mc.prime(context.Background()); err != nil {
			if cacheImpl != nil {
				_ = cacheImpl.Close()
			}
			return nil, fmt.Errorf("failed to auto-load model collection: %w", err)
		}
	}

	return mc, nil
}

// newModelPtr allocates a fresh zero value of P. P is a pointer-to-struct via
// the DocumentModelProvider constraint (Model() has a pointer receiver), but
// value types are also handled for robustness.
func newModelPtr[P data.DocumentModelProvider]() P {
	typ := reflect.TypeFor[P]()
	if typ.Kind() == reflect.Pointer {
		return reflect.New(typ.Elem()).Interface().(P)
	}
	return reflect.New(typ).Elem().Interface().(P)
}

// prime preloads all documents into the cache.
func (mc *ModelCollection[P]) prime(ctx context.Context) error {
	_, err := mc.Read(ctx, &query.Query{})
	return err
}

// Close stops the underlying cache's background goroutines and releases
// resources. Idempotent. Does not affect the embedded base.Collection.
func (mc *ModelCollection[P]) Close() error {
	if mc.cache != nil {
		return mc.cache.Close()
	}
	return nil
}

// ============================================================================
// Create Operations
// ============================================================================

func (mc *ModelCollection[P]) Create(ctx context.Context, doc P) (P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := data.NewDocumentFromStruct(doc, ctx)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create").
			WithMessage("failed to convert model to document")
	}

	res, err := mc.CreateOne(ctx, d)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create")
	}

	result := newModelPtr[P]()
	if err := res.Data.BindToWithContext(ctx, result); err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create").
			WithMessage("failed to bind result document to model")
	}

	if mc.cache != nil {
		mc.cache.Set(result.Model().ID, result)
	}

	return result, nil
}

func (mc *ModelCollection[P]) CreateMany(ctx context.Context, docs []P) ([]P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	if len(docs) == 0 {
		return []P{}, nil
	}

	input := make([]*data.Document, len(docs))
	for i, doc := range docs {
		d, err := data.NewDocumentFromStruct(doc, ctx)
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.CreateMany").
				WithPath(fmt.Sprintf("docs[%d]", i)).
				WithMessagef("failed to convert model at index %d to document", i)
		}
		input[i] = d
	}

	results, err := mc.Collection.CreateMany(ctx, input)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateMany")
	}

	output := make([]P, len(results))
	for i, res := range results {
		result := newModelPtr[P]()
		if err := res.Data.BindToWithContext(ctx, result); err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.CreateMany").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind result at index %d to model", i)
		}
		if mc.cache != nil {
			mc.cache.Set(result.Model().ID, result)
		}
		output[i] = result
	}

	return output, nil
}

// ============================================================================
// Read Operations
// ============================================================================

func (mc *ModelCollection[P]) FindByID(ctx context.Context, id string) (P, error) {
	if mc.cache != nil {
		val, status := mc.cache.GetStatus(id)
		switch status {
		case cache.CacheHitPositive:
			return val, nil
		case cache.CacheHitNegative:
			return newModelPtr[P](), ErrRecordNotFound.
				WithOperation("ModelCollection.FindByID").
				WithPath(id).
				WithMessagef("record with id '%s' not found", id)
		}
	}

	q := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Limit(1).
		Build()

	results, err := mc.Read(ctx, &q)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.FindByID").
			WithPath(id)
	}

	if len(results) == 0 {
		if mc.cache != nil {
			mc.cache.Nullify(id)
		}
		return newModelPtr[P](), ErrRecordNotFound.
			WithOperation("ModelCollection.FindByID").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	return results[0], nil
}

func (mc *ModelCollection[P]) Read(ctx context.Context, q *query.Query) ([]P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	res, err := mc.Collection.Read(ctx, q)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Read")
	}

	if len(res.Data) == 0 {
		return []P{}, nil
	}

	output := make([]P, len(res.Data))
	for i, doc := range res.Data {
		result := newModelPtr[P]()
		if err := doc.BindToWithContext(ctx, result); err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.Read").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind document at index %d to model", i)
		}
		output[i] = result
	}

	if mc.cache != nil {
		for _, m := range output {
			if id := m.Model().ID; id != "" {
				mc.cache.Set(id, m)
			}
		}
	}

	return output, nil
}

// ============================================================================
// Update Operations
// ============================================================================

func (mc *ModelCollection[P]) Update(ctx context.Context, id string, update P) (P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := data.NewPartialDocumentFromStruct(update, ctx)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessage("failed to convert update model to partial document")
	}

	filter := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Build().Filters

	result, err := mc.Collection.Update(ctx, &base.CollectionUpdate{
		Filter:         filter,
		Set:            d,
		ReturnDocument: true,
	})
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Update").
			WithPath(id)
	}

	if result.Count == 0 || len(result.Data) == 0 {
		return newModelPtr[P](), ErrRecordNotFound.
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	updated := newModelPtr[P]()
	if err := result.Data[0].BindToWithContext(ctx, updated); err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessage("failed to bind updated document to model")
	}

	if mc.cache != nil {
		mc.cache.Set(id, updated)
	}

	return updated, nil
}

func (mc *ModelCollection[P]) UpdateMany(ctx context.Context, f *query.QueryFilter, update P) (int, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	d, err := data.NewPartialDocumentFromStruct(update, ctx)
	if err != nil {
		return 0, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateMany").
			WithMessage("failed to convert update model to partial document")
	}

	result, err := mc.Collection.Update(ctx, &base.CollectionUpdate{
		Filter: f,
		Set:    d,
	})
	if err != nil {
		return 0, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateMany")
	}

	if result.Total == nil {
		return 0, nil
	}
	if mc.cache != nil {
		mc.cache.Clear()
	}
	return *result.Total, nil
}

func (mc *ModelCollection[P]) Replace(ctx context.Context, id string, replacement P) (P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := data.NewDocumentFromStruct(replacement, ctx)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Replace").
			WithPath(id).
			WithMessage("failed to convert replacement model to document")
	}

	filter := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Build().Filters

	result, err := mc.Collection.Update(ctx, &base.CollectionUpdate{
		Filter:         filter,
		Set:            d,
		ReturnDocument: true,
	})
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Replace").
			WithPath(id)
	}

	if result.Count == 0 || len(result.Data) == 0 {
		return newModelPtr[P](), ErrRecordNotFound.
			WithOperation("ModelCollection.Replace").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	replaced := newModelPtr[P]()
	if err := result.Data[0].BindToWithContext(ctx, replaced); err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Replace").
			WithPath(id).
			WithMessage("failed to bind replaced document to model")
	}

	if mc.cache != nil {
		mc.cache.Set(id, replaced)
	}

	return replaced, nil
}

// ============================================================================
// Delete Operations
// ============================================================================

func (mc *ModelCollection[P]) DeleteByID(ctx context.Context, id string) error {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	filter := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Build().Filters

	count, err := mc.Delete(ctx, filter, false)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.DeleteByID").
			WithPath(id)
	}

	if count == 0 {
		return ErrRecordNotFound.
			WithOperation("ModelCollection.DeleteByID").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	if mc.cache != nil {
		mc.cache.Evict(id)
	}

	return nil
}

func (mc *ModelCollection[P]) DeleteMany(ctx context.Context, f *query.QueryFilter, unsafe bool) (int, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	count, err := mc.Delete(ctx, f, unsafe)
	if err != nil {
		return 0, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.DeleteMany")
	}
	if mc.cache != nil {
		mc.cache.Clear()
	}
	return count, nil
}

// ============================================================================
// Validation Operations
// ============================================================================

func (mc *ModelCollection[P]) Validate(ctx context.Context, doc P, loose bool) error {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	d, err := data.NewDocumentFromStruct(doc, ctx)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Validate").
			WithMessage("failed to convert model to document for validation")
	}

	if issues, ok := mc.Collection.Validate(ctx, d, loose); !ok {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Validate").WithIssues(issues)
	}

	return nil
}

func (mc *ModelCollection[P]) ValidatePartial(ctx context.Context, doc P) error {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	d, err := data.NewPartialDocumentFromStruct(doc, ctx)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.ValidatePartial").
			WithMessage("failed to convert partial model to document for validation")
	}

	if issues, ok := mc.Collection.Validate(ctx, d, true); !ok {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.ValidatePartial").WithIssues(issues)
	}

	return nil
}

// ============================================================================
// Subscription Operations
// ============================================================================

func (mc *ModelCollection[P]) Subscribe(ctx context.Context, opt base.SubscriptionOptions) string {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	return mc.Collection.Subscribe(ctx, opt)
}

func (mc *ModelCollection[P]) Unsubscribe(ctx context.Context, id string) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	mc.Collection.Unsubscribe(ctx, id)
}

// ============================================================================
// Shape Operations (Projections)
// ============================================================================
//
// The collection is fixed to a single model P, but documents can be read or
// written through alternative shapes R — projections that embed
// data.DocumentModel and thus satisfy data.DocumentModelProvider. Shape
// operations bind the same underlying documents into R, so one collection
// instance serves any subset of the schema. Shape results are not stored in
// the model-typed cache.

// ReadAs reads documents matching q and binds each into a fresh R.
func (mc *ModelCollection[P]) ReadAs[R data.DocumentModelProvider](ctx context.Context, q *query.Query) ([]R, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	res, err := mc.Collection.Read(ctx, q)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.ReadAs")
	}

	if len(res.Data) == 0 {
		return []R{}, nil
	}

	output := make([]R, len(res.Data))
	for i, doc := range res.Data {
		result := newModelPtr[R]()
		if err := doc.BindToWithContext(ctx, result); err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.ReadAs").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind document at index %d to shape", i)
		}
		output[i] = result
	}
	return output, nil
}

// FindByIDAs reads a single document by id and binds it into R.
func (mc *ModelCollection[P]) FindByIDAs[R data.DocumentModelProvider](ctx context.Context, id string) (R, error) {
	var zero R

	q := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Limit(1).
		Build()

	results, err := mc.ReadAs[R](ctx, &q)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.FindByIDAs").
			WithPath(id)
	}

	if len(results) == 0 {
		return zero, ErrRecordNotFound.
			WithOperation("ModelCollection.FindByIDAs").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	return results[0], nil
}

// CreateAs persists a new document built from shape R and returns the
// hydrated shape.
func (mc *ModelCollection[P]) CreateAs[R data.DocumentModelProvider](ctx context.Context, doc R) (R, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := data.NewDocumentFromStruct(doc, ctx)
	if err != nil {
		return newModelPtr[R](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateAs").
			WithMessage("failed to convert shape to document")
	}

	res, err := mc.CreateOne(ctx, d)
	if err != nil {
		return newModelPtr[R](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateAs")
	}

	result := newModelPtr[R]()
	if err := res.Data.BindToWithContext(ctx, result); err != nil {
		return newModelPtr[R](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateAs").
			WithMessage("failed to bind result document to shape")
	}

	return result, nil
}

// UpdateAs applies a partial update built from shape R (system fields are
// ignored) and returns the updated document bound into R.
func (mc *ModelCollection[P]) UpdateAs[R data.DocumentModelProvider](ctx context.Context, id string, update R) (R, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := data.NewPartialDocumentFromStruct(update, ctx)
	if err != nil {
		return newModelPtr[R](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateAs").
			WithPath(id).
			WithMessage("failed to convert update shape to partial document")
	}

	filter := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Build().Filters

	result, err := mc.Collection.Update(ctx, &base.CollectionUpdate{
		Filter:         filter,
		Set:            d,
		ReturnDocument: true,
	})
	if err != nil {
		return newModelPtr[R](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateAs").
			WithPath(id)
	}

	if result.Count == 0 || len(result.Data) == 0 {
		return newModelPtr[R](), ErrRecordNotFound.
			WithOperation("ModelCollection.UpdateAs").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	updated := newModelPtr[R]()
	if err := result.Data[0].BindToWithContext(ctx, updated); err != nil {
		return newModelPtr[R](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateAs").
			WithPath(id).
			WithMessage("failed to bind updated document to shape")
	}

	return updated, nil
}
