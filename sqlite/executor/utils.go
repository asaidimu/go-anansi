package executor

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"go.uber.org/zap"
)

// ReadRows reads all rows from a *sql.Rows object and converts them into a slice
// of data.Document maps. It also handles type conversions for different field types.
// ReadRows reads all rows from a *sql.Rows object and converts them into a slice
// of data.Document maps. If no schema is provided, it returns raw row data without conversions.
func ReadRows(logger *zap.Logger, sc *schema.SchemaDefinition, rows *sql.Rows) ([]data.Document, error) {
	utilDocChan, utilErrChan := readRowsToDocs(rows)

	var results []data.Document

	// Define the transformation operation dynamically
	var processRow func(map[string]any) data.Document

	if sc == nil {
		// Fast path: schema is nil → no transformation
		processRow = func(row map[string]any) data.Document {
			return row
		}
	} else {
		// Schema-aware transformation
		processRow = func(row map[string]any) data.Document {
			globalResult := make(map[string]any)

			for col, value := range row {
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

				fieldDef := sc.FindField(fieldName)
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

	// Generic iteration — operation applied uniformly
	for row := range utilDocChan {
		results = append(results, processRow(row))
	}

	// Check for errors from reader
	if err := <-utilErrChan; err != nil {
		return nil, fmt.Errorf("error after scanning rows: %w", err)
	}

	return results, nil
}

func readRowsToDocs(rows *sql.Rows) (<-chan data.Document, <-chan error) {
	docChan := make(chan data.Document)
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
			row := make(data.Document, len(columns))
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
func fromSQLiteValue(fieldDef *schema.FieldDefinition, value any) (any, error) {
	if value == nil || fieldDef == nil {
		return value, nil
	}

	var convertedValue any
	var err error

	switch fieldDef.Type {
	case schema.FieldTypeBoolean:
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
