package query

import (
        "context"
        "encoding/json"
        "hash/fnv"
        "math"
        "time"

        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/document"
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
        retryPolicy      RetryPolicy // from backend capabilities, used for read retry
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
                retryPolicy:      capabilities.Retry,
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
//
// Read retry: if the backend's Capabilities.Retry.MaxAttempts > 1 and the database
// returns a retryable error (e.g., lock contention), the read is automatically
// retried with exponential backoff. This only applies to the database read —
// post-processing (in-memory filtering, sorting, pagination) is never retried.
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

        // Attach provenance for direct container scanning.
        dbQuery.Shape = InferShape(dbQuery)
        dbQuery.DocumentPool = dsl.DocumentPool

        // Execute the database part of the query with retry on transient errors.
        result, err := e.executeRead(ctx, interactor, schemaDef, dsl, dbQuery)
        if err != nil {
                return nil, common.NewSystemError("ERR_QUERY_DB_EXECUTION_FAILED", "database query execution failed").WithOperation("Query").WithCause(err)
        }

        // If there's no post-processing, return immediately.
        if postProcessingQuery.IsEmpty() {
                return result, nil
        }

        // Execute the in-memory part of the query.
        queryHelper, err := NewQueryHelper(postProcessingQuery, nil, nil, nil)
        if err != nil {
                return nil, common.NewSystemError("ERR_QUERY_HELPER_CREATION_FAILED", "failed to create query helper for post-processing").WithOperation("Query").WithCause(err)
        }

        queryHelper.RegisterComputeFunctions(e.computeFunctions)
        queryHelper.RegisterFilterFunctions(e.filterFunctions)

        rawDocs := make([]map[string]any, 0, len(result.Data))
        for _, d := range result.Data {
                rawDocs = append(rawDocs, d.ToMap())
        }
        processedDocs, err := e.runPostProcessing(queryHelper, rawDocs)
        if err != nil {
                return nil, err
        }

        queryHelper.query.Projection = dsl.Projection
        finalDocs, err := queryHelper.Project(processedDocs)
        if err != nil {
                return nil, common.NewSystemError("ERR_QUERY_FINAL_PROJECTION_FAILED", "final projection failed").WithOperation("Query").WithCause(err)
        }

        final := make([]*document.Document, 0, len(finalDocs))
        for _, m := range finalDocs {
                final = append(final, document.NewRecordView(m))
        }

        return &QueryResult{
                Data:           final,
                Count:          len(final),
                Total:          result.Total,
                PaginationInfo: computePaginationInfo(dsl.Pagination, len(final), result.Total),
        }, nil
}

// executeRead runs SelectDocuments with retry on transient errors.
// This is separated from Query so the retry logic is isolated to the
// database round-trip. Post-processing is never retried.
//
// An explicit loop (rather than common.Retry) is used so that RetryContext
// can be injected into the context before each attempt, allowing decorators
// to suppress non-idempotent side effects on retries.
func (e *QueryEngine) executeRead(
        ctx context.Context,
        interactor DatabaseInteractor,
        schemaDef *definition.Schema,
        dsl *Query,
        dbQuery *Query,
) (*QueryResult, error) {
        if e.retryPolicy.MaxAttempts <= 1 {
                return e.selectDocuments(ctx, interactor, schemaDef, dsl, dbQuery)
        }

        policy := e.retryPolicy

        // MaxTotalDuration circuit breaker.
        var deadline time.Time
        if policy.MaxTotalDuration > 0 {
                deadline = time.Now().Add(policy.MaxTotalDuration)
        }

        var lastErr error
        for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
                // Check circuit-breaker deadline before each attempt.
                if !deadline.IsZero() && time.Now().After(deadline) {
                        return nil, lastErr
                }

                if attempt > 0 {
                        delay := common.CalculateBackoff(policy, attempt)

                        if !deadline.IsZero() {
                                remaining := time.Until(deadline)
                                if remaining <= 0 {
                                        return nil, lastErr
                                }
                                if delay > remaining {
                                        delay = remaining
                                }
                        }

                        if policy.Jitter {
                                delay = time.Duration(common.JitterDelay(delay))
                                if delay == 0 {
                                        delay = time.Nanosecond
                                }
                        }

                        timer := time.NewTimer(delay)
                        select {
                        case <-ctx.Done():
                                timer.Stop()
                                return nil, ctx.Err()
                        case <-timer.C:
                        }

                        e.logger.Warn("retrying read query",
                                zap.Int("attempt", attempt),
                                zap.String("collection", schemaDef.Name),
                                zap.Error(lastErr),
                        )
                }

                // Inject RetryContext so decorators can suppress side effects.
                attemptCtx := common.WithRetryContext(ctx, common.RetryContext{Attempt: attempt})

                result, err := e.selectDocuments(attemptCtx, interactor, schemaDef, dsl, dbQuery)
                if err == nil {
                        return result, nil
                }
                lastErr = err

                if !common.IsRetryableError(err) {
                        return nil, err
                }
        }
        return nil, lastErr
}

// selectDocuments is a single read attempt — no retry.
func (e *QueryEngine) selectDocuments(
        ctx context.Context,
        interactor DatabaseInteractor,
        schemaDef *definition.Schema,
        dsl *Query,
        dbQuery *Query,
) (*QueryResult, error) {
        data, count, err := interactor.SelectDocuments(ctx, schemaDef, dbQuery)
        if err != nil {
                return nil, err
        }
        total := int(count)
        return &QueryResult{
                Data:           data,
                Count:          len(data),
                Total:          &total,
                PaginationInfo: computePaginationInfo(dsl.Pagination, len(data), &total),
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

        if len(helper.query.Aggregations) > 0 {
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
