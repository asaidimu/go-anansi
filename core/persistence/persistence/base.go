package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v6/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type basePersistence struct {
	interactor         *query.DatabaseInteractor
	engine             *query.QueryEngine
	bus                *events.TypedEventBus[base.PersistenceEvent]
	registry           *base.CollectionRegistry
	registryCollection *base.Collection
	subscriptions      map[string]*base.SubscriptionInfo
	subMu              sync.RWMutex
	collections        map[string]base.Collection
	collectionsMu      sync.RWMutex
	logger             *zap.Logger
	metadataOptions    *base.MetadataOptions
	decorators         []utils.DecoratorFunc[base.Collection]
	txmu sync.Mutex
}

var _ base.Persistence = (*basePersistence)(nil)

func newBasePersistence(
	interactor query.DatabaseInteractor,
	bus *events.TypedEventBus[base.PersistenceEvent],
	options base.MetadataOptions,
	logger *zap.Logger,
	decorators []utils.DecoratorFunc[base.Collection],
) (base.Persistence, error) {

	registrySchema := registry.RegistrySchema()
	engine := query.NewQueryEngine(interactor, logger)
	registryCollection, err := collection.NewCollection(bus,
		registry.REGISTRY_COLLECTION_NAME,
		registrySchema,
		engine,
		logger,
		&options,
		nil,
	)
	if err != nil {
		return nil, err
	}

	p := &basePersistence{
		bus:                bus,
		engine:             engine,
		interactor:         &interactor,
		subscriptions:      make(map[string]*base.SubscriptionInfo),
		collections:        make(map[string]base.Collection),
		logger:             logger,
		registryCollection: &registryCollection,
		metadataOptions:    &options,
		decorators:         decorators,
	}

	registry, err := registry.NewCollectionRegistry(p.createRegistryExecutor(registrySchema, &options), logger)

	if err != nil {
		return nil, err
	}

	p.registry = &registry
	return p, nil
}

func (p *basePersistence) Collection(ctx context.Context, name string) (base.Collection, error) {
	p.collectionsMu.RLock()
	c, ok := p.collections[name]
	p.collectionsMu.RUnlock()
	if ok {
		return c, nil
	}

	// Collection not in cache, so create and cache it
	p.collectionsMu.Lock()
	defer p.collectionsMu.Unlock()

	// Double-check in case it was created while waiting for the lock
	c, ok = p.collections[name]
	if ok {
		return c, nil
	}

	schema, err := (*p.registry).GetSchema(ctx, name)

	if err != nil {
		return nil, err
	}

	newCollection, err := collection.NewCollection(
		p.bus,
		name,
		schema,
		p.engine,
		p.logger,
		p.metadataOptions,
		func(ctx context.Context, logicalName string) (string, error) {
			return (*p.registry).ResolvePhysicalName(ctx, logicalName)
		},
	)

	if err != nil {
		return nil, err
	}

	decorated := utils.ApplyDecorators(newCollection, p.decorators)
	p.collections[name] = decorated
	return decorated, nil
}

func (p *basePersistence) Collections(ctx context.Context) ([]string, error) {
	entries, err := (*p.registry).List(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}

	return names, nil
}

func (p *basePersistence) Create(ctx context.Context, sc schema.SchemaDefinition) (base.Collection, error) {
	_, err := (*p.registry).CreateCollection(ctx, &sc)
	if err != nil {
		return nil, err
	}

	return p.Collection(ctx, sc.Name)
}

func (p *basePersistence) Delete(ctx context.Context, id string) (bool, error) {
	opts := base.DropCollectionOptions{DeletePhysicalData: true}
	err := (*p.registry).DropCollection(ctx, id, opts)
	if err != nil {
		return false, err
	}

	// Remove from cache
	p.collectionsMu.Lock()
	defer p.collectionsMu.Unlock()
	delete(p.collections, id)

	return true, nil
}

func (p *basePersistence) Metadata(ctx context.Context, filter *base.MetadataFilter) (base.Metadata, error) {
	if p.registry == nil {
		return base.Metadata{}, errors.New("registry is not initialized")
	}

	entries, err := (*p.registry).List(ctx)
	if err != nil {
		// If the registry is empty, it might return a not found error. In this case, we should return empty metadata.
		if errors.Is(err, registry.ErrCollectionNotFound) {
			return base.Metadata{}, nil
		}
		return base.Metadata{}, fmt.Errorf("failed to list collections from registry: %w", err)
	}

	collections := make([]*base.CollectionMetadata, len(entries))
	schemas := make([]*schema.SchemaDefinition, len(entries))
	for i, entry := range entries {
		// For now, we'll just use the entry's schema. A more complete implementation
		// might fetch detailed metadata from each collection.
		sc := entry.Versions[entry.ActiveVersion].Schema
		schemas[i] = &sc
		collections[i] = &base.CollectionMetadata{
			ID:            entry.Name,
			Name:          entry.Name,
			SchemaVersion: entry.ActiveVersion,
			Description:   entry.Description,
			Schema:        &sc,
		}
	}

	subscriptions, _ := p.Subscriptions(ctx)
	globalSubscriptions := make([]*base.SubscriptionInfo, len(subscriptions))
	for i := range subscriptions {
		globalSubscriptions[i] = &subscriptions[i]
	}

	collectionCount := int64(len(collections))

	return base.Metadata{
		Collections:     collections,
		Schemas:         schemas,
		Subscriptions:   globalSubscriptions,
		CollectionCount: &collectionCount,
	}, nil
}

