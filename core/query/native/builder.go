package native

import "github.com/asaidimu/go-anansi/v8/core/query"


// NewQueryBuilder constructs a QueryBuilder for the given dialect factory.
func NewNativeQueryBuilder[T any](factory QueryFactory[T]) NativeQueryBuilder[T] {
	return &queryBuilder[T]{factory: factory}
}

// Internal implementation of QueryBuilder.
type queryBuilder[T any] struct {
	factory QueryFactory[T]
}

func (b *queryBuilder[T]) Build(q *query.Query, stmtType StatementType, extra any) (Query[T], error) {
	built, err := b.factory.Build(q, stmtType, extra)
	if err != nil {
		return nil, err
	}
	// Promote the DSL's result-row shape and document pool onto the compiled
	// native query so executors can scan rows directly into pooled containers.
	// Every dialect query embeds BaseQuery (to satisfy the Query interface),
	// so this assertion always succeeds; doing it here keeps dialect factories
	// from duplicating the copy.
	if setter, ok := built.(ShapePoolSetter); ok {
		setter.SetShape(q.Shape)
		setter.SetDocumentPool(q.DocumentPool)
	}
	return built, nil
}
