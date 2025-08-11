package utils

import (
	"strconv"
	"strings"
)

// Primitive constrains T to sensible primitive types
type Primitive interface {
	~bool | ~string |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

func CoercePrimitiveValue[T Primitive](value any) (T, bool) {
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
		return coerceToBool[T](value)
	case string:
		return coerceToString[T](value)
	case int:
		return coerceToInt[T](value)
	case int8:
		return coerceToInt8[T](value)
	case int16:
		return coerceToInt16[T](value)
	case int32:
		return coerceToInt32[T](value)
	case int64:
		return coerceToInt64[T](value)
	case uint:
		return coerceToUint[T](value)
	case uint8:
		return coerceToUint8[T](value)
	case uint16:
		return coerceToUint16[T](value)
	case uint32:
		return coerceToUint32[T](value)
	case uint64:
		return coerceToUint64[T](value)
	case float32:
		return coerceToFloat32[T](value)
	case float64:
		return coerceToFloat64[T](value)
	}

	return zero, false
}

func coerceToBool[T Primitive](value any) (T, bool) {
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

func coerceToString[T Primitive](value any) (T, bool) {
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

func coerceToInt[T Primitive](value any) (T, bool) {
	return coerceToSignedInt[T](value, -9223372036854775808, 9223372036854775807)
}

func coerceToInt8[T Primitive](value any) (T, bool) {
	return coerceToSignedInt[T](value, -128, 127)
}

func coerceToInt16[T Primitive](value any) (T, bool) {
	return coerceToSignedInt[T](value, -32768, 32767)
}

func coerceToInt32[T Primitive](value any) (T, bool) {
	return coerceToSignedInt[T](value, -2147483648, 2147483647)
}

func coerceToInt64[T Primitive](value any) (T, bool) {
	return coerceToSignedInt[T](value, -9223372036854775808, 9223372036854775807)
}

func coerceToSignedInt[T Primitive](value any, min, max int64) (T, bool) {
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
		if float64(intVal) == floatVal && intVal >= min && intVal <= max {
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

func coerceToUint[T Primitive](value any) (T, bool) {
	return coerceToUnsignedInt[T](value, 18446744073709551615)
}

func coerceToUint8[T Primitive](value any) (T, bool) {
	return coerceToUnsignedInt[T](value, 255)
}

func coerceToUint16[T Primitive](value any) (T, bool) {
	return coerceToUnsignedInt[T](value, 65535)
}

func coerceToUint32[T Primitive](value any) (T, bool) {
	return coerceToUnsignedInt[T](value, 4294967295)
}

func coerceToUint64[T Primitive](value any) (T, bool) {
	return coerceToUnsignedInt[T](value, 18446744073709551615)
}

func coerceToUnsignedInt[T Primitive](value any, max uint64) (T, bool) {
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

func coerceToFloat32[T Primitive](value any) (T, bool) {
	return coerceToFloat[T](value, true)
}

func coerceToFloat64[T Primitive](value any) (T, bool) {
	return coerceToFloat[T](value, false)
}

func coerceToFloat[T Primitive](value any, isFloat32 bool) (T, bool) {
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
