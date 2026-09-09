package utils

import (
	"reflect"
	"strings"

)

func ConvertMaps(m map[string]any) {
	for k, v := range m {
		if subMap, ok := v.(map[any]any); ok {
			newSubMap := make(map[string]any)
			for subK, subV := range subMap {
				if subKStr, ok := subK.(string); ok {
					newSubMap[subKStr] = subV
				}
			}
			m[k] = newSubMap
			ConvertMaps(newSubMap)
		} else if subMap, ok := v.([]any); ok {
			for _, item := range subMap {
				if itemMap, ok := item.(map[any]any); ok {
					newSubMap := make(map[string]any)
					for subK, subV := range itemMap {
						if subKStr, ok := subK.(string); ok {
							newSubMap[subKStr] = subV
						}
					}
					ConvertMaps(newSubMap)
				}
			}
		}
	}
}

func BuildPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

func GetScopedPath(path string) string {
	if !strings.Contains(path, ".") {
		return ""
	}
	parts := strings.Split(path, ".")
	return strings.Join(parts[:len(parts)-1], ".")
}

func GetMapStringAny(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}

	// Fast path 1: Exact match
	if m, ok := value.(map[string]any); ok {
		return m, true
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}

	// Fallback path: Use MapIter to avoid slice allocation from rv.MapKeys()
	resultMap := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		// iter.Key() is guaranteed to be a string based on the check above
		resultMap[iter.Key().String()] = iter.Value().Interface()
	}

	return resultMap, true
}

// GetValueByPath - wrapper for backward compatibility
func GetValueByPath(data any, path string) (any, bool) {
    if path == "" {
        return data, true
    }
    parts := strings.Split(path, ".")
    return GetValueByParts(data, parts)
}

// GetValueByParts retrieves a value from a nested map using pre-split keys.
func GetValueByParts(data any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return data, true
	}

	current := data
	for _, part := range parts {
		if current == nil {
			return nil, false
		}

		// Direct type assertion is significantly faster than reflection
		if m, ok := current.(map[string]any); ok {
			val, exists := m[part]
			if !exists {
				return nil, false
			}
			current = val
			continue
		}

		// Fallback for non-standard maps (e.g., from external decoders)
		val := reflect.ValueOf(current)
		if val.Kind() == reflect.Pointer {
			val = val.Elem()
		}
		if val.Kind() != reflect.Map {
			return nil, false
		}
		mapValue := val.MapIndex(reflect.ValueOf(part))
		if !mapValue.IsValid() {
			return nil, false
		}
		current = mapValue.Interface()
	}

	return current, true
}
