package data

import (
	"fmt"
	"strconv"
	"strings"

	
)

// JSONPathQuery executes a JSONPath-like query on the document.
func (d Document) JSONPathQuery(path string) ([]any, error) {
	if path == "" || path == "$" {
		return []any{d}, nil
	}

	// Parse the JSONPath into segments
	segments, err := d.parseJSONPath(path)
	if err != nil {
		return nil, err
	}

	return d.executeJSONPath(segments)
}

// parseJSONPath breaks down a JSONPath string into individual segments
func (d Document) parseJSONPath(path string) ([]string, error) {
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
				return nil, fmt.Errorf("%w: unexpected ']' at position %d", ErrInvalidJSONPathSyntax, i)
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
		return nil, fmt.Errorf("%w: unclosed bracket in path", ErrInvalidJSONPathSyntax)
	}

	if current.Len() > 0 {
		segments = append(segments, current.String())
	}

	return segments, nil
}

// executeJSONPath is a recursive helper for JSONPathQuery. It traverses the document
// based on the provided path segments, supporting wildcard '*' and array indexing '[]'.
// It returns a slice of all values found at the specified path.
func (d Document) executeJSONPath(segments []string) ([]any, error) {
	current := []any{d}

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
func (d Document) processSegment(item any, segment string, isLastSegment bool) ([]any, error) {
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
func (d Document) processBracketSegment(item any, segment string) ([]any, error) {
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

	return nil, fmt.Errorf("%w: invalid bracket expression: %s", ErrInvalidJSONPathSyntax, segment)
}

// processWildcard handles wildcard operations on arrays and objects
func (d Document) processWildcard(item any) []any {
	var results []any

	if arr, ok := item.([]any); ok {
		results = append(results, arr...)
	} else if doc, ok := AsDocument(item); ok {
		for _, v := range doc {
			results = append(results, v)
		}
	}

	return results
}

// processFieldAccess handles regular field access on objects and array elements
func (d Document) processFieldAccess(item any, field string, isLastSegment bool) []any {
	var results []any

	if doc, ok := AsDocument(item); ok {
		if val, err := doc.Get(field); err == nil {
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
			if subDoc, ok := AsDocument(subItem); ok {
				if val, err := subDoc.Get(field); err == nil {
					results = append(results, val)
				}
			}
		}
	}

	return results
}
