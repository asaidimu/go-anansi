// Package native provides a database interactor that uses a generic query executor.
package native

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"go.uber.org/zap"
)

// NativeInteractor implements the core database interaction logic.
// It directly manages the query executor and transaction state, providing a unified
// implementation for all database operations.
type NativeInteractor[T any] struct {
	// ix is the active query executor. It can be a base database connection
	// or a specific transactional executor.
	ix QueryExecutor[T]

	// b is the query builder used to compile DSLs into native queries.
	b NativeQueryBuilder[T]

	// qf provides dialect-specific capabilities and query factory methods.
	qf QueryFactory[T]

	logger *zap.Logger

	// isTx is a flag indicating whether this interactor instance represents an
	// active transaction.
	txMu sync.Mutex // Each interactor has its own mutex
	isTx bool

	// tracks whether the transaction has already finished
	done atomic.Bool
}

// ensure NativeInteractor conforms to the required interfaces
var _ query.DatabaseInteractor = (*NativeInteractor[any])(nil)
var _ query.SchemaManager = (*NativeInteractor[any])(nil)

// NewNativeInteractor is the single constructor for creating a new database interactor.
// It initializes a base-level interactor that is not yet in a transaction.
func NewNativeInteractor[T any](ix QueryExecutor[T], qf QueryFactory[T], logger *zap.Logger) (query.DatabaseInteractor, error) {
	if ix == nil {
		return nil, common.NewSystemError("ERR_NATIVE_QUERY_EXECUTOR_NIL", "query executor cannot be nil")
	}
	if qf == nil {
		return nil, common.NewSystemError("ERR_NATIVE_QUERY_FACTORY_NIL", "query factory cannot be nil")
	}

	return &NativeInteractor[T]{
		ix:     ix,
		b:      NewNativeQueryBuilder(qf),
		qf:     qf,
		isTx:   false,
		logger: logger,
	}, nil
}

// Close closes the underlying database connection.
func (i *NativeInteractor[T]) Close() error {
	// If this is a transaction, attempting a rollback is a safe cleanup.
	if i.isTx {
		i.ix.Rollback(context.Background())
	}
	return i.ix.Close()
}

// SelectDocuments retrieves multiple documents matching the query.
func (i *NativeInteractor[T]) SelectDocuments(ctx context.Context, schema *definition.Schema, dsl *query.Query) ([]map[string]any, int64, error) {
	compiled, err := i.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, 0, common.SystemErrorFrom(err, ErrCouldNotBuildQuery.Code, ErrCouldNotBuildQuery.Message).WithOperation("native.NativeInteractor.SelectDocuments")
	}

	resultSchema, err := query.SchemaFromQuery(dsl, nil)
	if err != nil {
		return nil, 0, common.SystemErrorFrom(err, ErrCouldNotGetResultSchema.Code, ErrCouldNotGetResultSchema.Message).WithOperation("native.NativeInteractor.SelectDocuments")
	}

	return i.ix.Query(ctx, NativeQuery[T]{Query: compiled, Schema: resultSchema})
}

// SelectStream retrieves a stream of documents matching the query.
func (i *NativeInteractor[T]) SelectStream(ctx context.Context, sc *definition.Schema, dsl *query.Query) (<-chan map[string]any, <-chan error, error) {
	compiled, err := i.b.Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, nil, common.SystemErrorFrom(err, ErrCouldNotBuildQuery.Code, ErrCouldNotBuildQuery.Message).WithOperation("native.NativeInteractor.SelectStream")
	}

	resultSchema, err := query.SchemaFromQuery(dsl, nil)
	if err != nil {
		return nil, nil, common.SystemErrorFrom(err, ErrCouldNotGetResultSchema.Code, ErrCouldNotGetResultSchema.Message).WithOperation("native.NativeInteractor.SelectStream")
	}

	return i.ix.QueryStream(ctx, NativeQuery[T]{Query: compiled, Schema: resultSchema})
}

