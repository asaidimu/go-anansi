package data

import (
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// Get with generic type support
func Get[T any](doc *Document, key string) (T, error) {
	var zero T
	val, err := doc.Get(key)
	if err != nil {
		return zero, err
	}
	result, ok := val.(T)
	if !ok {
		return zero, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Get[T]").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to %T", val, zero)).WithCause(ErrTypeMismatch)
	}
	return result, nil
}

// GetWithCoercion attempts type coercion for common types
func GetWithCoercion[T any](doc *Document, key string) (T, error) {
	var zero T
	val, err := doc.Get(key)
	if err != nil {
		return zero, err
	}
	// Try direct type assertion first
	if result, ok := val.(T); ok {
		return result, nil
	}
	// Try coercion for common types
	switch any(zero).(type) {
	case string:
		if str, ok := utils.CoerceToPrimitiveValue[string](val); ok {
			if result, ok := any(str).(T); ok {
				return result, nil
			}
		}
	case int:
		if num, ok := utils.CoerceToPrimitiveValue[int](val); ok {
			if result, ok := any(num).(T); ok {
				return result, nil
			}
		}
	case float64:
		if num, ok := utils.CoerceToPrimitiveValue[float64](val); ok {
			if result, ok := any(num).(T); ok {
				return result, nil
			}
		}
	case bool:
		if b, ok := utils.CoerceToPrimitiveValue[bool](val); ok {
			if result, ok := any(b).(T); ok {
				return result, nil
			}
		}
	case time.Time:
		if t, ok := utils.CoerceTime(val); ok {
			if result, ok := any(t).(T); ok {
				return result, nil
			}
		}
	}
	return zero, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.GetWithCoercion[T]").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to %T", val, zero)).WithCause(ErrTypeMismatch)
}

// GetNested with generic type support
func GetNested[T any](doc *Document, path string) (T, error) {
	var zero T
	val, err := doc.GetNested(path)
	if err != nil {
		return zero, err
	}
	result, ok := val.(T)
	if !ok {
		return zero, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.GetNested[T]").WithPath(path).WithMessage(fmt.Sprintf("cannot convert %T to %T", val, zero)).WithCause(ErrTypeMismatch)
	}
	return result, nil

}
