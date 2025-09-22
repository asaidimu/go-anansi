package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

const (
	numDocuments = 100000
	numWorkers   = 10
)

// User schema definition
func getUserSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "User",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":     {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"name":   {Name: "name", Type: "string", Required: coreutils.BoolPtr(true)},
			"email":  {Name: "email", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"age":    {Name: "age", Type: "integer", Required: coreutils.BoolPtr(true)},
			"active": {Name: "active", Type: "boolean", Required: coreutils.BoolPtr(true)},
		},
	}
}

func main() {
	// 1. Setup Logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 2. Setup In-Memory SQLite Database
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared&_mutex=full")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 3. Create Database Interactor for SQLite
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		log.Fatalf("Failed to create SQLite interactor: %v", err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		log.Fatalf("Failed to create native interactor: %v", err)
	}

	// 4. Setup Document Factory Config
	factoryConfig := data.DocumentFactoryConfig{
		HmacSecret: []byte("benchmark-secret-key"),
	}

	// 5. Setup Decorators
	decorators := &utils.Decorators{}

	// 6. Initialize Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    decorators,
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 7. Create "users" collection
	userSchema := getUserSchema()
	usersCollection, err := p.CreateCollection(ctx, *userSchema)
	if err != nil {
		log.Fatalf("Failed to create users collection: %v", err)
	}
	logger.Info("Users collection created.")

	// --- Run Benchmarks ---
	runBenchmarks(ctx, usersCollection, p, logger)
}

func runBenchmarks(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	logger.Info("--- Starting Benchmarks ---", zap.Int("workers", numWorkers))

	// Benchmark Create
	benchmarkCreate(ctx, collection, logger)

	// Benchmark Read (Single)
	benchmarkReadSingle(ctx, collection, logger)

	// Benchmark Read (Multiple)
	benchmarkReadMultiple(ctx, collection, logger)

	// Benchmark Update
	benchmarkUpdate(ctx, collection, logger)

	// Benchmark Delete
	benchmarkDelete(ctx, collection, p, logger)

	logger.Info("--- Benchmarks Finished ---")
}

func benchmarkCreate(ctx context.Context, collection base.Collection, logger *zap.Logger) {
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			for i := workerID; i < numDocuments; i += numWorkers {
				user := data.MustNewDocument(map[string]any{
					"id":     fmt.Sprintf("user-%d", i),
					"name":   fmt.Sprintf("User %d", i),
					"email":  fmt.Sprintf("user%d@example.com", i),
					"age":    rand.Intn(50) + 20,
					"active": true,
				})
				_, err := collection.CreateOne(ctx, user)
				if err != nil {
					logger.Error("Failed to create user", zap.Error(err))
				}
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("CREATE: %d documents in %s. (%.2f docs/sec)", numDocuments, elapsed, float64(numDocuments)/elapsed.Seconds()))
}

func benchmarkReadSingle(ctx context.Context, collection base.Collection, logger *zap.Logger) {
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			for i := workerID; i < numDocuments; i += numWorkers {
				query := query.NewQueryBuilder().Where("id").Eq(fmt.Sprintf("user-%d", i)).Build()
				_, err := collection.Read(ctx, &query)
				if err != nil {
					logger.Error("Failed to read user", zap.Error(err))
				}
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("READ (Single): %d documents in %s. (%.2f docs/sec)", numDocuments, elapsed, float64(numDocuments)/elapsed.Seconds()))
}

func benchmarkReadMultiple(ctx context.Context, collection base.Collection, logger *zap.Logger) {
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for range numWorkers {
		go func() {
			defer wg.Done()
			query := query.NewQueryBuilder().Where("age").Gt(30).Build()
			_, err := collection.Read(ctx, &query)
			if err != nil {
				logger.Error("Failed to read users", zap.Error(err))
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("READ (Multiple): Query 'age > 30' %d times in %s.", numWorkers, elapsed))
}

func benchmarkUpdate(ctx context.Context, collection base.Collection, logger *zap.Logger) {
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			for i := workerID; i < numDocuments; i += numWorkers {
				update := data.MustNewDocument(map[string]any{"age": rand.Intn(50) + 20})
				filter := query.NewQueryBuilder().Where("id").Eq(fmt.Sprintf("user-%d", i)).Build().Filters
				_, err := collection.Update(ctx, &base.CollectionUpdate{Filter: filter, Data: update})
				if err != nil {
					logger.Error("Failed to update user", zap.Error(err))
				}
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("UPDATE: %d documents in %s. (%.2f docs/sec)", numDocuments, elapsed, float64(numDocuments)/elapsed.Seconds()))
}

func benchmarkDelete(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	start := time.Now()

	p.Transact(ctx, func(tctx context.Context, t base.BasePersistence) (any, error) {
		for w := range numWorkers {
			t.Async(tctx, func(rctx context.Context) (any, error) {
				for i := w; i < numDocuments; i += numWorkers {
					filter := query.NewQueryBuilder().Where("id").Eq(fmt.Sprintf("user-%d", i)).Build().Filters
					_, err := collection.Delete(rctx, filter, false)
					if err != nil {
						logger.Error("Failed to delete user", zap.Error(err))
						return nil, err
					}
				}
				return nil, nil
			})
		}
		return nil, nil
	})

	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("DELETE: %d documents in %s. (%.2f docs/sec)", numDocuments, elapsed, float64(numDocuments)/elapsed.Seconds()))
}
