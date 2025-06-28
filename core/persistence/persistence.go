package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Persistence implements the core.Persistence interface.
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

// NewPersistence creates a new instance of Persistence.
func NewPersistence(interactor DatabaseInteractor, fmap schema.FunctionMap) (PersistenceInterface, error) {
	bus, err := events.NewTypedEventBus[PersistenceEvent](events.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("Could not initialize event bus %v", err)
	}
	var schema schema.SchemaDefinition
	err = json.Unmarshal(schemasCollectionSchema, &schema)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling JSON: %v\n", err)
	}

	exists, err := interactor.CollectionExists(schema.Name)
	if err != nil {
		return nil, fmt.Errorf("Error looking up schema collection: %v\n", err)
	}

	if !exists {
		if err := interactor.CreateCollection(schema); err != nil {
			return nil, fmt.Errorf("Failed to create table for collections %s: %w", schema.Name, err)
		}
	}

	executor := NewExecutor(interactor, nil)
	collection, err := NewCollection(bus, &schema, executor, fmap)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize schemas collection collections %w", err)
	}

	return &Persistence{
		interactor: interactor,
		executor:   executor,
		fmap:       fmap,
		collection: collection,
		schema:     &schema,
		bus:        bus,
		logger:     zap.NewNop(),
	}, nil
}

// Collection returns a PersistenceCollectionInterface for the given name.
func (pi *Persistence) Collection(name string) (PersistenceCollectionInterface, error) {
	schema, err := pi.Schema(name)
	if err != nil {
		return nil, err
	}

	collection, err := NewCollection(pi.bus, schema, pi.executor, pi.fmap)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

func (pi *Persistence) Transact(callback func(tx PersistenceTransactionInterface) (any, error)) (any, error) {
	tx, err := pi.interactor.StartTransaction(context.Background())
	if err != nil {
		return nil, err
	}
	transactionCtx, err := NewPersistence(tx, pi.fmap)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}
	result, err := callback(transactionCtx)
	if err != nil {
		tx.Rollback(context.Background())
		return result, err
	}

	tx.Commit(context.Background())
	return result, err
}

// Collections returns a list of collection names.
func (pi *Persistence) Collections() ([]string, error) {
	q := query.NewQueryBuilder().Build()
	result, err := pi.collection.Read(&q)
	if err != nil {
		return nil, fmt.Errorf("error reading schemas to get collection names: %v", err)
	}

	var collectionNames []string
	if result.Data != nil {
		if docs, ok := result.Data.([]schema.Document); ok {
			for _, doc := range docs {
				record, err := mapToSchemaRecord(doc)
				if err != nil {
					pi.logger.Warn("Failed to convert map to SchemaRecord while listing collections", zap.Error(err))
					continue
				}
				collectionNames = append(collectionNames, record.Name)
			}
		} else if doc, ok := result.Data.(schema.Document); ok { // Handle case where Read returns a single document
			record, err := mapToSchemaRecord(doc)
			if err != nil {
				return nil, fmt.Errorf("error converting map to SchemaRecord for single collection: %v", err)
			}
			collectionNames = append(collectionNames, record.Name)
		}
	}
	return collectionNames, nil
}

// Create creates a new collection based on the schema.
func (pi *Persistence) Create(schema schema.SchemaDefinition) (PersistenceCollectionInterface, error) {
	exists, err := pi.interactor.CollectionExists(schema.Name)
	if err != nil {
		return nil, fmt.Errorf("Error accessing database: %v\n", err)
	}

	if exists {
		return nil, fmt.Errorf("A collection with a similar name exists\n")
	}

	if err := pi.interactor.CreateCollection(schema); err != nil {
		return nil, fmt.Errorf("Failed to create collection %s: %w", schema.Name, err)
	}

	schemaJson, err := schemaToRawJson(&schema)
	if err != nil {
		return nil, err
	}

	record := SchemaRecord{
		Name:        schema.Name,
		Description: *schema.Description,
		Version:     schema.Version,
		Schema:      schemaJson,
	}

	data, err := schemaRecordToMap(&record)
	if err != nil {
		return nil, err
	}
	_, err = pi.collection.Create(data)
	if err != nil {
		return nil, err
	}

	collection, err := NewCollection(pi.bus, &schema, pi.executor, pi.fmap)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

func (pi *Persistence) schemaCollection(tx DatabaseInteractor) (PersistenceCollectionInterface, error) {
	var executor *Executor
	if tx != nil {
		executor = NewExecutor(tx, nil)
	} else {
		executor = NewExecutor(pi.interactor, nil)
	}
	collection, err := NewCollection(pi.bus, pi.schema, executor, pi.fmap)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize schemas collection collections %w", err)
	}
	return collection, nil
}

