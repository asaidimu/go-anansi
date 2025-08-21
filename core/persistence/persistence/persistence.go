package persistence

import (
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"go.uber.org/zap"
)

func NewPersistence(
	interactor query.DatabaseInteractor,
	logger *zap.Logger,
	decorators *utils.Decorators,
) (base.Persistence, error) {

	if logger == nil {
		logger = zap.NewNop()
	}

	bus, err := createEventBus(logger)
	if err != nil {
		return nil, err
	}
	var collectionDecorators []utils.DecoratorFunc[base.Collection]

	if decorators != nil && decorators.CollectionDecorators != nil {
		collectionDecorators = decorators.CollectionDecorators
	}

	base, err := newBasePersistence(interactor, bus, logger, collectionDecorators)
	if err != nil {
		return nil, err
	}

	managed := newManagedPersistence(base)
	eventEmitting := newEventEmittingPersistence(managed, bus, logger)

	if decorators != nil {
		return utils.ApplyDecorators(eventEmitting, decorators.PersistenceDecorators), nil
	}

	return eventEmitting, nil
}


