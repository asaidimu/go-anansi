package definition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// LiteralType represents the type of field or value
type LiteralType byte

const (
	literalTypeZero LiteralType = iota
	LiteralTypeString
	LiteralTypeInteger
	LiteralTypeFloat
	LiteralTypeBoolean
	LiteralTypeObject
	LiteralTypeArray
	LiteralTypeNull
)

// String returns the string representation of LiteralType for error messages
func (lt LiteralType) String() string {
	switch lt {
	case LiteralTypeString:
		return "string"
	case LiteralTypeInteger:
		return "integer"
	case LiteralTypeFloat:
		return "float"
	case LiteralTypeBoolean:
		return "boolean"
	case LiteralTypeObject:
		return "object"
	case LiteralTypeArray:
		return "array"
	case LiteralTypeNull:
		return "null"
	default:
		return "unknown"
	}
}

// LiteralValue represents a union type for values used as default or in enum values.
// It can hold a primitive value (string, integer, float, boolean, object, array, or null)
type LiteralValue struct {
	kind  LiteralType
	value any
}

// Type returns the type of value currently held in the LiteralValue
func (lv LiteralValue) Type() (LiteralType, error) {
	if lv.kind == literalTypeZero {
		return literalTypeZero, ErrInvalidLiteralValue
	}
	return lv.kind, nil
}

// Value returns the underlying value as an any type
func (lv LiteralValue) Value() any {
	if lv.IsNull() || lv.IsZero() {
		return nil
	}
	return lv.value
}

// IsZero returns true if the LiteralValue is uninitialized
func (lv LiteralValue) IsZero() bool {
	return lv.kind == literalTypeZero
}

// IsNull returns true if the LiteralValue is explicitly set to null
func (lv LiteralValue) IsNull() bool {
	return lv.kind == LiteralTypeNull
}

// Validate checks if the LiteralValue is valid according to these rules:
// 1. If no fields are set (IsZero), the value is valid (undefined/optional)
// 2. If ArrayVal is set, each element must be valid recursively
// 3. If ObjectVal is set, all values must be valid literals (no structs/complex types)
// 4. Empty collections and objects are valid
func (lv LiteralValue) Validate() []common.Issue {
	// Zero value is valid (undefined/optional)
	if lv.IsZero() {
		return nil
	}

	var issues []common.Issue

	// Validate array elements recursively
	switch lv.kind {
	case LiteralTypeArray:
		arr, ok := lv.value.([]any)
		if !ok {
			issues = append(issues, common.Issue{
				Code:    "INVALID_ARRAY_VALUE",
				Message: "internal error: array kind with non-array value",
			})
			return issues
		}

		for i, elem := range arr {
			if !ValidateLiteral(elem) {
				issues = append(issues, common.Issue{
					Code:    "INVALID_ARRAY_VALUE",
					Index:   &i,
					Message: fmt.Sprintf("Array value at index '%d' is not a valid literal type", i),
				})
			}
		}
	case LiteralTypeObject:
		obj, ok := lv.value.(map[string]any)
		if !ok {
			issues = append(issues, common.Issue{
				Code:    "INVALID_OBJECT_VALUE",
				Message: "internal error: object kind with non-object value",
			})
			return issues
		}

		for key, val := range obj {
			if !ValidateLiteral(val) {
				issues = append(issues, common.Issue{
					Code:    "INVALID_OBJECT_VALUE",
					Path:    key,
					Message: fmt.Sprintf("object value at key '%s' is not a valid literal type", key),
				})
			}
		}
	default:
		if !ValidateLiteral(lv.value) {
			issues = append(issues, common.Issue{
				Code:    "INVALID_VALUE",
				Message: "Not a valid literal type",
			})
		}
	}

	return issues
}

// MarshalJSON implements json.Marshaler interface
// It serializes the actual value directly, not the wrapper struct
func (lv LiteralValue) MarshalJSON() ([]byte, error) {
	if lv.IsZero() || lv.IsNull() {
        return []byte("null"), nil
    }

	val, err := json.Marshal(lv.value)
	if err != nil {
		return nil, ErrMarshalFailed.WithCause(err).WithOperation("LiteralValue.MarshalJSON")
	}
	return val, nil
}

func (lv *LiteralValue) UnmarshalJSON(data []byte) error {
	// Reset fields
	lv.kind = literalTypeZero
	lv.value = nil

	// Handle null
	if string(data) == "null" {
		lv.kind = LiteralTypeNull
		lv.value = nil
		return nil
	}

	// Use a decoder with UseNumber to prevent float64 rounding of large ints
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return ErrUnmarshalFailed.WithCause(err).WithOperation("LiteralValue.UnmarshalJSON")
	}

	switch v := raw.(type) {
	case string:
		lv.kind = LiteralTypeString
		lv.value = v
	case json.Number:
		converted := convertNumbers(v)
		switch val := converted.(type) {
		case int64:
			lv.kind = LiteralTypeInteger
			lv.value = val
		case float64:
			lv.kind = LiteralTypeFloat
			lv.value = val
		}
	case bool:
		lv.kind = LiteralTypeBoolean
		lv.value = v
	case map[string]any:
		lv.kind = LiteralTypeObject
		lv.value = convertNumbers(v)
	case []any:
		lv.kind = LiteralTypeArray
		lv.value = convertNumbers(v)
	default:
		return ErrUnmarshalFailed.WithMessage(fmt.Sprintf("unsupported LiteralValue %T", v)).WithOperation("LiteralValue.UnmarshalJSON")
	}

	return nil
}

