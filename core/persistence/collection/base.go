package collection

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Collection implements the PersistenceCollectionInterface.
type baseCollection struct {
	bus           *events.TypedEventBus[base.PersistenceEvent]
	name          string
	schema        *schema.SchemaDefinition
	engine        *query.QueryEngine
	interactor    query.BaseDatabaseInteractor
	logger        *zap.Logger
	subscriptions map[string]*base.SubscriptionInfo // To store unsubscribe functions
	subMu         sync.RWMutex                      // Mutex to protect subscriptions map
	validator     *schema.DocumentValidator
	metadata      *base.CollectionMetadata
}

var _ base.Collection = (*baseCollection)(nil)

// newBaseCollection creates a new baseCollection instance, wrapping it with all necessary decorators.
func newBaseCollection(
	bus *events.TypedEventBus[base.PersistenceEvent],
	name string,
	sc *schema.SchemaDefinition,
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
		bus:           bus,
		name:          name,
		schema:        sc,
		engine:        engine,
		interactor:    engine.Interactor,
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
	operation func(interactor query.BaseDatabaseInteractor) (any, error),
) (any, error) {

	if !c.interactor.HasTransaction(ctx) {
		interactor := c.interactor.(query.DatabaseInteractor)
		tx, err := interactor.StartTransaction(ctx)
		if err != nil {
			return nil, base.NewPersistenceError("failed to start transaction", err)
		}

		result, err := operation(tx)
		if err != nil {
			tx.Rollback(ctx)
			return nil, err
		}

		if err := tx.Commit(ctx); err != nil {
			tx.Rollback(ctx)
			return nil, base.NewPersistenceError("failed to commit transaction", err)
		}

		return result, nil
	}

	return operation(c.interactor)
}

// CreateOne creates a single document.
func (c *baseCollection) CreateOne(ctx context.Context, doc data.Document) (*base.CreateResult, error) {
	results, err := c.CreateMany(ctx, []data.Document{doc})
	if err != nil {
		return nil, err
	}
	return &results[0], nil
}

// CreateMany creates multiple documents.
func (c *baseCollection) CreateMany(ctx context.Context, docs []data.Document) ([]base.CreateResult, error) {
	results := make([]base.CreateResult, len(docs))

	// Insert the documents
	inserted, err := c.withTransaction(ctx, func(interactor query.BaseDatabaseInteractor) (any, error) {
		return interactor.InsertDocuments(ctx, c.schema, docs)
	})

	if err != nil {
		for i, doc := range docs {
			results[i] = base.CreateResult{Status: base.StatusFailedPersistence, Data: doc, Error: err.Error()}
		}
		return results, base.NewPersistenceError("Failed to insert documents", err)
	}

	insertedDocs := inserted.([]data.Document)

	for i, doc := range insertedDocs {
		results[i] = base.CreateResult{Status: base.StatusCreated, Data: doc}
	}

	return results, nil
}

// Read retrieves documents from the collection that match the given QueryDSL.
func (c *baseCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	docs, err := c.engine.Query(ctx, c.schema, q)
	if err != nil {
		return nil, base.NewPersistenceError(fmt.Sprintf("failed to read documents: %v", err), base.ErrReadDocuments)
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
		return 0, base.NewPersistenceError("update operation requires filter parameters", base.ErrInvalidUpdateParams)
	}

	result, err := c.withTransaction(ctx, func(interactor query.BaseDatabaseInteractor) (any, error) {
		return interactor.UpdateDocuments(ctx, c.schema, params.Data, params.Filter)
	})

	if err != nil {
		return 0, base.NewPersistenceError(fmt.Sprintf("failed to update documents: %v", err), base.ErrUpdateDocuments)
	}

	rowsAffected := result.(int64)
	return int(rowsAffected), nil
}

// Delete removes documents from the collection that match the given query filter.
// The 'unsafe' flag can be used to bypass safety checks.
func (c *baseCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, base.NewPersistenceError("delete operation requires a filter or the unsafe flag set to true", base.ErrDeleteRequiresFilter)
	}

	result, err := c.withTransaction(ctx, func(interactor query.BaseDatabaseInteractor) (any, error) {
		return interactor.DeleteDocuments(ctx, c.schema, q, unsafe)
	})

	if err != nil {
		return 0, base.NewPersistenceError(fmt.Sprintf("failed to delete documents: %v", err), base.ErrDeleteDocuments)
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

// RegisterSubscription registers a subscription for an event that is specific to this collection.
func (c *baseCollection) RegisterSubscription(ctx context.Context, options base.RegisterSubscriptionOptions) string {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	unsubscribe := c.bus.SubscribeWithOptions(string(options.Event), options.Callback,
		events.SubscribeOptions{
			Filter: func(event events.Event) bool {
				payload, ok := event.Payload.(base.PersistenceEvent)
				if !ok {
					return false
				}

				return *payload.Collection == c.name
			},
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

// UnregisterSubscription removes a collection-specific subscription.
func (c *baseCollection) UnregisterSubscription(ctx context.Context, id string) {
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
	capabilities := c.interactor.Capabilities()
	return &capabilities
}