// UpdateDocuments updates documents matching the filter.
func (i *NativeInteractor[T]) UpdateDocuments(ctx context.Context, schema *definition.Schema, updates map[string]any, computedUpdates map[string]query.Query, filters *query.QueryFilter, returnDocs bool) ([]map[string]any, int64, error) {
	dsl := &query.Query{
		Target:  &query.QueryTarget{Name: schema.Name, Schema: schema},
		Filters: filters,
	}

	supportsReturning := i.qf.Capabilities().ReturnOnUpdate
	useNativeReturning := returnDocs && supportsReturning

	updatePayload := map[string]any{
		"set":       updates,
		"compute":   computedUpdates,
		"returning": useNativeReturning,
	}

	compiled, buildErr := i.b.Build(dsl, StmtUpdate, updatePayload)
	if buildErr != nil {
		return nil, 0, common.SystemErrorFrom(buildErr, ErrCouldNotBuildQuery.Code, ErrCouldNotBuildQuery.Message).WithOperation("native.NativeInteractor.UpdateDocuments")
	}

	// State 1: User wants documents AND interactor supports returning
	if useNativeReturning {
		returnedDocs, _, err := i.ix.Query(ctx, NativeQuery[T]{Query: compiled, Schema: schema})
		if err != nil {
			return nil, 0, common.SystemErrorFrom(err, ErrFailedToUpdateDocuments.Code, ErrFailedToUpdateDocuments.Message).WithOperation("native.NativeInteractor.UpdateDocuments")
		}
		affectedCount := int64(len(returnedDocs))
		return returnedDocs, affectedCount, nil
	}

	// State 2 & 3: Execute update without returning
	affectedCount, err := i.ix.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: schema})
	if err != nil {
		return nil, 0, common.SystemErrorFrom(err, ErrFailedToUpdateDocuments.Code, ErrFailedToUpdateDocuments.Message).WithOperation("native.NativeInteractor.UpdateDocuments")
	}

	// State 2: User wants documents but interactor doesn't support returning
	// Cannot reliably return updated documents without native RETURNING support
	// because the update may have modified fields used in the filter criteria
	if returnDocs {
		return []map[string]any{}, affectedCount, nil
	}

	// State 3: User doesn't want documents
	return nil, affectedCount, nil
}

// InsertDocuments inserts new documents.
func (i *NativeInteractor[T]) InsertDocuments(ctx context.Context, sc *definition.Schema, records []map[string]any) ([]map[string]any, error) {
	if len(records) == 0 {
		return []map[string]any{}, nil
	}
	dsl := &query.Query{
		Target: &query.QueryTarget{Name: sc.Name, Schema: sc},
	}
	compiled, err := i.b.Build(dsl, StmtInsert, records)
	if err != nil {
		return nil, common.SystemErrorFrom(err, ErrCouldNotBuildQuery.Code, ErrCouldNotBuildQuery.Message).WithOperation("native.NativeInteractor.InsertDocuments")
	}

	resultSchema, err := query.SchemaFromQuery(dsl, nil)
	if err != nil {
		return nil, common.SystemErrorFrom(err, ErrCouldNotGetResultSchema.Code, ErrCouldNotGetResultSchema.Message).WithOperation("native.NativeInteractor.InsertDocuments")
	}

	data, _, err := i.ix.Query(ctx, NativeQuery[T]{Query: compiled, Schema: resultSchema})
	return data, err
}

// DeleteDocuments deletes documents matching the filter.
func (i *NativeInteractor[T]) DeleteDocuments(ctx context.Context, schema *definition.Schema, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	if filters == nil && !unsafeDelete {
		return 0, ErrCouldNotDeleteWithoutFilters.WithOperation("native.NativeInteractor.DeleteDocuments")
	}

	dsl := &query.Query{
		Target:  &query.QueryTarget{Name: schema.Name, Schema: schema},
		Filters: filters,
	}
	compiled, err := i.b.Build(dsl, StmtDelete, nil)
	if err != nil {
		return 0, common.SystemErrorFrom(err, ErrCouldNotBuildQuery.Code, ErrCouldNotBuildQuery.Message).WithOperation("native.NativeInteractor.DeleteDocuments")
	}

	return i.ix.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: schema})
}

