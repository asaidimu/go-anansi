package native

import (
	"context"
	"fmt"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

type NativeInteractor[T any] struct {
	ix QueryExecutor[T]
	qf QueryFactory[T]
	b  NativeQueryBuilder[T]

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

type NativeTransactionInteractor[T any] struct {
	base *NativeInteractor[T]
	ctx  *transactionContext[T]
}

var _ query.TransactionalDatabaseInteractor = (*NativeTransactionInteractor[any])(nil)

func NewNativeInteractor[T any](ix QueryExecutor[T], qf QueryFactory[T]) (query.DatabaseInteractor, error) {
	b := NewNativeQueryBuilder(qf)
	return &NativeInteractor[T]{
		ix: ix,
		qf: qf,
		b:  b,
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

// getBaseExecutor always returns the base executor (never transaction)
// This is used by the base interactor to ensure it doesn't see uncommitted transaction data
func (i *NativeInteractor[T]) getBaseExecutor() QueryExecutor[T] {
	return i.ix
}

// getExecutor returns the transaction executor if available, otherwise base executor
// This is used by executeInTransaction to reuse existing transactions
func (i *NativeInteractor[T]) getExecutor() QueryExecutor[T] {
	i.txMu.RLock()
	defer i.txMu.RUnlock()

	if i.activeTx != nil {
		return i.activeTx.executor
	}
	return i.ix
}

// SelectDocuments retrieves documents from the Native database.
func (i *NativeInteractor[T]) SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {
	compiled, err := i.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getBaseExecutor()
	data, err := executor.Query(ctx, compiled)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SelectStream streams documents from the Native database.
func (i *NativeInteractor[T]) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan data.Document, <-chan error, error) {
	compiled, err := i.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getBaseExecutor()
	return executor.QueryStream(ctx, compiled)
}

// UpdateDocuments updates documents in the Native database.
func (i *NativeInteractor[T]) UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	compiled, err := i.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Alias: &schema.Name,
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtUpdate, updates)

	if err != nil {
		return 0, fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getBaseExecutor()
	return executor.Exec(ctx, compiled)
}

// InsertDocuments inserts documents into the Native database.
func (i *NativeInteractor[T]) InsertDocuments(ctx context.Context, sc *schema.SchemaDefinition, records []data.Document) ([]data.Document, error) {
	if len(records) == 0 {
		return []data.Document{}, nil
	}

	compiled, err := i.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   sc.Name,
			Schema: sc,
		},
	}, StmtInsert, records)

	if err != nil {
		return nil, fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getBaseExecutor()
	return executor.Query(ctx, compiled)
}

// DeleteDocuments deletes documents from the Native database.
func (i *NativeInteractor[T]) DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	if filters == nil && !unsafeDelete {
		return 0, fmt.Errorf("could not delete without filters")
	}

	compiled, err := i.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtDelete, nil)

	if err != nil {
		return 0, fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getBaseExecutor()
	return executor.Exec(ctx, compiled)
}

// StartTransaction begins a new database transaction.
func (i *NativeInteractor[T]) StartTransaction(ctx context.Context) (query.TransactionalDatabaseInteractor, error) {
	i.txMu.Lock()
	defer i.txMu.Unlock()

	if i.activeTx != nil {
		return nil, fmt.Errorf("cannot nest transactions: transaction already active")
	}

	tx, err := i.ix.BeginTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
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
		base: i,
		ctx:  txCtx,
	}, nil
}

// HasTransaction checks if there's an active transaction
func (i *NativeInteractor[T]) HasTransaction(ctx context.Context) bool {
	i.txMu.RLock()
	defer i.txMu.RUnlock()
	return i.activeTx != nil
}

// Transaction-specific methods for NativeTransactionInteractor

// SelectDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {
	compiled, err := ti.base.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, fmt.Errorf("could not get a query: %w", err)
	}

	data, err := ti.ctx.executor.Query(ctx, compiled)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SelectStream for transaction interactor