func (p *basePersistence) RegisterSubscription(ctx context.Context, options base.RegisterSubscriptionOptions) string {
	p.subMu.Lock()
	defer p.subMu.Unlock()

	unsubscribe := p.bus.Subscribe(string(options.Event), options.Callback)
	id := uuid.New().String()

	data := base.SubscriptionInfo{
		Id:          &id,
		Event:       options.Event,
		Unsubscribe: unsubscribe,
		Label:       options.Label,
		Description: options.Description,
	}

	p.subscriptions[id] = &data
	return id
}

func (p *basePersistence) Schema(ctx context.Context, id string, version ...string) (*schema.SchemaDefinition, error) {
	sc, err := (*p.registry).GetSchema(ctx, id, version...)
	if err != nil {
		return nil, err
	}
	sc.Name = id
	return sc, nil
}

func (p *basePersistence) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	p.subMu.RLock()
	defer p.subMu.RUnlock()

	subs := make([]base.SubscriptionInfo, 0, len(p.subscriptions))
	for _, sub := range p.subscriptions {
		subs = append(subs, *sub)
	}

	return subs, nil
}

func (p *basePersistence) Transact(ctx context.Context, callback func(tx base.BasePersistence) (any, error)) (any, error) {
	p.txmu.Lock()
	defer p.txmu.Unlock()

	tx, err := (*p.interactor).StartTransaction(ctx)
	if err != nil {
		return nil, err
	}

	engine := query.NewQueryEngine(tx, p.logger)

	registrySchema := registry.RegistrySchema()
	registryCollection, err := collection.NewCollection(p.bus,
		registry.REGISTRY_COLLECTION_NAME,
		registrySchema,
		engine,
		p.logger,
		p.metadataOptions,
		nil,
	)

	if err != nil {
		return nil, err
	}

	tp := &basePersistence{
		bus:                p.bus,
		engine:             engine,
		interactor:         nil,
		subscriptions:      make(map[string]*base.SubscriptionInfo),
		collections:        make(map[string]base.Collection),
		logger:             p.logger,
		registryCollection: &registryCollection,
		metadataOptions:    p.metadataOptions,
	}

	registry, err := registry.NewCollectionRegistry(p.createRegistryExecutor(registrySchema, p.metadataOptions), p.logger)

	if err != nil {
		return nil, err
	}

	tp.registry = &registry

	result, err := callback(tp)
	if err != nil {
		tx.Rollback(ctx)
		return result, err
	}

	tx.Commit(ctx)
	return result, nil
}

func (p *basePersistence) UnregisterSubscription(ctx context.Context, id string) {
	p.subMu.Lock()
	defer p.subMu.Unlock()

	if info, ok := p.subscriptions[id]; ok {
		info.Unsubscribe()
		delete(p.subscriptions, id)
	}
}

func (p *basePersistence) createRegistryExecutor(schema *schema.SchemaDefinition, options *base.MetadataOptions) registry.RegistryExecutor {
	executor := func(ctx context.Context, transaction bool, fn func(collection base.Collection, manager query.SchemaManager) (any, error)) (any, error) {
		if transaction {
			tx, err := (*p.interactor).StartTransaction(ctx)
			if err != nil {
				return nil, err
			}

			ix := tx.(query.BaseDatabaseInteractor)
			engine := query.NewQueryEngine(ix, p.logger)

			collection, err := collection.NewCollection(p.bus, registry.REGISTRY_COLLECTION_NAME, schema, engine, p.logger, options, nil)

			if err != nil {
				return nil, err
			}

			result, err := fn(collection, tx.SchemaManager())
			if err != nil {
				tx.Rollback(ctx)
				return nil, err
			}
			tx.Commit(ctx)
			return result, err
		}
		return fn(*p.registryCollection, (*p.interactor).SchemaManager())
	}

	return executor
}

func (p *basePersistence) Rollback(
	ctx context.Context,
	name string,
	version *string,
	dryRun *bool,
) (base.Collection, error) {
	return nil, nil
}

func (p *basePersistence) Migrate(
	ctx context.Context,
	name string,
	migration schema.Migration,
	dryRun *bool,
) (base.Collection, error) {
	return nil, nil
}

func (p *basePersistence) Close(ctx context.Context) {
	p.bus.Close()
	p.bus = nil
	p.registry = nil
	p.interactor = nil
}