// Query executes a raw query directly against the database.
func (i *NativeInteractor[T]) Query(ctx context.Context, raw *query.Query) (*query.RawQueryResult, error) {
	compiled, err := i.b.Build(raw, StmtRaw, nil)
	if err != nil {
		return nil, common.SystemErrorFrom(err, ErrCouldNotBuildQuery.Code, ErrCouldNotBuildQuery.Message).WithOperation("native.NativeInteractor.Query")
	}

	return i.ix.ExecuteQuery(ctx, NativeQuery[T]{Query: compiled, Schema: nil})
}


// StartTransaction begins a new transaction and returns a new interactor for it.
func (i *NativeInteractor[T]) StartTransaction(ctx context.Context) (query.DatabaseInteractor, error) {
	i.txMu.Lock()
	defer i.txMu.Unlock()

	if i.isTx { // The system allows nesting transactions, or the facade of nesting transactions
		return i, nil
	}

	txExecutor, err := i.ix.BeginTransaction(ctx)
	if err != nil {
		return nil, common.SystemErrorFrom(err, ErrFailedToBeginTransaction.Code, ErrFailedToBeginTransaction.Message).WithOperation("native.NativeInteractor.StartTransaction")
	}

	tx := &NativeInteractor[T]{
		ix:     txExecutor,
		b:      i.b,
		qf:     i.qf,
		isTx:   true,
		logger: i.logger,
	}

	// Watch for ctx cancellation: rollback if still active.
	go func() {
		<-ctx.Done()
		if !tx.done.Load() {
			_ = tx.Rollback(context.Background())
		}
	}()

	// Return a new interactor instance specifically for this transaction.
	return tx, nil
}

// Commit commits the current transaction.
func (i *NativeInteractor[T]) Commit(ctx context.Context) error {
	if !i.isTx {
		return common.NewSystemError("ERR_NATIVE_COMMIT_NOT_APPLICABLE", "interactor is not a transaction").WithOperation("native.NativeInteractor.Commit")
	}
	if !i.done.CompareAndSwap(false, true) {
		return common.NewSystemError("ERR_NATIVE_COMMIT_NOT_APPLICABLE", "transaction already finished").WithOperation("native.NativeInteractor.Commit")
	}
	return i.ix.Commit(ctx)
}

// Rollback rolls back the current transaction.
func (i *NativeInteractor[T]) Rollback(ctx context.Context) error {
	if !i.isTx {
		return common.NewSystemError("ERR_NATIVE_ROLLBACK_NOT_APPLICABLE", "interactor is not a transaction").WithOperation("native.NativeInteractor.Rollback")
	}
	if !i.done.CompareAndSwap(false, true) {
		return common.NewSystemError("ERR_NATIVE_ROLLBACK_NOT_APPLICABLE", "transaction already finished").WithOperation("native.NativeInteractor.Rollback")
	}
	return i.ix.Rollback(ctx)
}

// HasTransaction returns true if the interactor is in a transaction.
func (i *NativeInteractor[T]) HasTransaction(ctx context.Context) bool {
	return i.isTx
}

// SchemaManager returns the schema management interface.
func (i *NativeInteractor[T]) SchemaManager() query.SchemaManager {
	return i
}

// CollectionExists checks if a collection exists. (Placeholder)
func (i *NativeInteractor[T]) CollectionExists(ctx context.Context, name string) (bool, error) {

	compiled, err := i.b.Build(&query.Query{Target: &query.QueryTarget{Name: name}}, StmtCheckCollection, nil)
	if err != nil {
		return false, common.SystemErrorFrom(err, ErrCouldNotBuildQuery.Code, ErrCouldNotBuildQuery.Message).WithOperation("native.NativeInteractor.CollectionExists")
	}

	result, _, err := i.ix.Query(ctx, NativeQuery[T]{Query: compiled, Schema: &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: name,
		},
	}})

	if err != nil {
		return false, ErrCouldNotCheckCollection.WithCause(err).WithOperation("native.NativeInteractor.CollectionExists")
	}

	return len(result) > 0, nil
}

