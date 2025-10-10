package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Primitive constrains T to sensible primitive types
type Primitive interface {
	~bool | ~string |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

func CoerceToPrimitiveValue[T Primitive](value any) (T, bool) {
	var zero T

	// Handle nil input
	if value == nil {
		return zero, false
	}

	// Direct type match - fast path
	if v, ok := value.(T); ok {
		return v, true
	}

	// Determine target type and convert
	switch any(zero).(type) {
	case bool:
		return CoerceBool[T](value)
	case string:
		return CoerceString[T](value)
	case int:
		return CoerceInt[T](value)
	case int8:
		return CoerceInt8[T](value)
	case int16:
		return CoerceInt16[T](value)
	case int32:
		return CoerceInt32[T](value)
	case int64:
		return CoerceInt64[T](value)
	case uint:
		return CoerceUint[T](value)
	case uint8:
		return CoerceUint8[T](value)
	case uint16:
		return CoerceUint16[T](value)
	case uint32:
		return CoerceUint32[T](value)
	case uint64:
		return CoerceUint64[T](value)
	case float32:
		return CoerceFloat32[T](value)
	case float64:
		return CoerceFloat64[T](value)
	}

	return zero, false
}

// Public convenience functions that tests expect
func CoerceToString(value any) (string, bool) {
	if value == nil {
		return "", true
	}

	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case int:
		return strconv.Itoa(v), true
	case int8:
		return strconv.FormatInt(int64(v), 10), true
	case int16:
		return strconv.FormatInt(int64(v), 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint8:
		return strconv.FormatUint(uint64(v), 10), true
	case uint16:
		return strconv.FormatUint(uint64(v), 10), true
	case uint32:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), true
	case time.Time:
		return v.String(), true
	default:
		// For structs and other types, use fmt.Sprintf
		return fmt.Sprintf("%+v", v), true
	}
}

func CoerceToInt(value any) (int, bool) {
	if value == nil {
		return 0, false
	}

	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		// Check if it represents a whole number
		intVal := int(v)
		if float32(intVal) == v {
			return intVal, true
		}
		return int(v), true  // Still convert even if not exact
	case float64:
		// Check if it represents a whole number
		intVal := int(v)
		if float64(intVal) == v {
			return intVal, true
		}
		return int(v), true  // Still convert even if not exact
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	case string:
		v = strings.TrimSpace(v)
		if intVal, err := strconv.Atoi(v); err == nil {
			return intVal, true
		}
		if floatVal, err := strconv.ParseFloat(v, 64); err == nil {
			return int(floatVal), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func CoerceToFloat64(value any) (float64, bool) {
	if value == nil {
		return 0.0, false
	}

	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case bool:
		if v {
			return 1.0, true
		}
		return 0.0, true
	case string:
		v = strings.TrimSpace(v)
		if floatVal, err := strconv.ParseFloat(v, 64); err == nil {
			return floatVal, true
		}
		return 0.0, false
	default:
		return 0.0, false
	}
}

func CoerceToBool(value any) (bool, bool) {
	if value == nil {
		return false, true
	}

	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off", "":
			return false, true
		default:
			return false, false
		}
	case int, int8, int16, int32, int64:
		var intVal int64
		switch iv := v.(type) {
		case int:
			intVal = int64(iv)
		case int8:
			intVal = int64(iv)
		case int16:
			intVal = int64(iv)
		case int32:
			intVal = int64(iv)
		case int64:
			intVal = iv
		}
		return intVal != 0, true
	case uint, uint8, uint16, uint32, uint64:
		var uintVal uint64
		switch uv := v.(type) {
		case uint:
			uintVal = uint64(uv)
		case uint8:
			uintVal = uint64(uv)
		case uint16:
			uintVal = uint64(uv)
		case uint32:
			uintVal = uint64(uv)
		case uint64:
			uintVal = uv
		}
		return uintVal != 0, true
	case float32, float64:
		var floatVal float64
		switch fv := v.(type) {
		case float32:
			floatVal = float64(fv)
		case float64:
			floatVal = fv
		}
		return floatVal != 0.0, true
	default:
		return false, false
	}
}

