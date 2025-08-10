package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"
)

// readRows reads all rows from a *sql.Rows object and converts them into a slice
// of common.Document maps. It also handles type conversions for different field types.
func readRows(logger *zap.Logger, sc *schema.SchemaDefinition, rows *sql.Rows) ([]common.Document, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []common.Document
	for rows.Next() {
		row := make(common.Document, len(columns))
		values := make([]any, len(columns))
		scanArgs := make([]any, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		for i, col := range columns {
			fieldDef := sc.FindField(col)
			convertedValue, err := fromSQLiteValue(fieldDef, values[i])
			if err != nil {
				logger.Warn("Failed to convert value from SQLite", zap.String("column", col), zap.Error(err))
				row[col] = values[i] // Assign original value on error
			} else {
				row[col] = convertedValue
			}
		}
		results = append(results, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error after scanning rows: %w", err)
	}
	return results, nil
}

