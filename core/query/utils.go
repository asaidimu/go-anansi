// Package query provides a set of utility functions to support the query builder
// and processor. These helpers handle common tasks such as type conversions and
// pointer creation.
package query

import "strconv"

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
func ToFloat64(v interface{}) (float64, bool) {
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
