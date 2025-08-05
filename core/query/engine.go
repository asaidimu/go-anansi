package query

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

// QueryEngine is the central orchestrator for executing queries. It implements the new
// capabilities-based partitioning architecture.
type QueryEngine struct {
	Interactor       BaseDatabaseInteractor
	partitioner      *QueryPartitioner
	computeFunctions map[string]ComputeFunction
	filterFunctions  map[ComparisonOperator]PredicateFunction
	logger           *zap.Logger
	cache            QueryCache
}

// NewQueryEngine creates a new query executor.
func NewQueryEngine(interactor BaseDatabaseInteractor, logger *zap.Logger) *QueryEngine {
	if logger == nil {
		logger = zap.NewNop()
	}
	cache, _ := NewLRUCache(100)

	return &QueryEngine{
		Interactor:       interactor,
		partitioner:      NewQueryPartitioner(interactor.Capabilities()),
		computeFunctions: make(map[string]ComputeFunction),
		filterFunctions:  make(map[ComparisonOperator]PredicateFunction),
		logger:           logger,
		cache:            cache,
	}
}

// RegisterComputeFunction registers a custom compute function with the executor.
func (e *QueryEngine) RegisterComputeFunction(name string, fn ComputeFunction) {
	e.computeFunctions[name] = fn
}

// RegisterFilterFunction registers a custom filter function with the executor.
func (e *QueryEngine) RegisterFilterFunction(operator ComparisonOperator, fn PredicateFunction) {
	e.filterFunctions[operator] = fn
}

// Query orchestrates the entire query execution process, from partitioning to final result.
func (e *QueryEngine) Query(ctx context.Context, schemaDef *schema.SchemaDefinition, dsl *Query) ([]common.Document, error) {
	var dbQuery, postProcessingQuery *Query
	var err error

	if e.cache != nil {
		key, err := e.generateCacheKey(dsl)
		if err != nil {
			e.logger.Warn("Failed to generate cache key for query", zap.Error(err))
		} else {
			if cached, found := e.cache.Get(key); found {
				e.logger.Debug("Partitioned query cache hit", zap.Uint64("key", key))
				dbQuery = cached.DbQuery
				postProcessingQuery = cached.PostProcessingQuery
			} else {
				e.logger.Debug("Partitioned query cache miss", zap.Uint64("key", key))
			}
		}
	}

	if dbQuery == nil { // Cache miss or no cache
		startPartition := time.Now()
		dbQuery, postProcessingQuery, err = e.partitioner.Partition(dsl)
		partitionDuration := time.Since(startPartition)
		if err != nil {
			e.logger.Error("Query partitioning failed", zap.Error(err))
			return nil, fmt.Errorf("error partitioning query: %w", err)
		}
		e.logger.Debug("Query partitioned", zap.Duration("duration", partitionDuration))

		if e.cache != nil {
			key, _ := e.generateCacheKey(dsl) // Error already handled above
			e.cache.Set(key, &PartitionedQuery{DbQuery: dbQuery, PostProcessingQuery: postProcessingQuery})
		}
	}

	// 2. Execute the database part of the query.
	startDb := time.Now()
	docs, err := e.Interactor.SelectDocuments(ctx, schemaDef, dbQuery)
	dbDuration := time.Since(startDb)
	if err != nil {
		e.logger.Error("Database query execution failed", zap.Error(err))
		return nil, fmt.Errorf("database query execution failed: %w", err)
	}
	e.logger.Info("Database query executed", zap.Duration("duration", dbDuration), zap.Int("rows_returned", len(docs)))

	// 3. If there's no post-processing, we can return the results directly.
	if postProcessingQuery.IsEmpty() {
		e.logger.Debug("No post-processing required. Returning results directly.")
		return docs, nil
	}

	// 4. Execute the in-memory part of the query.
	startPostProcessing := time.Now()
	queryHelper, err := NewQueryHelper(postProcessingQuery, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create query helper for post-processing: %w", err)
	}

	// Register the custom functions with the helper.
	queryHelper.RegisterComputeFunctions(e.computeFunctions)
	queryHelper.RegisterFilterFunctions(e.filterFunctions)

	// 5. Apply post-processing steps.
	processedDocs, err := e.runPostProcessing(queryHelper, docs)
	if err != nil {
		return nil, err // Error is already descriptive
	}

	// 6. Apply the original, user-requested projection to the final dataset.
	queryHelper.query.Projection = dsl.Projection
	finalDocs, err := queryHelper.Project(processedDocs)
	if err != nil {
		e.logger.Error("Final projection failed", zap.Error(err))
		return nil, fmt.Errorf("final projection failed: %w", err)
	}

	postProcessingDuration := time.Since(startPostProcessing)
	e.logger.Info("Post-processing finished", zap.Duration("duration", postProcessingDuration), zap.Int("final_row_count", len(finalDocs)))

	return finalDocs, nil
}

func (e *QueryEngine) generateCacheKey(dsl *Query) (uint64, error) {
	bytes, err := json.Marshal(dsl)
	if err != nil {
		return 0, err
	}
	hasher := fnv.New64a()
	_, err = hasher.Write(bytes)
	if err != nil {
		return 0, err
	}
	return hasher.Sum64(), nil
}

func (e *QueryEngine) runPostProcessing(helper *QueryHelper, docs []common.Document) ([]common.Document, error) {
	processedDocs := docs
	var err error

	if helper.query.Filters != nil {
		processedDocs, err = helper.Filter(processedDocs)
		if err != nil {
			e.logger.Error("Post-processing filter failed", zap.Error(err))
			return nil, fmt.Errorf("post-processing filter failed: %w", err)
		}
	}

	// In-memory joins would be handled here if any were deferred.

	if len(helper.query.Aggregations) > 0 {
		// Aggregation returns a single result document, so we return it immediately.
		aggResult, err := helper.ApplyAggregations(processedDocs)
		if err != nil {
			e.logger.Error("Post-processing aggregation failed", zap.Error(err))
			return nil, fmt.Errorf("post-processing aggregation failed: %w", err)
		}
		return []common.Document{aggResult}, nil
	}

	processedDocs, err = helper.Sort(processedDocs)
	if err != nil {
		e.logger.Error("Post-processing sort failed", zap.Error(err))
		return nil, fmt.Errorf("post-processing sort failed: %w", err)
	}

	processedDocs, _, err = helper.Paginate(processedDocs)
	if err != nil {
		e.logger.Error("Post-processing pagination failed", zap.Error(err))
		return nil, fmt.Errorf("post-processing pagination failed: %w", err)
	}

	return processedDocs, nil
}
