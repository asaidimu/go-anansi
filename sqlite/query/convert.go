package query

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// toSQLiteValue converts a Go value to its SQLite representation based on the
// schema. When the value is drawn from a container-backed document, container
// fields are serialized directly from the container via the schema-driven
// stream serializer instead of materializing a map and reflect-marshaling it.
func toSQLiteValue(fieldDef *definition.Field, value any, doc data.Documenter) (any, error) {
	// Container fields (object/array/record) are stored as JSON text.
	if fieldDef != nil && fieldDef.Type.IsContainer() {
		// processDocumentFields pre-serializes container fields from the
		// container; accept the finished text without re-serializing.
		if s, ok := value.(string); ok {
			return s, nil
		}
		if d, ok := doc.(*document.Document); ok {
			s, present, err := d.SerializeFieldString(string(fieldDef.Name))
			if err != nil {
				return nil, ErrConvertMarshalFieldFailed.WithCause(fmt.Errorf("failed to serialize field '%s': %w", fieldDef.Name, err))
			}
			if !present {
				return nil, nil
			}
			return s, nil
		}
		if value == nil {
			return nil, nil
		}
		jsonBytes, err := utils.ToJSONBytes(value)
		if err != nil {
			return nil, ErrConvertMarshalFieldFailed.WithCause(fmt.Errorf("failed to marshal field '%s' to JSON: %w", fieldDef.Name, err))
		}
		return string(jsonBytes), nil
	}

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
