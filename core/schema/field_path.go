package schema

import (
	"fmt"
)

// ============================================================================
// FIELD PATH NAVIGATION
// ============================================================================

// GetFieldByPath returns field at dot-separated path (e.g., "address.city")
// This traverses through nested schemas to find deeply nested fields
func (s *SchemaDefinition) GetFieldByPath(path string) (*FieldDefinition, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	parts := SplitFieldPath(path)
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	// Find root field
	_, rootField, ok := s.GetFieldByName(parts[0])
	if !ok {
		return nil, NewFieldNameNotFoundError(parts[0])
	}

	// If path has only one part, return root field
	if len(parts) == 1 {
		return rootField, nil
	}

	// Navigate through nested fields
	return s.findNestedFieldByPath(rootField, parts[1:])
}

// findNestedFieldByPath navigates through nested fields
func (s *SchemaDefinition) findNestedFieldByPath(currentField *FieldDefinition, remainingPath []string) (*FieldDefinition, error) {
	if len(remainingPath) == 0 {
		return currentField, nil
	}

	// Field must be a complex type to have nested fields
	if !currentField.Type.IsComplex() {
		return nil, fmt.Errorf("field '%s' of type '%s' cannot have nested fields", currentField.Name, currentField.Type)
	}

	nextFieldName := remainingPath[0]

	switch currentField.Type {
	case FieldTypeObject:
		// Get the nested schema
		ref, ok := currentField.GetSchemaReference()
		if !ok {
			return nil, fmt.Errorf("field '%s' has no schema reference", currentField.Name)
		}

		nestedSchema, ok := s.GetNestedSchema(string(ref.ID))
		if !ok {
			return nil, NewNestedSchemaNotFoundError(string(ref.ID))
		}

		// Find the next field in the nested schema
		nextField, ok := nestedSchema.GetField(nextFieldName)
		if !ok {
			return nil, fmt.Errorf("field '%s' not found in nested schema '%s'", nextFieldName, nestedSchema.Name)
		}

		// Continue navigation
		if len(remainingPath) == 1 {
			return nextField, nil
		}
		return s.findNestedFieldByPath(nextField, remainingPath[1:])

	case FieldTypeArray, FieldTypeSet:
		// For arrays/sets, check if itemsType is object
		if currentField.ItemsType == nil || *currentField.ItemsType != FieldTypeObject {
			return nil, fmt.Errorf("field '%s' is array/set but itemsType is not object", currentField.Name)
		}

		// Get the nested schema for items
		ref, ok := currentField.GetSchemaReference()
		if !ok {
			return nil, fmt.Errorf("field '%s' has no schema reference for items", currentField.Name)
		}

		nestedSchema, ok := s.GetNestedSchema(string(ref.ID))
		if !ok {
			return nil, NewNestedSchemaNotFoundError(string(ref.ID))
		}

		// Find the next field in the nested schema
		nextField, ok := nestedSchema.GetField(nextFieldName)
		if !ok {
			return nil, fmt.Errorf("field '%s' not found in array item schema '%s'", nextFieldName, nestedSchema.Name)
		}

		// Continue navigation
		if len(remainingPath) == 1 {
			return nextField, nil
		}
		return s.findNestedFieldByPath(nextField, remainingPath[1:])

	case FieldTypeUnion:
		// For unions, we need to check all possible schemas
		refs, ok := currentField.GetSchemaReferences()
		if !ok {
			return nil, fmt.Errorf("field '%s' has no schema references", currentField.Name)
		}

		// Try to find the field in any of the union schemas
		for _, ref := range refs {
			nestedSchema, ok := s.GetNestedSchema(string(ref.ID))
			if !ok {
				continue
			}

			nextField, ok := nestedSchema.GetField(nextFieldName)
			if !ok {
				continue
			}

			// Found it in this schema
			if len(remainingPath) == 1 {
				return nextField, nil
			}
			return s.findNestedFieldByPath(nextField, remainingPath[1:])
		}

		return nil, fmt.Errorf("field '%s' not found in any union schema", nextFieldName)

	default:
		return nil, fmt.Errorf("field '%s' of type '%s' cannot have nested fields", currentField.Name, currentField.Type)
	}
}

// HasFieldAtPath checks if field exists at path
func (s *SchemaDefinition) HasFieldAtPath(path string) bool {
	_, err := s.GetFieldByPath(path)
	return err == nil
}

// GetFieldPathDepth returns the depth of a field path (number of segments)
func (s *SchemaDefinition) GetFieldPathDepth(path string) int {
	if path == "" {
		return 0
	}
	return len(SplitFieldPath(path))
}

// ============================================================================
// FIELD PATH UTILITIES
// ============================================================================

// GetAllFieldPaths returns all possible field paths in the schema (including nested)
// This is useful for documentation, validation, or query building
func (s *SchemaDefinition) GetAllFieldPaths() []string {
	paths := []string{}

	if s.Fields == nil {
		return paths
	}

	for _, field := range s.Fields {
		// Add the root field
		paths = append(paths, field.Name)

		// Add nested paths
		nestedPaths := s.getNestedFieldPaths(field, field.Name, make(map[string]bool))
		paths = append(paths, nestedPaths...)
	}

	return paths
}

