package definition

import "strings"

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	default:
		return 0
	}
}

func toUint64(v any) uint64 {
	switch x := v.(type) {
	case int:
		return uint64(x)
	case int8:
		return uint64(x)
	case int16:
		return uint64(x)
	case int32:
		return uint64(x)
	case int64:
		return uint64(x)
	case uint64:
		return x
	default:
		return 0
	}
}

func compareNumeric(a, b any) bool {
	fa, okA := toFloat64(a)
	fb, okB := toFloat64(b)
	if !okA || !okB {
		return false
	}
	const epsilon = 1e-12
	diff := fa - fb
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

func toFloat64(v any) (float64, bool) {
	switch i := v.(type) {
	case float64:
		return i, true
	case int64:
		return float64(i), true
	case int:
		return float64(i), true
	case float32:
		return float64(i), true
	case int32:
		return float64(i), true
	case uint64:
		return float64(i), true
	default:
		return 0, false
	}
}


// pathToString converts a path slice to a string key
func pathToString(path []string) string {
	return strings.Join(path, "/")
}
