package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	cevents "github.com/asaidimu/go-anansi/v8/core/events"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/persistence/migration"
	"github.com/asaidimu/go-anansi/v8/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v8/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v8/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
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
	registryProvider := collection.NewStaticSchemaProvider(registrySchema)
	registryCollection, err := collection.NewCollection(eventEmitter,
		registry.REGISTRY_COLLECTION_NAME,
		registryProvider,
		interactor,
		engine,
		logger,
		func(ctx context.Context, name string) (string, *definition.Schema, error) {
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
	// TODO: Fix memory leak.
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

	if _, err := schema.ValidateSchema(sc); err != nil {
		return nil, err
	}

	provider := collection.NewRegistrySchemaProvider(p.registry, name)
	newCollection, err := collection.NewCollection(
		p.eventEmitter,
		name,
		provider,
		p.interactor,
		p.engine,
		p.logger,
		func(ctx context.Context, name string) (string, *definition.Schema, error) {
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

func (p *basePersistence) CreateCollection(ctx context.Context, sc *definition.Schema) (base.Collection, error) {
	_, err := p.registry.CreateCollection(ctx, sc)
	if err != nil {
		return nil, err
	}

	return p.Collection(ctx, sc.Name)
}

func (p *basePersistence) CreateCollections(ctx context.Context, schemas []*definition.Schema) error {
	_, err := (p.registry).CreateCollections(ctx, schemas)
	if err != nil {
		return err
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
	// TODO: IMPLEMENT THIS METHOD PROPERLY
	// 2026-07-09
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
	schemas := make([]*definition.Schema, len(entries))
	for i, entry := range entries {
		// For now, we'll just use the entry's schema. A more complete implementation
		// might fetch detailed metadata from each collection
		// using collection.Metadata()
		sc := entry.Versions[entry.ActiveVersion.String()].Schema
		schemas[i] = &sc
		collections[i] = &base.CollectionMetadata{
			Name:        entry.Name,
			Version:     entry.ActiveVersion,
			Description: entry.Description,
			Schema:      &sc,
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

func (p *basePersistence) Schema(ctx context.Context, id string, version ...string) (*definition.Schema, error) {
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

func (p *basePersistence) createRegistryExecutor(_ *definition.Schema) registry.RegistryExecutor {
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
	entry, err := p.registry.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get registry entry: %w", err)
	}

	targetVer := version
	if targetVer == nil {
		prev, err := findPreviousVersion(entry)
		if err != nil {
			return nil, fmt.Errorf("find previous version: %w", err)
		}
		targetVer = &prev
	}

	if _, ok := entry.Versions[*targetVer]; !ok {
		return nil, base.ErrVersionNotFound.WithMessage(fmt.Sprintf(
			"version '%s' not found for collection '%s'", *targetVer, name,
		))
	}

	if dryRun != nil && *dryRun {
		p.logger.Info("rollback dry-run",
			zap.String("collection", name),
			zap.String("from", entry.ActiveVersion.String()),
			zap.String("to", *targetVer),
		)
		return p.Collection(ctx, name)
	}

	p.logger.Info("rolling back collection",
		zap.String("collection", name),
		zap.String("from", entry.ActiveVersion.String()),
		zap.String("to", *targetVer),
	)

	if _, err := p.registry.SetActiveVersion(ctx, name, *targetVer); err != nil {
		return nil, fmt.Errorf("set active version: %w", err)
	}

	// Invalidate the cached collection so subsequent operations re-resolve the
	// schema and validator from the registry under the new version.
	p.collectionsMu.Lock()
	delete(p.collections, name)
	p.collectionsMu.Unlock()
	return p.Collection(ctx, name)
}

func (p *basePersistence) Migrate(
	ctx context.Context,
	name string,
	migrationParam any,
	dryRun *bool,
) (base.Collection, error) {
	plan, ok := migrationParam.(*base.MigrationPlan)
	if !ok {
		return nil, errors.New("migration must be a *base.MigrationPlan")
	}

	entry, err := p.registry.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get registry entry: %w", err)
	}

	currentSchema := entry.Versions[entry.ActiveVersion.String()].Schema

	if plan.Diff == nil {
		if err := plan.ComputeDiff(&currentSchema); err != nil {
			return nil, fmt.Errorf("compute diff: %w", err)
		}
		if plan.VersionBump == definition.BumpNone {
			plan.VersionBump = definition.VersionImpact(plan.Diff)
		}
	}

	targetVer := plan.TargetVersion(entry.ActiveVersion)
	verStr := targetVer.String()

	if plan.Target != nil {
		plan.Target.Version = targetVer
	}

	// Resolve phase at runtime using backend capabilities if not explicitly set
	plan.ResolvePhase(func(diff *definition.SchemaDiff) bool {
		return canApplyAllInPlace(diff, p.interactor.Capabilities().SchemaEvolution)
	})

	p.logger.Info("migrating collection",
		zap.String("collection", name),
		zap.String("from", entry.ActiveVersion.String()),
		zap.String("to", verStr),
		zap.String("bump", plan.VersionBump.String()),
		zap.String("phase", string(plan.Phase)),
	)

	if dryRun != nil && *dryRun {
		p.logger.Info("migration dry-run (preview only)",
			zap.String("collection", name),
			zap.String("target", verStr),
			zap.Int("changes", len(plan.Diff.Changes)),
			zap.String("phase", string(plan.Phase)),
		)
		return p.Collection(ctx, name)
	}

	physicalName, resolveErr := p.registry.ResolvePhysicalName(ctx, name)
	if resolveErr != nil {
		return nil, fmt.Errorf("resolve physical name: %w", resolveErr)
	}

	switch plan.Phase {
	case base.PhaseSchemaOnly:
		if err := applyDDLInPlace(ctx, p.interactor, physicalName, plan.Diff); err != nil {
			return nil, fmt.Errorf("apply DDL in place: %w", err)
		}

	case base.PhaseDDL:
		if err := applyDDLInPlace(ctx, p.interactor, physicalName, plan.Diff); err != nil {
			p.logger.Info("in-place DDL not fully supported, falling back to full copy",
				zap.String("collection", name),
				zap.Error(err),
			)
			return p.migrateWithCopy(ctx, name, plan, entry, currentSchema, *targetVer, verStr)
		}

	case base.PhaseFull:
		return p.migrateWithCopy(ctx, name, plan, entry, currentSchema, *targetVer, verStr)

	default:
		return nil, fmt.Errorf("unknown migration phase: %s", plan.Phase)
	}

	newEntry, err := p.registry.AddSchemaVersion(ctx, name, verStr, plan.Target, physicalName)
	if err != nil {
		return nil, fmt.Errorf("add schema version: %w", err)
	}

	if _, err := p.registry.SetActiveVersion(ctx, name, verStr); err != nil {
		return nil, fmt.Errorf("set active version: %w", err)
	}

	_ = newEntry

	// Invalidate the cached collection — the next CRUD operation on any live
	// reference will re-resolve the current schema and validator from the
	// registry under the active version.
	p.collectionsMu.Lock()
	delete(p.collections, name)
	p.collectionsMu.Unlock()
	return p.Collection(ctx, name)
}

// applyDDLInPlace iterates through the diff's SemanticChanges and applies DDL
// for those the backend supports natively (add/drop/rename column, add/drop index).
// Returns an error if any change cannot be applied via DDL.
func applyDDLInPlace(ctx context.Context, interactor query.DatabaseInteractor, collection string, diff *definition.SchemaDiff) error {
	sm := interactor.SchemaManager()
	caps := interactor.Capabilities()

	for _, change := range diff.Changes {
		switch change.Kind {
		case definition.FieldAdded:
			if !caps.SchemaEvolution.AddColumn {
				return fmt.Errorf("add column not supported by backend")
			}
			for _, op := range change.Forward {
				if op.Type == definition.OpAdd {
					if f, ok := op.Value.(definition.Field); ok {
						if err := sm.AddColumn(ctx, collection, f); err != nil {
							return err
						}
					}
				}
			}

		case definition.FieldRemoved:
			if !caps.SchemaEvolution.DropColumn {
				return fmt.Errorf("drop column not supported by backend")
			}
			if err := sm.DropColumn(ctx, collection, change.EntityId); err != nil {
				return err
			}

		case definition.FieldModified:
			for _, op := range change.Forward {
				if op.Type != definition.OpSet {
					continue
				}
				lastSeg := op.Path.Segments[len(op.Path.Segments)-1]
				switch lastSeg.Type {
				case definition.PathName:
					if !caps.SchemaEvolution.RenameColumn {
						return fmt.Errorf("rename column not supported by backend")
					}
				case definition.PathType:
					if !caps.SchemaEvolution.AlterColumnType {
						return fmt.Errorf("alter column type not supported by backend")
					}
				case definition.PathRequired, definition.PathDefault, definition.PathDeprecated:
					// These are metadata-only changes that don't require DDL
					continue
				}
			}

		case definition.IndexAdded:
			for _, op := range change.Forward {
				if op.Type == definition.OpAdd {
					if idx, ok := op.Value.(definition.Index); ok {
						if err := sm.CreateIndex(ctx, collection, idx); err != nil {
							return err
						}
					}
				}
			}

		case definition.IndexRemoved:
			idx := definition.Index{Name: change.EntityId}
			if err := sm.DropIndex(ctx, collection, idx); err != nil {
				return err
			}

		case definition.ConstraintAdded:
			if !caps.SchemaEvolution.AddConstraint {
				return fmt.Errorf("add constraint not supported by backend")
			}

		case definition.ConstraintRemoved:
			if !caps.SchemaEvolution.DropConstraint {
				return fmt.Errorf("drop constraint not supported by backend")
			}

		case definition.ConstraintModified, definition.SchemaAdded, definition.SchemaRemoved,
			definition.SchemaModified, definition.MetadataAdded, definition.MetadataModified,
			definition.MetadataRemoved, definition.RootModified, definition.IndexModified:
			// These are schema-level metadata changes that don't have a direct DDL equivalent.
			// For PhaseSchemaOnly we allow them (they'll be recorded in the new schema version).
			continue
		}
	}
	return nil
}

// canApplyAllInPlace checks whether every change in the diff can be applied
// using only in-place DDL on a backend with the given capabilities.
func canApplyAllInPlace(diff *definition.SchemaDiff, se query.SchemaEvolution) bool {
	for _, change := range diff.Changes {
		switch change.Kind {
		case definition.FieldAdded:
			if !se.AddColumn {
				return false
			}
		case definition.FieldRemoved:
			if !se.DropColumn {
				return false
			}
		case definition.FieldModified:
			for _, op := range change.Forward {
				if op.Type != definition.OpSet {
					continue
				}
				lastSeg := op.Path.Segments[len(op.Path.Segments)-1]
				switch lastSeg.Type {
				case definition.PathName:
					if !se.RenameColumn {
						return false
					}
				case definition.PathType:
					if !se.AlterColumnType {
						return false
					}
				}
			}
		case definition.ConstraintAdded:
			if !se.AddConstraint {
				return false
			}
		case definition.ConstraintRemoved:
			if !se.DropConstraint {
				return false
			}
		case definition.ConstraintModified, definition.SchemaAdded, definition.SchemaRemoved,
			definition.SchemaModified, definition.MetadataAdded, definition.MetadataModified,
			definition.MetadataRemoved, definition.RootModified, definition.IndexModified:
			// These are schema-level or metadata changes that have no DDL equivalent.
			// They are always safe for schema-only phase.
		case definition.IndexAdded, definition.IndexRemoved:
			// Index DDL is always supported (creates/drops index).
		}
	}
	return true
}

// migrateWithCopy performs a full migration: creates a new physical collection
// under the target version and copies/transforms all data.
func (p *basePersistence) migrateWithCopy(
	ctx context.Context,
	name string,
	plan *base.MigrationPlan,
	entry *base.RegistryEntry,
	_ definition.Schema,
	_ common.Version,
	verStr string,
) (base.Collection, error) {
	// Register the new version and create its physical table.
	// If the data migration below fails, we prune this version so the
	// caller can retry without hitting "version already exists".
	newEntry, err := p.registry.AddSchemaVersion(ctx, name, verStr, plan.Target)
	if err != nil {
		return nil, fmt.Errorf("add schema version: %w", err)
	}

	var migrateErr error
	if plan.Transformer != nil {
		p.logger.Info("running data migration",
			zap.String("collection", name),
			zap.String("target", verStr),
		)

		migrator := migration.NewDefaultDataMigrator(p.interactor, p.registry)

		wrappedTransformer := func(tctx context.Context, doc data.Document) (data.Document, error) {
			return plan.Transformer(tctx, doc)
		}

		jobID, dmErr := migrator.Migrate(
			ctx, name, entry.ActiveVersion.String(), verStr, wrappedTransformer,
		)
		if dmErr != nil {
			migrateErr = fmt.Errorf("data migration failed (job=%s): %w", jobID, dmErr)
		} else {
			p.logger.Info("data migration complete",
				zap.String("collection", name),
				zap.String("job", jobID),
			)
		}
	}

	if migrateErr != nil {
		// Prune the version we just registered so the caller can retry.
		if _, pruneErr := p.registry.PruneVersion(ctx, name, verStr); pruneErr != nil {
			p.logger.Error("failed to prune failed migration version",
				zap.String("collection", name),
				zap.String("version", verStr),
				zap.Error(pruneErr),
			)
		}
		return nil, migrateErr
	}

	if _, err := p.registry.SetActiveVersion(ctx, name, verStr); err != nil {
		return nil, fmt.Errorf("set active version: %w", err)
	}

	_ = newEntry

	p.collectionsMu.Lock()
	delete(p.collections, name)
	p.collectionsMu.Unlock()

	return p.Collection(ctx, name)
}

func findPreviousVersion(entry *base.RegistryEntry) (string, error) {
	var versions []*common.Version
	for vStr := range entry.Versions {
		v, err := common.NewVersion(vStr)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}

	if len(versions) == 0 {
		return "", base.ErrVersionNotFound.WithMessage("no previous version found")
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Compare(versions[j]) < 0
	})

	currentStr := entry.ActiveVersion.String()
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i].String() == currentStr {
			if i == 0 {
				return "", base.ErrVersionNotFound.WithMessage("no previous version to roll back to")
			}
			return versions[i-1].String(), nil
		}
	}

	return versions[len(versions)-1].String(), nil
}

func (p *basePersistence) Close(ctx context.Context) {
	if p.registry != nil {
		_ = p.registry.Close(ctx)
	}
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
