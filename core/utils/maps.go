package utils

import (
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

func GetValueByPath(value any, path string) (any, bool) {
	if path == "" {
		return value, true
	}

	keys := strings.Split(path, ".")
	current := value
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
