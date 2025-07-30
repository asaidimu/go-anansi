package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Collection implements the PersistenceCollectionInterface.
type CollectionBase struct {
	bus           *events.TypedEventBus[PersistenceEvent]
	name          string
	schema        *schema.SchemaDefinition
	engine        *query.QueryEngine
	interactor    query.DatabaseInteractor
	logger        *zap.Logger
	subscriptions map[string]*SubscriptionInfo // To store unsubscribe functions
	subMu         sync.RWMutex                 // Mutex to protect subscriptions map
	validator     *schema.DocumentValidator
}

var _ PersistenceCollectionInterface = (*CollectionBase)(nil)

// NewCollection creates a new Collection instance.
func NewBaseCollection(
	bus *events.TypedEventBus[PersistenceEvent],
	name string,
	sc *schema.SchemaDefinition,
	engine *query.QueryEngine,
	logger *zap.Logger,
) (*Collection, error) {

	validator, err := schema.NewDocumentValidator(sc, nil)

	if err != nil {
		return nil, err
	}

	base := CollectionBase{
		bus:           bus,
		name:          name,
		schema:        sc,
		engine:        engine,
		interactor:    engine.Interactor,
		logger:        logger,
		subscriptions: make(map[string]*SubscriptionInfo),
		validator:     validator,
	}

	return NewEventEmittingCollection(&base), nil
}

// Create adds one or more new documents to the collection.
func (c *CollectionBase) Create(data any) (any, error) {
	var docs []schema.Document

	switch v := data.(type) {
	case schema.Document:
		docs = []schema.Document{v}
	case []schema.Document:
		docs = v
	case []any:
		docs = make([]schema.Document, len(v))
		for i, item := range v {
			if doc, ok := item.(schema.Document); ok {
				docs[i] = doc
			} else {
				return nil, fmt.Errorf("invalid data type for document at index %d: %T", i, item)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported data type for Create: %T", data)
	}

	insertedDocs, err := c.interactor.InsertDocuments(context.Background(), c.schema, docs)
	if err != nil {
		return nil, NewPersistenceError(fmt.Sprintf("failed to insert documents: %v", err), ErrInsertDocuments)
	}

	if len(insertedDocs) == 1 {
		return &CreateResult{ID: insertedDocs[0]["id"].(string), Data: insertedDocs[0]}, nil
	} else if len(insertedDocs) > 1 {
		results := make([]CreateResult, len(insertedDocs))
		for i, doc := range insertedDocs {
			results[i] = CreateResult{ID: doc["id"].(string), Data: doc}
		}
		return results, nil
	}

	return nil, nil
}

// Read retrieves documents from the collection that match the given QueryDSL.
func (c *CollectionBase) Read(q *query.Query) (*query.QueryResult, error) {
	docs, err := c.engine.Query(context.Background(), c.schema, q)
	if err != nil {
		return nil, NewPersistenceError(fmt.Sprintf("failed to read documents: %v", err), ErrReadDocuments)
	}

	return &query.QueryResult{
		Data:  docs,
		Count: len(docs),
	}, nil
}

// Update modifies documents in the collection that match the filter in CollectionUpdate.
func (c *CollectionBase) Update(params *CollectionUpdate) (int, error) {
	if params == nil || params.Filter == nil {
		return 0, NewPersistenceError("update operation requires filter parameters", ErrInvalidUpdateParams)
	}

	rowsAffected, err := c.interactor.UpdateDocuments(context.Background(), c.schema, params.Data, params.Filter)
	if err != nil {
		return 0, NewPersistenceError(fmt.Sprintf("failed to update documents: %v", err), ErrUpdateDocuments)
	}

	return int(rowsAffected), nil
}

// Delete removes documents from the collection that match the given query filter.
// The 'unsafe' flag can be used to bypass safety checks.
func (c *CollectionBase) Delete(q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, NewPersistenceError("delete operation requires a filter or the unsafe flag set to true", ErrDeleteRequiresFilter)
	}

	rowsAffected, err := c.interactor.DeleteDocuments(context.Background(), c.schema, q, unsafe)
	if err != nil {
		return 0, NewPersistenceError(fmt.Sprintf("failed to delete documents: %v", err), ErrDeleteDocuments)
	}

	return int(rowsAffected), nil
}

// Validate checks if the given data conforms to the collection's schema.
// The 'loose' flag allows for partial validation.
func (c *CollectionBase) Validate(data any, loose bool) (*schema.ValidationResult, error) {
	doc, ok := data.(schema.Document)
	if !ok {
		return nil, NewPersistenceError(fmt.Sprintf("invalid data type for validation: %T, expected schema.Document", data), ErrInvalidDataType)
	}
	issues, ok := c.validator.Validate(doc, loose)
	return &schema.ValidationResult{
		Valid:  ok,
		Issues: issues,
	}, nil
}

// Metadata retrieves metadata specifically for this collection, with an option to
// force a refresh of the data.
func (c *CollectionBase) Metadata(filter *MetadataFilter, forceRefresh bool) (Metadata, error) {
	// For now, ignoring filter and forceRefresh as per test requirements.

	// Populate CollectionMetadata for this collection
	collectionMeta := CollectionMetadata{
		ID:             c.name, // Using collection name as ID for simplicity
		SchemaVersion:  c.schema.Version,
		Name:           c.name,
		CollectionName: c.name, // Physical name is same as logical name for now
		Description:    *c.schema.Description,
		Status:         "active",
		CreatedAt:      fmt.Sprintf("%d", 0), // Placeholder, ideally from creation time
		CreatedBy:      "system",
		RecordCount:    0, // Not directly available from interactor yet
		DataSizeBytes:  0, // Not directly available from interactor yet
		Schema:         *c.schema,
		LastModified:   0,                    // Placeholder
		Subscriptions:  []SubscriptionInfo{}, // Collection-specific subscriptions not managed here yet
	}

	// Construct the overall Metadata struct
	metadata := Metadata{
		Collections: []CollectionMetadata{collectionMeta},
		// Other fields can be populated as global metadata is implemented
	}

	return metadata, nil
}

// RegisterSubscription registers a subscription for an event that is specific to this collection.
func (c *CollectionBase) RegisterSubscription(options RegisterSubscriptionOptions) string {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	unsubscribe := c.bus.Subscribe(string(options.Event), options.Callback)
	id := uuid.New().String()

	data := SubscriptionInfo{
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
func (c *CollectionBase) UnregisterSubscription(id string) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	if info, ok := c.subscriptions[id]; ok {
		info.Unsubscribe()
		delete(c.subscriptions, id)
	}
}

// Subscriptions returns a list of all active subscriptions for this collection.
func (c *CollectionBase) Subscriptions() ([]SubscriptionInfo, error) {
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	subs := make([]SubscriptionInfo, 0, len(c.subscriptions))
	for _, sub := range c.subscriptions {
		subs = append(subs, *sub)
	}

	return subs, nil
}

// Capabilities returns the features and limitations of the underlying database backend.
func (c *CollectionBase) Capabilities() *query.Capabilities {
	capabilities := c.interactor.Capabilities()
	return &capabilities
}
