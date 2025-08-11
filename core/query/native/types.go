package native

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

// StatementType represents the type of database operation.
type StatementType string

const (
	StmtSelect       StatementType = "SELECT"
	StmtUpdate       StatementType = "UPDATE"
	StmtDelete       StatementType = "DELETE"
	StmtInsert       StatementType = "INSERT"
	StmtCreateCollection  StatementType = "CREATE_COLLECTION"
	StmtDropCollection    StatementType = "DROP_COLLECTION"
	StmtCreateIndex  StatementType = "CREATE_INDEX"
	StmtDropIndex    StatementType = "DROP_INDEX"
)

// NativeQuery is a generic, type-safe representation of a database-native query.
// T is the database's native query payload type.
type NativeQuery[T any] interface {
	// Raw returns the database's native query payload.
	Raw() T
	// StatementType returns the high-level type of the statement.
	StatementType() StatementType
}

// QueryFactory is implemented by each dialect (SQL, Mongo, etc.).
// It converts the DSL Query into a dialect-specific NativeQuery[T].
type QueryFactory[T any] interface {
	Build(q *query.Query, stmtType StatementType, extra any) (NativeQuery[T], error)
	Capabilities() query.Capabilities
}

type QueryExecutor[T any] interface {
	Query(ctx context.Context, compiled NativeQuery[T]) ([]common.Document, error)
	Exec(ctx context.Context, compiled NativeQuery[T]) (int64, error)
	QueryStream(ctx context.Context, compiled NativeQuery[T]) (<-chan common.Document, <-chan error, error)
	BeginTransaction(ctx context.Context) (QueryExecutor[T], error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	Close() error
}

// QueryBuilder is the public entrypoint for converting a DSL Query into a NativeQuery[T].
type NativeQueryBuilder[T any] interface {
	Build(q *query.Query, stmtType StatementType, extra any) (NativeQuery[T], error)
}
