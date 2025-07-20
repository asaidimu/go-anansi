// Package persistence provides the core implementation of the PersistenceInterface,
// offering a concrete way to interact with the underlying database.
package persistence

import (
	"context"
	"errors" // Import the errors package
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Persistence is the main implementation of the PersistenceInterface. It orchestrates
// interactions with the database through a DatabaseInteractor, manages schema definitions,
// and handles event subscriptions for observability.
type Persistence struct {
	interactor    query.DatabaseInteractor
	collection    PersistenceCollectionInterface
	schema        *schema.SchemaDefinition
	executor      *query.Executor
	fmap          schema.FunctionMap
	logger        *zap.Logger
	subscriptions map[string]*SubscriptionInfo // To store unsubscribe functions
	subMu         sync.RWMutex                 // Mutex to protect subscriptions map
	bus           *events.TypedEventBus[PersistenceEvent]
	registry      *SchemaRegistry
}

// NewPersistence creates a new instance of the Persistence service. It initializes the
// event bus, ensures that the internal schema for managing collections exists, and sets
// up the necessary components for the persistence layer to function.
func NewPersistence(interactor query.DatabaseInteractor, fmap schema.FunctionMap, logger *zap.Logger) (PersistenceInterface, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	bus, err := events.NewTypedEventBus[PersistenceEvent](events.DefaultConfig())
	if err != nil {
		return nil, NewPersistenceError("Failed to initialize event bus", ErrFailedToInitializeEventBus)
	}

	registry, err := NewSchemaRegistry(interactor, fmap, logger)
	if err != nil {
		return nil, NewPersistenceError("Failed to create schema registry", ErrFailedToCreateSchemaRegistry)
	}

	return &Persistence{
		interactor:    interactor,
		fmap:          fmap,
		collection:    registry.collection,
		schema:        registry.schema,
		bus:           bus,
		logger:        logger,
		subscriptions: make(map[string]*SubscriptionInfo),
		registry:      registry,
	}, nil
}

// Collection returns a PersistenceCollectionInterface for a given collection name.
// This allows for performing operations like Create, Read, Update, and Delete on that
// specific collection.
func (p *Persistence) Collection(name string) (PersistenceCollectionInterface, error) {
	record, err := p.registry.Lookup(name)
	if err != nil {
		return nil, err
	}

	activeVersion, ok := record.Versions[record.ActiveVersion]
	if !ok {
		return nil, NewPersistenceError(fmt.Sprintf("active version '%s' not found for collection '%s'", record.ActiveVersion, name), ErrSchemaNotFound)
	}

	collection, err := NewCollection(p.bus, name, &activeVersion.Schema, p.interactor, p.fmap)
	if err != nil {
		return nil, NewPersistenceError("Failed to initialize collection", ErrCollectionInitialization)
	}
	return collection, nil
}

// Transact executes a callback function within a database transaction. If the callback
// returns an error, the transaction is rolled back; otherwise, it is committed.
func (p *Persistence) Transact(callback func(tx PersistenceTransactionInterface) (any, error)) (any, error) {
	tx, err := p.interactor.StartTransaction(context.Background())

	if err != nil {
		return nil, NewPersistenceError("Failed to start transaction", ErrFailedToStartTransaction)
	}

	transactionCtx, err := NewPersistence(tx, p.fmap, p.logger)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}

	result, err := callback(transactionCtx)
	if err != nil {
		if rbErr := tx.Rollback(context.Background()); rbErr != nil {
			p.logger.Error("Failed to rollback transaction", zap.Error(rbErr))
			return nil, NewPersistenceError("Failed to initialize event bus", ErrFailedToInitializeEventBus)
		}
		return result, err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return result, NewPersistenceError(fmt.Sprintf("Failed to commit transaction: %v", err), ErrFailedToCommitTransaction)
	}

	return result, nil
}

// Collections returns a list of all collection names currently managed by the persistence layer.
func (p *Persistence) Collections() ([]string, error) {
	q := query.NewQueryBuilder().Build()
	result, err := p.collection.Read(&q)
	if err != nil {
		return nil, NewPersistenceError("Error reading schemas to get collection names", ErrReadingSchemas)
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
				collectionNames = append(collectionNames, record.Name)
			}
		} else if doc, ok := result.Data.(schema.Document); ok { // Handle case where Read returns a single document
			record, err := utils.MapToStruct[SchemaRecord](doc)
			if err != nil {
				return nil, NewPersistenceError("Failed to convert map to struct", ErrMapToStructConversion)
			}
			collectionNames = append(collectionNames, record.Name)
		}
	}
	return collectionNames, nil
}

