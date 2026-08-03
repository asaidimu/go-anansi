package document

import (
	"fmt"
	"strconv"
)

// These coercion helpers mirror the reference decoder's value conversion so
// that arbitrary Go values (JSON-decoded floats, ints, strings, nested arrays)
// can be stored into the type-directed container slots.

func asInt64(v any) (int64, error) {
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
	return 0, fmt.Errorf("document: expected integer, got %T", v)
}

func asFloat64(v any) (float64, error) {
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
	return 0, fmt.Errorf("document: expected number, got %T", v)
}

func asString(v any) string {
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

func asGeometry(v any) ([][]float64, error) {
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
		return nil, fmt.Errorf("document: expected geometry (array of rings), got %T", v)
	}
	out := make([][]float64, len(outer))
	for i, ring := range outer {
		inner, ok := ring.([]any)
		if !ok {
			return nil, fmt.Errorf("document: geometry ring %d is not an array", i)
		}
		out[i] = make([]float64, len(inner))
		for j, coord := range inner {
			f, err := asFloat64(coord)
			if err != nil {
				return nil, fmt.Errorf("document: geometry ring %d coord %d: %w", i, j, err)
			}
			out[i][j] = f
		}
	}
	return out, nil
}

// toAnySlice normalizes any slice-like value (JSON-decoded []any or typed
// slices such as []string, []int64) into []any for element coercion.
func toAnySlice(v any) ([]any, bool) {
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

func asInt64Slice(v any) ([]int64, error) {
	arr, ok := toAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("document: expected array, got %T", v)
	}
	out := make([]int64, len(arr))
	for i, e := range arr {
		n, err := asInt64(e)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

func asFloat64Slice(v any) ([]float64, error) {
	arr, ok := toAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("document: expected array, got %T", v)
	}
	out := make([]float64, len(arr))
	for i, e := range arr {
		f, err := asFloat64(e)
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

func asStringSlice(v any) []string {
	arr, _ := toAnySlice(v)
	out := make([]string, len(arr))
	for i, e := range arr {
		out[i] = asString(e)
	}
	return out
}

func asBoolSlice(v any) ([]bool, error) {
	arr, ok := toAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("document: expected array, got %T", v)
	}
	out := make([]bool, len(arr))
	for i, e := range arr {
		b, ok := e.(bool)
		if !ok {
			return nil, fmt.Errorf("document: array element %d is not a boolean", i)
		}
		out[i] = b
	}
	return out, nil
}

func asBytesSlice(v any) [][]byte {
	arr, _ := toAnySlice(v)
	out := make([][]byte, len(arr))
	for i, e := range arr {
		out[i] = []byte(asString(e))
	}
	return out
}

func asGeometrySlice(v any) ([][][]float64, error) {
	arr, ok := toAnySlice(v)
	if !ok {
		return nil, fmt.Errorf("document: expected array of geometry, got %T", v)
	}
	out := make([][][]float64, len(arr))
	for i, e := range arr {
		g, err := asGeometry(e)
		if err != nil {
			return nil, err
		}
		out[i] = g
	}
	return out, nil
}
