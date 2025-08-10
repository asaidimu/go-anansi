package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteInteractor implements the query.DatabaseInteractor and query.SchemaManager interfaces for SQLite.
type SQLiteInteractor struct {
	db                    *sql.DB
	tx                    *sql.Tx
	queryGeneratorFactory query.QueryGeneratorFactory
	logger                *zap.Logger
	options               *SqliteInteractorOptions
}

var _ query.BaseDatabaseInteractor = (*SQLiteInteractor)(nil)
var _ query.DatabaseInteractor = (*SQLiteInteractor)(nil)
var _ query.TransactionalDatabaseInteractor = (*SQLiteInteractor)(nil)


// NewSQLiteInteractor creates a new instance of the SQLiteInteractor. It can be
// configured to operate in transactional mode by providing a non-nil *sql.Tx.
func NewSQLiteInteractor(db *sql.DB, logger *zap.Logger, options *SqliteInteractorOptions) (query.DatabaseInteractor, error) {
	return newSQLiteInteractor(db, logger, options, nil)
}

// NewSQLiteInteractor creates a new instance of the SQLiteInteractor. It can be
// configured to operate in transactional mode by providing a non-nil *sql.Tx.
func newSQLiteInteractor(db *sql.DB, logger *zap.Logger, options *SqliteInteractorOptions, tx *sql.Tx) (*SQLiteInteractor, error) {
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to SQLite database: %w", err)
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if options == nil {
		options = DefaultInteractorOptions()
	}
	return &SQLiteInteractor{
		db:                    db,
		tx:                    tx,
		options:               options,
		queryGeneratorFactory: NewSqliteQueryGeneratorFactory(),
		logger:                logger,
	}, nil
}

// Close closes the database connection.
func (i *SQLiteInteractor) Close() error {
	return i.db.Close()
}

// SelectDocuments retrieves documents from the SQLite database.
func (i *SQLiteInteractor) SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.Query) ([]common.Document, error) {
	queryGenerator, err := i.queryGeneratorFactory.CreateGenerator(schema)
	if err != nil {
		return nil, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateSelectSQL(dsl)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SQL query: %w", err)
	}

	i.logger.Debug("Executing SQL SELECT", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	rows, err := i.runner().QueryContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		i.logger.Error("Failed to execute SELECT query", zap.Error(err), zap.String("sql", sqlQuery))
		return nil, fmt.Errorf("failed to execute SELECT query: %w \n %s", err, sqlQuery)
	}
	defer rows.Close()
	return readRows(i.logger, schema, rows)
}

// SelectStream streams documents from the SQLite database.
func (i *SQLiteInteractor) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan common.Document, <-chan error, error) {
	queryGenerator, err := i.queryGeneratorFactory.CreateGenerator(sc)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateSelectSQL(dsl)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate SQL query: %w", err)
	}

	rows, err := i.runner().QueryContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute SELECT query: %w", err)
	}

	docChan := make(chan common.Document)
	errChan := make(chan error, 1)

	go func() {
		defer close(docChan)
		defer close(errChan)
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			errChan <- fmt.Errorf("failed to get columns: %w", err)
			return
		}

		for rows.Next() {
			row := make(common.Document, len(columns))
			values := make([]any, len(columns))
			scanArgs := make([]any, len(columns))
			for i := range values {
				scanArgs[i] = &values[i]
			}

			if err := rows.Scan(scanArgs...); err != nil {
				errChan <- fmt.Errorf("failed to scan row: %w", err)
				return
			}

			for i, col := range columns {
				row[col] = values[i]
			}

			select {
			case docChan <- row:
			case <-ctx.Done():
				return
			}
		}
		if err := rows.Err(); err != nil {
			errChan <- err
		}
	}()

	return docChan, errChan, nil
}

// UpdateDocuments updates documents in the SQLite database.
func (i *SQLiteInteractor) UpdateDocuments(ctx context.Context, schema *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	queryGenerator, err := i.queryGeneratorFactory.CreateGenerator(schema)
	if err != nil {
		return 0, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateUpdateSQL(updates, filters)
	if err != nil {
		return 0, fmt.Errorf("failed to generate SQL UPDATE query: %w", err)
	}

	i.logger.Debug("Executing SQL UPDATE", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	result, err := i.runner().ExecContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		i.logger.Error("Failed to execute UPDATE query", zap.Error(err), zap.String("sql", sqlQuery))
		return 0, fmt.Errorf("failed to execute UPDATE query: %w", err)
	}
	return result.RowsAffected()
}

// InsertDocuments inserts documents into the SQLite database.
func (i *SQLiteInteractor) InsertDocuments(ctx context.Context, sc *schema.SchemaDefinition, records []common.Document) ([]common.Document, error) {
	if len(records) == 0 {
		return []common.Document{}, nil
	}
	queryGenerator, err := i.queryGeneratorFactory.CreateGenerator(sc)
	if err != nil {
		return nil, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateInsertSQL(records)
	if err != nil {
		return nil, fmt.Errorf("failed to generate INSERT SQL: %w", err)
	}

	i.logger.Debug("Executing SQL INSERT with RETURNING clause", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	rows, err := i.runner().QueryContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		i.logger.Error("Failed to execute INSERT ... RETURNING query", zap.Error(err), zap.String("sql", sqlQuery))
		return nil, fmt.Errorf("failed to execute INSERT ... RETURNING query: %w", err)
	}
	defer rows.Close()
	return readRows(i.logger, sc, rows)
}

// DeleteDocuments deletes documents from the SQLite database.
func (i *SQLiteInteractor) DeleteDocuments(ctx context.Context, schema *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	queryGenerator, err := i.queryGeneratorFactory.CreateGenerator(schema)
	if err != nil {
		return 0, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateDeleteSQL(filters, unsafeDelete)
	if err != nil {
		return 0, fmt.Errorf("failed to generate DELETE SQL: %w", err)
	}

	i.logger.Debug("Executing SQL DELETE", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	result, err := i.runner().ExecContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		i.logger.Error("Failed to execute DELETE query", zap.Error(err), zap.String("sql", sqlQuery))
		return 0, fmt.Errorf("failed to execute DELETE query: %w", err)
	}
	return result.RowsAffected()
}


// StartTransaction begins a new database transaction.
func (i *SQLiteInteractor) StartTransaction(ctx context.Context) (query.TransactionalDatabaseInteractor, error) {
	if i.tx != nil {
		return i, nil
		// return nil, fmt.Errorf("cannot start a new transaction from an existing transactional interactor")
	}

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	i.logger.Debug("Transaction initiated, returning new transactional interactor")
	ti, err := newSQLiteInteractor(i.db, i.logger, i.options, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to create new interactor for transaction: %w", err)
	}
	return ti, nil
}

// Commit commits the current transaction.
func (i *SQLiteInteractor) Commit(ctx context.Context) error {
	if i.tx == nil {
		return fmt.Errorf("commit not applicable: not in a transactional context")
	}
	i.logger.Debug("Committing transaction")
	return i.tx.Commit()
}

// Rollback rolls back the current transaction.
func (i *SQLiteInteractor) Rollback(ctx context.Context) error {
	if i.tx == nil {
		return fmt.Errorf("rollback not applicable: not in a transactional context")
	}
	i.logger.Debug("Rolling back transaction")
	return i.tx.Rollback()
}

// SchemaManager returns the SchemaManager interface for SQLite.
func (i *SQLiteInteractor) SchemaManager() query.SchemaManager {
	return i
}
