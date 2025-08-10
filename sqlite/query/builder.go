package query

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
)

type SQLitePayload struct {
	SQL    string
	Params []any
}

type sqliteQuery struct {
	payload  SQLitePayload
	stmtType native.StatementType
}

func (q *sqliteQuery) Raw() SQLitePayload {
	return q.payload
}

func (q *sqliteQuery) StatementType() native.StatementType {
	return q.stmtType
}

// sqliteNativeQueryFactory implements NativeQueryFactory[SQLitePayload].
type sqliteFactory struct{
	paramCounter int
	aliases      map[string]string
}

func newSQLiteFactory() *sqliteFactory {
	return &sqliteFactory{
		paramCounter: 0,
		aliases:      make(map[string]string),
	}
}
// NewSQLiteFactory creates a new factory for building SQLite queries.
func NewSQLiteFactory() native.QueryFactory[SQLitePayload] {
	return newSQLiteFactory()
}

func (f *sqliteFactory) Build(
	q *query.Query,
	stmtType native.StatementType,
	extra any,
) (native.NativeQuery[SQLitePayload], error) {
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

	nativeQuery := &sqliteQuery{
		payload:  SQLitePayload{SQL: sql, Params: params},
		stmtType: stmtType,
	}

	return nativeQuery, nil
}
