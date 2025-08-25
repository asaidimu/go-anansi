package main

import (
	"database/sql"
	"log"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/example/api/model"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)



func SecurityDecorator(logger *zap.Logger) utils.DecoratorFunc[base.Collection] {
	return func(collection base.Collection) base.Collection {
		return collection
	}
}

func setup() (base.Persistence, func()) {
	// 1. Setup Logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 2. Setup In-Memory SQLite Database
	db, err := sql.Open("sqlite3", "database.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// 3. Create Database Interactor for SQLite
	executor, err := sqliteExecutor.NewSQLiteInteractor(db, logger)
	if err != nil {
		log.Fatalf("Failed to create SQLite interactor: %v", err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory)
	if err != nil {
		log.Fatalf("Failed to create native interactor: %v", err)
	}

	// 4. Setup Document Factory Config
	factoryConfig := data.DocumentFactoryConfig{
		HmacSecret: []byte("complex-example-secret-key"),
	}

	// 5. Setup Decorators
	decorators := &utils.Decorators{
		CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
			(utils.DecoratorFunc[base.Collection])(SecurityDecorator(logger)),
		},
	}


	schemas, err := model.GetAllSchemas()
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}

	// 7. Initialize Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    decorators,
		Schemas: schemas,
	}

	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")


	wrapped, err := NewDecoratedPersistence(p, logger)

	if err != nil {
		log.Fatal("Failed to create users model:", err)
	}

	return wrapped, func() { db.Close()}
}
