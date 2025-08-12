package native

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

type NativeInteractor[T any] struct {
	ix            *QueryExecutor[T]
	qf            *QueryFactory[T]
	b             *NativeQueryBuilder[T]
	isTransaction bool
}

var _ query.BaseDatabaseInteractor = (*NativeInteractor[any])(nil)
var _ query.DatabaseInteractor = (*NativeInteractor[any])(nil)
var _ query.TransactionalDatabaseInteractor = (*NativeInteractor[any])(nil)

func NewNativeInteractor[T any](ix *QueryExecutor[T], qf *QueryFactory[T]) (query.DatabaseInteractor, error) {
	return newNativeInteractor(ix, qf, false)
}

func newNativeInteractor[T any](ix *QueryExecutor[T], qf *QueryFactory[T], isTransaction bool) (*NativeInteractor[T], error) {
	b := NewNativeQueryBuilder(*qf)
	return &NativeInteractor[T]{
		ix:            ix,
		qf:            qf,
		b:             &b,
		isTransaction: isTransaction,
	}, nil
}

// Close closes the database connection.
func (i *NativeInteractor[T]) Close() error {
	return (*i.ix).Close()
}

// SelectDocuments retrieves documents from the Native database.
func (i *NativeInteractor[T]) SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {
	compiled, err := (*i.b).Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, fmt.Errorf("could not get a query: %w", err)
	}

	data, err := (*i.ix).Query(ctx, compiled)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SelectStream streams documents from the Native database.
func (i *NativeInteractor[T]) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan data.Document, <-chan error, error) {
	compiled, err := (*i.b).Build(dsl, StmtSelect, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get a query: %w", err)
	}

	return (*i.ix).QueryStream(ctx, compiled)
}

// UpdateDocuments updates documents in the Native database.
func (i *NativeInteractor[T]) UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	compiled, err := (*i.b).Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtUpdate, updates)

	if err != nil {
		return 0, fmt.Errorf("could not get a query: %w", err)
	}

	return (*i.ix).Exec(ctx, compiled)
}

// InsertDocuments inserts documents into the Native database.
func (i *NativeInteractor[T]) InsertDocuments(ctx context.Context, sc *schema.SchemaDefinition, records []data.Document) ([]data.Document, error) {
	if len(records) == 0 {
		return []data.Document{}, nil
	}

	compiled, err := (*i.b).Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   sc.Name,
			Schema: sc,
		},
	}, StmtInsert, records)

	if err != nil {
		return nil, fmt.Errorf("could not get a query: %w", err)
	}

	return (*i.ix).Query(ctx, compiled)
}

// DeleteDocuments deletes documents from the Native database.
func (i *NativeInteractor[T]) DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	if filters == nil && !unsafeDelete {
		return 0, fmt.Errorf("could not delete without filters")
	}

	compiled, err := (*i.b).Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   schema.Name,
			Schema: schema,
		},
		Filters: filters,
	}, StmtDelete, nil)

	if err != nil {
		return 0, fmt.Errorf("could not get a query: %w", err)
	}

	return (*i.ix).Exec(ctx, compiled)
}

// StartTransaction begins a new database transaction.
func (i *NativeInteractor[T]) StartTransaction(ctx context.Context) (query.TransactionalDatabaseInteractor, error) {
	if i.isTransaction {
		return nil, fmt.Errorf("cannot nest transactions")
	}

	tx, err := (*i.ix).BeginTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	ti, err := newNativeInteractor(&tx, i.qf, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create new interactor for transaction: %w", err)
	}
	return ti, nil
}

func (i *NativeInteractor[T]) HasTransaction(ctx context.Context) bool {
	return i.isTransaction
}

// Commit commits the current transaction.
func (i *NativeInteractor[T]) Commit(ctx context.Context) error {
	if i.isTransaction == false {
		return fmt.Errorf("commit not applicable: not in a transactional context")
	}
	return (*i.ix).Commit(ctx)
}

// Rollback rolls back the current transaction.
func (i *NativeInteractor[T]) Rollback(ctx context.Context) error {
	if i.isTransaction == false {
		return fmt.Errorf("rollback not applicable: not in a transactional context")
	}
	return (*i.ix).Commit(ctx)
}

// SchemaManager returns the SchemaManager interface for Native.
func (i *NativeInteractor[T]) SchemaManager() query.SchemaManager {
	return i
}

// Capabilities returns the capabilities of the Native database.
func (i *NativeInteractor[T]) Capabilities() query.Capabilities {
	return query.Capabilities{}
}

// CollectionExists checks if a collection exists in the Native database.
func (i *NativeInteractor[T]) CollectionExists(name string) (bool, error) {
	return false, nil
}

// CreateCollection creates a new collection in the Native database.
func (i *NativeInteractor[T]) CreateCollection(schema schema.SchemaDefinition) error {
	return nil
}

// CreateIndex creates a new index in the Native database.
func (i *NativeInteractor[T]) CreateIndex(name string, index schema.IndexDefinition) error {
	return nil
}

// DropCollection deletes a collection from the Native database.
func (i *NativeInteractor[T]) DropCollection(name string) error {
	return nil
}
