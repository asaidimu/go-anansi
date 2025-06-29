// Package persistence provides the core implementation of the PersistenceInterface,
// offering a concrete way to interact with the underlying database.
package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v2/core/query"
	"github.com/asaidimu/go-anansi/v2/core/schema"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Persistence is the main implementation of the PersistenceInterface. It orchestrates
// interactions with the database through a DatabaseInteractor, manages schema definitions,
// and handles event subscriptions for observability.
type Persistence struct {
	interactor    DatabaseInteractor
	collection    PersistenceCollectionInterface
	schema        *schema.SchemaDefinition
	executor      *Executor
	fmap          schema.FunctionMap
	logger        *zap.Logger
	subscriptions map[string]*SubscriptionInfo // To store unsubscribe functions
	subMu         sync.RWMutex                 // Mutex to protect subscriptions map
	bus           *events.TypedEventBus[PersistenceEvent]
}

// NewPersistence creates a new instance of the Persistence service. It initializes the
// event bus, ensures that the internal schema for managing collections exists, and sets
// up the necessary components for the persistence layer to function.
func NewPersistence(interactor DatabaseInteractor, fmap schema.FunctionMap) (PersistenceInterface, error) {
	bus, err := events.NewTypedEventBus[PersistenceEvent](events.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("could not initialize event bus: %w", err)
	}

	var s schema.SchemaDefinition
	if err := json.Unmarshal(schemasCollectionSchema, &s); err != nil {
		return nil, fmt.Errorf("error unmarshaling schemas collection schema: %w", err)
	}

	exists, err := interactor.CollectionExists(s.Name)
	if err != nil {
		return nil, fmt.Errorf("error looking up schema collection: %w", err)
	}

	if !exists {
		if err := interactor.CreateCollection(s); err != nil {
			return nil, fmt.Errorf("failed to create table for collections %s: %w", s.Name, err)
		}
	}

	executor := NewExecutor(interactor, nil)
	collection, err := NewCollection(bus, &s, executor, fmap)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize schemas collection: %w", err)
	}

	return &Persistence{
		interactor:    interactor,
		executor:      executor,
		fmap:          fmap,
		collection:    collection,
		schema:        &s,
		bus:           bus,
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*SubscriptionInfo),
	}, nil
}

// Collection returns a PersistenceCollectionInterface for a given collection name.
// This allows for performing operations like Create, Read, Update, and Delete on that
// specific collection.
func (p *Persistence) Collection(name string) (PersistenceCollectionInterface, error) {
	s, err := p.Schema(name)
	if err != nil {
		return nil, err
	}

	collection, err := NewCollection(p.bus, s, p.executor, p.fmap)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

// Transact executes a callback function within a database transaction. If the callback
// returns an error, the transaction is rolled back; otherwise, it is committed.
func (p *Persistence) Transact(callback func(tx PersistenceTransactionInterface) (any, error)) (any, error) {
	tx, err := p.interactor.StartTransaction(context.Background())
	if err != nil {
		return nil, err
	}

	transactionCtx, err := NewPersistence(tx, p.fmap)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}

	result, err := callback(transactionCtx)
	if err != nil {
		tx.Rollback(context.Background())
		return result, err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return result, err
	}

	return result, nil
}

// Collections returns a list of all collection names currently managed by the persistence layer.
func (p *Persistence) Collections() ([]string, error) {
	q := query.NewQueryBuilder().Build()
	result, err := p.collection.Read(&q)
	if err != nil {
		return nil, fmt.Errorf("error reading schemas to get collection names: %w", err)
	}

	var collectionNames []string
	if result.Data != nil {
		if docs, ok := result.Data.([]schema.Document); ok {
			for _, doc := range docs {
				record, err := mapToSchemaRecord(doc)
				if err != nil {
					p.logger.Warn("Failed to convert map to SchemaRecord while listing collections", zap.Error(err))
					continue
				}
				collectionNames = append(collectionNames, record.Name)
			}
		} else if doc, ok := result.Data.(schema.Document); ok { // Handle case where Read returns a single document
			record, err := mapToSchemaRecord(doc)
			if err != nil {
				return nil, fmt.Errorf("error converting map to SchemaRecord for single collection: %w", err)
			}
			collectionNames = append(collectionNames, record.Name)
		}
	}
	return collectionNames, nil
}

