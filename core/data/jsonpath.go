package data

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// JSONPathQuery executes a JSONPath-like query on the document.
func (d *Document) JSONPathQuery(path string) ([]any, error) {
	if path == "" || path == "$" {
		return []any{d.data}, nil
	}

	// Parse the JSONPath into segments
	segments, err := d.parseJSONPath(path)
	if err != nil {
		return nil, err
	}

	return d.executeJSONPath(segments)
}

// parseJSONPath breaks down a JSONPath string into individual segments
func (d *Document) parseJSONPath(path string) ([]string, error) {
	// Remove leading $. if present
	path = strings.TrimPrefix(path, ".")
	path = strings.TrimPrefix(path, "$")

	if path == "" {
		return []string{}, nil
	}

	var segments []string
	var current strings.Builder
	inBrackets := false

	for i, char := range path {
		switch char {
		case '[':
			if current.Len() > 0 {
				segments = append(segments, current.String())
				current.Reset()
			}
			inBrackets = true
			current.WriteRune(char)
		case ']':
			if !inBrackets {
				return nil, common.SystemErrorFrom(ErrInvalidJSONPathSyntax).WithOperation("data.Document.parseJSONPath").WithMessage(fmt.Sprintf("unexpected ']' at position %d", i))
			}
			current.WriteRune(char)
			segments = append(segments, current.String())
			current.Reset()
			inBrackets = false
		case '.':
			if inBrackets {
				current.WriteRune(char)
			} else {
				if current.Len() > 0 {
					segments = append(segments, current.String())
					current.Reset()
				}
			}
		default:
			current.WriteRune(char)
		}
	}

	if inBrackets {
		return nil, common.SystemErrorFrom(ErrInvalidJSONPathSyntax).WithOperation("data.Document.parseJSONPath").WithMessage("unclosed bracket in path")
	}

	if current.Len() > 0 {
		segments = append(segments, current.String())
	}

	return segments, nil
}

// executeJSONPath is a recursive helper for JSONPathQuery. It traverses the document
// based on the provided path segments, supporting wildcard '*' and array indexing '[]'.
// It returns a slice of all values found at the specified path.
func (d *Document) executeJSONPath(segments []string) ([]any, error) {
	current := []any{d.data}

	for i, segment := range segments {
		var next []any
		isLastSegment := i == len(segments)-1

		for _, item := range current {
			results, err := d.processSegment(item, segment, isLastSegment)
			if err != nil {
				return nil, err
			}
			next = append(next, results...)
		}

		current = next
		if len(current) == 0 {
			return []any{}, nil
		}
	}

	return current, nil
}

// processSegment handles a single path segment against a current item
func (d *Document) processSegment(item any, segment string, isLastSegment bool) ([]any, error) {
	// Handle bracket notation
	if strings.HasPrefix(segment, "[") && strings.HasSuffix(segment, "]") {
		return d.processBracketSegment(item, segment)
	}

	// Handle wildcard
	if segment == "*" {
		return d.processWildcard(item), nil
	}

	// Handle regular field access
	return d.processFieldAccess(item, segment, isLastSegment), nil
}

// processBracketSegment handles bracket notation like [*], [0], [1], etc.
func (d *Document) processBracketSegment(item any, segment string) ([]any, error) {
	inner := segment[1 : len(segment)-1] // Remove [ and ]

	// Handle wildcard [*]
	if inner == "*" {
		return d.processWildcard(item), nil
	}

	// Handle string keys ['key'] or ["key"]
	if (strings.HasPrefix(inner, "'") && strings.HasSuffix(inner, "'")) ||
		(strings.HasPrefix(inner, "\"") && strings.HasSuffix(inner, "\"")) {
		key := inner[1 : len(inner)-1]
		return d.processFieldAccess(item, key, true), nil
	}

	// Handle numeric index [0], [1], etc.
	if index, err := strconv.Atoi(inner); err == nil {
		if arr, ok := item.([]any); ok && index >= 0 && index < len(arr) {
			return []any{arr[index]}, nil
		}
		return []any{}, nil
	}

	return nil, common.SystemErrorFrom(ErrInvalidJSONPathSyntax).WithOperation("data.Document.processBracketSegment").WithMessage(fmt.Sprintf("invalid bracket expression: %s", segment))
}

// processWildcard handles wildcard operations on arrays and objects
func (d Document) processWildcard(item any) []any {
	var results []any

	if arr, ok := item.([]any); ok {
		results = append(results, arr...)
	} else if itemMap, ok := item.(map[string]any); ok {
		for _, v := range itemMap {
			results = append(results, v)
		}
	} else if doc, ok := item.(Document); ok {
		for _, v := range doc.data {
			results = append(results, v)
		}
	} else if docPtr, ok := item.(*Document); ok {
		for _, v := range docPtr.data {
			results = append(results, v)
		}
	}
	// For other types, wildcard on them yields no results
	return results
}

// processFieldAccess handles regular field access on objects and array elements
func (d Document) processFieldAccess(item any, field string, isLastSegment bool) []any {
	var results []any

	// Try to get a map from the item
	var currentMap map[string]any
	if doc, ok := item.(Document); ok {
		currentMap = doc.data
	} else if docPtr, ok := item.(*Document); ok {
		currentMap = docPtr.data
	} else if itemMap, ok := item.(map[string]any); ok {
		currentMap = itemMap
	}

	if currentMap != nil {
		if val, ok := currentMap[field]; ok {
			// Special handling: if this is the last segment and the value is an array,
			// flatten it into individual elements (for cases like $.store.book)
			if isLastSegment {
				if arr, ok := val.([]any); ok {
					results = append(results, arr...)
				} else {
					results = append(results, val)
				}
			} else {
				results = append(results, val)
			}
		}
	} else if arr, ok := item.([]any); ok {
		// Apply field access to each element in the array
		for _, subItem := range arr {
			var subItemMap map[string]any
			if subDoc, ok := subItem.(Document); ok {
				subItemMap = subDoc.data
			} else if subDocPtr, ok := subItem.(*Document); ok {
				subItemMap = subDocPtr.data
			} else if subActualMap, ok := subItem.(map[string]any); ok {
				subItemMap = subActualMap
			}

			if subItemMap != nil {
				if val, ok := subItemMap[field]; ok {
					results = append(results, val)
				}
			}
		}
	}

	return results
}
