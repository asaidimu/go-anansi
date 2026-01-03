package data

import (
	"context"
	"fmt"
	"strings"
)

// Flatten creates a flat map from nested document structure.
func (d Document) Flatten(separator string) map[string]any {
	if separator == "" {
		separator = "."
	}

	result := make(map[string]any)
	d.flattenInto(result, "", separator)
	return result
}

// flattenInto recursively flattens a Document into a single-level map,
// using the specified separator for nested keys. It handles nested Documents
// and slices, creating unique keys for each element.
func (d Document) flattenInto(result map[string]any, prefix, separator string) {
	for k, v := range d.data {
		key := k
		if prefix != "" {
			key = prefix + separator + k
		}

		if doc, ok := AsDocument(v); ok {
			doc.flattenInto(result, key, separator)
		} else if arr, ok := v.([]any); ok {
			for i, item := range arr {
				itemKey := fmt.Sprintf("%s[%d]", key, i)
				if itemDoc, ok := AsDocument(item); ok {
					itemDoc.flattenInto(result, itemKey, separator)
				} else {
					result[itemKey] = item
				}
			}
		} else {
			result[key] = v
		}
	}
}

// Unflatten reconstructs nested structure from flat map.
func Unflatten(flat map[string]any, separator string) Document {
	if separator == "" {
		separator = "."
	}

	docData := make(map[string]any)

	for key, value := range flat {
		parts := strings.Split(key, separator)
		current := docData

		for i, part := range parts {
			if i == len(parts)-1 {
				current[part] = value
			} else {
				next, ok := current[part]
				if !ok {
					next = make(map[string]any)
					current[part] = next
				}

				if nextMap, ok := next.(map[string]any); ok {
					current = nextMap
				}
			}
		}
	}

	return Document{ctx: context.Background(), data: docData}
}