// Create creates a new collection based on the provided schema definition. It ensures
// that a collection with the same name does not already exist before creating it.
func (p *Persistence) Create(s schema.SchemaDefinition) (PersistenceCollectionInterface, error) {
	exists, err := p.interactor.CollectionExists(s.Name)
	if err != nil {
		return nil, fmt.Errorf("error accessing database: %w", err)
	}

	if exists {
		return nil, fmt.Errorf("a collection with a similar name exists")
	}

	if err := p.interactor.CreateCollection(s); err != nil {
		return nil, fmt.Errorf("failed to create collection %s: %w", s.Name, err)
	}

	schemaJSON, err := schemaToRawJSON(&s)
	if err != nil {
		return nil, err
	}

	record := SchemaRecord{
		Name:        s.Name,
		Description: *s.Description,
		Version:     s.Version,
		Schema:      schemaJSON,
	}

	data, err := schemaRecordToMap(&record)
	if err != nil {
		return nil, err
	}
	_, err = p.collection.Create(data)
	if err != nil {
		return nil, err
	}

	collection, err := NewCollection(p.bus, &s, p.executor, p.fmap)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

// schemaCollection returns a PersistenceCollectionInterface for the internal schemas collection.
// It can be configured to run within a transaction.
func (p *Persistence) schemaCollection(tx DatabaseInteractor) (PersistenceCollectionInterface, error) {
	var executor *Executor
	if tx != nil {
		executor = NewExecutor(tx, nil)
	} else {
		executor = NewExecutor(p.interactor, nil)
	}
	collection, err := NewCollection(p.bus, p.schema, executor, p.fmap)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize schemas collection: %w", err)
	}
	return collection, nil
}

// Delete removes a collection by its name. This operation is transactional, ensuring that
// both the collection and its schema definition are removed atomically.
func (p *Persistence) Delete(name string) (bool, error) {
	tx, err := p.interactor.StartTransaction(context.Background())
	if err != nil {
		return false, err
	}

	collection, err := p.schemaCollection(tx)
	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	q := query.NewQueryBuilder().Where("name").Eq(name).Build()
	_, err = collection.Delete(q.Filters, false)
	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	if err := tx.DropCollection(name); err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	if err := tx.Commit(context.Background()); err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	return true, nil
}

// Schema retrieves the schema definition for a given collection name.
func (p *Persistence) Schema(name string) (*schema.SchemaDefinition, error) {
	exists, err := p.interactor.CollectionExists(name)
	if err != nil {
		return nil, fmt.Errorf("error accessing database: %w, exists: %t", err, exists)
	}

	if !exists {
		return nil, fmt.Errorf("collection %s does not exist", name)
	}

	q := query.NewQueryBuilder().Where("name").Eq(name).Build()

	result, err := p.collection.Read(&q)
	if err != nil {
		return nil, fmt.Errorf("error reading schema collection: %w", err)
	}

	if result.Count != 1 {
		return nil, fmt.Errorf("unexpected count for schema name: %d", result.Count)
	}

	record, err := mapToSchemaRecord(result.Data.(schema.Document))
	if err != nil {
		return nil, fmt.Errorf("error converting map to SchemaRecord: %w", err)
	}

	var s schema.SchemaDefinition
	if err := json.Unmarshal(record.Schema, &s); err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON: %w", err)
	}
	return &s, nil
}

// RegisterSubscription registers a callback for a specific persistence event. It returns
// a unique ID that can be used to unregister the subscription later.
func (p *Persistence) RegisterSubscription(options RegisterSubscriptionOptions) string {
	p.subMu.Lock()
	defer p.subMu.Unlock()

	unsubscribe := p.bus.Subscribe(string(options.Event), options.Callback)
	id := uuid.New().String()

	data := SubscriptionInfo{
		Id:          &id,
		Event:       options.Event,
		Unsubscribe: unsubscribe,
		Label:       options.Label,
		Description: options.Description,
	}

	p.subscriptions[id] = &data
	return id
}

// UnregisterSubscription removes a subscription by its ID.
func (p *Persistence) UnregisterSubscription(id string) {
	p.subMu.Lock()
	defer p.subMu.Unlock()

	if info, ok := p.subscriptions[id]; ok {
		info.Unsubscribe()
		delete(p.subscriptions, id)
	}
}

// Subscriptions returns a list of all currently active subscriptions.
func (p *Persistence) Subscriptions() ([]SubscriptionInfo, error) {
	p.subMu.RLock()
	defer p.subMu.RUnlock()

	subs := make([]SubscriptionInfo, 0, len(p.subscriptions))
	for _, sub := range p.subscriptions {
		subs = append(subs, *sub)
	}

	return subs, nil
}

// Metadata retrieves metadata about the persistence layer, optionally filtered by the
// provided criteria. This can include information about collections, schemas, and subscriptions.
func (p *Persistence) Metadata(filter *MetadataFilter) (Metadata, error) {
	// TODO: Implement metadata retrieval logic based on the filter.
	return Metadata{}, nil
}

