package collection

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-events"
	"go.uber.org/zap"
)

// NewCollection creates a new Collection instance, wrapping it with all necessary decorators.
func NewCollection(
	bus *events.TypedEventBus[base.PersistenceEvent],
	name string,
	sc *schema.SchemaDefinition,
	interactor query.DatabaseInteractor,
	engine *query.QueryEngine,
	logger *zap.Logger,
	resolveSchema func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error),
) (base.Collection, error) {
	base, err := newBaseCollection(bus, name, sc, interactor, engine, logger)

	// Decorate the base collection with the managed collection for metadata and versioning.
	managed, err := newManagedCollection(
		sc,
		name,
		sc.Name,
		base,
		resolveSchema)
	if err != nil {
		return nil, err
	}

	// Decorate the managed collection with event emission.
	eventEmitting := newEventEmittingCollection(managed, bus, sc, logger)
	return eventEmitting, nil
}
