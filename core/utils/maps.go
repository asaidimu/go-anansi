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

// GetValueByPath retrieves a value from a nested structure using a dot-separated path.
// It provides an efficient path for map[string]any and handles other map types via reflection.
func GetValueByPath(data any, path string) (any, bool) {
	if path == "" {
		return data, true
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if current == nil {
			return nil, false
		}

		// Fast path for map[string]any, which is common.
		// This also covers data.Document without creating an import cycle.
		if m, ok := current.(map[string]any); ok {
			if val, exists := m[part]; exists {
				current = val
				continue
			}
			return nil, false
		}

		// Fallback to reflection for other map types (e.g., map[any]any)
		val := reflect.ValueOf(current)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}

		if val.Kind() != reflect.Map {
			return nil, false // Not a map, cannot traverse further
		}

		// Use reflection to get the map value
		keyValue := reflect.ValueOf(part)
		mapValue := val.MapIndex(keyValue)

		if !mapValue.IsValid() {
			return nil, false // Key does not exist
		}

		current = mapValue.Interface()
	}

	return current, true
}