func (ti *NativeTransactionInteractor[T]) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan data.Document, <-chan error, error) {
	compiled, err := ti.base.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get a query: %w", err)
	}

	return ti.ctx.executor.QueryStream(ctx, compiled)
}

// UpdateDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	compiled, err := ti.base.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtUpdate, updates)

	if err != nil {
		return 0, fmt.Errorf("could not get a query: %w", err)
	}

	return ti.ctx.executor.Exec(ctx, compiled)
}

// InsertDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) InsertDocuments(ctx context.Context, sc *schema.SchemaDefinition, records []data.Document) ([]data.Document, error) {
	if len(records) == 0 {
		return []data.Document{}, nil
	}

	compiled, err := ti.base.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   sc.Name,
			Schema: sc,
		},
	}, StmtInsert, records)

	if err != nil {
		return nil, fmt.Errorf("could not get a query: %w", err)
	}

	return ti.ctx.executor.Query(ctx, compiled)
}

// DeleteDocuments for transaction interactor
func (ti *NativeTransactionInteractor[T]) DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	if filters == nil && !unsafeDelete {
		return 0, fmt.Errorf("could not delete without filters")
	}

	compiled, err := ti.base.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtDelete, nil)

	if err != nil {
		return 0, fmt.Errorf("could not get a query: %w", err)
	}

	return ti.ctx.executor.Exec(ctx, compiled)
}

// Commit commits the current transaction.
func (ti *NativeTransactionInteractor[T]) Commit(ctx context.Context) error {
	// Validate that this transaction context is still valid
	ti.base.txMu.RLock()
	isValid := ti.base.activeTx == ti.ctx
	ti.base.txMu.RUnlock()

	if !isValid {
		return fmt.Errorf("commit not applicable: transaction context invalid")
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
		return fmt.Errorf("rollback not applicable: transaction context invalid")
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
	return nil, fmt.Errorf("cannot nest transactions: already in a transaction")
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

// SchemaManager returns the SchemaManager interface for Native.
func (i *NativeInteractor[T]) SchemaManager() query.SchemaManager {
	return i
}

// CollectionExists checks if a collection exists in the Native database.
func (i *NativeInteractor[T]) CollectionExists(ctx context.Context, name string) (bool, error) {
	return false, nil
}

// CollectionExists for transaction interactor
func (ti *NativeTransactionInteractor[T]) CollectionExists(ctx context.Context, name string) (bool, error) {
	return false, nil
}

// CreateCollection creates a new collection in the Native database.
func (i *NativeInteractor[T]) CreateCollection(ctx context.Context, sc schema.SchemaDefinition) error {
	return i.executeInTransaction(ctx, func(executor QueryExecutor[T]) error {
		// Create the collection
		compiled, err := i.b.Build(&query.Query{
			Target: &query.QueryTarget{
				Name:   sc.Name,
				Schema: &sc,
			},
		}, StmtCreateCollection, nil)

		if err != nil {
			return fmt.Errorf("could not build create collection query: %w", err)
		}

		_, err = executor.Exec(ctx, compiled)
		if err != nil {
			return fmt.Errorf("could not create collection: %w", err)
		}

		// Create indexes if they exist
		if sc.Indexes != nil {
			for _, index := range sc.Indexes {
				// Skip primary key indexes as they're typically created automatically
				if index.Type == schema.IndexTypePrimary {
					continue
				}

				compiled, err := i.b.Build(&query.Query{
					Target: &query.QueryTarget{
						Name:   sc.Name,
						Schema: &sc,
					},
				}, StmtCreateIndex, index)

				if err != nil {
					return fmt.Errorf("could not build create index query for %s: %w", index.Name, err)
				}

				_, err = executor.Exec(ctx, compiled)
				if err != nil {
					return fmt.Errorf("could not create index %s: %w", index.Name, err)
				}
			}
		}

		return nil
	})
}

// CreateCollection for transaction interactor
func (ti *NativeTransactionInteractor[T]) CreateCollection(ctx context.Context, sc schema.SchemaDefinition) error {
	// Create the collection
	compiled, err := ti.base.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   sc.Name,
			Schema: &sc,
		},
	}, StmtCreateCollection, nil)

	if err != nil {
		return fmt.Errorf("could not build create collection query: %w", err)
	}

	_, err = ti.ctx.executor.Exec(ctx, compiled)
	if err != nil {
		return fmt.Errorf("could not create collection: %w", err)
	}

	// Create indexes if they exist
	if sc.Indexes != nil {
		for _, index := range sc.Indexes {
			// Skip primary key indexes as they're typically created automatically
			if index.Type == schema.IndexTypePrimary {
				continue
			}

			compiled, err := ti.base.b.Build(&query.Query{
				Target: &query.QueryTarget{
					Name:   sc.Name,
					Schema: &sc,
				},
			}, StmtCreateIndex, index)

			if err != nil {
				return fmt.Errorf("could not build create index query for %s: %w", index.Name, err)
			}

			_, err = ti.ctx.executor.Exec(ctx, compiled)
			if err != nil {
				return fmt.Errorf("could not create index %s: %w", index.Name, err)
			}
		}
	}

	return nil
}

