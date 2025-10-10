package executor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/sqlite/types"
	"go.uber.org/zap"
)

// runner is an interface that abstracts the common methods of *sql.DB and *sql.Tx,
// allowing for the same code to be used for both transactional and non-transactional
// database operations.
type runner interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type sqliteExecutor struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *zap.Logger
}

var _ (native.QueryExecutor[types.SQLitePayload]) = (*sqliteExecutor)(nil)

// NewSQLiteExecutor creates a new instance of the SQLiteInteractor. It can be
// configured to operate in transactional mode by providing a non-nil *sql.Tx.
func NewSQLiteExecutor(db *sql.DB, logger *zap.Logger) (native.QueryExecutor[types.SQLitePayload], error) {
	return newSQLiteExecutor(db, logger, nil)
}

func newSQLiteExecutor(db *sql.DB, logger *zap.Logger, tx *sql.Tx) (native.QueryExecutor[types.SQLitePayload], error) {
	return &sqliteExecutor{
		db:     db,
		logger: logger,
		tx:     tx,
	}, nil
}

func (s *sqliteExecutor) Query(ctx context.Context, query native.NativeQuery[types.SQLitePayload]) ([]data.Document, error) {
	q := query.Query
	payload := q.Raw()
	rows, err := s.runner().QueryContext(ctx, payload.SQL, payload.Params...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute %s query: %w", q.StatementType(), err)
	}
	defer rows.Close()

	var results []data.Document = nil
	if rows == nil {
		return make([]data.Document, 0), nil
	}

	results, err = ReadRows(s.logger, query.Schema, rows)
	return results, err
}

func (s *sqliteExecutor) QueryStream(ctx context.Context, compiled native.NativeQuery[types.SQLitePayload]) (<-chan data.Document, <-chan error, error) {
	q := compiled.Query
	payload := q.Raw()
	rows, err := s.runner().QueryContext(ctx, payload.SQL, payload.Params...)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute %s query: %w", q.StatementType(), err)
	}

	docChan := make(chan data.Document)
	errChan := make(chan error, 1)

	go func() {
		defer close(docChan)
		defer close(errChan)
		defer rows.Close()

		utilDocChan, utilErrChan := readRowsToDocs(rows)

		for {
			select {
			case row, ok := <-utilDocChan:
				if !ok {
					close(docChan)
					return
				}
				select {
				case docChan <- row:
				case <-ctx.Done():
					return
				}
			case err, ok := <-utilErrChan:
				if ok {
					errChan <- err
				}
				close(docChan)
				close(errChan)
				return
			}
		}
	}()

	return docChan, errChan, nil
}

func (s *sqliteExecutor) Exec(ctx context.Context, compiled native.NativeQuery[types.SQLitePayload]) (int64, error) {
	q := compiled.Query
	payload := q.Raw()
	result, err := s.runner().ExecContext(ctx, payload.SQL, payload.Params...)
	if err != nil {
		return 0, fmt.Errorf("failed to execute %s query: %w", q.StatementType(), err)
	}
	return result.RowsAffected()
}

func (s *sqliteExecutor) ExecuteQuery(ctx context.Context, compiled native.NativeQuery[types.SQLitePayload]) (*query.RawQueryResult, error) {
	q := compiled.Query
	payload := q.Raw()
	template := strings.TrimSpace(strings.ToUpper(payload.SQL))

	if strings.HasPrefix(template, "SELECT") {
		rows, err := s.runner().QueryContext(ctx, payload.SQL, payload.Params...)
		if err != nil {
			return &query.RawQueryResult{Success: false, Message: fmt.Sprintf("failed to execute raw SELECT query: %v", err)}, err
		}
		defer rows.Close()

		results, err := ReadRows(s.logger, compiled.Schema, rows)
		if err != nil {
			return &query.RawQueryResult{Success: false, Message: fmt.Sprintf("failed to read raw SELECT query results: %v", err)}, err
		}

		return &query.RawQueryResult{
			Data:    results,
			Count:   len(results),
			Success: true,
			Message: "Raw SELECT query executed successfully",
		}, nil
	} else {
		result, err := s.runner().ExecContext(ctx, payload.SQL, payload.Params...)
		if err != nil {
			return &query.RawQueryResult{Success: false, Message: fmt.Sprintf("failed to execute raw EXEC query: %v", err)}, err
		}
		affectedRows, _ := result.RowsAffected()
		return &query.RawQueryResult{
			AffectedRows: affectedRows,
			Success:      true,
			Message:      "Raw EXEC query executed successfully",
		}, nil
	}
}

func (s *sqliteExecutor) BeginTransaction(ctx context.Context) (native.QueryExecutor[types.SQLitePayload], error) {
	if s.tx != nil {
		return nil, fmt.Errorf("cannot begin a new transaction: already in a transaction")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	ti, err := newSQLiteExecutor(s.db, s.logger, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to create new interactor for transaction: %w", err)
	}

	return ti, nil
}

func (i *sqliteExecutor) Commit(ctx context.Context) error {
	if i.tx == nil {
		return fmt.Errorf("commit not applicable: not in a transactional context")
	}
	return i.tx.Commit()
}

func (i *sqliteExecutor) Rollback(ctx context.Context) error {
	if i.tx == nil {
		return fmt.Errorf("rollback not applicable: not in a transactional context")
	}
	return i.tx.Rollback()
}

func (i *sqliteExecutor) Close() error {
	if i.tx == nil {
		return i.db.Close()
	}

	i.tx.Rollback()
	i.db.Close()
	return nil
}

// runner returns the appropriate dbRunner for the current context, either the
// database connection pool or the active transaction.
func (i *sqliteExecutor) runner() runner {
	if i.tx != nil {
		return i.tx
	}
	return i.db
}
