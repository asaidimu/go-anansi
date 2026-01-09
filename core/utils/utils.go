package utils

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
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
		return nil, common.SystemErrorFrom(ErrInputNil).WithOperation("StructToMap")
	}

	// If the input is a pointer, dereference it to get the underlying value
	if val.Kind() == reflect.Ptr {
		// If it's a nil pointer, return an error
		if val.IsNil() {
			return nil, common.SystemErrorFrom(ErrInputNilPointer).WithOperation("StructToMap")
		}
		val = val.Elem()
	}

	// Validate that the underlying value is a struct
	if val.Kind() != reflect.Struct {
		return nil, common.SystemErrorFrom(ErrInputNotStruct).WithOperation("StructToMap")
	}

	// Marshal the input struct into JSON bytes.
	// This respects `json:"tag"` annotations, `omitempty`, etc.,
	// and correctly serializes all nested structs into their JSON object forms.
	jsonBytes, err := ToJSONBytes(record)
	if err != nil {
		return nil, common.SystemErrorFrom(ErrMarshalJSON).WithOperation("StructToMap")
	}

	// Unmarshal these JSON bytes into a map[string]any directly.
	// Nested JSON objects will automatically be unmarshaled into nested
	// map[string]any values by the `encoding/json` package.
	var resultMap map[string]any
	if err := FromJSON(jsonBytes, &resultMap); err != nil {
		return nil, common.SystemErrorFrom(ErrUnmarshalJSON).WithOperation("StructToMap")
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
		return zero, common.SystemErrorFrom(ErrMapToStructInputNil).WithOperation("MapToStruct")
	}

	// Validate that `T` is a struct type (or a pointer to a struct).
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return zero, common.SystemErrorFrom(ErrMapToStructTargetNotStruct).WithOperation("MapToStruct")
	}

	// Marshal the input `map[string]any` into JSON bytes.
	// When nested maps exist, `json.Marshal` will correctly convert them
	// into nested JSON objects.
	jsonBytes, err := ToJSONBytes(input)
	if err != nil {
		return zero, common.SystemErrorFrom(ErrMarshalJSON).WithOperation("MapToStruct")
	}

	// Unmarshal these JSON bytes into a new instance of `T`.
	var result T
	if err := FromJSON(jsonBytes, &result); err != nil {
		return zero, common.SystemErrorFrom(ErrUnmarshalJSON).WithOperation("MapToStruct")
	}

	return result, nil
}

func PrimitivePtr[T any](t T) *T {
	return &t
}

// StringPtr is a helper function that returns a pointer to a string.
// Deprecated
func StringPtr(s string) *string {
	return &s
}

// Int64Ptr is a helper function that returns a pointer to an int64.
// Deprecated
func Int64Ptr(i int64) *int64 {
	return &i
}

// BoolPtr is a helper function that returns a pointer to a bool.
// Deprecated
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


// isComplexValue checks if a value is a slice or map (complex type) using reflection
func IsComplexValue(value any) bool {
	val := reflect.ValueOf(value)
	kind := val.Kind()
	return kind == reflect.Slice || kind == reflect.Map
}

// Helper to handle the "is not an integer" check across multiple possible types
func IsInteger(v any) bool {
	switch v := v.(type) {
	case int, int32, int64, uint, uint32, uint64:
		return true
	case float64:
		// Check if the float is actually a whole number (e.g., 1.0)
		f := v
		return f == float64(int64(f))
	default:
		return false
	}
}
