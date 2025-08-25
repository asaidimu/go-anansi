package native

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// ExecutorStrategy defines how an interactor selects its executor
type ExecutorStrategy[T any] interface {
	GetExecutor() QueryExecutor[T]
}

// SharedInteractorLogic contains all the shared business logic
type SharedInteractorLogic[T any] struct {
	b  NativeQueryBuilder[T]
	qf QueryFactory[T]
}

// NewSharedInteractorLogic creates a new shared logic instance
func NewSharedInteractorLogic[T any](qf QueryFactory[T]) *SharedInteractorLogic[T] {
	return &SharedInteractorLogic[T]{
		b:  NewNativeQueryBuilder(qf),
		qf: qf,
	}
}

// SelectDocuments shared implementation
func (s *SharedInteractorLogic[T]) SelectDocuments(ctx context.Context, strategy ExecutorStrategy[T], schema *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {
	compiled, err := s.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, &NativeError{
			Operation: "SelectDocuments", // This operation name will be used for both SelectDocuments and SelectStream
			Message:   ErrCouldNotGetQuery.Error(),
			Cause:     err,
		}
	}

	resultSchema, err := query.SchemaFromQuery(dsl, nil)
	if err != nil {
		return nil, &NativeError{
			Operation: "SchemaFromQuery", // This operation name will be used for both SelectDocuments and SelectStream
			Message:   ErrCouldNotGetResultSchema.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	data, err := executor.Query(ctx, NativeQuery[T]{Query: compiled, Schema: resultSchema})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SelectStream shared implementation
func (s *SharedInteractorLogic[T]) SelectStream(ctx context.Context, strategy ExecutorStrategy[T], sc *schema.SchemaDefinition, dsl *query.Query) (<-chan data.Document, <-chan error, error) {
	compiled, err := s.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, nil, &NativeError{Operation: "SelectStream", Message: ErrCouldNotGetQuery.Error(), Cause: err}
	}

	resultSchema, err := query.SchemaFromQuery(dsl, nil)
	if err != nil {
		return nil, nil, &NativeError{Operation: "SelectStream", Message: ErrCouldNotGetResultSchema.Error(), Cause: err}
	}

	executor := strategy.GetExecutor()
	return executor.QueryStream(ctx, NativeQuery[T]{Query: compiled, Schema: resultSchema})
}

// UpdateDocuments shared implementation
func (s *SharedInteractorLogic[T]) UpdateDocuments(ctx context.Context, strategy ExecutorStrategy[T], schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	compiled, err := s.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Alias:  &schema.Name,
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtUpdate, updates)

	if err != nil {
		return 0, &NativeError{
			Operation: "UpdateDocuments", // This operation name will be used for both UpdateDocuments and DeleteDocuments
			Message:   ErrCouldNotGetQuery.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	return executor.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: schema})
}

// InsertDocuments shared implementation
func (s *SharedInteractorLogic[T]) InsertDocuments(ctx context.Context, strategy ExecutorStrategy[T], sc *schema.SchemaDefinition, records []data.Document) ([]data.Document, error) {
	if len(records) == 0 {
		return []data.Document{}, nil
	}

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   sc.Name,
			Schema: sc,
		}}

	resultSchema, err := query.SchemaFromQuery(q, nil)
	if err != nil {
		return nil, &NativeError{
			Operation: "SchemaFromQuery", // This operation name will be used for both SelectDocuments and SelectStream
			Message:   ErrCouldNotGetResultSchema.Error(),
			Cause:     err,
		}
	}

	compiled, err := s.b.Build(q, StmtInsert, records)
	if err != nil {
		return nil, &NativeError{
			Operation: "SelectDocuments", // This operation name will be used for both SelectDocuments and SelectStream
			Message:   ErrCouldNotGetQuery.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	return executor.Query(ctx, NativeQuery[T]{Query: compiled, Schema: resultSchema})
}

// DeleteDocuments shared implementation
func (s *SharedInteractorLogic[T]) DeleteDocuments(ctx context.Context, strategy ExecutorStrategy[T], schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	if filters == nil && !unsafeDelete {
		return 0, &NativeError{
			Operation: "DeleteDocuments",
			Message:   ErrCouldNotDeleteWithoutFilters.Error(),
			Cause:     ErrCouldNotDeleteWithoutFilters,
		}
	}

	compiled, err := s.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtDelete, nil)

	if err != nil {
		return 0, &NativeError{
			Operation: "UpdateDocuments", // This operation name will be used for both UpdateDocuments and DeleteDocuments
			Message:   ErrCouldNotGetQuery.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	return executor.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: schema})
}

