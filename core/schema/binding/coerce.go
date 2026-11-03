package binding

import (
	"fmt"
	"strconv"
)

// These coercion helpers mirror the reference decoder's value conversion so
// that arbitrary Go values (JSON-decoded floats, ints, strings, nested arrays)
// can be stored into the type-directed container slots.

// AsInt64 coerces a scalar into an int64.
func AsInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case int32:
		return int64(t), nil
	case uint64:
		return int64(t), nil
	case string:
		return strconv.ParseInt(t, 10, 64)
	}
	return 0, fmt.Errorf("binding: expected integer, got %T", v)
}

// AsFloat64 coerces a scalar into a float64.
func AsFloat64(v any) (float64, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case int64:
		return float64(t), nil
	case int:
		return float64(t), nil
	case int32:
		return float64(t), nil
	case string:
		return strconv.ParseFloat(t, 64)
	}
	return 0, fmt.Errorf("binding: expected number, got %T", v)
}

// AsString renders a scalar as a string.
func AsString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// AsGeometry coerces a value into geometry coordinates.
func AsGeometry(v any) ([][]float64, error) {
	switch g := v.(type) {
	case [][]float64:
		out := make([][]float64, len(g))
		for i, ring := range g {
			out[i] = append([]float64(nil), ring...)
		}
		return out, nil
	case []float64:
		return [][]float64{append([]float64(nil), g...)}, nil
	}
	outer, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("binding: expected geometry (array of rings), got %T", v)
	}
	out := make([][]float64, len(outer))
	for i, ring := range outer {
		inner, ok := ring.([]any)
		if !ok {
			return nil, fmt.Errorf("binding: geometry ring %d is not an array", i)
		}
		out[i] = make([]float64, len(inner))
		for j, coord := range inner {
			f, err := AsFloat64(coord)
			if err != nil {
				return nil, fmt.Errorf("binding: geometry ring %d coord %d: %w", i, j, err)
			}
			out[i][j] = f
		}
	}
	return out, nil
}

// ToAnySlice normalizes any slice-like value (JSON-decoded []any or typed
// slices such as []string, []int64) into []any for element coercion.
func ToAnySlice(v any) ([]any, bool) {
	switch arr := v.(type) {
	case []any:
		return arr, true
	case []string:
		out := make([]any, len(arr))
		for i, e := range arr {
			out[i] = e
		}
		return out, true
	case []int:
		out := make([]any, len(arr))
		for i, e := range arr {
			out[i] = e
		}
		return out, true
	case []int64:
		out := make([]any, len(arr))
		for i, e := range arr {
			out[i] = e
		}
		return out, true
	case []float64:
		out := make([]any, len(arr))
		for i, e := range arr {
			out[i] = e
		}
		return out, true
	case []bool:
		out := make([]any, len(arr))
		for i, e := range arr {
			out[i] = e
		}
		return out, true
	case []map[string]any:
		out := make([]any, len(arr))
		for i, e := range arr {
			out[i] = e
		}
		return out, true
	case [][]byte:
		out := make([]any, len(arr))
		for i, e := range arr {
			out[i] = e
		}
		return out, true
	default:
		return nil, false
	}
}

// AsInt64Slice coerces a value into an int64 slice.
func AsInt64Slice(v any) ([]int64, error) {
	arr, ok := ToAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("binding: expected array, got %T", v)
	}
	out := make([]int64, len(arr))
	for i, e := range arr {
		n, err := AsInt64(e)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// AsFloat64Slice coerces a value into a float64 slice.
func AsFloat64Slice(v any) ([]float64, error) {
	arr, ok := ToAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("binding: expected array, got %T", v)
	}
	out := make([]float64, len(arr))
	for i, e := range arr {
		f, err := AsFloat64(e)
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

// AsStringSlice coerces a value into a string slice.
func AsStringSlice(v any) []string {
	arr, _ := ToAnySlice(v)
	out := make([]string, len(arr))
	for i, e := range arr {
		out[i] = AsString(e)
	}
	return out
}

// AsBoolSlice coerces a value into a bool slice.
func AsBoolSlice(v any) ([]bool, error) {
	arr, ok := ToAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("binding: expected array, got %T", v)
	}
	out := make([]bool, len(arr))
	for i, e := range arr {
		b, ok := e.(bool)
		if !ok {
			return nil, fmt.Errorf("binding: array element %d is not a boolean", i)
		}
		out[i] = b
	}
	return out, nil
}

// AsBytesSlice coerces a value into a byte-slice slice.
func AsBytesSlice(v any) [][]byte {
	arr, _ := ToAnySlice(v)
	out := make([][]byte, len(arr))
	for i, e := range arr {
		switch b := e.(type) {
		case []byte:
			out[i] = b
		default:
			out[i] = []byte(AsString(e))
		}
	}
	return out
}

// AsGeometrySlice coerces a value into geometry coordinate slices.
func AsGeometrySlice(v any) ([][][]float64, error) {
	arr, ok := ToAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("binding: expected array of geometry, got %T", v)
	}
	out := make([][][]float64, len(arr))
	for i, e := range arr {
		g, err := AsGeometry(e)
		if err != nil {
			return nil, err
		}
		out[i] = g
	}
	return out, nil
}