// executeInTransaction executes a function within a transaction context.
// If already in a transaction, reuses it; otherwise creates a new one.
func (i *NativeInteractor[T]) executeInTransaction(ctx context.Context, fn func(QueryExecutor[T]) error) error {
	// Check if we're already in a transaction
	if i.HasTransaction(ctx) {
		executor := i.getExecutor()
		return fn(executor)
	}

	// Start a new transaction
	tx, err := i.ix.BeginTransaction(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Execute the function
	err = fn(tx)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("operation failed: %w, rollback failed: %v", err, rollbackErr)
		}
		return err
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CreateIndex creates a new index in the Native database.
func (i *NativeInteractor[T]) CreateIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	compiled, err := i.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: collection,
		},
	}, StmtCreateIndex, index)
	if err != nil {
		return fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getBaseExecutor()
	_, err = executor.Exec(ctx, compiled)
	return err
}

// CreateIndex for transaction interactor
func (ti *NativeTransactionInteractor[T]) CreateIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	compiled, err := ti.base.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: collection,
		},
	}, StmtCreateIndex, index)
	if err != nil {
		return fmt.Errorf("could not get a query: %w", err)
	}

	_, err = ti.ctx.executor.Exec(ctx, compiled)
	return err
}

// DropIndex drops an index from the Native database.
func (i *NativeInteractor[T]) DropIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	compiled, err := i.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: collection,
		},
	}, StmtDropIndex, index)
	if err != nil {
		return fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getBaseExecutor()
	_, err = executor.Exec(ctx, compiled)
	return err
}

// DropIndex for transaction interactor
func (ti *NativeTransactionInteractor[T]) DropIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	compiled, err := ti.base.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: collection,
		},
	}, StmtDropIndex, index)
	if err != nil {
		return fmt.Errorf("could not get a query: %w", err)
	}

	_, err = ti.ctx.executor.Exec(ctx, compiled)
	return err
}

// DropCollection deletes a collection from the Native database.
func (i *NativeInteractor[T]) DropCollection(ctx context.Context, name string) error {
	compiled, err := i.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   name,
		},
	}, StmtDropCollection, nil)
	if err != nil {
		return fmt.Errorf("could not get a query: %w", err)
	}

	executor := i.getExecutor()
	_, err = executor.Exec(ctx, compiled)
	return err
}

// DropCollection for transaction interactor
func (ti *NativeTransactionInteractor[T]) DropCollection(ctx context.Context, name string) error {
	compiled, err := ti.base.b.Build(&query.Query{
		Target: &query.QueryTarget{
			Name: name,
		},
	}, StmtDropCollection, nil)
	if err != nil {
		return fmt.Errorf("could not get a query: %w", err)
	}

	_, err = ti.ctx.executor.Exec(ctx, compiled)
	return err
}

// Capabilities returns the capabilities of the Native database.
func (i *NativeInteractor[T]) Capabilities() query.Capabilities {
	return i.qf.Capabilities()
}
