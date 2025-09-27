package persistence

import (
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	cevents "github.com/asaidimu/go-anansi/v6/core/events"
	"go.uber.org/zap"
)

func NewPersistence(
	interactor query.DatabaseInteractor,
	bus cevents.EventBus[base.PersistenceEvent],
	logger *zap.Logger,
	decorators *utils.Decorators,
) (base.Persistence, error) {

	if logger == nil {
		logger = zap.NewNop()
	}

	var collectionDecorators []utils.DecoratorFunc[base.Collection]

	if decorators != nil && decorators.CollectionDecorators != nil {
		collectionDecorators = decorators.CollectionDecorators
	}

	factory := events.NewPersistenceEventFactory("__anansi_persistence__", logger)
	eventEmitter := cevents.NewEventEmitter(bus, factory.CreateEvent, logger)

	base, err := newBasePersistence(interactor, eventEmitter, logger, collectionDecorators)
	if err != nil {
		return nil, err
	}


	managed := newManagedPersistence(base)
	eventEmitting := newEventEmittingPersistence(managed, eventEmitter, logger)

	if decorators != nil {
		return utils.ApplyDecorators(eventEmitting, decorators.PersistenceDecorators), nil
	}

	return eventEmitting, nil
}


