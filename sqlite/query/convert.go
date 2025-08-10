package query

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// toSQLiteValue converts a Go value to its SQLite representation based on the schema.
func toSQLiteValue(fieldDef *schema.FieldDefinition, value any) (any, error) {
	if value == nil {
		return nil, nil
	}

	// If there's no field definition, perform a default conversion for slices and maps.
	if fieldDef == nil {
		val := reflect.ValueOf(value)
		switch val.Kind() {
		case reflect.Slice, reflect.Map:
			jsonBytes, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal value to JSON: %w", err)
			}
			return string(jsonBytes), nil
		default:
			return value, nil
		}
	}

	// Use the schema's field type to determine the conversion logic.
	switch fieldDef.Type {
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal field '%s' to JSON: %w", fieldDef.Name, err)
		}
		return string(jsonBytes), nil
	case schema.FieldTypeBoolean:
		if b, ok := value.(bool); ok {
			if b {
				return 1, nil
			}
			return 0, nil
		}
		return value, nil
	default:
		return value, nil
	}
}