// Delete deletes a collection by its ID (name).
func (pi *Persistence) Delete(name string) (bool, error) {
	tx, err := pi.interactor.StartTransaction(context.Background())
	if err != nil {
		return false, err
	}
	collection, err := pi.schemaCollection(tx)
	if err != nil {
		return false, err
	}
	q := query.NewQueryBuilder().Where("name").Eq(name).Build()
	_, err = collection.Delete(q.Filters, false)
	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	err = tx.DropCollection(name)
	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	err = tx.Commit(context.Background())

	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}
	return true, nil
}

// Schema returns the schema definition for a given ID.
func (pi *Persistence) Schema(name string) (*schema.SchemaDefinition, error) {
	exists, err := pi.interactor.CollectionExists(name)
	if err != nil {
		return nil, fmt.Errorf("Error accessing database: %v, %t\n", err, exists)
	}

	if !exists {
		return nil, fmt.Errorf("Collection %s does not exist: %v\n", name, err)
	}

	q := query.NewQueryBuilder().Where("name").Eq(name).Build()

	result, err := pi.collection.Read(&q)
	if err != nil {
		return nil, fmt.Errorf("Error reading schema collection: %v\n", err)
	}

	if result.Count != 1 {
		return nil, fmt.Errorf("Unexpected count for schema name: %v\n", result.Count)
	}

	record, err := mapToSchemaRecord(result.Data.(schema.Document))
	if err != nil {
		return nil, fmt.Errorf("Error converting map to SchemaRecord: %v\n", err)
	}

	var schema schema.SchemaDefinition
	err = json.Unmarshal(record.Schema, &schema)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling JSON: %v\n", err)
	}
	return &schema, nil
}

// RegisterSubscription registers a new subscription.
func (pi *Persistence) RegisterSubscription(options RegisterSubscriptionOptions) string {
	pi.subMu.Lock()
	defer pi.subMu.Unlock()
	unsubscribe := pi.bus.Subscribe(string(options.Event), options.Callback)
	id := uuid.New()
	callbackID := id.String()

	data := SubscriptionInfo{
		Id:          &callbackID,
		Event:       options.Event,
		Unsubscribe: unsubscribe,
		Label:       options.Label,
		Description: options.Description,
	}

	pi.subscriptions[callbackID] = &data
	return callbackID
}

// UnregisterSubscription unregisters an existing subscription.
func (pi *Persistence) UnregisterSubscription(callback string) {
	pi.subMu.Lock()
	defer pi.subMu.Unlock()
	info := pi.subscriptions[callback]
	if info != nil {
		info.Unsubscribe()
		delete(pi.subscriptions, callback)
	}
}

// Subscriptions returns all registered subscriptions.
func (pi *Persistence) Subscriptions() ([]SubscriptionInfo, error) {
	pi.subMu.RLock()
	defer pi.subMu.RUnlock()

	subs := make([]SubscriptionInfo, 0, len(pi.subscriptions))
	for _, sub := range pi.subscriptions {
		subs = append(subs, *sub)
	}

	return subs, nil
}

// Metadata retrieves metadata based on the filter.
func (pi *Persistence) Metadata(
	filter *MetadataFilter,
) (Metadata, error) {
	metadata := Metadata{}
	return metadata, nil
}

