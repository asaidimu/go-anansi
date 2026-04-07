package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

const (
	// numDocuments is the total number of documents to process in the benchmark.
	numDocuments = 100000
	// numWorkers is the number of concurrent goroutines used in Transact/Async operations.
	numWorkers = 10
)

// getUserSchema defines the structure of the "User" documents.
func getUserSchema() *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "User",
			Fields: map[definition.FieldId]definition.Field{
				// "ida" is the primary unique identifier used for lookups
				"ida":    {Name: "ida", Required: true, Unique: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name":   {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"email":  {Name: "email", Required: true, Unique: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":    {Name: "age", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"active": {Name: "active", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"key_ida": {
					Name:   "key_ida",
					Fields: []definition.FieldId{"ida"},
					Type:   definition.IndexTypeNormal,
					Unique: true,
				},
				"key_age": {
					Name:   "key_age",
					Fields: []definition.FieldId{"age"},
					Type:   definition.IndexTypeNormal,
				},
			},
		},
	}
}

func main() {
	start := time.Now()

	// 1. Setup Logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 2. Setup SQLite Database (using WAL mode for better concurrency)
	// We use the same configuration as the original file for consistency.
	db, err := sql.Open("sqlite3", "anansi.db?_mutex=full&_cache_size=10000&_journal_mode=WAL&_synchronous=NORMAL")
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

	// 4. Setup Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		DocumentFactoryConfig: data.DocumentFactoryConfig{},
		Decorators:    &utils.Decorators{},
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("Anansi persistence layer initialized successfully in %s.", elapsed))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 5. Create "users" collection (Users table must exist for benchmarks)
	userSchema := getUserSchema()
	usersCollection, err := p.CreateCollection(ctx, userSchema)
	if err != nil {
		log.Fatalf("Failed to create users collection: %v", err)
	}
	logger.Info("Users collection created.")

	// --- Run Benchmarks ---
	runBenchmarks(ctx, usersCollection, p, logger)
}

func runBenchmarks(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	logger.Info("--- Starting Benchmarks ---", zap.Int("workers", numWorkers))

	// 1. Benchmark Create (Data Seeding)
	// This must run first and successfully commit all 1M documents
	// to ensure subsequent READ, UPDATE, and DELETE benchmarks operate on real data.
	benchmarkCreate(ctx, collection, p, logger)

	// 2. Benchmark Read (Single) - Read 1M documents
	benchmarkReadSingle(ctx, collection, p, logger)

	// 3. Benchmark Read (Multiple) - Query 10 times
	benchmarkReadMultiple(ctx, collection, p, logger)

	// 4. Benchmark Update - Update 1M documents
	benchmarkUpdate(ctx, collection, p, logger)

	// 5. Benchmark Delete - Delete 1M documents
	benchmarkDelete(ctx, collection, p, logger)

	logger.Info("--- Benchmarks Finished ---")
}

func benchmarkCreate(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	start := time.Now()

	// FIX: Use the correct loop structure to distribute all numDocuments across numWorkers.
	p.Transact(ctx, func(tctx context.Context, t base.BasePersistence) (any, error) {
		for w := 0; w < numWorkers; w++ {
			// This goroutine handles a slice of the total documents
			t.Async(tctx, func(rctx context.Context) (any, error) {
				for i := w; i < numDocuments; i += numWorkers {
					// FIX: Ensure 'ida' is unique by using the document index 'i'
					user := data.MustNewDocument(map[string]any{
						"ida":    fmt.Sprintf("user-%d", i),
						"name":   fmt.Sprintf("User %d", i),
						"email":  fmt.Sprintf("user%d@example.com", i),
						"age":    rand.Intn(50) + 20, // Age 20-69
						"active": true,
					})
					// Use 'ctx' for collection operations based on original implementation
					_, err := collection.CreateOne(ctx, user)
					if err != nil {
						logger.Error("Failed to create user", zap.Int("user_id", i), zap.Error(err))
						// We allow returning the error to halt the transaction if necessary
						return nil, err
					}
				}
				return nil, nil
			})
		}
		return nil, nil
	})

	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("CREATE: %d documents in %s. (%.2f docs/sec)", numDocuments, elapsed, float64(numDocuments)/elapsed.Seconds()))
}