// CreateCollection creates a new collection and its indexes within a single transaction.
func (i *NativeInteractor[T]) CreateCollection(ctx context.Context, sc definition.Schema) error {
	// This operation must be atomic, so we wrap it in a transaction.
	// If already in a transaction, it will use the existing one.
	tx, err := i.StartTransaction(ctx)
	if err != nil {
		// If we are already in a transaction, StartTransaction will fail,
		// which is not what we want. We want to *reuse* it.
		// So we check the error type. If it's a nesting error, we proceed with `i`.
		if errors.Is(err, ErrCannotNestTransactions) {
			tx = i
		} else {
			return err
		}
	}
	txInteractor := tx.(*NativeInteractor[T])

	// The core logic for creating the collection.
	err = txInteractor.createCollectionLogic(ctx, sc)
	if err != nil {
		txInteractor.Rollback(context.Background()) // Attempt rollback
		return err
	}

	// Only the top-level transaction manager should commit.
	if !i.isTx {
		return txInteractor.Commit(ctx)
	}

	return nil
}

// createCollectionLogic contains the core implementation for collection and index creation.
func (i *NativeInteractor[T]) createCollectionLogic(ctx context.Context, sc definition.Schema) error {
	// 1. Create the collection
	dsl := &query.Query{Target: &query.QueryTarget{Name: sc.Name, Schema: &sc}}
	compiled, err := i.b.Build(dsl, StmtCreateCollection, nil)
	if err != nil {
		return common.SystemErrorFrom(err, ErrCouldNotBuildCreateCollectionQuery.Code, ErrCouldNotBuildCreateCollectionQuery.Message).WithOperation("native.NativeInteractor.createCollectionLogic")
	}
	if _, err = i.ix.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: &sc}); err != nil {
		return common.SystemErrorFrom(err, ErrCouldNotCreateCollection.Code, ErrCouldNotCreateCollection.Message).WithOperation("native.NativeInteractor.createCollectionLogic")
	}

	// 2. Create associated indexes
	for _, index := range sc.Indexes {
		if index.Type == definition.IndexTypePrimary {
			continue // Primary indexes are usually created automatically
		}
		if err := i.CreateIndex(ctx, sc.Name, index); err != nil {
			return err // CreateIndex already returns a detailed NativeError
		}
	}
	return nil
}

// CreateIndex creates a new index.
func (i *NativeInteractor[T]) CreateIndex(ctx context.Context, collection string, index definition.Index) error {
	dsl := &query.Query{Target: &query.QueryTarget{Name: collection}}
	compiled, err := i.b.Build(dsl, StmtCreateIndex, index)
	if err != nil {
		return common.SystemErrorFrom(err, ErrCouldNotBuildCreateIndexQuery.Code, ErrCouldNotBuildCreateIndexQuery.Message).WithOperation("native.NativeInteractor.CreateIndex")
	}
	_, err = i.ix.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: nil})
	return err
}

// DropIndex drops an existing index.
func (i *NativeInteractor[T]) DropIndex(ctx context.Context, collection string, index definition.Index) error {
	dsl := &query.Query{Target: &query.QueryTarget{Name: collection}}
	compiled, err := i.b.Build(dsl, StmtDropIndex, index)
	if err != nil {
		return common.SystemErrorFrom(err, ErrCouldNotBuildDropIndexQuery.Code, ErrCouldNotBuildDropIndexQuery.Message).WithOperation("native.NativeInteractor.DropIndex")
	}
	_, err = i.ix.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: nil})
	return err
}

// DropCollection drops an entire collection.
func (i *NativeInteractor[T]) DropCollection(ctx context.Context, name string) error {
	dsl := &query.Query{Target: &query.QueryTarget{Name: name}}
	compiled, err := i.b.Build(dsl, StmtDropCollection, nil)
	if err != nil {
		return common.SystemErrorFrom(err, ErrCouldNotBuildDropCollectionQuery.Code, ErrCouldNotBuildDropCollectionQuery.Message).WithOperation("native.NativeInteractor.DropCollection")
	}
	_, err = i.ix.Exec(ctx, NativeQuery[T]{Query: compiled, Schema: nil})
	return err
}

// Capabilities returns the capabilities of the underlying database dialect.
func (i *NativeInteractor[T]) Capabilities() query.Capabilities {
	return i.qf.Capabilities()
}
