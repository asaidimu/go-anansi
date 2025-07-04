// Package persistence provides the core implementation of the PersistenceInterface,
// offering a concrete way to interact with the underlying database.
package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/utils"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Persistence is the main implementation of the PersistenceInterface. It orchestrates
// interactions with the database through a DatabaseInteractor, manages schema definitions,
// and handles event subscriptions for observability.
type Persistence struct {
	interactor      DatabaseInteractor
	collection      PersistenceCollectionInterface
	schema          *schema.SchemaDefinition
	executor        *Executor
	fmap            schema.FunctionMap
	logger          *zap.Logger
	subscriptions   map[string]*SubscriptionInfo // To store unsubscribe functions
	subMu           sync.RWMutex                 // Mutex to protect subscriptions map
	collectionNames map[string]string
	bus             *events.TypedEventBus[PersistenceEvent]
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
		tx, err := interactor.StartTransaction(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to create system schema collection: %w", err)
		}
		if err := tx.CreateCollection(s); err != nil {
			tx.Rollback(context.Background())
			return nil, fmt.Errorf("failed to create table for collections %s: %w", s.Name, err)
		}
		tx.Commit(context.Background())
	}

	executor := NewExecutor(interactor, nil)
	collection, err := NewCollection(bus, s.Name, &s, executor, fmap)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize schemas collection: %w", err)
	}

	q := query.NewQueryBuilder().Select().Include("name").End().Build()
	result, err := collection.Read(&q)

	if err != nil {
		return nil, fmt.Errorf("Failed to get collection names: %w", err)
	}

	names := make(map[string]string)

	if result.Count == 1 {
		data, _ := result.Data.(schema.Document)
		name, _ := data["name"].(map[string]string)
		names[name["logical"]] = name["physical"]
	} else {
		data, _ := result.Data.([]schema.Document)
		for _, doc := range data {
			name, _ := doc["name"].(map[string]string)
			names[name["logical"]] = name["physical"]
		}
	}

	fmt.Printf("%v", names)
	return &Persistence{
		interactor:      interactor,
		executor:        executor,
		fmap:            fmap,
		collection:      collection,
		schema:          &s,
		bus:             bus,
		logger:          zap.NewNop(),
		subscriptions:   make(map[string]*SubscriptionInfo),
		collectionNames: names,
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

	s.Name = p.collectionNames[s.Name]

	collection, err := NewCollection(p.bus, name, s, p.executor, p.fmap)
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
				record, err := utils.MapToStruct[SchemaRecord](doc)
				if err != nil {
					p.logger.Warn("Failed to convert map to SchemaRecord while listing collections", zap.Error(err))
					continue
				}
				collectionNames = append(collectionNames, record.Name.Logical)
			}
		} else if doc, ok := result.Data.(schema.Document); ok { // Handle case where Read returns a single document
			record, err := utils.MapToStruct[SchemaRecord](doc)
			if err != nil {
				return nil, fmt.Errorf("error converting map to SchemaRecord for single collection: %w", err)
			}
			collectionNames = append(collectionNames, record.Name.Logical)
		}
	}
	return collectionNames, nil
}

// Create creates a new collection based on the provided schema definition. It ensures
// that a collection with the same name does not already exist before creating it.
func (p *Persistence) Create(s schema.SchemaDefinition) (PersistenceCollectionInterface, error) {
	if exists, err := p.Schema(s.Name); exists != nil {
		return nil, fmt.Errorf("Collection with a similar name exists or error : %w", err)
	}

	physicalName := uuid.New().String()

	record := SchemaRecord{
		Name: NameRecord{
			Logical:  s.Name,
			Physical: physicalName,
		},
		Description: *s.Description,
		Version:     s.Version,
		Schema:      s,
	}

	tx, err := p.interactor.StartTransaction(context.Background())
	if err != nil {
		return nil, err
	}

	recordData, err := utils.StructToMap(&record)

	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}

	collection, err := p.SchemaCollection(tx)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}

	_, err = collection.Create(recordData)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}

	s.Name = physicalName
	if err := tx.CreateCollection(s); err != nil {
		tx.Rollback(context.Background())
		return nil, fmt.Errorf("failed to create collection %s: %w", s.Name, err)
	}

	tx.Commit(context.Background())
	result, err := NewCollection(p.bus, s.Name, &s, p.executor, p.fmap)

	if err != nil {
		return nil, err
	}

	return result, err
}

// SchemaCollection returns a PersistenceCollectionInterface for the internal schemas collection.
// It can be configured to run within a transaction.
func (p *Persistence) SchemaCollection(tx DatabaseInteractor) (PersistenceCollectionInterface, error) {
	var executor *Executor
	if tx != nil {
		executor = NewExecutor(tx, nil)
	} else {
		executor = NewExecutor(p.interactor, nil)
	}
	collection, err := NewCollection(p.bus, p.schema.Name, p.schema, executor, p.fmap)
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

	collection, err := p.SchemaCollection(tx)
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
	q := query.NewQueryBuilder().Where("name").Eq(name).Build()

	result, err := p.collection.Read(&q)
	if err != nil {
		return nil, fmt.Errorf("error reading schema collection: %w", err)
	}

	if result.Count == 0 {
		return nil, fmt.Errorf("Schema %s does not exists", name)
	}

	if result.Count > 1 {
		return nil, fmt.Errorf("unexpected count for schema name: %d", result.Count)
	}

	record, err := DocumentToStruct[SchemaRecord](result.Data)
	if err != nil {
		return nil, fmt.Errorf("error converting map to SchemaRecord: %w", err)
	}

	return &record.Schema, nil
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

func (p *Persistence) Migrate(
	name string,
	migration schema.Migration,
	dryRun *bool,
) (PersistenceCollectionInterface, error) {
	// TODO: Implement schema Migration
	return p.Collection(name)
}

func (p *Persistence) Rollback(
	name string,
	version *string,
	dryRun *bool,
) (PersistenceCollectionInterface, error) {
	// TODO: Implement schema rollback

	return p.Collection(name)
}
