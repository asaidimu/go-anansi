package collection

import (
	"context"

	"github.com/asaidimu/go-anansi/v7/core/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"go.uber.org/zap"
)

// NewCollection creates a new Collection instance, wrapping it with all necessary decorators.
// The schema and validator are resolved on-demand through the provided SchemaProvider,
// so the collection always operates on the active schema version.
func NewCollection(
	eventEmitter *events.EventEmitter[base.PersistenceEvent],
	name string,
	provider base.SchemaProvider,
	interactor query.DatabaseInteractor,
	engine *query.QueryEngine,
	logger *zap.Logger,
	resolveSchema func(ctx context.Context, name string) (string, *definition.Schema, error),
	processor base.RawQueryProcessor,
) (base.Collection, error) {
	base, err := newBaseCollection(eventEmitter, name, provider, interactor, engine, logger)
	if err != nil {
		return nil, err
	}

	// Decorate the base collection with polyfills for missing database features.
	polyfilled := newPolyfillCollection(base, interactor, logger)

	// Resolve the initial physical name from the provider for managed collection setup.
	physicalName, err := provider.PhysicalName(context.Background())
	if err != nil {
		return nil, err
	}

	// Decorate the polyfilled collection with the managed collection for metadata and versioning.
	managed, err := newManagedCollection(
		provider,
		name,
		physicalName,
		polyfilled,
		resolveSchema,
		processor,
	)
	if err != nil {
		return nil, err
	}

	// Decorate the managed collection with event emission.
	eventEmitting := newEventEmittingCollection(name, managed, eventEmitter, logger)
	return eventEmitting, nil
}
