package anansi

import (
	"context"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

// Package-level variables to ensure Setup runs only once
var (
	setupOnce           sync.Once
	persistenceInstance base.Persistence
	setupError          error
)

// SetupConfig holds configuration options for the Anansi Setup function.
type SetupConfig struct {
	// Interactor is the database interactor responsible for database connectivity.
	// This should be configured with the desired database implementation (e.g., SQLite, PostgreSQL).
	Interactor query.DatabaseInteractor
	// Logger is the zap logger instance for logging.
	Logger *zap.Logger
	// FactoryConfig is the configuration for the document factory.
	FactoryConfig data.DocumentFactoryConfig

	// Decorators allows applying custom decorators to the persistence and collection layers.
	Decorators *utils.Decorators

	// Schemas allows consumers to easily set up schemas
	Schemas []schema.SchemaDefinition
}

// Setup initializes and configures the Anansi persistence layer and document factory.
// It takes a SetupConfig struct containing all necessary configuration options.
// This function ensures that the setup process runs only once, even if called multiple times.
func Setup(config SetupConfig) (base.Persistence, error) {
	setupOnce.Do(func() {
		ctx := context.Background()
		// Configure the document factory
		if err := data.ConfigureDocumentFactory(config.FactoryConfig); err != nil {
			setupError = err
			return
		}

		// Initialize the persistence layer.
		p, err := persistence.NewPersistence(config.Interactor, config.Logger, config.Decorators)
		if err != nil {
			setupError = err
			return
		}

		persistenceInstance = p

		if len(config.Schemas) == 0 {
			return
		}

		newSchemas := make([]schema.SchemaDefinition, 0)
		for _, schema := range config.Schemas {
			ok, err := p.HasCollection(ctx, schema.Name);
			if err != nil {
				setupError = err
				return
			}

			if !ok {
				newSchemas = append(newSchemas, schema)
			}
		}

		// Create all collections in a single transaction
		err = p.CreateCollections(ctx, newSchemas)
		if err != nil {
			setupError = err
			return
		}

	})

	return persistenceInstance, setupError
}
