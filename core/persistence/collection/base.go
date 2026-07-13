package collection

import (
	"context"
	"sync"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/transaction"

	"github.com/asaidimu/go-anansi/v8/core/events"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Collection implements the PersistenceCollectionInterface.
type baseCollection struct {
	eventEmitter   *events.EventEmitter[base.PersistenceEvent]
	name           string
	schemaProvider base.SchemaProvider
	engine         *query.QueryEngine
	interactor     query.DatabaseInteractor
	logger         *zap.Logger
	// TODO: Subscriptions map is moved to parent persistence
	subscriptions map[string]*base.SubscriptionInfo // To store unsubscribe functions
	subMu         sync.RWMutex                      // Mutex to protect subscriptions map
	metadata      *base.CollectionMetadata
}

var _ base.Collection = (*baseCollection)(nil)

// newBaseCollection creates a new baseCollection instance, wrapping it with all necessary decorators.
func newBaseCollection(
	eventEmitter *events.EventEmitter[base.PersistenceEvent],
	name string,
	schemaProvider base.SchemaProvider,
	interactor query.DatabaseInteractor,
	engine *query.QueryEngine,
	logger *zap.Logger,
) (base.Collection, error) {
	if schemaProvider == nil {
		return nil, common.NewSystemError("ERR_PERSISTENCE_INVALID_SCHEMA", "Collection access requires a non nil schema provider")
	}

	base := &baseCollection{
		eventEmitter:   eventEmitter,
		name:           name,
		schemaProvider: schemaProvider,
		engine:         engine,
		interactor:     interactor,
		logger:         logger,
		subscriptions:  make(map[string]*base.SubscriptionInfo),
	}

	return base, nil
}

// withTransaction is a higher-order function that wraps a database operation in a transaction.
// If the interactor is already transactional, it simply executes the operation. Otherwise, it
// starts a new transaction, executes the operation, and then commits or rolls back.
func (c *baseCollection) withTransaction(
	ctx context.Context,
	operation func(interactor query.DatabaseInteractor) (any, error),
) (any, error) {

	return transaction.Execute(ctx, c.getCurrentInteractor(ctx), c.logger, func(ctx context.Context, interactor query.DatabaseInteractor) (any, error) {
		return operation(interactor)
	})
}

// CreateOne creates a single document.
func (c *baseCollection) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	results, err := c.CreateMany(ctx, []*data.Document{doc})
	result := base.CreateResult{}

	if len(results) > 0 {
		result = results[0]
	}

	if err != nil {
		return result, err
	}

	return result, nil
}

// CreateMany creates multiple documents.
func (c *baseCollection) CreateMany(ctx context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	results := make([]base.CreateResult, len(docs))

	// Insert the documents
	inserted, err := c.withTransaction(ctx, func(interactor query.DatabaseInteractor) (any, error) {
		sc, err := c.currentSchema(ctx)
		if err != nil {
			return nil, err
		}
		values := data.DocumentSet(docs).ToMaps()
		return interactor.InsertDocuments(ctx, sc, values)
	})

	if err != nil {
		return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_INSERT_DOCUMENTS_FAILED")
	}

	insertedDocs := inserted.([]map[string]any)

	for i, doc := range insertedDocs {
		results[i] = base.CreateResult{Status: base.StatusCreated, Data: data.MustNewDocument(doc, ctx)}
	}

	return results, nil
}

// Read retrieves documents from the collection that match the given QueryDSL.
func (c *baseCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	rctx := query.WithInteractor(ctx, c.getCurrentInteractor(ctx))
	sc, err := c.currentSchema(ctx)
	if err != nil {
		return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_RESOLVE_SCHEMA_FAILED")
	}
	docs, err := c.engine.Query(rctx, sc.DeepCopy(), q)
	if err != nil {
		return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_READ_DOCUMENTS_FAILED")
	}

	values, ok := data.NewDocumentSet(docs.Data, ctx)
	if !ok {
		return nil, common.NewSystemError("ERR_PERSISTENCE_READ_DOCUMENTS_FAILED", "Failed to convert results to documents")
	}
	result := base.ReadResult{
		Data:           values,
		Count:          len(values),
		Total:          docs.Total,
		PaginationInfo: docs.PaginationInfo,
	}

	return &result, nil
}