// Create creates a new collection based on the provided schema definition. It ensures
// that a collection with the same name does not already exist before creating it.
func (p *Persistence) Create(s schema.SchemaDefinition) (PersistenceCollectionInterface, error) {
	s.AddVersionField()
	if exists, err := p.Schema(s.Name); exists != nil || (err != nil && !errors.Is(err, ErrSchemaNotFound)) {
		// If a schema exists, or an error occurred that is *not* ErrSchemaNotFound
		if exists != nil {
			return nil, ErrCollectionAlreadyExists
		}
		return nil, NewPersistenceError("Collection already exists", ErrCollectionAlreadyExists)
	}

	tx, err := p.interactor.StartTransaction(context.Background())
	if err != nil {
		return nil, NewPersistenceError("Failed to start transaction", ErrFailedToStartTransaction)
	}

	err = p.registry.RegisterSchema(context.Background(), tx, s)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, NewPersistenceError("Failed to register schema", ErrFailedToRegisterSchema)
	}

	// After successful registration, retrieve the schema record to get the full details
	// including the active physical name and schema from the versions map.
	record, err := p.registry.Lookup(s.Name)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, NewPersistenceError("Error reading schema collection", ErrSchemaRead)
	}

	activeVersion, ok := record.Versions[record.ActiveVersion]
	if !ok {
		tx.Rollback(context.Background())
		return nil, NewPersistenceError(fmt.Sprintf("active version '%s' not found in versions map after registration for collection '%s'", record.ActiveVersion, s.Name), ErrSchemaNotFound)
	}

	// Use the physical name and schema from the active version for collection creation
	activeVersion.Schema.Name = activeVersion.Physical // Ensure the schema's name field is set to the physical name for DB operations
	if err := tx.CreateCollection(activeVersion.Schema); err != nil {
		tx.Rollback(context.Background())
		return nil, NewPersistenceError("Failed to create collection", ErrCollectionCreation)
	}

	if err := tx.Commit(context.Background()); err != nil {
		return nil, NewPersistenceError("Failed to commit transaction", ErrFailedToCommitTransaction)
	}

	// Use the logical name and the active schema for NewCollection
	result, err := NewCollection(p.bus, s.Name, &activeVersion.Schema, p.interactor, p.fmap)

	if err != nil {
		return nil, NewPersistenceError("Failed to initialize collection", ErrCollectionInitialization)
	}

	p.registry.RefreshNames()
	return result, err
}

// SchemaCollection returns a PersistenceCollectionInterface for the internal schemas collection.
// It can be configured to run within a transaction.
func (p *Persistence) SchemaCollection(tx query.DatabaseInteractor) (PersistenceCollectionInterface, error) {
	var interactor query.DatabaseInteractor
	if tx != nil {
		interactor = tx
	} else {
		interactor = p.interactor
	}
	collection, err := NewCollection(p.bus, p.schema.Name, p.schema, interactor, p.fmap)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchemaCollectionInit, err)
	}
	return collection, nil
}

// Delete removes a collection by its name. This operation is transactional, ensuring that
// both the collection and its schema definition are removed atomically.
func (p *Persistence) Delete(name string) (bool, error) {
	tx, err := p.interactor.StartTransaction(context.Background())
	if err != nil {
		return false, NewPersistenceError("Failed to start transaction", ErrFailedToStartTransaction)
	}

	if err := p.registry.UnregisterSchema(context.Background(), tx, name); err != nil {
		tx.Rollback(context.Background())
		return false, fmt.Errorf("%w: %w", ErrFailedToUnregisterSchema, err)
	}

	if err := tx.DropCollection(name); err != nil {
		tx.Rollback(context.Background())
		return false, NewPersistenceError("Failed to drop collection from database", ErrDropCollection)
	}

	if err := tx.Commit(context.Background()); err != nil {
		tx.Rollback(context.Background())
		return false, NewPersistenceError("Failed to commit transaction", ErrFailedToCommitTransaction)
	}

	// Refresh the in-memory names map after a collection is deleted
	if err := p.registry.RefreshNames(); err != nil {
		return false, fmt.Errorf("%w: %w", ErrFailedToRefreshNames, err)
	}

	return true, nil
}

// Schema retrieves the schema definition for a given collection name.
func (p *Persistence) Schema(name string) (*schema.SchemaDefinition, error) {
	record, err := p.registry.Lookup(name)
	if err != nil {
		return nil, err
	}

	activeVersion, ok := record.Versions[record.ActiveVersion]
	if !ok {
		return nil, NewPersistenceError(fmt.Sprintf("active version '%s' not found in schema record for collection '%s'", record.ActiveVersion, name), ErrSchemaNotFound)
	}
	return &activeVersion.Schema, nil
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
