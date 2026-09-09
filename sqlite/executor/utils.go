package executor

import (
        "context"
        "database/sql"
        "errors"
        "strings"

        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/document"
        "github.com/asaidimu/go-anansi/v8/core/query"
        "github.com/asaidimu/go-anansi/v8/core/query/native"
        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
        "github.com/asaidimu/go-anansi/v8/core/utils"
        "github.com/mattn/go-sqlite3"
        "go.uber.org/zap"
)

// ReadRowsIntoContainer scans result rows directly into pooled, schema-bound
// documents — no map materialization and no record-view wrapper. The schema is
// consulted once to build the column plan; scalar values land in typed slots
// and JSON-fragment columns are decoded leniently. matchCol names the query's
// internal total-count column; its value is captured from the first row.
func ReadRowsIntoContainer(ctx context.Context, dp *document.DocumentPool, rows *sql.Rows, matchCol string) ([]*document.Document, int64, error) {
        columns, err := rows.Columns()
        if err != nil {
                return nil, 0, native.ErrFailedToReadRows.WithCause(err).WithMessage("failed to get columns")
        }
        plan, err := dp.PlanRow(columns, matchCol)
        if err != nil {
                return nil, 0, native.ErrFailedToReadRows.WithCause(err).WithMessage("failed to plan row scan")
        }

        values := make([]any, len(columns))
        scanArgs := make([]any, len(columns))
        for i := range values {
                scanArgs[i] = &values[i]
        }

        var results []*document.Document
        var totalMatches int64
        totalCaptured := false

        for rows.Next() {
                if err := rows.Scan(scanArgs...); err != nil {
                        return nil, 0, native.ErrFailedToReadRows.WithCause(err).WithMessage("failed to scan row")
                }
                if !totalCaptured && plan.TotalIndex() >= 0 {
                        if v, ok := values[plan.TotalIndex()].(int64); ok {
                                totalMatches = v
                        }
                        totalCaptured = true
                }
                d, err := dp.ScanRow(ctx, plan, values)
                if err != nil {
                        return nil, 0, native.ErrFailedToReadRows.WithCause(err).WithMessage("failed to scan row into document")
                }
                results = append(results, d)
        }
        if err := rows.Err(); err != nil {
                return nil, 0, native.ErrFailedToReadRows.WithCause(err)
        }
        return results, totalMatches, nil
}

// ReadRows reads all rows from a *sql.Rows object and converts them into a slice
// of *document.Document. If no schema is provided, it returns raw row data.
func ReadRows(ctx context.Context, logger *zap.Logger, sc *definition.Schema, rows *sql.Rows) ([]*document.Document, int64, error) {
        utilDocChan, utilErrChan := readRowsToDocs(rows)

        var results []*document.Document
        var totalMatches int64 = 0
        countCaptured := false

        // Define the transformation operation dynamically
        var processRow func(map[string]any) map[string]any

        if sc == nil {
                processRow = func(row map[string]any) map[string]any {
                        // Even without schema, we should hide the internal match count from the final map
                        delete(row, query.MatchCountName)
                        return row
                }
        } else {
                processRow = func(row map[string]any) map[string]any {
                        globalResult := make(map[string]any)

                        for col, value := range row {
                                // Skip the system field so it doesn't get processed by schema logic
                                if col == query.MatchCountName {
                                        continue
                                }

                                var tableName, fieldName string
                                if dotIndex := strings.Index(col, "."); dotIndex != -1 {
                                        tableName = col[:dotIndex]
                                        fieldName = col[dotIndex+1:]
                                } else {
                                        tableName = sc.Name
                                        fieldName = col
                                }

                                tableObj, ok := globalResult[tableName].(map[string]any)
                                if !ok {
                                        tableObj = make(map[string]any)
                                        globalResult[tableName] = tableObj
                                }

                                _, fieldDef := sc.FindField(fieldName)
                                cv, err := fromSQLiteValue(fieldDef, value)
                                if err != nil {
                                        logger.Warn("failed to convert value", zap.String("field", fieldName), zap.Error(err))
                                        tableObj[fieldName] = value
                                } else {
                                        tableObj[fieldName] = cv
                                }
                        }

                        // Flatten if there’s only one table
                        if len(globalResult) == 1 {
                                for _, tableObj := range globalResult {
                                        return tableObj.(map[string]any)
                                }
                        }
                        return globalResult
                }
        }

        for row := range utilDocChan {
                // Capture the total count from the first row available
                if !countCaptured {
                        if val, ok := row[query.MatchCountName]; ok {
                                if c, ok := val.(int64); ok {
                                        totalMatches = c
                                }
                                countCaptured = true
                        }
                }

                results = append(results, document.NewRecordView(processRow(row), ctx))
        }

        if err := <-utilErrChan; err != nil {
                return nil, 0, err
        }

        return results, totalMatches, nil
}