// CreateCollection shared implementation
func (s *SharedInteractorLogic[T]) CreateCollection(ctx context.Context, strategy ExecutorStrategy[T], sc schema.SchemaDefinition) error {
	// Create the collection
	compiled, err := s.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   sc.Name,
			Schema: &sc,
		},
	}, StmtCreateCollection, nil)

	if err != nil {
		return &NativeError{
			Operation: "CreateCollection",
			Message:   ErrCouldNotBuildCreateCollectionQuery.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	_, err = executor.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: &sc})
	if err != nil {
		return &NativeError{
			Operation: "CreateCollection",
			Message:   ErrCouldNotCreateCollection.Error(),
			Cause:     err,
		}
	}

	// Create indexes if they exist
	if sc.Indexes != nil {
		for _, index := range sc.Indexes {
			// Skip primary key indexes as they're typically created automatically
			if index.Type == schema.IndexTypePrimary {
				continue
			}

			compiled, err := s.b.Build(&query.Query{
				Target: &query.QueryTarget{
					Name:   sc.Name,
					Schema: &sc,
				},
			}, StmtCreateIndex, index)

			if err != nil {
				return &NativeError{
					Operation: "CreateCollection", // Still part of CreateCollection
					Message:   fmt.Sprintf("could not build create index query for %s", index.Name),
					Cause:     err,
				}
			}

			_, err = executor.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: &sc})
			if err != nil {
				return &NativeError{
					Operation: "CreateCollection", // Still part of CreateCollection
					Message:   fmt.Sprintf("could not create index %s", index.Name),
					Cause:     err,
				}
			}
		}
	}

	return nil
}

// CreateIndex shared implementation
func (s *SharedInteractorLogic[T]) CreateIndex(ctx context.Context, strategy ExecutorStrategy[T], collection string, index schema.IndexDefinition) error {
	compiled, err := s.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: collection,
		},
	}, StmtCreateIndex, index)
	if err != nil {
		return &NativeError{
			Operation: "CreateIndex", // This operation name will be used for CreateIndex, DropIndex, DropCollection
			Message:   ErrCouldNotGetQuery.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	_, err = executor.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: nil})
	return err
}

// DropIndex shared implementation
func (s *SharedInteractorLogic[T]) DropIndex(ctx context.Context, strategy ExecutorStrategy[T], collection string, index schema.IndexDefinition) error {
	compiled, err := s.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: collection,
		},
	}, StmtDropIndex, index)
	if err != nil {
		return &NativeError{
			Operation: "CreateIndex", // This operation name will be used for CreateIndex, DropIndex, DropCollection
			Message:   ErrCouldNotGetQuery.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	_, err = executor.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: nil})
	return err
}

// DropCollection shared implementation
func (s *SharedInteractorLogic[T]) DropCollection(ctx context.Context, strategy ExecutorStrategy[T], name string) error {
	compiled, err := s.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: name,
		},
	}, StmtDropCollection, nil)
	if err != nil {
		return &NativeError{
			Operation: "CreateIndex", // This operation name will be used for CreateIndex, DropIndex, DropCollection
			Message:   ErrCouldNotGetQuery.Error(),
			Cause:     err,
		}
	}

	executor := strategy.GetExecutor()
	_, err = executor.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: nil})
	return err
}

// NativeInteractor with strategy pattern
type NativeInteractor[T any] struct {
	ix     QueryExecutor[T]
	shared *SharedInteractorLogic[T]

	// Transaction management
	txMu     sync.RWMutex
	activeTx *transactionContext[T]
}