func CoerceToTime(value any) (time.Time, bool) {
	return CoerceTime(value)
}

func CoerceBool[T Primitive](value any) (T, bool) {
	var zero T
	var result any

	switch v := value.(type) {
	case bool:
		result = v
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		switch lower {
		case "true":
			result = true
		case "false":
			result = false
		default:
			return zero, false
		}
	case int, int8, int16, int32, int64:
		var intVal int64
		switch iv := v.(type) {
		case int:
			intVal = int64(iv)
		case int8:
			intVal = int64(iv)
		case int16:
			intVal = int64(iv)
		case int32:
			intVal = int64(iv)
		case int64:
			intVal = iv
		}
		result = intVal != 0
	case uint, uint8, uint16, uint32, uint64:
		var uintVal uint64
		switch uv := v.(type) {
		case uint:
			uintVal = uint64(uv)
		case uint8:
			uintVal = uint64(uv)
		case uint16:
			uintVal = uint64(uv)
		case uint32:
			uintVal = uint64(uv)
		case uint64:
			uintVal = uv
		}
		result = uintVal != 0
	case float32, float64:
		var floatVal float64
		switch fv := v.(type) {
		case float32:
			floatVal = float64(fv)
		case float64:
			floatVal = fv
		}
		result = floatVal != 0.0
	default:
		return zero, false
	}

	if converted, ok := result.(T); ok {
		return converted, true
	}
	return zero, false
}

func CoerceString[T Primitive](value any) (T, bool) {
	var zero T
	var result string

	switch v := value.(type) {
	case string:
		result = v
	case bool:
		if v {
			result = "true"
		} else {
			result = "false"
		}
	case int:
		result = strconv.Itoa(v)
	case int8:
		result = strconv.FormatInt(int64(v), 10)
	case int16:
		result = strconv.FormatInt(int64(v), 10)
	case int32:
		result = strconv.FormatInt(int64(v), 10)
	case int64:
		result = strconv.FormatInt(v, 10)
	case uint:
		result = strconv.FormatUint(uint64(v), 10)
	case uint8:
		result = strconv.FormatUint(uint64(v), 10)
	case uint16:
		result = strconv.FormatUint(uint64(v), 10)
	case uint32:
		result = strconv.FormatUint(uint64(v), 10)
	case uint64:
		result = strconv.FormatUint(v, 10)
	case float32:
		result = strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		result = strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return zero, false
	}

	if converted, ok := any(result).(T); ok {
		return converted, true
	}
	return zero, false
}

func CoerceInt[T Primitive](value any) (T, bool) {
	return CoerceSignedInt[T](value, -9223372036854775808, 9223372036854775807)
}

func CoerceInt8[T Primitive](value any) (T, bool) {
	return CoerceSignedInt[T](value, -128, 127)
}

func CoerceInt16[T Primitive](value any) (T, bool) {
	return CoerceSignedInt[T](value, -32768, 32767)
}

func CoerceInt32[T Primitive](value any) (T, bool) {
	return CoerceSignedInt[T](value, -2147483648, 2147483647)
}

func CoerceInt64[T Primitive](value any) (T, bool) {
	return CoerceSignedInt[T](value, -9223372036854775808, 9223372036854775807)
}

