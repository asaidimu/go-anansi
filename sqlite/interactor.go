package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/core"
	"github.com/asaidimu/go-anansi/core/persistence"
	"go.uber.org/zap"
)

// dbRunner interface abstracts the common methods of *sql.DB and *sql.Tx
type dbRunner interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLiteInteractor handles database connection management, SQL generation, and raw database operations.
// It can operate in either a non-transactional mode (if tx is nil) or transactional mode (if tx is not nil).
type SQLiteInteractor struct {
	db                    *sql.DB // The underlying database connection pool
	tx                    *sql.Tx // The active transaction (nil if not in a transaction)
	queryGeneratorFactory core.QueryGeneratorFactory
	logger                *zap.Logger
	options *persistence.InteractorOptions
}

// Ensure SQLiteInteractor implements persistence.DatabaseInteractor
var _ persistence.DatabaseInteractor = (*SQLiteInteractor)(nil)

// NewSQLiteInteractor creates a new DatabaseInteractor instance.
// If 'tx' is provided, the instance operates in transactional mode,
// otherwise, it operates in non-transactional mode using 'db'.
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
		options: options,
		queryGeneratorFactory: NewSqliteQueryGeneratorFactory(),
		logger:                logger,
	}
}

// runner determines whether to use the underlying sql.DB or the active sql.Tx
// for executing a database operation. It returns the appropriate dbRunner.
func (e *SQLiteInteractor) runner() dbRunner {
	if e.tx != nil {
		return e.tx
	}
	return e.db
}

