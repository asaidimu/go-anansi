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
	interactor    DatabaseInteractor
	collection    PersistenceCollectionInterface
	schema        *schema.SchemaDefinition
	executor      *Executor
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
func NewPersistence(interactor DatabaseInteractor, fmap schema.FunctionMap, logger *zap.Logger) (PersistenceInterface, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	executor := NewExecutor(interactor, logger)
	registry, err := NewSchemaRegistry(interactor, executor, fmap, logger)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFailedToCreateSchemaRegistry, err)
	}

	return &Persistence{
		interactor:    interactor,
		executor:      executor,
		fmap:          fmap,
		collection:    registry.collection,
		schema:        registry.schema,
		bus:           registry.bus,
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
		return nil, fmt.Errorf("active version '%s' not found for collection '%s'", record.ActiveVersion, name)
	}

	collection, err := NewCollection(p.bus, name, &activeVersion.Schema, p.executor, p.fmap)
	if err != nil {
		return nil, fmt.Errorf("%w for %s: %v", ErrCollectionInitialization, name, err)
	}
	return collection, nil
}

// Transact executes a callback function within a database transaction. If the callback
// returns an error, the transaction is rolled back; otherwise, it is committed.
func (p *Persistence) Transact(callback func(tx PersistenceTransactionInterface) (any, error)) (any, error) {
	tx, err := p.interactor.StartTransaction(context.Background())

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFailedToStartTransaction, err)
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
			return result, fmt.Errorf("%w: %v, rollback failed", err, rbErr)
		}
		return result, err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return result, fmt.Errorf("%w: %v", ErrFailedToCommitTransaction, err)
	}

	return result, nil
}

// Collections returns a list of all collection names currently managed by the persistence layer.
func (p *Persistence) Collections() ([]string, error) {
	q := query.NewQueryBuilder().Build()
	result, err := p.collection.Read(&q)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadingSchemas, err)
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
				return nil, fmt.Errorf("%w for single collection: %v", ErrMapToStructConversion, err)
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
		return nil, fmt.Errorf("Error checking for existing collection: %w", err)
	}

	tx, err := p.interactor.StartTransaction(context.Background())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFailedToStartTransaction, err)
	}

	err = p.registry.RegisterSchema(context.Background(), tx, s)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, fmt.Errorf("failed to register schema: %w", err)
	}

	// After successful registration, retrieve the schema record to get the full details
	// including the active physical name and schema from the versions map.
	record, err := p.registry.Lookup(s.Name)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, fmt.Errorf("failed to lookup schema after registration: %w", err)
	}

	activeVersion, ok := record.Versions[record.ActiveVersion]
	if !ok {
		tx.Rollback(context.Background())
		return nil, fmt.Errorf("active version '%s' not found in versions map after registration for collection '%s'", record.ActiveVersion, s.Name)
	}

	// Use the physical name and schema from the active version for collection creation
	activeVersion.Schema.Name = activeVersion.Physical // Ensure the schema's name field is set to the physical name for DB operations
	if err := tx.CreateCollection(activeVersion.Schema); err != nil {
		tx.Rollback(context.Background())
		return nil, fmt.Errorf("failed to create collection %s: %w", activeVersion.Physical, err)
	}

	if err := tx.Commit(context.Background()); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFailedToCommitTransaction, err)
	}

	// Use the logical name and the active schema for NewCollection
	result, err := NewCollection(p.bus, s.Name, &activeVersion.Schema, p.executor, p.fmap)

	if err != nil {
		return nil, fmt.Errorf("%w for %s: %v", ErrCollectionInitialization, s.Name, err)
	}

	p.registry.RefreshNames()
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
		return nil, fmt.Errorf("%w: %v", ErrSchemaCollectionInit, err)
	}
	return collection, nil
}

// Delete removes a collection by its name. This operation is transactional, ensuring that
// both the collection and its schema definition are removed atomically.
func (p *Persistence) Delete(name string) (bool, error) {
	tx, err := p.interactor.StartTransaction(context.Background())
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrFailedToStartTransaction, err)
	}

	if err := p.registry.UnregisterSchema(context.Background(), tx, name); err != nil {
		tx.Rollback(context.Background())
		return false, fmt.Errorf("failed to unregister schema: %w", err)
	}

	if err := tx.DropCollection(name); err != nil {
		tx.Rollback(context.Background())
		return false, fmt.Errorf("%w for %s: %v", ErrDropCollection, name, err)
	}

	if err := tx.Commit(context.Background()); err != nil {
		tx.Rollback(context.Background())
		return false, fmt.Errorf("%w: %v", ErrFailedToCommitTransaction, err)
	}

	// Refresh the in-memory names map after a collection is deleted
	if err := p.registry.RefreshNames(); err != nil {
		return false, fmt.Errorf("failed to refresh names after collection deletion: %w", err)
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
		return nil, fmt.Errorf("active version '%s' not found in schema record for collection '%s'", record.ActiveVersion, name)
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