// getNestedFieldPaths recursively gets all nested field paths
func (s *SchemaDefinition) getNestedFieldPaths(field *FieldDefinition, currentPath string, visited map[string]bool) []string {
	paths := []string{}

	// Prevent infinite recursion for circular references
	if visited[currentPath] {
		return paths
	}
	visited[currentPath] = true

	if !field.Type.IsComplex() {
		return paths
	}

	switch field.Type {
	case FieldTypeObject:
		ref, ok := field.GetSchemaReference()
		if !ok {
			return paths
		}

		nestedSchema, ok := s.GetNestedSchema(string(ref.ID))
		if !ok {
			return paths
		}

		if !nestedSchema.IsStructured() {
			return paths
		}

		// Get all fields from the nested schema
		fieldNames := nestedSchema.FieldNames()
		for _, fieldName := range fieldNames {
			nestedField, ok := nestedSchema.GetField(fieldName)
			if !ok {
				continue
			}

			nestedPath := JoinFieldPath([]string{currentPath, fieldName})
			paths = append(paths, nestedPath)

			// Recurse for deeper nesting
			deeperPaths := s.getNestedFieldPaths(nestedField, nestedPath, visited)
			paths = append(paths, deeperPaths...)
		}

	case FieldTypeArray, FieldTypeSet:
		if field.ItemsType == nil || *field.ItemsType != FieldTypeObject {
			return paths
		}

		ref, ok := field.GetSchemaReference()
		if !ok {
			return paths
		}

		nestedSchema, ok := s.GetNestedSchema(string(ref.ID))
		if !ok {
			return paths
		}

		if !nestedSchema.IsStructured() {
			return paths
		}

		// For arrays, paths are like "items.fieldName"
		fieldNames := nestedSchema.FieldNames()
		for _, fieldName := range fieldNames {
			nestedField, ok := nestedSchema.GetField(fieldName)
			if !ok {
				continue
			}

			// Use special notation for array items
			nestedPath := JoinFieldPath([]string{currentPath, "[]", fieldName})
			paths = append(paths, nestedPath)

			// Recurse for deeper nesting
			deeperPaths := s.getNestedFieldPaths(nestedField, nestedPath, visited)
			paths = append(paths, deeperPaths...)
		}

	case FieldTypeUnion:
		refs, ok := field.GetSchemaReferences()
		if !ok {
			return paths
		}

		// Collect paths from all union variants
		allFields := make(map[string]*FieldDefinition)
		for _, ref := range refs {
			nestedSchema, ok := s.GetNestedSchema(string(ref.ID))
			if !ok {
				continue
			}

			if !nestedSchema.IsStructured() {
				continue
			}

			fieldNames := nestedSchema.FieldNames()
			for _, fieldName := range fieldNames {
				if _, exists := allFields[fieldName]; !exists {
					if nestedField, ok := nestedSchema.GetField(fieldName); ok {
						allFields[fieldName] = nestedField
					}
				}
			}
		}

		// Add paths for all unique fields across variants
		for fieldName, nestedField := range allFields {
			nestedPath := JoinFieldPath([]string{currentPath, fieldName})
			paths = append(paths, nestedPath)

			// Recurse for deeper nesting
			deeperPaths := s.getNestedFieldPaths(nestedField, nestedPath, visited)
			paths = append(paths, deeperPaths...)
		}
	}

	return paths
}

// GetFieldPathsByType returns all field paths of a specific type
func (s *SchemaDefinition) GetFieldPathsByType(fieldType FieldType) []string {
	allPaths := s.GetAllFieldPaths()
	matching := []string{}

	for _, path := range allPaths {
		field, err := s.GetFieldByPath(path)
		if err == nil && field.Type == fieldType {
			matching = append(matching, path)
		}
	}

	return matching
}

// GetFieldPathsByPredicate returns field paths matching a predicate
func (s *SchemaDefinition) GetFieldPathsByPredicate(predicate func(*FieldDefinition) bool) []string {
	allPaths := s.GetAllFieldPaths()
	matching := []string{}

	for _, path := range allPaths {
		field, err := s.GetFieldByPath(path)
		if err == nil && predicate(field) {
			matching = append(matching, path)
		}
	}

	return matching
}

// ============================================================================
// FIELD PATH VALIDATION
// ============================================================================

// ValidateFieldPath checks if a field path is valid and exists in the schema
func (s *SchemaDefinition) ValidateFieldPath(path string) error {
	if path == "" {
		return fmt.Errorf("field path cannot be empty")
	}

	if !IsValidFieldPath(path) {
		return fmt.Errorf("field path '%s' is not valid", path)
	}

	_, err := s.GetFieldByPath(path)
	return err
}




// ============================================================================
// FIELD PATH INFORMATION
// ============================================================================

// FieldPathInfo contains information about a field path
type FieldPathInfo struct {
	Path       string
	Depth      int
	IsArray    bool
	IsNested   bool
	Parts      []string
	ParentPath string
	FieldName  string
	Field      *FieldDefinition
}

// GetFieldPathInfo returns detailed information about a field path
func (s *SchemaDefinition) GetFieldPathInfo(path string) (*FieldPathInfo, error) {
	field, err := s.GetFieldByPath(path)
	if err != nil {
		return nil, err
	}

	parts := SplitFieldPath(path)

	info := &FieldPathInfo{
		Path:       path,
		Depth:      len(parts),
		IsArray:    IsArrayPath(path),
		IsNested:   len(parts) > 1,
		Parts:      parts,
		ParentPath: GetParentFieldPath(path),
		FieldName:  GetFieldName(path),
		Field:      field,
	}

	return info, nil
}

// MustGetFieldPathInfo returns field path info, panics on error
func (s *SchemaDefinition) MustGetFieldPathInfo(path string) *FieldPathInfo {
	info, err := s.GetFieldPathInfo(path)
	if err != nil {
		panic(err)
	}
	return info
}