func benchmarkReadSingle(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	start := time.Now()

	// FIX: Use the correct loop structure to distribute all numDocuments across numWorkers.
	p.Transact(ctx, func(tctx context.Context, t base.BasePersistence) (any, error) {
		for w := 0; w < numWorkers; w++ {
			// This goroutine handles a slice of the total documents
			t.Async(tctx, func(rctx context.Context) (any, error) {
				for i := w; i < numDocuments; i += numWorkers {
					// Query for a specific document by its unique 'ida'
					query := query.NewQueryBuilder().Where("ida").Eq(fmt.Sprintf("user-%d", i)).Build()
					_, err := collection.Read(ctx, &query)
					if err != nil {
						logger.Error("Failed to read user", zap.Int("user_id", i), zap.Error(err))
						return nil, err
					}
				}
				return nil, nil
			})
		}
		return nil, nil
	})

	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("READ (Single): %d documents in %s. (%.2f docs/sec)", numDocuments, elapsed, float64(numDocuments)/elapsed.Seconds()))
}

func benchmarkReadMultiple(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	start := time.Now()
	// NOTE: This test remains largely unchanged as it was originally designed to run a small number of parallel queries (10).
	// We run 10 separate, concurrent queries against the 'age' field (which should benefit from an index).
	p.Transact(ctx, func(tctx context.Context, t base.BasePersistence) (any, error) {
		for w := range numWorkers {
			t.Async(tctx, func(rctx context.Context) (any, error) {
				query := query.NewQueryBuilder().Where("age").Gt(30).Build()
				starta := time.Now()
				_, err := collection.Read(ctx, &query)
				elapseda := time.Since(starta)
				if err != nil {
					logger.Error("Failed to read users", zap.Error(err))
					return nil, err
				}
				logger.Info(fmt.Sprintf("READ (%d): Query 'age > 30' in %s.", w, elapseda))
				return nil, nil
			})
		}
		return nil, nil
	})

	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("READ (Multiple): Query 'age > 30' %d times in %s.", numWorkers, elapsed))
}

func benchmarkUpdate(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	start := time.Now()

	// This function had the correct logic and is retained for consistency.
	p.Transact(ctx, func(tctx context.Context, t base.BasePersistence) (any, error) {
		for w := 0; w < numWorkers; w++ {
			t.Async(tctx, func(rctx context.Context) (any, error) {
				for i := w; i < numDocuments; i += numWorkers {
					// Update age to a new random value
					update := data.MustNewDocument(map[string]any{"age": rand.Intn(50) + 20})
					filter := query.NewQueryBuilder().Where("ida").Eq(fmt.Sprintf("user-%d", i)).Build().Filters
					_, err := collection.Update(ctx, &base.CollectionUpdate{Filter: filter, Set: update})
					if err != nil {
						logger.Error("Failed to update user", zap.Int("user_id", i), zap.Error(err))
						return nil, err
					}
				}
				return nil, nil
			})
		}
		return nil, nil
	})

	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("UPDATE: %d documents in %s. (%.2f docs/sec)", numDocuments, elapsed, float64(numDocuments)/elapsed.Seconds()))
}

func benchmarkDelete(ctx context.Context, collection base.Collection, p base.Persistence, logger *zap.Logger) {
	start := time.Now()

	// This function had the correct distribution logic but now runs on a correctly populated dataset.
	p.Transact(ctx, func(tctx context.Context, t base.BasePersistence) (any, error) {
		for w := 0; w < numWorkers; w++ {
			t.Async(tctx, func(rctx context.Context) (any, error) {
				for i := w; i < numDocuments; i += numWorkers {
					// Delete each document individually
					filter := query.NewQueryBuilder().Where("ida").Eq(fmt.Sprintf("user-%d", i)).Build().Filters
					_, err := collection.Delete(rctx, filter, false)
					if err != nil {
						logger.Error("Failed to delete user", zap.Int("user_id", i), zap.Error(err))
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
