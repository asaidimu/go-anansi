package utils

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
