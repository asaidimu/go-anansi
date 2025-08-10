package sqlite

import (
	"context"
	"database/sql"
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

// runner returns the appropriate dbRunner for the current context, either the
// database connection pool or the active transaction.
func (i *SQLiteInteractor) runner() runner {
	if i.tx != nil {
		return i.tx
	}
	return i.db
}