type transactionContext[T any] struct {
	executor QueryExecutor[T]
	cleanup  func()
}

var _ query.BaseDatabaseInteractor = (*NativeInteractor[any])(nil)
var _ query.DatabaseInteractor = (*NativeInteractor[any])(nil)

// BaseExecutorStrategy always returns the base executor (never transaction)
type BaseExecutorStrategy[T any] struct {
	interactor *NativeInteractor[T]
}

func (s *BaseExecutorStrategy[T]) GetExecutor() QueryExecutor[T] {
	return s.interactor.ix
}

// TransactionExecutorStrategy returns the transaction executor if available, otherwise base executor
type TransactionExecutorStrategy[T any] struct {
	interactor *NativeInteractor[T]
}

func (s *TransactionExecutorStrategy[T]) GetExecutor() QueryExecutor[T] {
	s.interactor.txMu.RLock()
	defer s.interactor.txMu.RUnlock()

	if s.interactor.activeTx != nil {
		return s.interactor.activeTx.executor
	}
	return s.interactor.ix
}

// NativeTransactionInteractor with strategy pattern
type NativeTransactionInteractor[T any] struct {
	base   *NativeInteractor[T]
	ctx    *transactionContext[T]
	shared *SharedInteractorLogic[T]
}

var _ query.TransactionalDatabaseInteractor = (*NativeTransactionInteractor[any])(nil)

// TransactionOnlyExecutorStrategy always returns the transaction executor
type TransactionOnlyExecutorStrategy[T any] struct {
	ctx *transactionContext[T]
}

func (s *TransactionOnlyExecutorStrategy[T]) GetExecutor() QueryExecutor[T] {
	return s.ctx.executor
}

func NewNativeInteractor[T any](ix QueryExecutor[T], qf QueryFactory[T]) (query.DatabaseInteractor, error) {
	shared := NewSharedInteractorLogic(qf)
	return &NativeInteractor[T]{
		ix:     ix,
		shared: shared,
	}, nil
}

// Close closes the database connection.
func (i *NativeInteractor[T]) Close() error {
	i.txMu.Lock()
	defer i.txMu.Unlock()

	// Clean up any active transaction
	if i.activeTx != nil {
		i.activeTx.executor.Rollback(context.Background())
		i.activeTx.cleanup()
		i.activeTx = nil
	}

	return i.ix.Close()
}

// SelectDocuments retrieves documents from the Native database.
func (i *NativeInteractor[T]) SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {
	strategy := &BaseExecutorStrategy[T]{interactor: i}
	return i.shared.SelectDocuments(ctx, strategy, schema, dsl)
}

// SelectStream streams documents from the Native database.
func (i *NativeInteractor[T]) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan data.Document, <-chan error, error) {
	strategy := &BaseExecutorStrategy[T]{interactor: i}
	return i.shared.SelectStream(ctx, strategy, sc, dsl)
}

// UpdateDocuments updates documents in the Native database.
func (i *NativeInteractor[T]) UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	strategy := &BaseExecutorStrategy[T]{interactor: i}
	return i.shared.UpdateDocuments(ctx, strategy, schema, updates, filters)
}

// InsertDocuments inserts documents into the Native database.
func (i *NativeInteractor[T]) InsertDocuments(ctx context.Context, sc *schema.SchemaDefinition, records []data.Document) ([]data.Document, error) {
	strategy := &BaseExecutorStrategy[T]{interactor: i}
	return i.shared.InsertDocuments(ctx, strategy, sc, records)
}

// DeleteDocuments deletes documents from the Native database.
func (i *NativeInteractor[T]) DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	strategy := &BaseExecutorStrategy[T]{interactor: i}
	return i.shared.DeleteDocuments(ctx, strategy, schema, filters, unsafeDelete)
}