func readRowsToDocs(rows *sql.Rows) (<-chan map[string]any, <-chan error) {
        docChan := make(chan map[string]any)
        errChan := make(chan error, 1)

        go func() {
                defer close(docChan)
                defer close(errChan)
                defer rows.Close()

                columns, err := rows.Columns()
                if err != nil {
                        errChan <- native.ErrFailedToReadRows.WithCause(err).WithMessage("failed to get columns")
                        return
                }

                for rows.Next() {
                        row := make(map[string]any, len(columns))
                        values := make([]any, len(columns))
                        scanArgs := make([]any, len(columns))
                        for i := range values {
                                scanArgs[i] = &values[i]
                        }

                        if err := rows.Scan(scanArgs...); err != nil {
                                errChan <- native.ErrFailedToReadRows.WithCause(err).WithMessage("failed to scan row")
                                return
                        }

                        for i, col := range columns {
                                row[col] = values[i]
                        }

                        docChan <- row
                }
                if err := rows.Err(); err != nil {
                        errChan <- err
                }
        }()

        return docChan, errChan
}

// unmarshalJSON attempts to unmarshal data from string or byte slice, returning original value on failure
func unmarshalJSON(value any) (any, error) {
        var data any
        var bytes []byte
        var err error

        switch v := value.(type) {
        case string:
                bytes = []byte(v)
        case []byte:
                bytes = v
        default:
                return value, nil
        }

        if data, err = utils.Unmarshal[any](bytes); err != nil {
                // Return original value as string to avoid breaking clients
                if str, ok := value.(string); ok {
                        return str, nil
                }
                if b, ok := value.([]byte); ok {
                        return string(b), nil
                }
                return value, nil
        }
        return data, nil
}

// convertBooleanFromSQLite converts integer representations back to booleans
func convertBooleanFromSQLite(value any) (any, error) {
        if i, ok := value.(int64); ok {
                return i == 1, nil
        }
        return value, nil
}

// fromSQLiteValue converts a value from SQLite to its Go representation based on the schema.
func fromSQLiteValue(fieldDef *definition.Field, value any) (any, error) {
        if value == nil || fieldDef == nil {
                return value, nil
        }

        var convertedValue any
        var err error

        switch fieldDef.Type {
        case definition.FieldTypeBoolean:
                convertedValue, err = convertBooleanFromSQLite(value)
        default:
                if fieldDef.Type.IsComplex() {
                        convertedValue, err = unmarshalJSON(value)
                } else {
                        convertedValue = value
                }
        }
        return convertedValue, err
}

// translateError converts a driver-specific SQLite error into a standardized
// native error from the core package. This is crucial for abstracting away the
// underlying database implementation.
func translateError(err error) *common.SystemError {
        if err == nil {
                return nil
        }

        var sqliteErr sqlite3.Error
        if errors.As(err, &sqliteErr) {
                switch sqliteErr.ExtendedCode {
                case sqlite3.ErrConstraintUnique:
                        return native.ErrUniqueConstraintViolation.WithCause(err)
                case sqlite3.ErrConstraintForeignKey:
                        return native.ErrForeignKeyConstraintViolation.WithCause(err)
                }

                // Check primary error code for BUSY/LOCKED (catches extended
                // codes not in the sqlite3 constants, e.g. SQLITE_BUSY_SNAPSHOT).
                switch sqliteErr.Code {
                case sqlite3.ErrBusy, sqlite3.ErrLocked:
                        return native.ErrOperationFailed.
                                WithCode("ERR_BACKEND_LOCKED").
                                WithMessage("database is locked, operation is retryable").
                                WithCause(err).
                                WithRetryable(true)
                }
        }

        // Fallback for generic or unmapped errors
        return native.ErrOperationFailed.WithCause(err)
}
