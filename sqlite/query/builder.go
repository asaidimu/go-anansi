package query

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
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


// NewSQLiteFactory creates a new factory for building SQLite queries.
func NewSQLiteFactory() native.QueryFactory[types.SQLitePayload] {
	return newSQLiteFactory()
}

func (x *sqliteFactory) Build(
	q *query.Query,
	stmtType native.StatementType,
	extra any,
) (native.Query[types.SQLitePayload], error) {

	f := newSQLiteFactory()

	// Check for raw query first - raw takes precedence
	if q.Raw != nil {
		return f.buildRawQuery(q, stmtType)
	}

	var sqlTree SQLNode
	var err error

	switch stmtType {
	case native.StmtSelect:
		sqlTree, err = f.buildSelectTree(q)
	case native.StmtUpdate:
		updatePayload, ok := extra.(map[string]any)
		if !ok {
			return nil, ErrBuilderInvalidUpdatePayload.WithCause(fmt.Errorf("invalid update payload type: expected map[string]any, got %T", extra))
		}
		returnDocs := false
		if ret, ok := updatePayload["returning"].(bool); ok {
			returnDocs = ret
		}
		sqlTree, err = f.buildUpdateTree(q, updatePayload, returnDocs)
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
	case native.StmtCheckCollection:
		sqlTree, err = f.buildCheckTableTree(q)
	default:
		return nil, ErrBuilderUnsupportedStatementType.WithCause(fmt.Errorf("unsupported statement type: %s", stmtType))
	}

	if err != nil {
		return nil, err
	}

	sql, params, err := sqlTree.Value()
	if err != nil {
		return nil, err
	}

	nativeQuery := &sqliteQuery{
		payload:  types.SQLitePayload{SQL: sql, Params: params},
		stmtType: stmtType,
	}

	return nativeQuery, nil
}
func (f *sqliteFactory) buildRawQuery(q *query.Query, stmtType native.StatementType) (native.Query[types.SQLitePayload], error) {
	raw := q.Raw
	finalSQL := raw.Template

	var sc *schema.SchemaDefinition
	if q.Target != nil {
		sc = q.Target.Schema
	}

	nativeQuery := &sqliteQuery{
		payload:  types.SQLitePayload{SQL: finalSQL, Params: raw.Parameters, Schema: sc},
		stmtType: stmtType,
	}

	return nativeQuery, nil
}
