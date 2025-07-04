// Package persistence provides the core implementation of the PersistenceCollectionInterface,
// defining the behavior of a single collection in the database.
package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
)

// CollectionBase provides the fundamental implementation of the PersistenceCollectionInterface.
// It encapsulates the logic for data manipulation (CRUD), validation, and event handling
// for a specific collection, governed by a schema.
// This struct is not meant to be used directly but rather to be embedded in other structs
// that might add more specialized functionality, such as event emitting.
type CollectionBase struct {
	name            string
	schema          *schema.SchemaDefinition
	processor       *query.DataProcessor
	executor        *Executor
	validator       *schema.Validator
	bus             *events.TypedEventBus[PersistenceEvent]
	subscriptions   map[string]*SubscriptionInfo // To store unsubscribe functions
	subMu           sync.RWMutex                 // Mutex to protect subscriptions map
	fmap            schema.FunctionMap           // Map of custom functions for validation and processing
}

// NewCollection creates a new instance of a collection that implements the
// PersistenceCollectionInterface. It wraps the base collection logic with event-emitting
// capabilities, ensuring that operations on the collection are observable.
func NewCollection(bus *events.TypedEventBus[PersistenceEvent], name string, sc *schema.SchemaDefinition, executor *Executor, fmap schema.FunctionMap) (PersistenceCollectionInterface, error) {
	validator := schema.NewValidator(sc, fmap)

	collection := NewEventEmittingCollection(&CollectionBase{
		schema:        sc,
		executor:      executor,
		validator:     validator,
		bus:           bus,
		subscriptions: make(map[string]*SubscriptionInfo),
		fmap:          fmap,
	})

	return collection, nil
}

// Create adds one or more new documents to the collection. Before insertion, it validates
// each document against the collection's schema to ensure data integrity.
func (c *CollectionBase) Create(data any) (any, error) {
	var records []map[string]any
	switch v := data.(type) {
	case map[string]any:
		records = []map[string]any{v}
	case []map[string]any:
		records = v
	default:
		return nil, fmt.Errorf("invalid data type for Create: expected map[string]any or []map[string]any, got %T", data)
	}

	for _, record := range records {
		validation, err := c.Validate(record, false)
		if err != nil {
			return nil, fmt.Errorf("an error occurred when trying to validate an entry: %w", err)
		}

		if !validation.Valid {
			return nil, fmt.Errorf("provided data does not conform to the collection's schema,  \n %v", validation)
		}
	}

	result, err := c.executor.Insert(context.Background(), c.schema, records)
	if err != nil {
		return nil, fmt.Errorf("failed to insert data into collection '%s': %w", c.schema.Name, err)
	}

	return result, nil
}

// Read retrieves documents from the collection based on a QueryDSL query.
func (c *CollectionBase) Read(q *query.QueryDSL) (*query.QueryResult, error) {
	result, err := c.executor.Query(context.Background(), c.schema, q)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from collection '%s': %w", c.schema.Name, err)
	}

	return result, nil
}

// Update modifies documents in the collection that match the provided filter.
func (c *CollectionBase) Update(params *CollectionUpdate) (int, error) {
	result, err := c.executor.Update(context.Background(), c.schema, params.Data, params.Filter)
	if err != nil {
		return 0, fmt.Errorf("failed to update data in collection '%s': %w", c.schema.Name, err)
	}

	return int(result), nil
}

// Delete removes documents from the collection that match the given query filter.
func (c *CollectionBase) Delete(filter *query.QueryFilter, unsafe bool) (int, error) {
	ctx := context.Background()
	affected, err := c.executor.Delete(ctx, c.schema, filter, unsafe)
	if err != nil {
		return 0, fmt.Errorf("failed to delete data from collection '%s': %w", c.schema.Name, err)
	}

	return int(affected), nil
}

// Validate checks if the given data conforms to the collection's schema. The 'loose'
// parameter allows for partial validation, where not all fields are required.
func (c *CollectionBase) Validate(data any, loose bool) (*schema.ValidationResult, error) {
	values, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("failed to convert data to a map for validation")
	}

	valid, issues := c.validator.Validate(values, loose)
	return &schema.ValidationResult{
		Valid:  valid,
		Issues: issues,
	}, nil
}

// Metadata retrieves metadata specifically for this collection, with an option to
// force a refresh of the data.
// NOTE: This method is not yet implemented.
func (c *CollectionBase) Metadata(
	filter *MetadataFilter,
	forceRefresh bool,
) (Metadata, error) {
	// TODO: Implement collection metadata retrieval.
	return Metadata{}, fmt.Errorf("collection metadata method not implemented for '%s'", c.schema.Name)
}

// RegisterSubscription registers a subscription for an event that is specific to this collection.
// It filters events from the main event bus, ensuring that the callback is only invoked
// for events relevant to this collection.
func (c *CollectionBase) RegisterSubscription(options RegisterSubscriptionOptions) string {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	unsubscribe := c.bus.Subscribe(string(options.Event),
		func(ctx context.Context, payload PersistenceEvent) error {
			if payload.Collection == nil || *payload.Collection != c.schema.Name {
				return nil // Not for this collection
			}
			return options.Callback(ctx, payload)
		})

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

// UnregisterSubscription removes a collection-specific subscription by its ID.
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