func CoerceSignedInt[T Primitive](value any, min, max int64) (T, bool) {
	var zero T
	var result int64
	var valid bool

	switch v := value.(type) {
	case string:
		if intVal, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			if intVal >= min && intVal <= max {
				result = intVal
				valid = true
			}
		} else if floatVal, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			intVal := int64(floatVal)
			if float64(intVal) == floatVal && intVal >= min && intVal <= max {
				result = intVal
				valid = true
			}
		}
	case bool:
		if v {
			result = 1
		} else {
			result = 0
		}
		valid = true
	case int:
		intVal := int64(v)
		if intVal >= min && intVal <= max {
			result = intVal
			valid = true
		}
	case int8, int16, int32, int64:
		var intVal int64
		switch iv := v.(type) {
		case int8:
			intVal = int64(iv)
		case int16:
			intVal = int64(iv)
		case int32:
			intVal = int64(iv)
		case int64:
			intVal = iv
		}
		if intVal >= min && intVal <= max {
			result = intVal
			valid = true
		}
	case uint, uint8, uint16, uint32, uint64:
		var uintVal uint64
		switch uv := v.(type) {
		case uint:
			uintVal = uint64(uv)
		case uint8:
			uintVal = uint64(uv)
		case uint16:
			uintVal = uint64(uv)
		case uint32:
			uintVal = uint64(uv)
		case uint64:
			uintVal = uv
		}
		if uintVal <= uint64(max) {
			result = int64(uintVal)
			valid = true
		}
	case float32, float64:
		var floatVal float64
		switch fv := v.(type) {
		case float32:
			floatVal = float64(fv)
		case float64:
			floatVal = fv
		}
		intVal := int64(floatVal)
		// Allow conversion even if not exact whole number, but prefer exact matches
		if float64(intVal) == floatVal && intVal >= min && intVal <= max {
			result = intVal
			valid = true
		} else if intVal >= min && intVal <= max {
			result = intVal
			valid = true
		}
	}

	if !valid {
		return zero, false
	}

	if converted, ok := any(result).(T); ok {
		return converted, true
	}

	// Handle type-specific conversions
	switch any(zero).(type) {
	case int:
		if converted, ok := any(int(result)).(T); ok {
			return converted, true
		}
	case int8:
		if converted, ok := any(int8(result)).(T); ok {
			return converted, true
		}
	case int16:
		if converted, ok := any(int16(result)).(T); ok {
			return converted, true
		}
	case int32:
		if converted, ok := any(int32(result)).(T); ok {
			return converted, true
		}
	case int64:
		if converted, ok := any(result).(T); ok {
			return converted, true
		}
	}

	return zero, false
}

func CoerceUint[T Primitive](value any) (T, bool) {
	return CoerceUnsignedInt[T](value, 18446744073709551615)
}

func CoerceUint8[T Primitive](value any) (T, bool) {
	return CoerceUnsignedInt[T](value, 255)
}

func CoerceUint16[T Primitive](value any) (T, bool) {
	return CoerceUnsignedInt[T](value, 65535)
}

func CoerceUint32[T Primitive](value any) (T, bool) {
	return CoerceUnsignedInt[T](value, 4294967295)
}

func CoerceUint64[T Primitive](value any) (T, bool) {
	return CoerceUnsignedInt[T](value, 18446744073709551615)
}

func CoerceUnsignedInt[T Primitive](value any, max uint64) (T, bool) {
	var zero T
	var result uint64
	var valid bool

	switch v := value.(type) {
	case string:
		if uintVal, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64); err == nil {
			if uintVal <= max {
				result = uintVal
				valid = true
			}
		}
	case bool:
		if v {
			result = 1
		} else {
			result = 0
		}
		valid = true
	case int, int8, int16, int32, int64:
		var intVal int64
		switch iv := v.(type) {
		case int:
			intVal = int64(iv)
		case int8:
			intVal = int64(iv)
		case int16:
			intVal = int64(iv)
		case int32:
			intVal = int64(iv)
		case int64:
			intVal = iv
		}
		if intVal >= 0 && uint64(intVal) <= max {
			result = uint64(intVal)
			valid = true
		}
	case uint, uint8, uint16, uint32, uint64:
		var uintVal uint64
		switch uv := v.(type) {
		case uint:
			uintVal = uint64(uv)
		case uint8:
			uintVal = uint64(uv)
		case uint16:
			uintVal = uint64(uv)
		case uint32:
			uintVal = uint64(uv)
		case uint64:
			uintVal = uv
		}
		if uintVal <= max {
			result = uintVal
			valid = true
		}
	case float32, float64:
		var floatVal float64
		switch fv := v.(type) {
		case float32:
			floatVal = float64(fv)
		case float64:
			floatVal = fv
		}
		if floatVal >= 0 {
			uintVal := uint64(floatVal)
			if float64(uintVal) == floatVal && uintVal <= max {
				result = uintVal
				valid = true
			}
		}
	}

	if !valid {
		return zero, false
	}

	// Handle type-specific conversions
	switch any(zero).(type) {
	case uint:
		if converted, ok := any(uint(result)).(T); ok {
			return converted, true
		}
	case uint8:
		if converted, ok := any(uint8(result)).(T); ok {
			return converted, true
		}
	case uint16:
		if converted, ok := any(uint16(result)).(T); ok {
			return converted, true
		}
	case uint32:
		if converted, ok := any(uint32(result)).(T); ok {
			return converted, true
		}
	case uint64:
		if converted, ok := any(result).(T); ok {
			return converted, true
		}
	}

	return zero, false
}

