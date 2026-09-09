package json

import (
	"strings"
)

// escapeJsonPointer escapes special characters in a JSON Pointer path segment
func escapeJsonPointer(part string) string {
	// ~ must be encoded first
	result := strings.ReplaceAll(part, "~", "~0")
	// then /
	result = strings.ReplaceAll(result, "/", "~1")
	return result
}

// NormalizePath converts a path string from dot notation to JSON Pointer notation
func NormalizePath(path string) string {
	if path == "" || path == "/" {
		return ""
	}

	// If already in slash notation, ensure proper escaping
	if strings.HasPrefix(path, "/") {
		parts := strings.Split(path[1:], "/")
		escaped := make([]string, len(parts))
		for i, part := range parts {
			escaped[i] = escapeJsonPointer(part)
		}
		return "/" + strings.Join(escaped, "/")
	}

	// Convert from dot notation and escape
	parts := strings.Split(path, ".")
	escaped := make([]string, len(parts))
	for i, part := range parts {
		escaped[i] = escapeJsonPointer(part)
	}
	return "/" + strings.Join(escaped, "/")
}

