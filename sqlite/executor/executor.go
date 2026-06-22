package executor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/sqlite/types"
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

func (s *sqliteExecutor) Query(ctx context.Context, query native.NativeQuery[types.SQLitePayload]) ([]map[string]any, int64, error) {
	q := query.Query
	payload := q.Raw()
	rows, err := s.runner().QueryContext(ctx, payload.SQL, payload.Params...)
	if err != nil {
		return nil, 0, translateError(err).WithOperation("Query")
	}
	defer rows.Close()

	var results []map[string]any = nil
	if rows == nil {
		return make([]map[string]any, 0), 0, nil
	}

	results, count, err := ReadRows(s.logger, query.Schema, rows)
	return results, count, err
}

func (s *sqliteExecutor) QueryStream(ctx context.Context, compiled native.NativeQuery[types.SQLitePayload]) (<-chan map[string]any, <-chan error, error) {
	q := compiled.Query
	payload := q.Raw()
	rows, err := s.runner().QueryContext(ctx, payload.SQL, payload.Params...)

	if err != nil {
		return nil, nil, translateError(err).WithOperation("QueryStream")
	}

	docChan := make(chan map[string]any)
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
		return 0, translateError(err).WithOperation("Exec")
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
			return nil, translateError(err).WithOperation("ExecuteQuery")
		}
		defer rows.Close()

		results, _, err := ReadRows(s.logger, compiled.Schema, rows)
		if err != nil {
			return nil, err
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
			return nil, translateError(err).WithOperation("ExecuteQuery")
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
		return nil, native.ErrCannotNestTransactions.WithOperation("BeginTransaction")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, native.ErrTransactionFailed.WithCause(err).WithOperation("BeginTransaction")
	}

	ti, err := newSQLiteExecutor(s.db, s.logger, tx)
	if err != nil {
		// If creating the executor fails, we must roll back the transaction
		if rbErr := tx.Rollback(); rbErr != nil {
			return nil, native.ErrTransactionFailed.WithCause(err).WithIssue(common.Issue{
				Code:    native.ErrTransactionFailed.Code,
				Message: fmt.Sprintf("failed to rollback after failing to create transaction executor: %v", rbErr),
			}).WithOperation("BeginTransaction")
		}
		return nil, native.ErrTransactionFailed.WithCause(err).WithOperation("BeginTransaction")
	}

	return ti, nil
}

func (i *sqliteExecutor) Commit(ctx context.Context) error {
	if i.tx == nil {
		return native.ErrTransactionFailed.WithOperation("Commit").WithMessage("not in a transactional context")
	}
	err := i.tx.Commit()
	i.tx = nil // Clear the transaction reference
	if err != nil {
		return native.ErrTransactionFailed.WithCause(err).WithOperation("Commit")
	}
	return nil
}

func (i *sqliteExecutor) Rollback(ctx context.Context) error {
	if i.tx == nil {
		return native.ErrTransactionFailed.WithOperation("Rollback").WithMessage("not in a transactional context")
	}
	err := i.tx.Rollback()
	i.tx = nil // Clear the transaction reference
	if err != nil {
		return native.ErrTransactionFailed.WithCause(err).WithOperation("Rollback")
	}
	return nil
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
