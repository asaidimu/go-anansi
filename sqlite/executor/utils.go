package executor

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/core/utils"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// ReadRows reads all rows from a *sql.Rows object and converts them into a slice
// of map[string]any maps. It also handles type conversions for different field types.
// ReadRows reads all rows from a *sql.Rows object and converts them into a slice
// of map[string]any maps. If no schema is provided, it returns raw row data without conversions.
// Add the constant here (or import it from your constants package)
func ReadRows(logger *zap.Logger, sc *definition.Schema, rows *sql.Rows) ([]map[string]any, int64, error) {
	utilDocChan, utilErrChan := readRowsToDocs(rows)

	var results []map[string]any
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

		results = append(results, processRow(row))
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
		// Add other specific mappings here as needed
		}
	}

	// Fallback for generic or unmapped errors
	return native.ErrOperationFailed.WithCause(err)
}