// readRows reads all rows from a sql.Rows result and converts them into a slice of Row maps.
func readRows(logger *zap.Logger, schema *core.SchemaDefinition, rows *sql.Rows) ([]core.Document, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []core.Document
	for rows.Next() {
		row := make(core.Document, len(columns))
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

			fieldDef, ok := schema.Fields[col]
			if !ok {
				logger.Warn("Column not found in schema, using raw value", zap.String("column", col))
				row[col] = val
				continue
			}

			switch fieldDef.Type {
			case core.FieldTypeBoolean:
				if intVal, isInt := val.(int64); isInt {
					row[col] = intVal != 0
				} else if boolVal, isBool := val.(bool); isBool {
					row[col] = boolVal
				} else {
					logger.Warn("Unexpected type for boolean field, using raw value", zap.String("column", col), zap.Any("value", val), zap.String("type", fmt.Sprintf("%T", val)))
					row[col] = val
				}
			case core.FieldTypeString, core.FieldTypeEnum:
				if byteVal, isByte := val.([]byte); isByte {
					row[col] = string(byteVal)
				} else if strVal, isString := val.(string); isString {
					row[col] = strVal
				} else {
					logger.Warn("Unexpected type for string/enum field, using raw value", zap.String("column", col), zap.Any("value", val), zap.String("type", fmt.Sprintf("%T", val)))
					row[col] = val
				}
			case core.FieldTypeInteger:
				if intVal, isInt := val.(int64); isInt {
					row[col] = intVal
				} else if floatVal, isFloat := val.(float64); isFloat {
					row[col] = int64(floatVal)
				} else {
					logger.Warn("Unexpected type for integer field, using raw value", zap.String("column", col), zap.Any("value", val), zap.String("type", fmt.Sprintf("%T", val)))
					row[col] = val
				}
			case core.FieldTypeNumber, core.FieldTypeDecimal:
				if floatVal, isFloat := val.(float64); isFloat {
					row[col] = floatVal
				} else if intVal, isInt := val.(int64); isInt {
					row[col] = float64(intVal)
				} else {
					logger.Warn("Unexpected type for number/decimal field, using raw value", zap.String("column", col), zap.Any("value", val), zap.String("type", fmt.Sprintf("%T", val)))
					row[col] = val
				}
			case core.FieldTypeObject, core.FieldTypeArray, core.FieldTypeSet, core.FieldTypeRecord, core.FieldTypeUnion:
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
						logger.Error("Failed to unmarshal JSON for complex field", zap.String("column", col), zap.String("raw_value", string(byteVal)), zap.Error(err))
						row[col] = val
					}
				} else {
					logger.Warn("Complex field value not string/byte slice, using raw value", zap.String("column", col), zap.Any("value", val), zap.String("type", fmt.Sprintf("%T", val)))
					row[col] = val
				}
			default:
				logger.Debug("Unsupported or unhandled FieldType, using raw value", zap.String("column", col), zap.String("fieldType", string(fieldDef.Type)), zap.Any("value", val))
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


func (e *SQLiteInteractor) SelectDocuments(ctx context.Context, schema *core.SchemaDefinition, dsl *core.QueryDSL) ([]core.Document, error) {
	queryGenerator, err := e.queryGeneratorFactory.CreateGenerator(schema)
	if err != nil {
		return nil, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateSelectSQL(dsl)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SQL query: %w", err)
	}

	e.logger.Debug("Executing SQL SELECT", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	rows, err := e.runner().QueryContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		e.logger.Error("Failed to execute SELECT query", zap.Error(err), zap.String("sql", sqlQuery))
		return nil, fmt.Errorf("failed to execute SELECT query: %w \n %s", err, sqlQuery)
	}
	defer rows.Close()
	return readRows(e.logger, schema, rows)
}

func (e *SQLiteInteractor) UpdateDocuments(ctx context.Context, schema *core.SchemaDefinition, updates map[string]any, filters *core.QueryFilter) (int64, error) {
	queryGenerator, err := e.queryGeneratorFactory.CreateGenerator(schema)
	if err != nil {
		return 0, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateUpdateSQL(updates, filters)
	if err != nil {
		return 0, fmt.Errorf("failed to generate SQL UPDATE query: %w", err)
	}

	e.logger.Debug("Executing SQL UPDATE", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	result, err := e.runner().ExecContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		e.logger.Error("Failed to execute UPDATE query", zap.Error(err), zap.String("sql", sqlQuery))
		return 0, fmt.Errorf("failed to execute UPDATE query: %w", err)
	}
	return result.RowsAffected()
}

func (e *SQLiteInteractor) InsertDocuments(ctx context.Context, schema *core.SchemaDefinition, records []map[string]any) ([]core.Document, error) {
	if len(records) == 0 {
		return []core.Document{}, nil
	}
	queryGenerator, err := e.queryGeneratorFactory.CreateGenerator(schema)
	if err != nil {
		return nil, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateInsertSQL(records)
	if err != nil {
		return nil, fmt.Errorf("failed to generate INSERT SQL: %w", err)
	}

	e.logger.Debug("Executing SQL INSERT with RETURNING clause", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	rows, err := e.runner().QueryContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		e.logger.Error("Failed to execute INSERT ... RETURNING query", zap.Error(err), zap.String("sql", sqlQuery))
		return nil, fmt.Errorf("failed to execute INSERT ... RETURNING query: %w", err)
	}
	defer rows.Close()
	return readRows(e.logger, schema, rows)
}

func (e *SQLiteInteractor) DeleteDocuments(ctx context.Context, schema *core.SchemaDefinition, filters *core.QueryFilter, unsafeDelete bool) (int64, error) {
	queryGenerator, err := e.queryGeneratorFactory.CreateGenerator(schema)
	if err != nil {
		return 0, fmt.Errorf("could not get a query generator instance: %w", err)
	}

	sqlQuery, queryParams, err := queryGenerator.GenerateDeleteSQL(filters, unsafeDelete)
	if err != nil {
		return 0, fmt.Errorf("failed to generate DELETE SQL: %w", err)
	}

	e.logger.Debug("Executing SQL DELETE", zap.String("sql", sqlQuery), zap.Any("params", queryParams))

	result, err := e.runner().ExecContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		e.logger.Error("Failed to execute DELETE query", zap.Error(err), zap.String("sql", sqlQuery))
		return 0, fmt.Errorf("failed to execute DELETE query: %w", err)
	}
	return result.RowsAffected()
}

// StartTransaction initiates a new database transaction.
// It returns a *new* SQLiteInteractor instance that operates
// within the scope of that transaction.
// The original interactor instance remains unchanged.
func (e *SQLiteInteractor) StartTransaction(ctx context.Context) (persistence.DatabaseInteractor, error) {
	if e.tx != nil {
		return nil, fmt.Errorf("Cannot start a new transaction from an existing transactional interactor")
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	e.logger.Debug("Transaction initiated, returning new transactional interactor")
	return NewSQLiteInteractor(e.db, e.logger, e.options, tx), nil
}

// Commit commits the transaction.
// This method should only be called on an SQLiteInteractor instance
// that was created with an active transaction (i.e., e.tx is not nil).
func (e *SQLiteInteractor) Commit(ctx context.Context) error {
	if e.tx == nil {
		return fmt.Errorf("Commit not applicable: not in a transactional context")
	}
	e.logger.Debug("Committing transaction")
	return e.tx.Commit()
}

// Rollback rolls back the transaction.
// This method should only be called on an SQLiteInteractor instance
// that was created with an active transaction (i.e., e.tx is not nil).
func (e *SQLiteInteractor) Rollback(ctx context.Context) error {
	if e.tx == nil {
		return fmt.Errorf("Rollback not applicable: not in a transactional context")
	}
	e.logger.Debug("Rolling back transaction")
	return e.tx.Rollback()
}

