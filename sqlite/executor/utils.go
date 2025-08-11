package executor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

// ReadRows reads all rows from a *sql.Rows object and converts them into a slice
// of common.Document maps. It also handles type conversions for different field types.
func ReadRows(logger *zap.Logger, sc *schema.SchemaDefinition, rows *sql.Rows) ([]common.Document, error) {
	utilDocChan, utilErrChan := readRowsToDocs(rows)

	var results []common.Document
	for row := range utilDocChan {
		// Create global result object
		globalResult := make(map[string]any)

		for col, value := range row {
			var tableName, fieldName string

			// Check if column has table prefix
			if dotIndex := strings.Index(col, "."); dotIndex != -1 {
				tableName = col[:dotIndex]
				fieldName = col[dotIndex+1:]
			} else {
				// No table prefix, use empty string as table name
				tableName = ""
				fieldName = col
			}

			// Get or create table entry in global result
			var tableObj map[string]any
			if existing, exists := globalResult[tableName]; exists {
				tableObj = existing.(map[string]any)
			} else {
				tableObj = make(map[string]any)
				globalResult[tableName] = tableObj
			}

			// Find field definition using clean field name
			fieldDef := sc.FindField(fieldName)
			convertedValue, err := fromSQLiteValue(fieldDef, value)
			if err != nil {
				logger.Warn("Failed to convert value from SQLite", zap.String("column", col), zap.Error(err))
				tableObj[fieldName] = value
			} else {
				tableObj[fieldName] = convertedValue
			}
		}

		// Determine what to return based on number of tables
		var finalResult common.Document
		if len(globalResult) == 1 {
			// Single table - return the unwrapped table object
			for _, tableObj := range globalResult {
				finalResult = tableObj.(map[string]any)
				break
			}
		} else {
			// Multiple tables (joins) - return the nested structure
			finalResult = globalResult
		}

		results = append(results, finalResult)
	}

	// Check for any error that might have occurred during row reading
	if err := <-utilErrChan; err != nil {
		return nil, fmt.Errorf("error after scanning rows: %w", err)
	}

	return results, nil
}

func readRowsToDocs(rows *sql.Rows) (<-chan common.Document, <-chan error) {
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

	if err = json.Unmarshal(bytes, &data); err != nil {
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

// marshalJSON converts a value to JSON string
func marshalJSON(value any, fieldName string) (string, error) {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		if fieldName != "" {
			return "", fmt.Errorf("failed to marshal field '%s' to JSON: %w", fieldName, err)
		}
		return "", fmt.Errorf("failed to marshal value to JSON: %w", err)
	}
	return string(jsonBytes), nil
}

// convertBooleanFromSQLite converts integer representations back to booleans
func convertBooleanFromSQLite(value any) (any, error) {
	if i, ok := value.(int64); ok {
		return i == 1, nil
	}
	return value, nil
}

// convertBooleanToSQLite converts boolean values to integer representations for SQLite
func convertBooleanToSQLite(fieldDef *schema.FieldDefinition, value any) (any, error) {
	if b, ok := fieldDef.Type.Coerce(value); ok {
		val := b.(bool)
		if val {
			return 1, nil
		}
		return 0, nil
	}
	return value, nil
}

// fromSQLiteValue converts a value from SQLite to its Go representation based on the schema.
func fromSQLiteValue(fieldDef *schema.FieldDefinition, value any) (any, error) {
	if value == nil || fieldDef == nil {
		return value, nil
	}

	switch fieldDef.Type {
	case schema.FieldTypeBoolean:
		return convertBooleanFromSQLite(value)
	default:
		if fieldDef.Type.IsComplex() {
			return unmarshalJSON(value)
		}
		return value, nil
	}
}
