package query

import (
        "fmt"

        "github.com/asaidimu/go-anansi/v8/core/query"
)

// buildCreateViewTree dispatches a StmtCreateView to a createViewTree
// that emits CREATE TABLE <name> AS SELECT ...
//
// The `extra` parameter must be the view's stored *query.Query (the
// SELECT to materialize). The query's Target.Name must carry the
// physical name of the underlying collection; joins must reference
// physical names too. The caller is responsible for resolving logical
// names to physical names before invoking this builder.
//
// The materialized view's physical table name is taken from q.Target.Name
// (the registry rewrites this to the generated physical name before
// calling Build).
func (f *sqliteFactory) buildCreateViewTree(q *query.Query, extra any) (SQLNode, error) {
        selectQuery, ok := extra.(*query.Query)
        if !ok {
                return nil, ErrCollectionViewSelectNotDefined.WithCause(
                        fmt.Errorf("StmtCreateView: expected *query.Query, got %T", extra))
        }
        name := ""
        if q.Target != nil {
                name = q.Target.Name
        }
        return &createViewTree{
                name:        name,
                selectQuery: selectQuery,
                factory:     f,
        }, nil
}

// buildRefreshViewTree dispatches a StmtRefreshView to a sequence of
// DROP TABLE + CREATE TABLE AS SELECT. The two statements are emitted
// as a single SQL string (semicolon-separated) so they execute in the
// same Exec call.
//
// The `extra` parameter must be the view's stored *query.Query. The
// physical name comes from q.Target.Name.
func (f *sqliteFactory) buildRefreshViewTree(q *query.Query, extra any) (SQLNode, error) {
        selectQuery, ok := extra.(*query.Query)
        if !ok {
                return nil, ErrCollectionViewSelectNotDefined.WithCause(
                        fmt.Errorf("StmtRefreshView: expected *query.Query, got %T", extra))
        }
        name := ""
        if q.Target != nil {
                name = q.Target.Name
        }
        return &refreshViewTree{
                name:        name,
                selectQuery: selectQuery,
                factory:     f,
        }, nil
}

// refreshViewTree emits DROP TABLE IF EXISTS <name>; CREATE TABLE <name> AS SELECT ...
// in a single Value() call. The two statements are concatenated with a
// semicolon so the executor runs them in order within one Exec.
type refreshViewTree struct {
        name        string
        selectQuery *query.Query
        factory     *sqliteFactory
}

func (t *refreshViewTree) Value() (string, []any, error) {
        if t.name == "" {
                return "", nil, ErrCollectionViewNameNotDefined
        }
        if t.selectQuery == nil {
                return "", nil, ErrCollectionViewSelectNotDefined
        }
        if t.factory == nil {
                return "", nil, ErrCollectionViewFactoryNotDefined
        }

        // Build the SELECT sub-tree in a child scope so the factory's
        // alias/schema maps don't leak.
        selectFactory := t.factory.createChildScope()
        selectTree, err := selectFactory.buildSelectTree(t.selectQuery)
        if err != nil {
                return "", nil, err
        }
        selectSQL, selectParams, err := selectTree.Value()
        if err != nil {
                return "", nil, err
        }

        // DROP TABLE IF EXISTS <name>; CREATE TABLE <name> AS <select>;
        sql := fmt.Sprintf("DROP TABLE IF EXISTS %s; CREATE TABLE IF NOT EXISTS %s AS %s;",
                quoteIdentifier(t.name), quoteIdentifier(t.name), selectSQL)
        return sql, selectParams, nil
}

// createViewTree emits CREATE TABLE <name> AS SELECT ... — the SQLite
// equivalent of a materialized view. The SELECT is produced by delegating
// to the existing buildSelectTree against the view's stored query, then
// wrapping it in `CREATE TABLE <name> AS (...)`.
//
// SQLite does not have a native "materialized view" concept; the
// convention is to create a regular table populated by a SELECT. The
// table can then have its own indexes (created separately via
// CreateIndex after the CTAS runs). Refresh is DROP TABLE + CREATE TABLE
// AS SELECT again.
//
// The caller is responsible for resolving all logical names in the
// view's stored query to physical names before invoking this tree —
// the emitted SQL references physical table names directly.
type createViewTree struct {
        // name is the physical name of the materialized view table.
        name string
        // selectQuery is the view's stored query, already resolved to
        // physical table names. It must have Target.Name = physical name
        // of the underlying collection, and any joins must reference
        // physical names too.
        selectQuery *query.Query
        // factory builds the SELECT sub-tree. We reuse the existing
        // sqliteFactory so the SELECT's WHERE/JOIN/ORDER BY/LIMIT clauses
        // are generated by the same code path as a regular read.
        factory *sqliteFactory
}

func (t *createViewTree) Value() (string, []any, error) {
        if t.name == "" {
                return "", nil, ErrCollectionViewNameNotDefined
        }
        if t.selectQuery == nil {
                return "", nil, ErrCollectionViewSelectNotDefined
        }
        if t.factory == nil {
                return "", nil, ErrCollectionViewFactoryNotDefined
        }

        // Build the SELECT sub-tree using a child scope so the factory's
        // alias/schema maps don't leak into the outer (CREATE VIEW) scope.
        // The registry already added a projection with explicit column
        // aliases to the resolved query, so the SELECT will produce
        // plain column names (not table-qualified).
        selectFactory := t.factory.createChildScope()
        selectTree, err := selectFactory.buildSelectTree(t.selectQuery)
        if err != nil {
                return "", nil, err
        }
        selectSQL, selectParams, err := selectTree.Value()
        if err != nil {
                return "", nil, err
        }

        // CREATE TABLE <name> AS <select>;
        sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s AS %s;",
                quoteIdentifier(t.name), selectSQL)
        return sql, selectParams, nil
}

// dropViewTree emits DROP TABLE IF EXISTS <name>. Materialized views are
// regular tables in SQLite, so dropping them uses the same DDL as dropping
// any collection. This tree exists only for symmetry with createViewTree
// and to allow the registry to dispatch via a distinct statement type
// if it ever needs to (e.g. to also drop view-only indexes).
type dropViewTree struct {
        name string
}

func (t *dropViewTree) Value() (string, []any, error) {
        if t.name == "" {
                return "", nil, ErrCollectionViewNameNotDefined
        }
        return fmt.Sprintf("DROP TABLE IF EXISTS %s;", quoteIdentifier(t.name)), nil, nil
}
