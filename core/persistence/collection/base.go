package collection

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"

	"github.com/asaidimu/go-anansi/v6/core/events"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Collection implements the PersistenceCollectionInterface.
type baseCollection struct {
	eventEmitter  *events.EventEmitter[base.PersistenceEvent]
	name          string
	schema        *schema.SchemaDefinition
	engine        *query.QueryEngine
	interactor    query.DatabaseInteractor
	logger        *zap.Logger
	subscriptions map[string]*base.SubscriptionInfo // To store unsubscribe functions
	subMu         sync.RWMutex                      // Mutex to protect subscriptions map
	validator     *schema.DocumentValidator
	metadata      *base.CollectionMetadata
}

var _ base.Collection = (*baseCollection)(nil)

// newBaseCollection creates a new baseCollection instance, wrapping it with all necessary decorators.
func newBaseCollection(
	eventEmitter *events.EventEmitter[base.PersistenceEvent],
	name string,
	sc *schema.SchemaDefinition,
	interactor query.DatabaseInteractor,
	engine *query.QueryEngine,
	logger *zap.Logger,
) (base.Collection, error) {
	if sc == nil || sc.Validate() != nil {
		return nil, base.NewPersistenceError("Collection access requires a valid schema", base.ErrInvalidSchema)
	}

	validator, err := schema.NewDocumentValidator(sc, nil)
	if err != nil {
		return nil, err
	}

	base := &baseCollection{
		eventEmitter:  eventEmitter,
		name:          name,
		schema:        sc,
		engine:        engine,
		interactor:    interactor,
		logger:        logger,
		subscriptions: make(map[string]*base.SubscriptionInfo),
		validator:     validator,
		metadata: &base.CollectionMetadata{
			ID:             name, // Using collection name as ID for simplicity
			SchemaVersion:  sc.Version,
			Name:           name,
			CollectionName: name, // Physical name is now sc.Name
			Description:    sc.Description,
			Status:         "active",
			CreatedAt:      fmt.Sprintf("%d", 0), // Placeholder, ideally from creation time
			CreatedBy:      "system",
			RecordCount:    0, // Not directly available from interactor yet
			DataSizeBytes:  0, // Not directly available from interactor yet
			Schema:         sc,
			LastModified:   0,                         // Placeholder
			Subscriptions:  []base.SubscriptionInfo{}, // Collection-specific subscriptions not managed here yet
		},
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
func (c *baseCollection) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	results, err := c.CreateMany(ctx, []data.Document{doc})
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
func (c *baseCollection) CreateMany(ctx context.Context, docs []data.Document) ([]base.CreateResult, error) {
	results := make([]base.CreateResult, len(docs))

	// Insert the documents
	inserted, err := c.withTransaction(ctx, func(interactor query.DatabaseInteractor) (any, error) {
		return interactor.InsertDocuments(ctx, c.schema, docs)
	})

	if err != nil {
		for i, doc := range docs {
			results[i] = base.CreateResult{Status: base.StatusFailedPersistence, Data: doc, Error: err.Error()}
		}
		return results, base.NewPersistenceError(base.ErrInsertDocuments.Error(), err)
	}

	insertedDocs := inserted.([]data.Document)

	for i, doc := range insertedDocs {
		doc.MustVerifyHash()
		results[i] = base.CreateResult{Status: base.StatusCreated, Data: doc}
	}

	return results, nil
}

// Read retrieves documents from the collection that match the given QueryDSL.
func (c *baseCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	rctx := query.WithInteractor(ctx, c.getCurrentInteractor(ctx))
	docs, err := c.engine.Query(rctx, c.schema, q)
	if err != nil {
		return nil, base.NewPersistenceError(fmt.Sprintf("%s: %v", base.ErrReadDocuments.Error(), err), base.ErrReadDocuments)
	}

	count := len(docs)
	result := base.ReadResult{
		Data:  docs,
		Count: count,
	}

	if count == 0 {
		result.Data = nil
	}

	if count == 1 {
		result.Data = docs[0]
	}

	return &result, nil
}

// Update modifies documents in the collection that match the filter in CollectionUpdate.
func (c *baseCollection) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	if params == nil || params.Filter == nil {
		return 0, base.NewPersistenceError(base.ErrInvalidUpdateParams.Error(), base.ErrInvalidUpdateParams)
	}

	result, err := c.withTransaction(ctx, func(interactor query.DatabaseInteractor) (any, error) {
		return interactor.UpdateDocuments(ctx, c.schema, params.Data, params.Filter)
	})

	if err != nil {
		return 0, base.NewPersistenceError(fmt.Sprintf("%s: %v", base.ErrUpdateDocuments.Error(), err), base.ErrUpdateDocuments)
	}

	rowsAffected := result.(int64)
	return int(rowsAffected), nil
}

// Delete removes documents from the collection that match the given query filter.
// The 'unsafe' flag can be used to bypass safety checks.
func (c *baseCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, base.NewPersistenceError(base.ErrDeleteRequiresFilter.Error(), base.ErrDeleteRequiresFilter)
	}

	result, err := c.withTransaction(ctx, func(interactor query.DatabaseInteractor) (any, error) {
		return interactor.DeleteDocuments(ctx, c.schema, q, unsafe)
	})

	if err != nil {
		return 0, base.NewPersistenceError(fmt.Sprintf("%s: %v", base.ErrDeleteDocuments.Error(), err), base.ErrDeleteDocuments)
	}

	rowsAffected := result.(int64)
	return int(rowsAffected), nil
}

// Validate checks if the given data conforms to the collection's schema.
// The 'loose' flag allows for partial validation.
func (c *baseCollection) Validate(ctx context.Context, data data.Document, loose bool) (*schema.ValidationResult, error) {
	issues, ok := c.validator.Validate(data, loose)
	return &schema.ValidationResult{
		Valid:  ok,
		Issues: issues,
	}, nil
}

// Metadata retrieves metadata specifically for this collection, with an option to
// force a refresh of the data.
func (c *baseCollection) Metadata(ctx context.Context, filter *base.MetadataFilter, forceRefresh bool) (*base.CollectionMetadata, error) {
	metadata := *c.metadata
	return &metadata, nil
}

// Subscribe registers a subscription for an event that is specific to this collection.
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

func (c *baseCollection) getCurrentInteractor(ctx context.Context) query.DatabaseInteractor {
	if result, ok := query.GetInteractor(ctx); ok {
		return result
	}

	// Not in a transaction - use base interactor with no-op cleanup
	return c.interactor
}
