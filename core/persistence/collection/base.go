package collection

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v6/core/schema/validator"

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
	validator     *validator.DocumentValidator
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
	if sc == nil {
		return nil, schema.ErrInvalidSchema.WithMessage("Collection access requires a non nil schema")
	}

	validator, err := validator.NewDocumentValidator(sc, nil)
	if err != nil {
		return nil, err
	}

	description := ""
	if sc.Description != nil {
		description = *sc.Description
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
			ID:             name,
			SchemaVersion:  sc.Version,
			Name:           name,
			CollectionName: sc.Name,
			Description:    description,
			Status:         "active",
			CreatedAt:      fmt.Sprintf("%d", 0), // get this from the registry
			CreatedBy:      "system",             // this field is being used wrong
			RecordCount:    0,                    // For this and the next two below, we should have methods in the interactor to expose these
			DataSizeBytes:  0,                    // Not directly available from interactor yet
			LastModified:   0,                    // Placehold
			Schema:         sc,
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
		values := data.DocumentSet(docs).ToMaps()
		return interactor.InsertDocuments(ctx, c.schema, values)
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
	docs, err := c.engine.Query(rctx, c.schema.MustClone(), q)
	if err != nil {
		return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_READ_DOCUMENTS_FAILED")
	}

	values, ok := data.NewDocumentSet(docs.Data, ctx)
	if !ok {
		return nil, common.NewSystemError("ERR_PERSISTENCE_READ_DOCUMENTS_FAILED", "Failed to convert results to documents")
	}
	result := base.ReadResult{
		Data:  values,
		Count: docs.Count,
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
		var affectedIDs []query.FilterValue
		var docs []map[string]any
		var count int64

		// If documents are requested and interactor doesn't support native RETURNING,
		// fetch IDs before the update so we can retrieve them afterward
		needsIDsFetch := params.ReturnDocument && !c.Capabilities(ctx).ReturnOnUpdate

		if needsIDsFetch {
			rctx := query.WithInteractor(ctx, interactor)
			idQuery := query.NewQueryBuilder().
				Select().
				Include(data.DocumentIDField).
				End().
				AndFilter(*params.Filter).
				Build()

			idQuery.Target = &query.QueryTarget{
				Name:   c.schema.Name,
				Alias:  &c.name,
				Schema: c.schema.MustClone(),
			}

			idDocs, queryErr := c.engine.Query(rctx, c.schema, &idQuery)
			if queryErr != nil {
				return nil, common.SystemErrorFrom(queryErr, "ERR_PERSISTENCE_FETCH_IDS_FAILED")
			}

			// Extract IDs from the documents
			affectedIDs = make([]query.FilterValue, 0, idDocs.Count)
			for _, doc := range idDocs.Data {
				if id, exists := doc[data.DocumentIDField]; exists {
					ids := id.(string)
					affectedIDs = append(affectedIDs, query.FilterValue{
						StringVal: &ids,
					})
				}
			}
		}

		// Perform the update
		docs, count, err := interactor.UpdateDocuments(ctx, c.schema, updatesMap, params.Compute, params.Filter, params.ReturnDocument)
		if err != nil {
			return nil, err
		}

		// If we fetched IDs beforehand and got documents back from update, fetch the updated documents
		if needsIDsFetch && len(affectedIDs) > 0 {
			rctx := query.WithInteractor(ctx, interactor)
			fetchQuery := query.NewQueryBuilder().
				AndFilter(query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    data.DocumentIDField,
						Operator: query.ComparisonOperatorIn,
						Value:    query.FilterValue{ArrayVal: affectedIDs},
					},
				}).
				Build()

			fetchQuery.Target = &query.QueryTarget{
				Name:   c.schema.Name,
				Alias:  &c.name,
				Schema: c.schema.MustClone(),
			}

			result, err := c.engine.Query(rctx, c.schema, &fetchQuery)
			if err != nil {
				// Return empty docs with count - the update succeeded
				docs = []map[string]any{}
			}
			docs = result.Data
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

	return &base.ReadResult{
		Data:  documentSet,
		Count: int(updateResult.Count),
	}, nil
}

// Delete removes documents from the collection that match the given query filter.
// The 'unsafe' flag can be used to bypass safety checks.
func (c *baseCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, base.ErrDeleteRequiresFilter
	}

	result, err := c.withTransaction(ctx, func(interactor query.DatabaseInteractor) (any, error) {
		return interactor.DeleteDocuments(ctx, c.schema, q, unsafe)
	})

	if err != nil {
		return 0, common.SystemErrorFrom(err, "ERR_PERSISTENCE_DELETE_DOCUMENTS_FAILED")
	}

	rowsAffected := result.(int64)
	return int(rowsAffected), nil
}

// Validate checks if the given data conforms to the collection's schema.
// The 'loose' flag allows for partial validation.
func (c *baseCollection) Validate(ctx context.Context, data *data.Document, loose bool) (*schema.ValidationResult, error) {
	issues, ok := c.validator.Validate(data.ToMap(), loose)
	return &schema.ValidationResult{
		Valid:  ok,
		Issues: issues,
	}, nil
}

// Metadata retrieves metadata specifically for this collection, with an option to
// force a refresh of the data.
func (c *baseCollection) Metadata(ctx context.Context, filter *base.MetadataFilter, forceRefresh bool) *base.CollectionMetadata {
	// TODO improve this method
	metadata := *c.metadata
	schema := *metadata.Schema
	schema.Name = metadata.Name
	metadata.Schema = &schema
	return &metadata
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
