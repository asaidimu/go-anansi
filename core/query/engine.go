package query

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"go.uber.org/zap"
)

// QueryEngine is the central orchestrator for executing queries. It implements the new
// capabilities-based partitioning architecture.
type QueryEngine struct {
	partitioner      *QueryPartitioner
	computeFunctions map[string]ComputeFunction
	filterFunctions  map[ComparisonOperator]PredicateFunction
	logger           *zap.Logger
	cache            QueryCache
}

// NewQueryEngine creates a new query executor.
func NewQueryEngine(capabilities Capabilities, logger *zap.Logger) *QueryEngine {
	if logger == nil {
		logger = zap.NewNop()
	}
	cache, err := NewLRUCache(100)
	if err != nil {
		logger.Error("Failed to create LRU cache for query engine", zap.Error(err))
	}

	return &QueryEngine{
		partitioner:      NewQueryPartitioner(capabilities),
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
func (e *QueryEngine) Query(ctx context.Context, schemaDef *definition.Schema, dsl *Query) (*QueryResult, error) {
	interactor, ok := GetInteractor(ctx)
	if !ok {
		return nil, common.NewSystemError("ERR_QUERY_INTERACTOR_NOT_FOUND", "could not get interactor").WithOperation("Query")
	}
	var dbQuery, postProcessingQuery *Query
	var err error

	if e.cache != nil {
		key, err := e.generateCacheKey(dsl)
		if err == nil {
			if cached, found := e.cache.Get(key); found {
				dbQuery = cached.DbQuery
				postProcessingQuery = cached.PostProcessingQuery
			}
		}
	}

	if dbQuery == nil { // Cache miss or no cache
		dbQuery, postProcessingQuery, err = e.partitioner.Partition(dsl)
		if err != nil {
			return nil, common.NewSystemError("ERR_QUERY_PARTITIONING_FAILED", "error partitioning query").WithOperation("Query").WithCause(err)
		}

		if e.cache != nil {
			key, _ := e.generateCacheKey(dsl) // Error already handled above
			e.cache.Set(key, &PartitionedQuery{DbQuery: dbQuery, PostProcessingQuery: postProcessingQuery})
		}
	}

	// 2. Execute the database part of the query.
	result, err := common.ExecuteWithContext(ctx, func() (*QueryResult, error) {
		data, count, err := interactor.SelectDocuments(ctx, schemaDef, dbQuery)
		total := int(count)
		return &QueryResult{Data: data, Count: len(data), Total: &total, PaginationInfo: computePaginationInfo(dsl.Pagination, len(data), &total)}, err
	})

	if err != nil {
		return nil, common.NewSystemError("ERR_QUERY_DB_EXECUTION_FAILED", "database query execution failed").WithOperation("Query").WithCause(err)
	}

	// 3. If there's no post-processing, we can return the results directly.
	if postProcessingQuery.IsEmpty() {
		return result, nil
	}

	// 4. Execute the in-memory part of the query.
	queryHelper, err := NewQueryHelper(postProcessingQuery, nil, nil, nil)
	if err != nil {
		return nil, common.NewSystemError("ERR_QUERY_HELPER_CREATION_FAILED", "failed to create query helper for post-processing").WithOperation("Query").WithCause(err)
	}

	// Register the custom functions with the helper.
	queryHelper.RegisterComputeFunctions(e.computeFunctions)
	queryHelper.RegisterFilterFunctions(e.filterFunctions)

	// 5. Apply post-processing steps.
	processedDocs, err := e.runPostProcessing(queryHelper, result.Data)
	if err != nil {
		return nil, err // Error is already descriptive
	}

	// 6. Apply the original, user-requested projection to the final dataset.
	queryHelper.query.Projection = dsl.Projection
	finalDocs, err := queryHelper.Project(processedDocs)
	if err != nil {
		return nil, common.NewSystemError("ERR_QUERY_FINAL_PROJECTION_FAILED", "final projection failed").WithOperation("Query").WithCause(err)
	}

	return &QueryResult{
		Data:           finalDocs,
		Count:          len(finalDocs),
		Total:          result.Total,
		PaginationInfo: computePaginationInfo(dsl.Pagination, len(finalDocs), result.Total),
	}, nil
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

// computePaginationInfo derives PaginationInfo from the original pagination options and query results.
func computePaginationInfo(pagination *PaginationOptions, count int, total *int) *PaginationInfo {
	if total == nil {
		return nil
	}
	t := *total

	switch {
	case pagination == nil || pagination.Type == "" || pagination.Limit <= 0:
		return &PaginationInfo{
			Number: 1,
			Size:   int(math.Min(float64(count), float64(t))),
			Count:  count,
			Total:  t,
			Pages:  1,
		}

	case pagination.Type == PaginationTypeCursor:
		return nil

	default:
			offset := 0
		if pagination.Offset != nil {
			offset = *pagination.Offset
		}
		pageNumber := offset/pagination.Limit + 1
		totalPages := t / pagination.Limit
		if t%pagination.Limit != 0 {
			totalPages++
		}
		return &PaginationInfo{
			Number: pageNumber,
			Size:   int(math.Min(float64(count), float64(pagination.Limit))),
			Count:  count,
			Total:  t,
			Pages:  totalPages,
		}
	}
}

func (e *QueryEngine) runPostProcessing(helper *QueryHelper, docs []map[string]any) ([]map[string]any, error) {
	processedDocs := docs
	var err error

	if helper.query.Filters != nil {
		processedDocs, err = helper.Filter(processedDocs)
		if err != nil {
			return nil, common.NewSystemError("ERR_QUERY_POST_PROCESSING_FILTER_FAILED", "post-processing filter failed").WithOperation("runPostProcessing").WithCause(err)
		}
	}

	// In-memory joins would be handled here if any were deferred.

	if len(helper.query.Aggregations) > 0 {
		// Aggregation returns a single result document, so we return it immediately.
		aggResult, err := helper.ApplyAggregations(processedDocs)
		if err != nil {
			return nil, common.NewSystemError("ERR_QUERY_POST_PROCESSING_AGGREGATION_FAILED", "post-processing aggregation failed").WithOperation("runPostProcessing").WithCause(err)
		}
		return []map[string]any{aggResult}, nil
	}

	processedDocs, err = helper.Sort(processedDocs)
	if err != nil {
		return nil, common.NewSystemError("ERR_QUERY_POST_PROCESSING_SORT_FAILED", "post-processing sort failed").WithOperation("runPostProcessing").WithCause(err)
	}

	processedDocs, _, err = helper.Paginate(processedDocs)
	if err != nil {
		return nil, common.NewSystemError("ERR_QUERY_POST_PROCESSING_PAGINATION_FAILED", "post-processing pagination failed").WithOperation("runPostProcessing").WithCause(err)
	}

	return processedDocs, nil
}