func CoerceFloat32[T Primitive](value any) (T, bool) {
	return CoerceFloat[T](value, true)
}

func CoerceFloat64[T Primitive](value any) (T, bool) {
	return CoerceFloat[T](value, false)
}

func CoerceFloat[T Primitive](value any, isFloat32 bool) (T, bool) {
	var zero T
	var result float64
	var valid bool

	switch v := value.(type) {
	case string:
		if floatVal, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			result = floatVal
			valid = true
		}
	case bool:
		if v {
			result = 1.0
		} else {
			result = 0.0
		}
		valid = true
	case int, int8, int16, int32, int64:
		var intVal int64
		switch iv := v.(type) {
		case int:
			intVal = int64(iv)
		case int8:
			intVal = int64(iv)
		case int16:
			intVal = int64(iv)
		case int32:
			intVal = int64(iv)
		case int64:
			intVal = iv
		}
		result = float64(intVal)
		valid = true
	case uint, uint8, uint16, uint32, uint64:
		var uintVal uint64
		switch uv := v.(type) {
		case uint:
			uintVal = uint64(uv)
		case uint8:
			uintVal = uint64(uv)
		case uint16:
			uintVal = uint64(uv)
		case uint32:
			uintVal = uint64(uv)
		case uint64:
			uintVal = uv
		}
		result = float64(uintVal)
		valid = true
	case float32:
		result = float64(v)
		valid = true
	case float64:
		result = v
		valid = true
	}

	if !valid {
		return zero, false
	}

	if isFloat32 {
		if converted, ok := any(float32(result)).(T); ok {
			return converted, true
		}
	} else {
		if converted, ok := any(result).(T); ok {
			return converted, true
		}
	}

	return zero, false
}

// CoerceTime attempts to convert any value to time.Time.
// CoerceTime attempts to convert any value to time.Time.
func CoerceTime(v any) (time.Time, bool) {
	switch val := v.(type) {
	case time.Time:
		return val, true

	case string:
		// First, try to parse the string as a Unix timestamp (integer).
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			// Heuristic to determine if it's seconds or nanoseconds based on magnitude.
			// A 10-digit number is seconds-level precision (up to year 2286).
			// Anything larger is likely milliseconds, microseconds, or nanoseconds.
			if len(val) > 10 {
				return time.Unix(0, i), true // Treat as nanoseconds
			}
			return time.Unix(i, 0), true // Treat as seconds
		}

		// If it's not a numeric timestamp, try common layout formats.
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			time.RFC822,
			time.RFC822Z,
			time.RFC1123,
			time.RFC1123Z,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05.000000000",
			"2006-01-02",
			"15:04:05",
		}

		for _, format := range formats {
			if t, err := time.Parse(format, val); err == nil {
				return t, true
			}
		}
		return time.Time{}, false // Failed all parsing attempts

	case int64:
		// Assume value is in seconds for broad compatibility.
		return time.Unix(val, 0).UTC(), true

	case float64:
		// Assume value is in seconds.
		return time.Unix(int64(val), 0).UTC(), true

	default:
		return time.Time{}, false
	}
}
