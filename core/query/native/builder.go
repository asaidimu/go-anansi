package native

import "github.com/asaidimu/go-anansi/v6/core/query"


// NewQueryBuilder constructs a QueryBuilder for the given dialect factory.
func NewNativeQueryBuilder[T any](factory QueryFactory[T]) NativeQueryBuilder[T] {
	return &queryBuilder[T]{factory: factory}
}

// Internal implementation of QueryBuilder.
type queryBuilder[T any] struct {
	factory QueryFactory[T]
}

func (b *queryBuilder[T]) Build(q *query.Query, stmtType StatementType, extra any) (NativeQuery[T], error) {
	return b.factory.Build(q, stmtType, extra)
}
