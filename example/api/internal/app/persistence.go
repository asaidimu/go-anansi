package app

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v7"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/example/api/schema"
	"github.com/asaidimu/go-anansi/v7/utils"
	"go.uber.org/zap"
)

// PersistenceManager manages the Anansi persistence layer.
type PersistenceManager struct {
	Anansi base.Persistence
}

// NewPersistenceManager sets up the Anansi persistence layer.
func NewPersistenceManager(schemaLoader *schema.SchemaLoader, cfg *Config, logger *zap.Logger) (*PersistenceManager, func(), error) {
	db, err := NewDatabase(cfg, logger)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup Anansi: %w", err)
	}
	p, err := anansi.Setup(anansi.SetupConfig{
		Logger:  logger,
		Schemas: schemaLoader.Schemas,
		Interactor: db.Interactor,
	})

	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup Anansi: %w", err)
	}

	logger.Info("Anansi persistence layer initialized successfully.")

	sanitizationPolicyStore, err := utils.NewSanitizationPolicyStore(p, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup Anansi: %w", err)
	}
	reg := data.GetSanitizationRegistry()
	reg.SetPersistence(sanitizationPolicyStore)
	err = reg.LoadFromPersistence(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup Anansi: %w", err)
	}

	return &PersistenceManager{
		Anansi: p,
	}, func() { db.Close() }, nil
}

// Collection retrieves a collection by name.
func (pm *PersistenceManager) Collection(ctx context.Context, name string) (base.Collection, error) {
	return pm.Anansi.Collection(ctx, name)
}