// convertNumbers recursively walks through maps and arrays, converting json.Number
// values to int64 or float64 to ensure consistent numeric types throughout the structure.
func convertNumbers(v any) any {
	switch val := v.(type) {
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i
		}
		f, _ := val.Float64()
		return f
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v := range val {
			result[k] = convertNumbers(v)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, elem := range val {
			result[i] = convertNumbers(elem)
		}
		return result
	default:
		return val
	}
}

type LiteralValueType interface {
	string | int64 | float64 | bool | map[string]any | []any
}

// NewLiteralValueStrict creates a LiteralValue
func NewLiteralValue[T LiteralValueType](val T) (LiteralValue, error) {
	if !ValidateLiteral(val) {
		return LiteralValue{}, ErrInvalidLiteralValue
	}

	lv := LiteralValue{value: val}

	switch any(val).(type) {
	case string:
		lv.kind = LiteralTypeString
	case bool:
		lv.kind = LiteralTypeBoolean
	case int64:
		lv.kind = LiteralTypeInteger
	case float64:
		lv.kind = LiteralTypeFloat
	case []any:
		lv.kind = LiteralTypeArray
	case map[string]any:
		lv.kind = LiteralTypeObject
	default:
		return LiteralValue{}, ErrTypeMismatch
	}

	return lv, nil
}

// NewNullLiteral creates a LiteralValue explicitly set to null
func NewNullLiteral() LiteralValue {
	return LiteralValue{kind: LiteralTypeNull, value: nil}
}

// LiteralValueAs attempts to extract the LiteralValue as the specified type T.
// It requires an EXACT type match - no automatic conversions are performed.
// This strict behavior prevents silent type coercion bugs.
func LiteralValueAs[T LiteralValueType](lv LiteralValue) (T, error) {
	var zero T

	if lv.IsZero() {
		return zero, ErrInvalidLiteralValue.WithMessage("LiteralValue is zero/uninitialized")
	}

	if lv.IsNull() {
		return zero, ErrInvalidLiteralValue.WithMessage("LiteralValue is null")
	}

	// Require exact type match - no conversions
	result, ok := lv.value.(T)
	if !ok {
		return zero, ErrTypeMismatch.WithMessage(
			fmt.Sprintf("type mismatch: LiteralValue contains %s but %T was requested", lv.kind, zero),
		)
	}

	return result, nil
}

// String returns a string representation of the LiteralValue
func (lv LiteralValue) String() string {
	if lv.IsZero() {
		return "null"
	}

	if lv.IsNull() {
		return "null"
	}

	switch lv.kind {
	case LiteralTypeString:
		return fmt.Sprintf(`"%s"`, lv.value)
	case LiteralTypeInteger:
		return fmt.Sprintf("%d", lv.value)
	case LiteralTypeFloat:
		return fmt.Sprintf("%v", lv.value)
	case LiteralTypeBoolean:
		return fmt.Sprintf("%v", lv.value)
	case LiteralTypeObject:
		data, _ := json.Marshal(lv.value)
		return string(data)
	case LiteralTypeArray:
		data, _ := json.Marshal(lv.value)
		return string(data)
	default:
		return "null"
	}
}

// ValidateLiteral checks if a value corresponds to a valid LiteralType.
// Valid types: string, number (int/float), boolean, null, array, object
// Uses reflection to handle typed slices ([]string, []int) and typed maps (map[string]int)
// Rejects structs and recursively validates nested structures.
func ValidateLiteral(val any) bool {
	if val == nil {
		return true
	}

	// Fast path: check common types first without reflection
	switch v := val.(type) {
	case string, bool:
		return true
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case []any:
		// Check each element at runtime
		for _, elem := range v {
			if !ValidateLiteral(elem) {
				return false
			}
		}
		return true
	case map[string]any:
		// Check each value at runtime
		for _, val := range v {
			if !ValidateLiteral(val) {
				return false
			}
		}
		return true
	}

	// Slow path: use reflection for typed collections and pointers
	rv := reflect.ValueOf(val)

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		// Check element type - reject if elements are structs
		return isTypeLiteral(rv.Type().Elem())
	case reflect.Map:
		// Only maps with string keys are valid objects
		if rv.Type().Key().Kind() != reflect.String {
			return false
		}
		// Check value type - reject if values are structs
		return isTypeLiteral(rv.Type().Elem())
	case reflect.Pointer:
		// Pointers are valid if they point to a valid type
		if rv.IsNil() {
			return true
		}
		// Recursively check the pointed-to value
		return ValidateLiteral(rv.Elem().Interface())
	case reflect.Struct, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		// These types are not valid literal types
		return false
	default:
		return false
	}
}

// isTypeLiteral checks if a reflect.Type represents a valid LiteralType.
// This is used to validate element/value types in collections without instantiating them.
func isTypeLiteral(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Interface:
		return true
	case reflect.Slice, reflect.Array:
		return isTypeLiteral(t.Elem())
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return false
		}
		return isTypeLiteral(t.Elem())
	case reflect.Pointer:
		return isTypeLiteral(t.Elem())
	case reflect.Struct, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return false
	default:
		return false
	}
}
