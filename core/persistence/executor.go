package persistence

import (
	"context"

	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
	"go.uber.org/zap"
)

// Executor orchestrates database operations by coordinating between QueryExecutor and DataProcessor.
type Executor struct {
	queryExecutor DatabaseInteractor
	dataProcessor *query.DataProcessor
	logger        *zap.Logger
}

func NewExecutor(interactor DatabaseInteractor, logger *zap.Logger) *Executor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Executor{
		queryExecutor: interactor,
		dataProcessor: query.NewDataProcessor(logger),
		logger:        logger,
	}
}

// RegisterComputeFunction registers a Go function for computed fields.
func (e *Executor) RegisterComputeFunction(name string, fn query.ComputeFunction) {
	e.dataProcessor.RegisterComputeFunction(name, fn)
}

// RegisterFilterFunction registers a Go function for custom filtering.
func (e *Executor) RegisterFilterFunction(operator query.ComparisonOperator, fn query.PredicateFunction) {
	e.dataProcessor.RegisterFilterFunction(operator, fn)
}

// RegisterComputeFunctions registers multiple GoComputeFunction functions from a map.
func (e *Executor) RegisterComputeFunctions(functionMap map[string]query.ComputeFunction) {
	e.dataProcessor.RegisterComputeFunctions(functionMap)
}

// RegisterFilterFunctions registers multiple GoFilterFunction functions from a map.
func (e *Executor) RegisterFilterFunctions(functionMap map[query.ComparisonOperator]query.PredicateFunction) {
	e.dataProcessor.RegisterFilterFunctions(functionMap)
}

// Query runs a query against the database based on the provided core.
func (e *Executor) Query(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.QueryDSL) (*query.QueryResult, error) {
	// Determine all fields needed for Go functions
	fieldsToSelect := e.dataProcessor.DetermineFieldsToSelect(dsl)

	// Create modified DSL for SQL execution with all required fields
	sqlDsl := *dsl
		sqlDsl.Projection = &query.ProjectionConfiguration{
		Include: fieldsToSelect,
	}

	// Execute SQL query to get raw rows
	dbRows, err := e.queryExecutor.SelectDocuments(ctx, schema, &sqlDsl)
	if err != nil {
		return nil, err
	}
	e.logger.Debug("Fetched rows from DB before Go processing", zap.Int("count", len(dbRows)))

	// Process rows with Go functions and projections
	finalResults, err := e.dataProcessor.ProcessRows(dbRows, dsl, nil)
	if err != nil {
		return nil, err
	}

	// Format result
	var data any
	count := len(finalResults)
	if count == 1 {
		data = finalResults[0]
	} else {
		data = finalResults
	}

	return &query.QueryResult{Data: data, Count: count}, nil
}

// Update performs an update operation on the database.
func (e *Executor) Update(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	return e.queryExecutor.UpdateDocuments(ctx, schema, updates, filters)
}

// Insert performs an insert operation and atomically returns the inserted records.
func (e *Executor) Insert(ctx context.Context, schema *schema.SchemaDefinition, records []map[string]any) (*query.QueryResult, error) {
	insertedRows, err := e.queryExecutor.InsertDocuments(ctx, schema, records)
	if err != nil {
		return nil, err
	}

	var data any
	count := len(insertedRows)
	if count == 1 {
		data = insertedRows[0]
	} else {
		data = insertedRows
	}

	return &query.QueryResult{Data: data, Count: count}, nil
}

// Delete performs a delete operation with optional filters for safety.
func (e *Executor) Delete(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	return e.queryExecutor.DeleteDocuments(ctx, schema, filters, unsafeDelete)
}

