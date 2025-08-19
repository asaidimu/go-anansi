package data

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// StructBinder handles automatic struct binding
type StructBinder struct {
	doc Document
}

// Bind returns a struct binder helper
func (d Document) Bind() *StructBinder {
	return &StructBinder{doc: d}
}

// To binds document data to a struct using 'doc' tags
func (sb *StructBinder) To(target any) error {
	return sb.ToWithContext(context.Background(), target)
}

// ToWithContext binds with context support
func (sb *StructBinder) ToWithContext(ctx context.Context, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return &DocumentError{
			Operation: "BindTo",
			Message:   ErrInvalidTargetType.Error(),
			Cause:     ErrInvalidTargetType,
		}
	}

	rv = rv.Elem()
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		field := rt.Field(i)
		fieldValue := rv.Field(i)

		if !fieldValue.CanSet() {
			continue
		}

		docTag := field.Tag.Get("doc")
		if docTag == "" || docTag == "-" {
			continue
		}

		// Parse tag options
		tagParts := strings.Split(docTag, ",")
		fieldName := tagParts[0]
		options := tagParts[1:]

		// Check if field is optional
		omitEmpty := false
		for _, opt := range options {
			if opt == "omitempty" {
				omitEmpty = true
			}
		}

		// Get value from document
		value, err := sb.doc.Get(fieldName)
		if err != nil {
			if omitEmpty {
				continue
			}
			return &DocumentError{
				Operation: "BindTo",
				Key:       fieldName,
				Message:   fmt.Sprintf("%s for struct field %s", ErrRequiredFieldNotFound.Error(), field.Name),
				Cause:     fmt.Errorf("%w: %w", ErrRequiredFieldNotFound, err),
			}
		}

		// Set the field value
		if err := setFieldValue(fieldValue, value); err != nil {
			return &DocumentError{
				Operation: "BindTo",
				Key:       fieldName,
				Message:   fmt.Sprintf("%s for field %s", ErrFailedToSetField.Error(), field.Name),
				Cause:     fmt.Errorf("%w: %w", ErrFailedToSetField, err),
			}
		}
	}

	return nil
}

// BindTo is a generic version that returns the bound struct
func BindTo[T any](doc Document) (T, error) {
	var result T
	err := doc.Bind().To(&result)
	return result, err
}

// setFieldValue sets a reflect.Value (representing a struct field) from an arbitrary
// 'any' value. It attempts direct assignment if types are compatible, otherwise, it
// performs type coercion for common primitive types (string, int, float, bool, time.Time).
// It also recursively handles nested slices and maps. Returns an error if the value
// cannot be set or coerced to the field's type.
func setFieldValue(field reflect.Value, value any) error {
	if value == nil {
		return nil
	}

	fieldType := field.Type()
	valueType := reflect.TypeOf(value)

	// Direct assignment if types match
	if valueType.AssignableTo(fieldType) {
		field.Set(reflect.ValueOf(value))
		return nil
	}

	// Type coercion for common cases
	switch fieldType.Kind() {
	case reflect.String:
		if str, ok := CoerceToString(value); ok {
			field.SetString(str)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if num, ok := CoerceToInt(value); ok {
			field.SetInt(int64(num))
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if num, ok := CoerceToInt(value); ok {
			field.SetUint(uint64(num))
			return nil
		}
	case reflect.Float32, reflect.Float64:
		if num, ok := CoerceToFloat64(value); ok {
			field.SetFloat(num)
			return nil
		}
	case reflect.Bool:
		if b, ok := CoerceToBool(value); ok {
			field.SetBool(b)
			return nil
		}
	case reflect.Struct:
		if fieldType == reflect.TypeOf(time.Time{}) {
			if t, ok := CoerceToTime(value); ok {
				field.Set(reflect.ValueOf(t))
				return nil
			}
		} else { // Handle nested structs
			if valMap, ok := value.(map[string]any); ok {
				nestedDoc := Document(valMap)
				// Create a new instance of the nested struct type
				newStruct := reflect.New(fieldType).Interface()
				// Recursively bind the nested document to the new struct
				if err := nestedDoc.Bind().To(newStruct); err != nil {
					return err
				}
				field.Set(reflect.ValueOf(newStruct).Elem())
				return nil
			}
		}
	case reflect.Slice:
		if valueSlice, ok := value.([]any); ok {
			return setSliceField(field, valueSlice)
		}
	case reflect.Map:
		if valueMap, ok := value.(map[string]any); ok {
			return setMapField(field, valueMap)
		}
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(fieldType.Elem()))
		}
		return setFieldValue(field.Elem(), value)
	}

	return fmt.Errorf("%w: cannot convert %T to %v", ErrTypeConversionFailed, value, fieldType)
}

// setSliceField handles assigning a slice of 'any' values to a reflect.Value
// representing a slice field in a struct. It iterates through the input values,
// recursively calls setFieldValue for each element to handle nested types and
// type coercion, and constructs a new slice of the correct element type.
func setSliceField(field reflect.Value, values []any) error {
	elementType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), len(values), len(values))

	for i, val := range values {
		elem := slice.Index(i)
		if elementType.Kind() == reflect.Ptr {
			elem.Set(reflect.New(elementType.Elem()))
			elem = elem.Elem()
		}
		if err := setFieldValue(elem, val); err != nil {
			return err
		}
	}

	field.Set(slice)
	return nil
}

// setMapField handles assigning a map[string]any to a reflect.Value representing
// a map field in a struct. It iterates through the input map, recursively calls
// setFieldValue for each value to handle nested types and type coercion, and
// constructs a new map of the correct key and element types.
func setMapField(field reflect.Value, values map[string]any) error {
	mapType := field.Type()
	newMap := reflect.MakeMap(mapType)

	for k, v := range values {
		keyVal := reflect.ValueOf(k)
		valueVal := reflect.New(mapType.Elem()).Elem()

		if err := setFieldValue(valueVal, v); err != nil {
			return err
		}

		newMap.SetMapIndex(keyVal, valueVal)
	}

	field.Set(newMap)
	return nil
}

// FromStructWithTags creates a Document from a struct using 'doc' tags
func FromStructWithTags(s any) (Document, error) {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return FromStruct(s) // Fallback to JSON marshaling
	}

	rt := rv.Type()
	doc := make(Document)

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fieldValue := rv.Field(i)

		docTag := field.Tag.Get("doc")
		if docTag == "" || docTag == "-" {
			continue
		}

		// Parse tag options
		tagParts := strings.Split(docTag, ",")
		fieldName := tagParts[0]
		options := tagParts[1:]

		// Check omitempty option
		omitEmpty := false
		for _, opt := range options {
			if opt == "omitempty" {
				omitEmpty = true
			}
		}

		// Skip zero values if omitempty
		if omitEmpty && fieldValue.IsZero() {
			continue
		}

		// Convert field value
		value := fieldValue.Interface()
		doc[fieldName] = value
	}

	return doc, nil
}
