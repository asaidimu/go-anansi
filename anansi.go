// Package anansi is the top-level entry point for the Anansi persistence platform.
//
// It provides two ways to initialise the system:
//
//  1. Setup – the *production* path.  Full control over every component
//     (database interactor, logger, event bus, decorators, schemas…).
//
//  2. Playground – a *development-only* helper that spins up a complete
//     SQLite-based environment with optional logging and events.  It is
//     **not** intended for production; the function returns a cleanup
//     closure that must be called on shutdown.
//
// The package guarantees that Setup (and, by extension, Playground) is
// executed only once per process via sync.Once.  Subsequent calls return
// the same Persistence instance and cached error.
package anansi

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	u "github.com/asaidimu/go-anansi/v6/utils"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------
//  Global once-guard
// ---------------------------------------------------------------------

var (
	// setupOnce guarantees that the persistence layer is configured exactly once.
	setupOnce sync.Once

	// persistenceInstance holds the singleton Persistence after a successful Setup/Playground.
	persistenceInstance base.Persistence

	// setupError caches the first error encountered during initialisation.
	setupError error
)

// ---------------------------------------------------------------------
//  Production configuration
// ---------------------------------------------------------------------

// SetupConfig contains every knob required for a production-grade
// Anansi deployment.
type SetupConfig struct {
	// Interactor is the concrete database implementation (SQLite, PostgreSQL,
	// MySQL, …).  It must satisfy query.DatabaseInteractor.
	Interactor query.DatabaseInteractor

	// Logger is a *zap.Logger used throughout the framework.  Production
	// code should supply a configured logger (JSON, stackdriver, etc.).
	Logger *zap.Logger

	// EventBus is the pub/sub backbone.  Use utils.NewWatermillEventBus in
	// production; it integrates with external message brokers if desired.
	EventBus events.EventBus[base.PersistenceEvent]

	// DocumentFactoryConfig configures the Document factory (hashing, metadata, etc.).
	DocumentFactoryConfig data.DocumentFactoryConfig

	// Decorators inject cross-cutting concerns (security, audit, encryption…).
	Decorators *utils.Decorators

	// Schemas are created automatically on first start if they do not exist.
	Schemas []schema.SchemaDefinition
}

// Setup builds the persistence layer.  It is safe to call multiple times –
// only the first invocation performs work.  Returns the singleton
// Persistence and any error from the initial call.
func Setup(config SetupConfig) (base.Persistence, error) {
	setupOnce.Do(func() {
		if config.Logger == nil {
			config.Logger = zap.NewNop()
		}

		ctx := context.Background()

			// config.SanitizerConfig,
			// config.CollectionSanitizerConfigs,
		// 1. Initialise the global Document factory.
		if err := data.ConfigureDocumentFactory(config.DocumentFactoryConfig, config.Logger); err != nil {
			setupError = err
			return
		}


		// 2. Core persistence object.
		p, err := persistence.NewPersistence(
			config.Interactor,
			config.EventBus,
			config.Logger,
			config.Decorators,
		)
		if err != nil {
			setupError = err
			return
		}
		persistenceInstance = p

		// 3. Auto-create any supplied schemas that are missing.
		if len(config.Schemas) == 0 {
			return
		}

		newSchemas := make([]schema.SchemaDefinition, 0, len(config.Schemas))
		for _, s := range config.Schemas {
			exists, err := p.HasCollection(ctx, s.Name) // We check for the existence of the collection so as not to re-create it
			if err != nil {
				setupError = err
				return
			}
			if !exists {
				newSchemas = append(newSchemas, s)
			}
		}

		if err := p.CreateCollections(ctx, newSchemas); err != nil {
			setupError = err
			return
		}
	})

	return persistenceInstance, setupError
}

// ---------------------------------------------------------------------
//  Development / Playground
// ---------------------------------------------------------------------

// PlaygroundConfig controls the dev-only environment.
type PlaygroundConfig struct {
	// Logger is a *zap.Logger used throughout the framework.  Production
	// code should supply a configured logger (JSON, stackdriver, etc.).
	Logger *zap.Logger

	// DBPath is the SQLite DSN.
	//   * ":memory:"  -> in-memory (default)
	//   * "file.db"   -> persistent file on disk
	DBPath string

	// EnableLogging turns on zap.NewDevelopment() output.
	EnableLogging bool

	// EnableEvents spins up a WatermillEventBus (pub/sub) with the logger.
	EnableEvents bool

	// Schemas are created automatically on first start if they do not exist.
	Schemas []schema.SchemaDefinition

	// EnableSanitization adds default sanitization patterns to protect
	// sensitive data in logs and events. Recommended for any playground
	// that handles real or realistic test data.
	EnableSanitization bool

	// CustomSanitizerConfig allows custom sanitization configuration.
	// If nil and EnableSanitization is true, uses NewSecureDefaultConfig().
	CustomSanitizerConfig *data.FieldMaskConfig
}

// Playground returns a fully-functional Persistence together with a
// cleanup function that **must** be deferred in dev code.
//
//	p, cleanup, err := anansi.Playground(...)
//	defer cleanup()
//
// It is deliberately **not** part of the production path – the function
// panics if used after a real Setup has already run.
func Playground(cfg PlaygroundConfig) (base.Persistence, func(), error) {
	// Default to in-memory if the caller omitted a path.
	if cfg.DBPath == "" {
		cfg.DBPath = ":memory:"
	}

	// -----------------------------------------------------------------
	//  Logger
	// -----------------------------------------------------------------
	var logger *zap.Logger = cfg.Logger
	if cfg.EnableLogging && logger == nil {
		l, err := zap.NewDevelopment()
		if err != nil {
			return nil, nil, fmt.Errorf("playground: logger creation failed: %w", err)
		}
		logger = l
	} else {
		logger = zap.NewNop()
	}

	// -----------------------------------------------------------------
	//  Event Bus
	// -----------------------------------------------------------------
	var bus events.EventBus[base.PersistenceEvent]
	var busCleanup func()
	if cfg.EnableEvents {
		b := u.NewWatermillEventBus[base.PersistenceEvent](logger)
		busCleanup = func() { _ = b.Close() }
		bus = b
	}

	// -----------------------------------------------------------------
	//  Sanitization
	// -----------------------------------------------------------------
	var sanitizerConfig *data.FieldMaskConfig
	if cfg.EnableSanitization {
		if cfg.CustomSanitizerConfig != nil {
			sanitizerConfig = cfg.CustomSanitizerConfig
		} else {
			defaultConfig := data.NewSecureDefaultConfig()
			sanitizerConfig = defaultConfig
		}
	}

	// -----------------------------------------------------------------
	//  Database
	// -----------------------------------------------------------------
	dsn := cfg.DBPath
	if cfg.DBPath != ":memory:" {
		dsn = fmt.Sprintf("file:%s?cache=shared&_fk=1", cfg.DBPath)
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, func() {}, err
	}

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		db.Close()
		return nil, func() {}, err
	}

	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		db.Close()
		return nil, func() {}, err
	}

	cleanup := func() {
		_ = db.Close()
		if busCleanup != nil {
			busCleanup()
		}
	}

	p, err := Setup(SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		EventBus:      bus,
		DocumentFactoryConfig: data.DocumentFactoryConfig{
			GlobalSanitizer: sanitizerConfig,
		},
		Decorators:    &utils.Decorators{},
		Schemas:       cfg.Schemas,
	})

	if err != nil {
		cleanup()
		return nil, nil, err
	}

	return p, cleanup, nil
}
