package collection

import (
	"context"
	"fmt"
	"reflect"

	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v8/core/cache"
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// ============================================================================
// Model Collection Implementation
// ============================================================================

// ModelIdentity is the minimal contract ModelCollection requires of its model
// type P and shape types R/S: it must expose the document ID used as the cache
// key. Both data.DocumentModel and document.DocumentModel embeds promote GetID
// to the embedding struct, so models built on either pipeline satisfy it.
type ModelIdentity interface {
	GetID() string
}

// ModelConverter converts a model (or projection shape) instance into the
// Documenter that will be persisted. Both the collection's model P and shape
// R satisfy ModelIdentity, so a single converter serves every operation. It
// receives the model as any so the shape-typed methods (CreateFrom/UpdateFrom)
// can reuse the same hook.
type ModelConverter func(ctx context.Context, model any) (data.Documenter, error)

// ModelCollectionOptions configures the creation of a model collection.
type ModelCollectionOptions[P ModelIdentity] struct {
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

	// ToDocumenter converts a model instance into the Documenter that is
	// persisted. Defaults to the data-factory pipeline
	// (data.NewDocumentFromStruct). Set it to produce a different document
	// implementation — e.g. a container-backed document.Document built from
	// the embedded document.DocumentPool — or to inject custom
	// identity/metadata.
	ToDocumenter ModelConverter

	// ToPartialDocumenter converts a model instance into the partial
	// Documenter used for updates. Defaults to
	// data.NewPartialDocumentFromStruct.
	ToPartialDocumenter ModelConverter
}

// ModelCollection is a type-safe wrapper over base.Collection bound to a
// single model P — a pointer to a struct embedding a document model
// (data.DocumentModel or document.DocumentModel). All standard CRUD operations
// are fixed to P. Documents can additionally be read or written through
// alternative shapes (projections) R that also satisfy ModelIdentity via the
// generic ReadAs/CreateFrom/UpdateFrom methods, so one collection instance
// serves any subset of the underlying schema.
//
// ModelCollection embeds base.Collection — so it remains usable wherever a
// base.Collection is expected — alongside *document.DocumentPool, the
// schema-bound container-backed document factory owned by the wrapped
// collection (the base collection's pool is reused; a fresh pool is compiled
// from the active schema only when the wrapped collection exposes none).
// Promoted methods (FromStruct, FromPartialStruct, FromMap, New, ...) construct
// container-backed document.Documents that share the pool, and the default
// ToDocumenter/ToPartialDocumenter hooks are the pool's
// FromStruct/FromPartialStruct.
type ModelCollection[P ModelIdentity] struct {
	base.Collection
	*document.DocumentPool
	collectionName      string
	logger              *zap.Logger
	cache               cache.RepositoryCache[P]
	toDocumenter        ModelConverter
	toPartialDocumenter ModelConverter
}

func NewModelCollection[P ModelIdentity](raw base.Collection, logger *zap.Logger, opts ...ModelCollectionOptions[P]) (*ModelCollection[P], error) {
	// The container-backed document factory is mandatory: reuse the wrapped
	// collection's pool when it exposes one (the base collection owns it),
	// otherwise resolve the active schema and build the pool eagerly.
	dc, err := documentPoolFor(context.Background(), raw)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.NewModelCollection").
			WithMessage("failed to resolve container-backed document pool")
	}

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

	toDocumenter := opt.ToDocumenter
	if toDocumenter == nil {
		toDocumenter = func(ctx context.Context, model any) (data.Documenter, error) {
			return dc.FromStruct(model, document.WithContext(ctx))
		}
	}
	toPartialDocumenter := opt.ToPartialDocumenter
	if toPartialDocumenter == nil {
		toPartialDocumenter = func(ctx context.Context, model any) (data.Documenter, error) {
			return dc.FromPartialStruct(model, document.WithContext(ctx))
		}
	}

	mc := &ModelCollection[P]{
		Collection:          raw,
		DocumentPool:        dc,
		collectionName:      metadata.Name,
		logger:              logger,
		cache:               cacheImpl,
		toDocumenter:        toDocumenter,
		toPartialDocumenter: toPartialDocumenter,
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
// the ModelIdentity constraint (GetID has a pointer receiver on the embedded
// document model), but value types are also handled for robustness.
func newModelPtr[P ModelIdentity]() P {
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

	d, err := mc.toDocumenter(ctx, doc)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create").
			WithMessage("failed to convert model to document")
	}

	res, err := mc.Collection.CreateOne(ctx, d)
	d.Release() // converted doc is consumed by persistence; return pooled containers
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create")
	}

	result := newModelPtr[P]()
	bindErr := res.Data.BindToWithContext(ctx, result)
	res.Data.Release() // read-back containers are consumed by binding; return them to the pool
	if bindErr != nil {
		return newModelPtr[P](), common.SystemErrorFrom(bindErr).
			WithOperation("ModelCollection.Create").
			WithMessage("failed to bind result document to model")
	}

	if mc.cache != nil {
		mc.cache.Set(result.GetID(), result)
	}

	return result, nil
}

