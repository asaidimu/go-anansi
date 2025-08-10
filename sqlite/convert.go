package sqlite

import (
	"encoding/json"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// fromSQLiteValue converts a value from SQLite to its Go representation based on the schema.
func fromSQLiteValue(fieldDef *schema.FieldDefinition, value any) (any, error) {
	if value == nil || fieldDef == nil {
		return value, nil
	}

	switch fieldDef.Type {
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
		// For complex types, attempt to unmarshal from a JSON string.
		if str, ok := value.(string); ok {
			var data any
			if err := json.Unmarshal([]byte(str), &data); err != nil {
				// If unmarshalling fails, return the original string to avoid breaking clients that don't expect a structured type.
				return str, nil
			}
			return data, nil
		}
		// Also handle byte slices, which the driver might return.
		if bytes, ok := value.([]byte); ok {
			var data any
			if err := json.Unmarshal(bytes, &data); err != nil {
				return string(bytes), nil
			}
			return data, nil
		}
		return value, nil
	case schema.FieldTypeBoolean:
		// Convert integer representations back to booleans.
		if i, ok := value.(int64); ok {
			return i == 1, nil
		}
		return value, nil
	default:
		return value, nil
	}
}
