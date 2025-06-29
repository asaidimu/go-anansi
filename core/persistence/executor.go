// Package persistence provides the Executor, a key component for orchestrating database
// operations by coordinating between the high-level query DSL and the underlying
// database interactor.
package persistence

import (
	"context"

	"github.com/asaidimu/go-anansi/v5/core/query"
	"github.com/asaidimu/go-anansi/v5/core/schema"
	"go.uber.org/zap"
)

// Executor is responsible for orchestrating database operations. It acts as a bridge
// between the high-level QueryDSL and the low-level DatabaseInteractor. It uses a
// DataProcessor to handle in-memory computations and filtering after the initial
// data has been fetched from the database.
type Executor struct {
	queryExecutor DatabaseInteractor
	dataProcessor *query.DataProcessor
	logger        *zap.Logger
}

// NewExecutor creates a new instance of an Executor. It requires a DatabaseInteractor
// to communicate with the database and an optional logger for logging.
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

// RegisterComputeFunction registers a Go function that can be used to compute field
// values dynamically. These functions are executed in-memory after data is fetched.
func (e *Executor) RegisterComputeFunction(name string, fn query.ComputeFunction) {
	e.dataProcessor.RegisterComputeFunction(name, fn)
}

// RegisterFilterFunction registers a Go function for custom filtering operations.
// This allows for complex filtering logic that may not be directly translatable to SQL.
func (e *Executor) RegisterFilterFunction(operator query.ComparisonOperator, fn query.PredicateFunction) {
	e.dataProcessor.RegisterFilterFunction(operator, fn)
}

// RegisterComputeFunctions registers multiple compute functions from a map.
func (e *Executor) RegisterComputeFunctions(functionMap map[string]query.ComputeFunction) {
	e.dataProcessor.RegisterComputeFunctions(functionMap)
}

// RegisterFilterFunctions registers multiple filter functions from a map.
func (e *Executor) RegisterFilterFunctions(functionMap map[query.ComparisonOperator]query.PredicateFunction) {
	e.dataProcessor.RegisterFilterFunctions(functionMap)
}

// Query executes a read query against the database. It first determines which fields
// need to be selected to satisfy any in-memory computations or filters, then executes
// the query, and finally processes the results using the DataProcessor.
func (e *Executor) Query(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.QueryDSL) (*query.QueryResult, error) {
	// Determine all fields needed for Go functions (computed fields, custom filters).
	fieldsToSelect := e.dataProcessor.DetermineFieldsToSelect(dsl)

	// Create a modified DSL for SQL execution that includes all fields required for
	// in-memory processing.
	sqlDsl := *dsl
	sqlDsl.Projection = &query.ProjectionConfiguration{
		Include: fieldsToSelect,
	}

	// Execute the database query to get the raw rows.
	dbRows, err := e.queryExecutor.SelectDocuments(ctx, schema, &sqlDsl)
	if err != nil {
		return nil, err
	}
	e.logger.Debug("Fetched rows from DB before Go processing", zap.Int("count", len(dbRows)))

	// Process the fetched rows with in-memory functions (filters, computed fields, projections).
	finalResults, err := e.dataProcessor.ProcessRows(dbRows, dsl, nil)
	if err != nil {
		return nil, err
	}

	// Format the result into a standard QueryResult.
	var data any
	count := len(finalResults)
	if count == 1 {
		data = finalResults[0]
	} else {
		data = finalResults
	}

	return &query.QueryResult{Data: data, Count: count}, nil
}

// Update performs an update operation on the database. It directly passes the update
// instructions to the DatabaseInteractor.
func (e *Executor) Update(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	return e.queryExecutor.UpdateDocuments(ctx, schema, updates, filters)
}

// Insert performs an insert operation on the database. It passes the records to the
// DatabaseInteractor and returns the inserted documents.
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

// Delete performs a delete operation on the database. It passes the filters to the
// DatabaseInteractor to determine which documents to delete.
func (e *Executor) Delete(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	return e.queryExecutor.DeleteDocuments(ctx, schema, filters, unsafeDelete)
}