func (mc *ModelCollection[P]) CreateMany(ctx context.Context, docs []P) ([]P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	if len(docs) == 0 {
		return []P{}, nil
	}

	input := make([]data.Documenter, len(docs))
	for i, doc := range docs {
		d, err := mc.toDocumenter(ctx, doc)
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.CreateMany").
				WithPath(fmt.Sprintf("docs[%d]", i)).
				WithMessagef("failed to convert model at index %d to document", i)
		}
		input[i] = d
	}

	results, err := mc.Collection.CreateMany(ctx, input)
	for _, d := range input {
		d.Release() // converted docs are consumed by persistence; return pooled containers
	}
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateMany")
	}

	output := make([]P, len(results))
	for i, res := range results {
		result := newModelPtr[P]()
		bindErr := res.Data.BindToWithContext(ctx, result)
		res.Data.Release() // read-back containers are consumed by binding; return them to the pool
		if bindErr != nil {
			return nil, common.SystemErrorFrom(bindErr).
				WithOperation("ModelCollection.CreateMany").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind result at index %d to model", i)
		}
		if mc.cache != nil {
			mc.cache.Set(result.GetID(), result)
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
		bindErr := doc.BindToWithContext(ctx, result)
		doc.Release() // read-back containers are consumed by binding; return them to the pool
		if bindErr != nil {
			return nil, common.SystemErrorFrom(bindErr).
				WithOperation("ModelCollection.Read").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind document at index %d to model", i)
		}
		output[i] = result
	}

	if mc.cache != nil {
		for _, m := range output {
			if id := m.GetID(); id != "" {
				mc.cache.Set(id, m)
			}
		}
	}

	return output, nil
}

// ============================================================================
// Update Operations
// ============================================================================

// mergeUpdateOptions overlays caller-provided update options onto the
// method-generated CollectionUpdate. Set from the options is ignored (the
// shape-derived document always wins); a caller-supplied Filter overrides the
// id filter; Compute maps are merged; and Version is passed through.
// ReturnDocument is merged only when explicitly set (non-nil), so the
// method-generated default (true for the binding operations) is preserved
// unless the caller opts out.
func mergeUpdateOptions(cu base.CollectionUpdate, opts ...base.CollectionUpdate) base.CollectionUpdate {
	for _, opt := range opts {
		if opt.Filter != nil {
			cu.Filter = opt.Filter
		}
		if len(opt.Compute) > 0 {
			if cu.Compute == nil {
				cu.Compute = make(map[string]query.Query, len(opt.Compute))
			}
			for k, v := range opt.Compute {
				cu.Compute[k] = v
			}
		}
		if opt.Version != nil {
			cu.Version = opt.Version
		}
		if opt.ReturnDocument != nil {
			cu.ReturnDocument = opt.ReturnDocument
		}
	}
	return cu
}

