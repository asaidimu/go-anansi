package schema

import (
	"regexp"
	"strings"
)

// ============================================================================
// PATH PARSING AND CONSTRUCTION
// ============================================================================

// ParseHierarchicalName splits a hierarchical constraint path
// Example: "group1/group2/constraint" -> ["group1", "group2", "constraint"]
func ParseHierarchicalName(path string) []string {
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// BuildHierarchicalName constructs a hierarchical path from parts
// Example: ["group1", "group2", "constraint"] -> "group1/group2/constraint"
func BuildHierarchicalName(parts []string) string {
	return strings.Join(parts, "/")
}

// SplitFieldPath splits a dot-separated field path
// Example: "parent.child.field" -> ["parent", "child", "field"]
func SplitFieldPath(path string) []string {
	if path == "" {
		return []string{}
	}
	return strings.Split(path, ".")
}

// JoinFieldPath constructs a field path from parts
// Example: ["parent", "child", "field"] -> "parent.child.field"
func JoinFieldPath(parts []string) string {
	return strings.Join(parts, ".")
}

// IsValidFieldPath checks if a field path is valid
// Valid paths contain alphanumeric characters, underscores, and dots
func IsValidFieldPath(path string) bool {
	if path == "" {
		return false
	}
	// Must start with letter or underscore
	// Can contain letters, numbers, underscores, and dots
	// Cannot end with a dot
	// Cannot have consecutive dots
	matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)*$`, path)
	return matched
}

// IsValidConstraintPath checks if a constraint path is valid
// Valid paths contain alphanumeric characters, underscores, hyphens, and forward slashes
func IsValidConstraintPath(path string) bool {
	if path == "" {
		return false
	}
	// Must start with letter or underscore
	// Can contain letters, numbers, underscores, hyphens, and forward slashes
	// Cannot end with a forward slash
	// Cannot have consecutive forward slashes
	matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_-]*(/[a-zA-Z_][a-zA-Z0-9_-]*)*$`, path)
	return matched
}

// IsValidIdentifier checks if a string is a valid identifier (for names, IDs)
// Valid identifiers start with a letter or underscore and contain only alphanumeric characters and underscores
func IsValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, s)
	return matched
}

// NormalizeFieldPath normalizes a field path by removing extra whitespace and converting to lowercase
func NormalizeFieldPath(path string) string {
	parts := SplitFieldPath(path)
	normalized := make([]string, len(parts))
	for i, part := range parts {
		normalized[i] = strings.TrimSpace(part)
	}
	return JoinFieldPath(normalized)
}

// NormalizeConstraintPath normalizes a constraint path
func NormalizeConstraintPath(path string) string {
	parts := ParseHierarchicalName(path)
	normalized := make([]string, len(parts))
	for i, part := range parts {
		normalized[i] = strings.TrimSpace(part)
	}
	return BuildHierarchicalName(normalized)
}

// GetFieldPathDepth returns the depth of a field path (number of segments)
func GetFieldPathDepth(path string) int {
	if path == "" {
		return 0
	}
	return len(SplitFieldPath(path))
}

// GetConstraintPathDepth returns the depth of a constraint path
func GetConstraintPathDepth(path string) int {
	if path == "" {
		return 0
	}
	return len(ParseHierarchicalName(path))
}

// GetParentFieldPath returns the parent path of a field path
// Example: "parent.child.field" -> "parent.child"
// Returns empty string if path has no parent
func GetParentFieldPath(path string) string {
	parts := SplitFieldPath(path)
	if len(parts) <= 1 {
		return ""
	}
	return JoinFieldPath(parts[:len(parts)-1])
}

// GetFieldName returns the field name from a field path (the last segment)
// Example: "parent.child.field" -> "field"
func GetFieldName(path string) string {
	parts := SplitFieldPath(path)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// GetParentConstraintPath returns the parent path of a constraint path
// Example: "group1/group2/constraint" -> "group1/group2"
// Returns empty string if path has no parent
func GetParentConstraintPath(path string) string {
	parts := ParseHierarchicalName(path)
	if len(parts) <= 1 {
		return ""
	}
	return BuildHierarchicalName(parts[:len(parts)-1])
}

// GetConstraintName returns the constraint name from a constraint path (the last segment)
// Example: "group1/group2/constraint" -> "constraint"
func GetConstraintName(path string) string {
	parts := ParseHierarchicalName(path)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// IsArrayPath returns true if the path contains array notation
func IsArrayPath(path string) bool {
	return strings.Contains(path, ".[].")
}
