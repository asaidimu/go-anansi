package query

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/sqlite/types"
)

type sqliteQuery struct {
	payload  types.SQLitePayload
	stmtType native.StatementType
}

func (q *sqliteQuery) Raw() types.SQLitePayload {
	return q.payload
}

func (q *sqliteQuery) StatementType() native.StatementType {
	return q.stmtType
}

// sqliteNativeQueryFactory implements NativeQueryFactory[types.SQLitePayload].
type sqliteFactory struct{
	paramCounter int
	aliases      map[string]string
}

// NewSQLiteFactory creates a new factory for building SQLite queries.
func NewSQLiteFactory() native.QueryFactory[types.SQLitePayload] {
	return newSQLiteFactory()
}

func newSQLiteFactory() *sqliteFactory {
	return &sqliteFactory{
		paramCounter: 0,
		aliases:      make(map[string]string),
	}
}

func (f *sqliteFactory) Build(
	q *query.Query,
	stmtType native.StatementType,
	extra any,
) (native.NativeQuery[types.SQLitePayload], error) {
	// Reset internal state for each build.
	f.paramCounter = 0
	f.aliases = make(map[string]string)


	var sqlTree SQLNode
	var err error


	 switch stmtType {
	case native.StmtSelect:
		sqlTree, err = f.buildSelectTree(q)
	case native.StmtUpdate:
		sqlTree, err = f.buildUpdateTree(q, extra)
	case native.StmtDelete:
		sqlTree, err = f.buildDeleteTree(q)
	case native.StmtInsert:
		sqlTree, err = f.buildInsertTree(q, extra)
	case native.StmtCreateCollection:
		sqlTree, err = f.buildCreateTableTree(q)
	case native.StmtDropCollection:
		sqlTree, err = f.buildDropTableTree(q)
	case native.StmtCreateIndex:
		sqlTree, err = f.buildCreateIndexTree(q, extra)
	case native.StmtDropIndex:
		sqlTree, err = f.buildDropIndexTree(q, extra)
	default:
		return nil, fmt.Errorf("unsupported statement type: %s", stmtType)
	}

	if err != nil {
		return nil, err
	}

	sql, params, err := sqlTree.Value()
	if err != nil {
		return nil, err
	}

	resultSchema,  err := query.SchemaFromQuery(q, nil)

	if (stmtType == native.StmtInsert || stmtType == native.StmtSelect) && (err != nil || resultSchema == nil)  {
		return nil, fmt.Errorf("failed to get schema from query: %w", err)
	}

	nativeQuery := &sqliteQuery{
		payload:  types.SQLitePayload{SQL: sql, Params: params, Schema: resultSchema},
		stmtType: stmtType,
	}

	return nativeQuery, nil
}
