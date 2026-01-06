package app

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/example/api/schema"
	"go.uber.org/zap"
)

// PersistenceManager manages the Anansi persistence layer.
type PersistenceManager struct {
	Anansi base.Persistence
}

// NewPersistenceManager sets up the Anansi persistence layer.
func NewPersistenceManager(schemaLoader *schema.SchemaLoader, cfg *Config, logger *zap.Logger) (*PersistenceManager, func (), error) {
	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		Logger: logger,
		DBPath: cfg.DBPath,
		EnableLogging: true,
		EnableSanitization: true,
		CustomSanitizerConfig: data.NewSecureDefaultConfig(),
		Schemas:       schemaLoader.Schemas,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup Anansi: %w", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	return &PersistenceManager{
		Anansi: p,
	}, cleanup, nil
}

// Collection retrieves a collection by name.
func (pm *PersistenceManager) Collection(ctx context.Context, name string) (base.Collection, error) {
	return pm.Anansi.Collection(ctx, name)
}
