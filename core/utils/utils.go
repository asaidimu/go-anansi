package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// StructToMap converts a Go struct into a map[string]any.
//
// This revised function directly marshals the struct to JSON and then
// unmarshals it into a map[string]any. Nested structs will appear as
// nested map[string]any values in the resulting map, rather than
// json.RawMessage.
//
// The input `record` must be a struct or a pointer to a struct. If `record` is
// nil, or not a struct/pointer to a struct, an error is returned.
func StructToMap[T any](record T) (map[string]any, error) {
	val := reflect.ValueOf(record)

	// Handle nil interface input directly (e.g., if `record` is `nil any`)
	if !val.IsValid() {
		return nil, fmt.Errorf("input record cannot be nil")
	}

	// If the input is a pointer, dereference it to get the underlying value
	if val.Kind() == reflect.Ptr {
		// If it's a nil pointer, return an error
		if val.IsNil() {
			return nil, fmt.Errorf("input record cannot be a nil pointer to a struct")
		}
		val = val.Elem()
	}

	// Validate that the underlying value is a struct
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("input record must be a struct or a pointer to a struct, got %s", val.Kind())
	}

	// Marshal the input struct into JSON bytes.
	// This respects `json:"tag"` annotations, `omitempty`, etc.,
	// and correctly serializes all nested structs into their JSON object forms.
	jsonBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("StructToMap: failed to marshal input record to JSON: %w", err)
	}

	// Unmarshal these JSON bytes into a map[string]any directly.
	// Nested JSON objects will automatically be unmarshaled into nested
	// map[string]any values by the `encoding/json` package.
	var resultMap map[string]any
	if err := json.Unmarshal(jsonBytes, &resultMap); err != nil {
		return nil, fmt.Errorf("StructToMap: failed to unmarshal JSON to map[string]any: %w", err)
	}

	return resultMap, nil
}

// MapToStruct is a generic function that converts a `map[string]any` into
// a new instance of the specified generic struct type `T`.
//
// This revised function directly marshals the input map to JSON and then
// unmarshals it into the target struct. It expects nested JSON objects
// within the map to be represented as `map[string]any` (which `json.Marshal`
// handles correctly).
//
// The generic type `T` must be a struct type. If `T` is specified as a pointer
// type (e.g., `*MyStruct`), the function will unmarshal into the dereferenced
// struct and return a pointer to it.
//
// Returns a new instance of `T` populated with data from `input`, or the
// zero value of `T` and an error if conversion fails, if `input` is nil,
// or if `T` is not a struct type.
func MapToStruct[T any](input map[string]any) (T, error) {
	var zero T // Represents the zero value of type T, used for error returns

	if input == nil {
		return zero, fmt.Errorf("MapToStruct: input map cannot be nil")
	}

	// Validate that `T` is a struct type (or a pointer to a struct).
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return zero, fmt.Errorf("MapToStruct: generic type T must be a struct type (or pointer to struct), got %s", typ.Kind())
	}

	// Marshal the input `map[string]any` into JSON bytes.
	// When nested maps exist, `json.Marshal` will correctly convert them
	// into nested JSON objects.
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return zero, fmt.Errorf("MapToStruct: failed to marshal input map to JSON: %w", err)
	}

	// Unmarshal these JSON bytes into a new instance of `T`.
	var result T
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return zero, fmt.Errorf("MapToStruct: failed to unmarshal JSON to target struct: %w", err)
	}

	return result, nil
}

// StringPtr is a helper function that returns a pointer to a string.
func StringPtr(s string) *string {
	return &s
}

// Int64Ptr is a helper function that returns a pointer to an int64.
func Int64Ptr(i int64) *int64 {
	return &i
}

// BoolPtr is a helper function that returns a pointer to a bool.
func BoolPtr(b bool) *bool {
	return &b
}

// ToFloat64 is a utility function that converts a value of various numeric types
// to a float64. It returns the converted float64 and a boolean indicating whether
// the conversion was successful.
func ToFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// compareValues compares two values and returns -1, 0, or 1.
func CompareValues(a, b any) int {
	// Handle nil values
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Try to compare as numbers first (most robust numeric comparison)
	aNum, aIsNum := ToFloat64(a)
	bNum, bIsNum := ToFloat64(b)
	if aIsNum && bIsNum {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	}

	// Try to compare as strings
	aStr, aIsString := a.(string)
	bStr, bIsString := b.(string)
	if aIsString && bIsString {
		return strings.Compare(aStr, bStr)
	}

	// Try to compare as booleans
	aBool, aIsBool := a.(bool)
	bBool, bIsBool := b.(bool)
	if aIsBool && bIsBool {
		if !aBool && bBool {
			return -1
		}
		if aBool && !bBool {
			return 1
		}
		return 0
	}

	// If types are different or not directly comparable, fall back to string comparison of their representation.
	return strings.Compare(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
}

