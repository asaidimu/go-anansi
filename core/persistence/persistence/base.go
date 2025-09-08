package persistence

import (
	"context"
	"errors"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v6/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type basePersistence struct {
	interactor         query.DatabaseInteractor
	engine             *query.QueryEngine
	bus                *events.TypedEventBus[base.PersistenceEvent]
	registry           base.CollectionRegistry
	registryCollection base.Collection
	subscriptions      map[string]*base.SubscriptionInfo
	subMu              sync.RWMutex
	collections        map[string]base.Collection
	collectionsMu      sync.RWMutex
	logger             *zap.Logger
	decorators         []utils.DecoratorFunc[base.Collection]
	txMu               sync.RWMutex
}

var _ base.Persistence = (*basePersistence)(nil)

func newBasePersistence(
	interactor query.DatabaseInteractor,
	bus *events.TypedEventBus[base.PersistenceEvent],
	logger *zap.Logger,
	decorators []utils.DecoratorFunc[base.Collection],
) (base.Persistence, error) {

	registrySchema := registry.RegistrySchema()
	engine := query.NewQueryEngine(interactor.Capabilities(), logger)
	registryCollection, err := collection.NewCollection(bus,
		registry.REGISTRY_COLLECTION_NAME,
		registrySchema,
		interactor,
		engine,
		logger,
		nil,
	)
	if err != nil {
		return nil, err
	}

	p := &basePersistence{
		bus:                bus,
		engine:             engine,
		interactor:         interactor,
		subscriptions:      make(map[string]*base.SubscriptionInfo),
		collections:        make(map[string]base.Collection),
		logger:             logger,
		registryCollection: registryCollection,
		decorators:         decorators,
	}

	registry, err := registry.NewCollectionRegistry(p.createRegistryExecutor(registrySchema), logger)

	if err != nil {
		return nil, err
	}

	p.registry = registry
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

	sc, err := (p.registry).GetSchema(ctx, name)

	if err != nil {
		return nil, err
	}

	newCollection, err := collection.NewCollection(
		p.bus,
		name,
		sc,
		p.interactor,
		p.engine,
		p.logger,
		func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error) {
			sc, err := (p.registry).GetSchema(ctx, name)
			if err != nil {
				return "", nil, err
			}
			return sc.Name, sc, nil
		},
	)

	if err != nil {
		return nil, err
	}

	decorated := utils.ApplyDecorators(newCollection, p.decorators)
	p.collections[name] = decorated
	return decorated, nil
}

func (p *basePersistence) ListCollections(ctx context.Context) ([]string, error) {
	entries, err := (p.registry).List(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}

	return names, nil
}

func (p *basePersistence) CreateCollection(ctx context.Context, sc schema.SchemaDefinition) (base.Collection, error) {
	_, err := (p.registry).CreateCollection(ctx, &sc)
	if err != nil {
		return nil, err
	}

	return p.Collection(ctx, sc.Name)
}

func (p *basePersistence) CreateCollections(ctx context.Context, schemas []schema.SchemaDefinition) error {
	_, err := (p.registry).CreateCollections(ctx, schemas)
	if err != nil {
		return nil
	}
	return nil
}

func (p *basePersistence) HasCollection(ctx context.Context, name string) (bool, error) {
	_, err := (p.registry).GetRegistryEntry(ctx, name)
	if err != nil {
		if errors.Is(err, registry.ErrCollectionNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *basePersistence) Delete(ctx context.Context, id string) (bool, error) {
	opts := base.DropCollectionOptions{DeletePhysicalData: true}
	err := (p.registry).DropCollection(ctx, id, opts)
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
		return base.Metadata{}, registry.ErrRegistryNotInitialized
	}

	entries, err := (p.registry).List(ctx)
	if err != nil {
		// If the registry is empty, it might return a not found error. In this case, we should return empty metadata.
		if errors.Is(err, registry.ErrCollectionNotFound) {
			return base.Metadata{}, nil
		}
		return base.Metadata{}, &PersistenceError{
			Operation: "Metadata",
			Message:   registry.ErrFailedToListCollections.Error(),
			Cause:     errors.Join(registry.ErrFailedToListCollections, err),
		}
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

func (p *basePersistence) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
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
	sc, err := (p.registry).GetSchema(ctx, id, version...)
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

func (p *basePersistence) Transact(ctx context.Context, callback func(ctx context.Context, tx base.BasePersistence) (any, error)) (any, error) {
	execute := func() (any, error) {
		return transaction.Execute(ctx, p.interactor, p.logger, func(tctx context.Context, tx query.DatabaseInteractor) (any, error) {
			return callback(tctx, p)
		})
	}

	if _, ok := transaction.GetCurrentTransaction(ctx); ok || !p.interactor.Capabilities().RequiresTransactionSerialization {
		return execute()
	}

	p.txMu.Lock()
	defer p.txMu.Unlock()

	return execute()
}

func (p *basePersistence) Unsubscribe(ctx context.Context, id string) {
	p.subMu.Lock()
	defer p.subMu.Unlock()

	if info, ok := p.subscriptions[id]; ok {
		info.Unsubscribe()
		delete(p.subscriptions, id)
	}
}

func (p *basePersistence) createRegistryExecutor(_ *schema.SchemaDefinition) registry.RegistryExecutor {
	executor := func(ctx context.Context, transact bool, fn func(tctx context.Context, collection base.Collection, manager query.SchemaManager) (any, error)) (any, error) {
		if transact {
			return transaction.Execute(ctx, p.interactor, p.logger, func(tctx context.Context, tx query.DatabaseInteractor) (any, error) {
				return fn(tctx, p.registryCollection, tx.SchemaManager())
			})
		}
		return fn(ctx, p.registryCollection, (p.interactor).SchemaManager())
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

func (p *basePersistence) Async(ctx context.Context, f func(ctx context.Context) error) {
	if tx, ok := transaction.GetCurrentTransaction(ctx); ok {
		cleanup := tx.AddOperation()
		go func() {
			defer cleanup(f(ctx))
		}()
	} else {
		go f(ctx)
	}
}
