// Package sqlite provides a concrete implementation of the persistence.DatabaseInteractor
// interface for SQLite databases. It handles the specifics of connecting to, querying,
// and managing a SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/core/persistence"
	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
	"go.uber.org/zap"
)

// dbRunner is an interface that abstracts the common methods of *sql.DB and *sql.Tx,
// allowing for the same code to be used for both transactional and non-transactional
// database operations.
type dbRunner interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLiteInteractor is a concrete implementation of the persistence.DatabaseInteractor
// interface for SQLite. It manages the database connection, generates SQL queries,
// and executes them against the database. It can operate in both transactional and
// non-transactional modes.	type SQLiteInteractor struct {
	db                    *sql.DB
	tx                    *sql.Tx
	queryGeneratorFactory query.QueryGeneratorFactory
	logger                *zap.Logger
	options               *persistence.InteractorOptions
}

// Ensure SQLiteInteractor implements the persistence.DatabaseInteractor interface.
var _ persistence.DatabaseInteractor = (*SQLiteInteractor)(nil)

// NewSQLiteInteractor creates a new instance of the SQLiteInteractor. It can be
// configured to operate in transactional mode by providing a non-nil *sql.Tx.
func NewSQLiteInteractor(db *sql.DB, logger *zap.Logger, options *persistence.InteractorOptions, tx *sql.Tx) persistence.DatabaseInteractor {
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
	}
}

// runner returns the appropriate dbRunner for the current context, either the
// database connection pool or the active transaction.
func (i *SQLiteInteractor) runner() dbRunner {
	if i.tx != nil {
		return i.tx
	}
	return i.db
}

// readRows reads all rows from a *sql.Rows object and converts them into a slice
// of schema.Document maps. It also handles type conversions for different field types.
func readRows(logger *zap.Logger, sc *schema.SchemaDefinition, rows *sql.Rows) ([]schema.Document, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []schema.Document
	for rows.Next() {
		row := make(schema.Document, len(columns))
		values := make([]any, len(columns))
		scanArgs := make([]any, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		for i, col := range columns {
			val := values[i]
			if val == nil {
				row[col] = nil
				continue
			}

			fieldDef, ok := sc.Fields[col]
			if !ok {
				logger.Warn("Column not found in schema, using raw value", zap.String("column", col))
				row[col] = val
				continue
			}

			switch fieldDef.Type {
			case schema.FieldTypeBoolean:
				if intVal, isInt := val.(int64); isInt {
					row[col] = intVal != 0
				} else if boolVal, isBool := val.(bool); isBool {
					row[col] = boolVal
				} else {
					row[col] = val
				}
			case schema.FieldTypeString, schema.FieldTypeEnum:
				if byteVal, isByte := val.([]byte); isByte {
					row[col] = string(byteVal)
				} else if strVal, isString := val.(string); isString {
					row[col] = strVal
				} else {
					row[col] = val
				}
			case schema.FieldTypeInteger:
				if intVal, isInt := val.(int64); isInt {
					row[col] = intVal
				} else if floatVal, isFloat := val.(float64); isFloat {
					row[col] = int64(floatVal)
				} else {
					row[col] = val
				}
			case schema.FieldTypeNumber, schema.FieldTypeDecimal:
				if floatVal, isFloat := val.(float64); isFloat {
					row[col] = floatVal
				} else if intVal, isInt := val.(int64); isInt {
					row[col] = float64(intVal)
				} else {
					row[col] = val
				}
			case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
				var byteVal []byte
				if b, ok := val.([]byte); ok {
					byteVal = b
				} else if s, ok := val.(string); ok {
					byteVal = []byte(s)
				}

				if byteVal != nil {
					var decodedValue any
					if err := json.Unmarshal(byteVal, &decodedValue); err == nil {
						row[col] = decodedValue
					} else {
						row[col] = val
					}
				} else {
					row[col] = val
				}
			default:
				row[col] = val
			}
		}
		results = append(results, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error after scanning rows: %w", err)
	}
	return results, nil
}

// SelectDocuments executes a SELECT query against the database.
func (i *SQLiteInteractor) SelectDocuments(ctx context.Context, schema *schema.SchemaDefinition, dsl *query.QueryDSL) ([]schema.Document, error) {
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

// UpdateDocuments executes an UPDATE query against the database.
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

// InsertDocuments executes an INSERT query against the database.
func (i *SQLiteInteractor) InsertDocuments(ctx context.Context, sc *schema.SchemaDefinition, records []map[string]any) ([]schema.Document, error) {
	if len(records) == 0 {
		return []schema.Document{}, nil
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

// DeleteDocuments executes a DELETE query against the database.
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

// StartTransaction begins a new database transaction and returns a new SQLiteInteractor
// that is scoped to that transaction.
func (i *SQLiteInteractor) StartTransaction(ctx context.Context) (persistence.DatabaseInteractor, error) {
	if i.tx != nil {
		return nil, fmt.Errorf("cannot start a new transaction from an existing transactional interactor")
	}

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	i.logger.Debug("Transaction initiated, returning new transactional interactor")
	return NewSQLiteInteractor(i.db, i.logger, i.options, tx), nil
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

