package app

import (
	"context"
	"fmt"
	"time"

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
func NewPersistenceManager(db *Database, schemaLoader *schema.SchemaLoader, cfg *Config, logger *zap.Logger) (*PersistenceManager, error) {
	factoryConfig := data.DocumentFactoryConfig{
		GlobalSanitizer: &data.FieldMaskConfig{
			Patterns:      data.CommonSecurityPatterns(),
			DefaultPolicy: data.MaskPreserve,
			ObscureConfig: data.DefaultObscureConfig(),
			Fields: map[string]data.MaskedFieldPolicy{
				"checksum": data.MaskObscure,
				"role": data.MaskHash,
			},
		},
	}

	setupCfg := anansi.SetupConfig{
		Interactor:            db.Interactor,
		Logger:                logger,
		DocumentFactoryConfig: factoryConfig,
		Schemas:               schemaLoader.Schemas,
	}
	p, err := anansi.Setup(setupCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to setup Anansi: %w", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	// Create collections for the loaded schemas
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, s := range schemaLoader.Schemas {
		_, err := p.Collection(ctx, s.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to create collection %s: %w", s.Name, err)
		}
		logger.Info(fmt.Sprintf("Collection '%s' created.", s.Name))
	}

	return &PersistenceManager{
		Anansi: p,
	}, nil
}

// Collection retrieves a collection by name.
func (pm *PersistenceManager) Collection(ctx context.Context, name string) (base.Collection, error) {
	return pm.Anansi.Collection(ctx, name)
}
