package main

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/example/api/model/user"
	"go.uber.org/zap"
)

type DecoratedPersistence struct {
	base.Persistence
	models map[string]func(ctx context.Context, p base.Persistence) (base.Collection, error)
}

// NewDecoratedPersistence creates a new decorated persistence layer
func NewDecoratedPersistence(persistence base.Persistence, logger *zap.Logger) (base.Persistence, error) {
	dp := &DecoratedPersistence{
		Persistence:    persistence,
		models: make(map[string]func(ctx context.Context, p base.Persistence) (base.Collection, error)),
	}

	dp.models[user.ModelName] = func(ctx context.Context, p base.Persistence) (base.Collection, error) {
		return user.GetUserModel(ctx, p, logger)
	}

	return dp, nil
}

func (dp *DecoratedPersistence) Collection(ctx context.Context, name string) (base.Collection, error) {
	// Check if we have a model constructor for this collection
	if constructor, exists := dp.models[name]; exists {
		return constructor(ctx, dp.Persistence)
	}

	// Return base collection
	return dp.Persistence.Collection(ctx, name)
}