// StartTransaction begins a new database transaction.
func (i *NativeInteractor[T]) StartTransaction(ctx context.Context) (query.TransactionalDatabaseInteractor, error) {
	i.txMu.Lock()
	defer i.txMu.Unlock()

	if i.activeTx != nil {
		return nil, &NativeError{
			Operation: "StartTransaction",
			Message:   ErrCannotNestTransactions.Error(),
			Cause:     ErrCannotNestTransactions,
		}
	}

	tx, err := i.ix.BeginTransaction(ctx)
	if err != nil {
		return nil, &NativeError{
			Operation: "StartTransaction",
			Message:   ErrFailedToBeginTransaction.Error(),
			Cause:     err,
		}
	}

	// Create transaction context
	txCtx := &transactionContext[T]{
		executor: tx,
		cleanup: func() {
			i.txMu.Lock()
			i.activeTx = nil
			i.txMu.Unlock()
		},
	}

	i.activeTx = txCtx

	return &NativeTransactionInteractor[T]{
		base:   i,
		ctx:    txCtx,
		shared: i.shared,
	}, nil
}

// HasTransaction checks if there's an active transaction
func (i *NativeInteractor[T]) HasTransaction(ctx context.Context) bool {
	i.txMu.RLock()
	defer i.txMu.RUnlock()
	return i.activeTx != nil
}

// executeInTransaction executes a function within a transaction context.
// If already in a transaction, reuses it; otherwise creates a new one.
func (i *NativeInteractor[T]) executeInTransaction(ctx context.Context, fn func(QueryExecutor[T]) error) error {
	// Check if we're already in a transaction
	if i.HasTransaction(ctx) {
		strategy := &TransactionExecutorStrategy[T]{interactor: i}
		executor := strategy.GetExecutor()
		return fn(executor)
	}

	// Start a new transaction
	tx, err := i.ix.BeginTransaction(ctx)
	if err != nil {
		return &NativeError{
			Operation: "executeInTransaction",
			Message:   ErrFailedToBeginTransaction.Error(),
			Cause:     err,
		}
	}

	// Execute the function
	err = fn(tx)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return &NativeError{
				Operation: "executeInTransaction",
				Message:   fmt.Sprintf("%s, %s: %v", ErrOperationFailed.Error(), ErrRollbackFailed.Error(), rollbackErr),
				Cause:     err,
			}
		}
		return err
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return &NativeError{
			Operation: "executeInTransaction",
			Message:   ErrFailedToCommitTransaction.Error(),
			Cause:     err,
		}
	}

	return nil
}

// SchemaManager returns the SchemaManager interface for Native.
func (i *NativeInteractor[T]) SchemaManager() query.SchemaManager {
	return i
}

// CollectionExists checks if a collection exists in the Native database.
func (i *NativeInteractor[T]) CollectionExists(ctx context.Context, name string) (bool, error) {
	return false, nil
}

// CreateCollection creates a new collection in the Native database.
func (i *NativeInteractor[T]) CreateCollection(ctx context.Context, sc schema.SchemaDefinition) error {
	return i.executeInTransaction(ctx, func(executor QueryExecutor[T]) error {
		strategy := &TransactionExecutorStrategy[T]{interactor: i}
		return i.shared.CreateCollection(ctx, strategy, sc)
	})
}

// CreateIndex creates a new index in the Native database.
func (i *NativeInteractor[T]) CreateIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	strategy := &BaseExecutorStrategy[T]{interactor: i}
	return i.shared.CreateIndex(ctx, strategy, collection, index)
}

// DropIndex drops an index from the Native database.
func (i *NativeInteractor[T]) DropIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	strategy := &BaseExecutorStrategy[T]{interactor: i}
	return i.shared.DropIndex(ctx, strategy, collection, index)
}

// DropCollection deletes a collection from the Native database.
func (i *NativeInteractor[T]) DropCollection(ctx context.Context, name string) error {
	strategy := &TransactionExecutorStrategy[T]{interactor: i}
	return i.shared.DropCollection(ctx, strategy, name)
}

// Capabilities returns the capabilities of the Native database.
func (i *NativeInteractor[T]) Capabilities() query.Capabilities {
	return i.shared.qf.Capabilities()
}