func (mc *ModelCollection[P]) Update(ctx context.Context, id string, update P, opts ...base.CollectionUpdate) (P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := mc.toPartialDocumenter(ctx, update)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessage("failed to convert update model to partial document")
	}

	filter := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Build().Filters

	cu := mergeUpdateOptions(base.CollectionUpdate{
		Filter:         filter,
		Set:            d,
		ReturnDocument: utils.BoolPtr(true),
	}, opts...)

	result, err := mc.Collection.Update(ctx, &cu)
	d.Release() // converted partial doc is consumed by persistence; return pooled containers

	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Update").
			WithPath(id)
	}

	if !cu.ReturnsDocument() {
		if mc.cache != nil {
			mc.cache.Evict(id)
		}
		var zero P
		return zero, nil
	}

	if result.Count == 0 || len(result.Data) == 0 {
		return newModelPtr[P](), ErrRecordNotFound.
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	updated := newModelPtr[P]()
	bindErr := result.Data[0].BindToWithContext(ctx, updated)
	result.Data[0].Release() // read-back containers are consumed by binding; return them to the pool
	if bindErr != nil {
		return newModelPtr[P](), common.SystemErrorFrom(bindErr).
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessage("failed to bind updated document to model")
	}

	if mc.cache != nil {
		mc.cache.Set(id, updated)
	}

	return updated, nil
}

func (mc *ModelCollection[P]) UpdateMany(ctx context.Context, f *query.QueryFilter, update P, opts ...base.CollectionUpdate) (int, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	d, err := mc.toPartialDocumenter(ctx, update)
	if err != nil {
		return 0, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateMany").
			WithMessage("failed to convert update model to partial document")
	}

	cu := mergeUpdateOptions(base.CollectionUpdate{
		Filter: f,
		Set:    d,
	}, opts...)

	result, err := mc.Collection.Update(ctx, &cu)
	d.Release() // converted partial doc is consumed by persistence; return pooled containers
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

func (mc *ModelCollection[P]) Replace(ctx context.Context, id string, replacement P, opts ...base.CollectionUpdate) (P, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := mc.toDocumenter(ctx, replacement)
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Replace").
			WithPath(id).
			WithMessage("failed to convert replacement model to document")
	}

	filter := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Build().Filters

	cu := mergeUpdateOptions(base.CollectionUpdate{
		Filter:         filter,
		Set:            d,
		ReturnDocument: utils.BoolPtr(true),
	}, opts...)

	result, err := mc.Collection.Update(ctx, &cu)
	d.Release() // converted doc is consumed by persistence; return pooled containers
	if err != nil {
		return newModelPtr[P](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Replace").
			WithPath(id)
	}

	if !cu.ReturnsDocument() {
		if mc.cache != nil {
			mc.cache.Evict(id)
		}
		var zero P
		return zero, nil
	}

	if result.Count == 0 || len(result.Data) == 0 {
		return newModelPtr[P](), ErrRecordNotFound.
			WithOperation("ModelCollection.Replace").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	replaced := newModelPtr[P]()
	bindErr := result.Data[0].BindToWithContext(ctx, replaced)
	result.Data[0].Release() // read-back containers are consumed by binding; return them to the pool
	if bindErr != nil {
		return newModelPtr[P](), common.SystemErrorFrom(bindErr).
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

	count, err := mc.Collection.Delete(ctx, filter, false)
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
	count, err := mc.Collection.Delete(ctx, f, unsafe)
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
	d, err := mc.toDocumenter(ctx, doc)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Validate").
			WithMessage("failed to convert model to document for validation")
	}

	if issues, ok := mc.Collection.Validate(ctx, d, loose); !ok {
		d.Release()
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Validate").WithIssues(issues)
	}

	d.Release()
	return nil
}

func (mc *ModelCollection[P]) ValidatePartial(ctx context.Context, doc P) error {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)
	d, err := mc.toPartialDocumenter(ctx, doc)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.ValidatePartial").
			WithMessage("failed to convert partial model to document for validation")
	}

	if issues, ok := mc.Collection.Validate(ctx, d, true); !ok {
		d.Release()
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.ValidatePartial").WithIssues(issues)
	}

	d.Release()
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
// written through alternative shapes R — projections that embed a document
// model and thus satisfy ModelIdentity. Shape
// operations bind the same underlying documents into R, so one collection
// instance serves any subset of the schema. Shape results are not stored in
// the model-typed cache.

// ReadAs reads documents matching q and binds each into a fresh R.
func (mc *ModelCollection[P]) ReadAs[R ModelIdentity](ctx context.Context, q *query.Query) ([]R, error) {
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
		bindErr := doc.BindToWithContext(ctx, result)
		doc.Release() // read-back containers are consumed by binding; return them to the pool
		if bindErr != nil {
			return nil, common.SystemErrorFrom(bindErr).
				WithOperation("ModelCollection.ReadAs").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind document at index %d to shape", i)
		}
		output[i] = result
	}
	return output, nil
}

