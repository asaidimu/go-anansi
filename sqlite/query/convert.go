package query

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// toSQLiteValue converts a Go value to its SQLite representation based on the schema.
func toSQLiteValue(fieldDef *definition.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}

	// If there's no field definition, perform a default conversion for slices and maps.
	if fieldDef == nil {
		val := reflect.ValueOf(value)
		switch val.Kind() {
		case reflect.Slice, reflect.Map:
			jsonBytes, err := utils.ToJSONBytes(value)
			if err != nil {
				return nil, ErrConvertMarshalValueFailed.WithCause(fmt.Errorf("failed to marshal value to JSON: %w", err))
			}
			return string(jsonBytes), nil
		default:
			return value, nil
		}
	}

	// Use the schema's field type to determine the conversion logic.
	if fieldDef.Type.IsContainer() {
		jsonBytes, err := utils.ToJSONBytes(value)
		if err != nil {
			return nil, ErrConvertMarshalFieldFailed.WithCause(fmt.Errorf("failed to marshal field '%s' to JSON: %w", fieldDef.Name, err))
		}
		return string(jsonBytes), nil
	}

	switch fieldDef.Type {
	case definition.FieldTypeBoolean:
		// Simple boolean conversion for SQLite (1 for true, 0 for false)
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