// Transaction-specific methods for NativeTransactionInteractor

// SelectDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.SelectDocuments(ctx, strategy, schema, dsl)
}

// SelectStream for transaction interactor
func (ti *NativeTransactionInteractor[T]) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan data.Document, <-chan error, error) {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.SelectStream(ctx, strategy, sc, dsl)
}

// UpdateDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.UpdateDocuments(ctx, strategy, schema, updates, filters)
}

// InsertDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) InsertDocuments(ctx context.Context, sc *schema.SchemaDefinition, records []data.Document) ([]data.Document, error) {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.InsertDocuments(ctx, strategy, sc, records)
}

// DeleteDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.DeleteDocuments(ctx, strategy, schema, filters, unsafeDelete)
}

// Commit commits the current transaction.
func (ti *NativeTransactionInteractor[T]) Commit(ctx context.Context) error {
	// Validate that this transaction context is still valid
	ti.base.txMu.RLock()
	isValid := ti.base.activeTx == ti.ctx
	ti.base.txMu.RUnlock()

	if !isValid {
		return &NativeError{
			Operation: "Commit",
			Message:   ErrCommitNotApplicable.Error(),
			Cause:     ErrCommitNotApplicable,
		}
	}

	err := ti.ctx.executor.Commit(ctx)
	ti.ctx.cleanup()
	return err
}

// Rollback rolls back the current transaction.
func (ti *NativeTransactionInteractor[T]) Rollback(ctx context.Context) error {
	// Validate that this transaction context is still valid
	ti.base.txMu.RLock()
	isValid := ti.base.activeTx == ti.ctx
	ti.base.txMu.RUnlock()

	if !isValid {
		return &NativeError{
			Operation: "Rollback",
			Message:   ErrRollbackNotApplicable.Error(),
			Cause:     ErrRollbackNotApplicable,
		}
	}

	err := ti.ctx.executor.Rollback(ctx)
	ti.ctx.cleanup()
	return err
}

// HasTransaction for transaction interactor
func (ti *NativeTransactionInteractor[T]) HasTransaction(ctx context.Context) bool {
	return true
}

// StartTransaction for transaction interactor (should return error for nested transactions)
func (ti *NativeTransactionInteractor[T]) StartTransaction(ctx context.Context) (query.TransactionalDatabaseInteractor, error) {
	return nil, &NativeError{
		Operation: "StartTransaction",
		Message:   ErrCannotNestTransactions.Error(),
		Cause:     ErrCannotNestTransactions,
	}
}

// SchemaManager for transaction interactor
func (ti *NativeTransactionInteractor[T]) SchemaManager() query.SchemaManager {
	return ti
}

// Close for transaction interactor
func (ti *NativeTransactionInteractor[T]) Close() error {
	return ti.base.Close()
}

// Capabilities for transaction interactor
func (ti *NativeTransactionInteractor[T]) Capabilities() query.Capabilities {
	return ti.base.Capabilities()
}

// CollectionExists for transaction interactor
func (ti *NativeTransactionInteractor[T]) CollectionExists(ctx context.Context, name string) (bool, error) {
	return false, nil
}

// CreateCollection for transaction interactor
func (ti *NativeTransactionInteractor[T]) CreateCollection(ctx context.Context, sc schema.SchemaDefinition) error {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.CreateCollection(ctx, strategy, sc)
}

// CreateIndex for transaction interactor
func (ti *NativeTransactionInteractor[T]) CreateIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.CreateIndex(ctx, strategy, collection, index)
}

// DropIndex for transaction interactor
func (ti *NativeTransactionInteractor[T]) DropIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.DropIndex(ctx, strategy, collection, index)
}

// DropCollection for transaction interactor
func (ti *NativeTransactionInteractor[T]) DropCollection(ctx context.Context, name string) error {
	strategy := &TransactionOnlyExecutorStrategy[T]{ctx: ti.ctx}
	return ti.shared.DropCollection(ctx, strategy, name)
}