// CreateFrom persists a new document built from shape R and returns the
// hydrated shape.
func (mc *ModelCollection[P]) CreateFrom[R ModelIdentity, S ModelIdentity](ctx context.Context, doc R) (S, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := mc.toDocumenter(ctx, doc)
	if err != nil {
		return newModelPtr[S](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateFrom").
			WithMessage("failed to convert shape to document")
	}

	res, err := mc.Collection.CreateOne(ctx, d)
	d.Release() // converted doc is consumed by persistence; return pooled containers
	if err != nil {
		return newModelPtr[S](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateFrom")
	}

	result := newModelPtr[S]()
	bindErr := res.Data.BindToWithContext(ctx, result)
	res.Data.Release() // read-back containers are consumed by binding; return them to the pool
	if bindErr != nil {
		return newModelPtr[S](), common.SystemErrorFrom(bindErr).
			WithOperation("ModelCollection.CreateFrom").
			WithMessage("failed to bind result document to shape")
	}

	return result, nil
}

// UpdateFrom applies a partial update built from shape R (system fields are
// ignored) and returns the updated document bound into R. Additional update
// options may be supplied, e.g. Compute expressions for atomic server-side
// increments; see mergeUpdateOptions for how they overlay the built-in
// id filter and shape-derived set.
func (mc *ModelCollection[P]) UpdateFrom[R ModelIdentity, S ModelIdentity](
	ctx context.Context,
	id string,
	update R,
	opts ...base.CollectionUpdate,
) (S, error) {
	ctx = common.ContextWithCollectionName(ctx, mc.collectionName)

	d, err := mc.toPartialDocumenter(ctx, update)
	if err != nil {
		return newModelPtr[S](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateFrom").
			WithPath(id).
			WithMessage("failed to convert update shape to partial document")
	}

	filter := query.NewQueryBuilder().
		Where(data.DocumentIDField).Eq(id).
		Build().Filters

	cu := mergeUpdateOptions(base.CollectionUpdate{
		Filter:         filter,
		Set:            d,
		ReturnDocument: utils.BoolPtr(true),
	}, opts...)

	result, err := mc.Collection.Update(ctx, &cu)
	d.Release() // converted partial doc is consumed by persistence; return pooled containers

	if err != nil {
		return newModelPtr[S](), common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateFrom").
			WithPath(id)
	}

	if !cu.ReturnsDocument() {
		var zero S
		return zero, nil
	}

	if result.Count == 0 || len(result.Data) == 0 {
		return newModelPtr[S](), ErrRecordNotFound.
			WithOperation("ModelCollection.UpdateFrom").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	updated := newModelPtr[S]()
	bindErr := result.Data[0].BindToWithContext(ctx, updated)
	result.Data[0].Release() // read-back containers are consumed by binding; return them to the pool
	if bindErr != nil {
		return newModelPtr[S](), common.SystemErrorFrom(bindErr).
			WithOperation("ModelCollection.UpdateFrom").
			WithPath(id).
			WithMessage("failed to bind updated document to shape")
	}

	return updated, nil
}
