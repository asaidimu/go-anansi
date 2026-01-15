package persistence

import (
	"context"
	"errors"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	cevents "github.com/asaidimu/go-anansi/v6/core/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v6/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type basePersistence struct {
	interactor         query.DatabaseInteractor
	engine             *query.QueryEngine
	eventEmitter       *cevents.EventEmitter[base.PersistenceEvent]
	registry           base.CollectionRegistry
	registryCollection base.Collection
	subscriptions      map[string]*base.SubscriptionInfo
	subMu              sync.RWMutex
	collections        map[string]base.Collection
	collectionsMu      sync.RWMutex
	logger             *zap.Logger
	decorators         []utils.DecoratorFunc[base.Collection]
	rawQueryProcessor  base.RawQueryProcessor
	txMu               sync.RWMutex
}

var _ base.Persistence = (*basePersistence)(nil)

func newBasePersistence(
	interactor query.DatabaseInteractor,
	eventEmitter *cevents.EventEmitter[base.PersistenceEvent],
	logger *zap.Logger,
	decorators []utils.DecoratorFunc[base.Collection],
) (base.Persistence, error) {

	registrySchema := registry.RegistrySchema()
	engine := query.NewQueryEngine(interactor.Capabilities(), logger)
	registryCollection, err := collection.NewCollection(eventEmitter,
		registry.REGISTRY_COLLECTION_NAME,
		registrySchema,
		interactor,
		engine,
		logger,
		func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error) {
			if name != registrySchema.Name {
				return "", nil, common.NewSystemError("ERR_INVALID_QUERY", "INVALID_QUERY_ON_REGISTRY")
			}
			return registrySchema.Name, registrySchema, nil
		},
		nil,
	)

	if err != nil {
		return nil, err
	}

	p := &basePersistence{
		eventEmitter:       eventEmitter,
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
	p.rawQueryProcessor = newRawQueryProcessor(registry)

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

	if issues := sc.ValidateAll(); len(issues) > 0 {
		return nil, common.NewSystemError("ERR_INVALID_SCHEMA").WithIssues(issues)
	}
	newCollection, err := collection.NewCollection(
		p.eventEmitter,
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
		p.rawQueryProcessor,
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

func (p *basePersistence) CreateCollection(ctx context.Context, sc *schema.SchemaDefinition) (base.Collection, error) {
	_, err := p.registry.CreateCollection(ctx, sc)
	if err != nil {
		return nil, err
	}

	return p.Collection(ctx, sc.Name)
}

func (p *basePersistence) CreateCollections(ctx context.Context, schemas []*schema.SchemaDefinition) error {
	_, err := (p.registry).CreateCollections(ctx, schemas)
	if err != nil {
		return nil
	}
	return nil
}

func (p *basePersistence) HasCollection(ctx context.Context, name string) (bool, error) {
	_, err := (p.registry).GetRegistryEntry(ctx, name)
	if err != nil {
		if errors.Is(err, base.ErrCollectionNotFound) {
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
		if errors.Is(err, base.ErrCollectionNotFound) {
			return base.Metadata{}, nil
		}
		return base.Metadata{}, common.NewSystemError("ERR_PERSISTENCE_METADATA_FAILED", base.ErrFailedToListCollections.Error()).WithCause(err)
	}

	collections := make([]*base.CollectionMetadata, len(entries))
	schemas := make([]*schema.SchemaDefinition, len(entries))
	for i, entry := range entries {
		// For now, we'll just use the entry's schema. A more complete implementation
		// might fetch detailed metadata from each collection.
		sc := entry.Versions[entry.ActiveVersion].Schema
		schemas[i] = &sc
		collections[i] = &base.CollectionMetadata{
			Name:          entry.Name,
			Version: entry.ActiveVersion,
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

	unsubscribe := p.eventEmitter.Subscribe(string(options.Event), options.Callback)
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
		return transaction.Execute(ctx, p.interactor, p.logger, func(tctx context.Context, txInteractor query.DatabaseInteractor) (any, error) {
			txBasePersistence, err := newBasePersistence(txInteractor, p.eventEmitter, p.logger, p.decorators)
			if err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_TRANSACTION_PERSISTENCE_CREATION_FAILED", "failed to create transaction persistence instance").WithOperation("basePersistence.Transact")
			}
			return callback(tctx, txBasePersistence)
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
	p.eventEmitter = nil
	p.registry = nil
	p.interactor = nil
}

// future is a concrete implementation of the Future interface.
type future struct {
	result any
	err    error
	done   chan struct{}
}

// Await waits for the operation to complete and returns the result and the error.
func (f *future) Await() (any, error) {
	<-f.done
	return f.result, f.err
}

// newFuture creates a new future.
func newFuture() *future {
	return &future{
		done: make(chan struct{}),
	}
}

func (p *basePersistence) Async(ctx context.Context, f func(ctx context.Context) (any, error)) base.Future {
	fut := newFuture()
	if tx, ok := transaction.GetCurrentTransaction(ctx); ok {
		cleanup := tx.AddOperation()
		go func() {
			result, err := f(ctx)
			fut.result = result
			fut.err = err
			close(fut.done)
			cleanup(err)
		}()
	} else {
		go func() {
			result, err := f(ctx)
			fut.result = result
			fut.err = err
			close(fut.done)
		}()
	}
	return fut
}

func (p *basePersistence) Query(ctx context.Context, rawQuery *query.RawQuery) (*query.RawQueryResult, error) {
	// Resolve collection placeholders in the template
	resolvedTemplate, err := p.rawQueryProcessor.ProcessRawQueryTemplate(ctx, rawQuery.Template, rawQuery.Collections)
	if err != nil {
		return nil, err
	}

	// Create a new RawQuery with the resolved template
	processedRawQuery := &query.RawQuery{
		Template:    resolvedTemplate,
		Options:     rawQuery.Options,
		Collections: rawQuery.Collections,
		Parameters:  rawQuery.Parameters,
	}

	return p.interactor.Query(ctx, &query.Query{Raw: processedRawQuery})
}