// Update modifies documents in the collection that match the filter in CollectionUpdate.
func (c *baseCollection) Update(ctx context.Context, params *base.CollectionUpdate) (*base.ReadResult, error) {
	if params == nil || params.Filter == nil {
		return nil, base.ErrInvalidUpdateParams
	}

	var updatesMap map[string]any
	if params.Set != nil {
		updatesMap = params.Set.ToMap()
	}

	result, err := c.withTransaction(ctx, func(interactor query.DatabaseInteractor) (any, error) {
		sc, err := c.currentSchema(ctx)
		if err != nil {
			return nil, err
		}
		docs, count, err := interactor.UpdateDocuments(ctx, sc, updatesMap, params.Compute, params.Filter, params.ReturnDocument)
		if err != nil {
			return nil, err
		}

		return struct {
			Docs  []map[string]any
			Count int64
		}{Docs: docs, Count: count}, nil
	})

	if err != nil {
		return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_UPDATE_DOCUMENTS_FAILED")
	}

	updateResult := result.(struct {
		Docs  []map[string]any
		Count int64
	})

	documentSet, ok := data.NewDocumentSet(updateResult.Docs, ctx)
	if !ok {
		return nil, common.NewSystemError("ERR_PERSISTENCE_UPDATE_DOCUMENTS_FAILED", "Failed to convert updated results to documents")
	}

	total := int(updateResult.Count)
	return &base.ReadResult{
		Data:  documentSet,
		Count: len(documentSet),
		Total: &total,
	}, nil
}

// Delete removes documents from the collection that match the given query filter.
// The 'unsafe' flag can be used to bypass safety checks.
func (c *baseCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, base.ErrDeleteRequiresFilter
	}

	result, err := c.withTransaction(ctx, func(interactor query.DatabaseInteractor) (any, error) {
		sc, err := c.currentSchema(ctx)
		if err != nil {
			return nil, err
		}
		return interactor.DeleteDocuments(ctx, sc, q, unsafe)
	})

	if err != nil {
		return 0, common.SystemErrorFrom(err, "ERR_PERSISTENCE_DELETE_DOCUMENTS_FAILED")
	}

	rowsAffected := result.(int64)
	return int(rowsAffected), nil
}

// Validate checks if the given data conforms to the collection's schema.
// The 'loose' flag allows for partial validation.
func (c *baseCollection) Validate(ctx context.Context, doc *data.Document, partial bool) ([]common.Issue, bool) {
	v, err := c.currentValidator(ctx)
	if err != nil {
		return []common.Issue{{Message: err.Error()}}, false
	}
	if partial {
		return v.ValidatePartial(doc.ToMap())
	}
	return v.Validate(doc.ToMap())
}

// Metadata retrieves metadata specifically for this collection, with an option to
// force a refresh of the data.
func (c *baseCollection) Metadata(ctx context.Context, filter *base.MetadataFilter, forceRefresh bool) *base.CollectionMetadata {
	sc, err := c.currentSchema(ctx)
	if err != nil {
		return &base.CollectionMetadata{Name: c.name}
	}
	clone := sc.DeepCopy()
	clone.Name = c.name
	return &base.CollectionMetadata{
		Version:    sc.Version,
		Name:       c.name,
		Collection: sc.Name,
		Schema:     clone,
	}
}

// Subscribe registers a subscription for an event that is specific to this collection.
// TODO: Subscribe should return a Subscription directly.
func (c *baseCollection) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	unsubscribe := c.eventEmitter.Subscribe(string(options.Event), options.Callback, func(_ context.Context, payload base.PersistenceEvent) bool {
		return *payload.Collection == c.name
	})

	id := uuid.New().String()

	data := base.SubscriptionInfo{
		Id:          &id,
		Event:       options.Event,
		Unsubscribe: unsubscribe,
		Label:       options.Label,
		Description: options.Description,
	}

	c.subscriptions[id] = &data
	return id
}

// Unsubscribe removes a collection-specific subscription.
func (c *baseCollection) Unsubscribe(ctx context.Context, id string) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	if info, ok := c.subscriptions[id]; ok {
		info.Unsubscribe()
		delete(c.subscriptions, id)
	}
}

// Subscriptions returns a list of all active subscriptions for this collection.
func (c *baseCollection) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	subs := make([]base.SubscriptionInfo, 0, len(c.subscriptions))
	for _, sub := range c.subscriptions {
		subs = append(subs, *sub)
	}

	return subs, nil
}

// Capabilities returns the features and limitations of the underlying database backend.
func (c *baseCollection) Capabilities(ctx context.Context) *query.Capabilities {
	capabilities := c.getCurrentInteractor(ctx).Capabilities()
	return &capabilities
}

// currentSchema resolves the active schema from the provider on-demand.
func (c *baseCollection) currentSchema(ctx context.Context) (*definition.Schema, error) {
	return c.schemaProvider.CurrentSchema(ctx)
}

// currentValidator resolves the active validator from the provider on-demand.
func (c *baseCollection) currentValidator(ctx context.Context) (*definition.DocumentValidator, error) {
	return c.schemaProvider.CurrentValidator(ctx)
}

func (c *baseCollection) getCurrentInteractor(ctx context.Context) query.DatabaseInteractor {
	if result, ok := query.GetInteractor(ctx); ok {
		return result
	}

	// Not in a transaction - use base interactor with no-op cleanup
	return c.interactor
}
