package schema

import (
	"slices"
	"fmt"
	"sort"
)

// ============================================================================
// FIELD NAVIGATION
// ============================================================================

// GetField returns the field definition if it exists
func (s *SchemaDefinition) GetField(id string) (*FieldDefinition, bool) {
	if s.Fields == nil {
		return nil, false
	}
	field, ok := s.Fields[id]
	return field, ok
}

// GetFieldByName returns the ID and definition of the field with the given name
func (s *SchemaDefinition) GetFieldByName(name string) (string, *FieldDefinition, bool) {
	if s.Fields == nil {
		return "", nil, false
	}
	for id, field := range s.Fields {
		if field.Name == name {
			return id, field, true
		}
	}
	return "", nil, false
}

// HasField checks if a field with this ID exists
func (s *SchemaDefinition) HasField(id string) bool {
	_, ok := s.GetField(id)
	return ok
}

// HasFieldWithName checks if any field has this name
func (s *SchemaDefinition) HasFieldWithName(name string) bool {
	_, _, ok := s.GetFieldByName(name)
	return ok
}

// FieldIDs returns all field IDs in the schema, sorted alphabetically
func (s *SchemaDefinition) FieldIDs() []string {
	if s.Fields == nil {
		return []string{}
	}
	ids := make([]string, 0, len(s.Fields))
	for id := range s.Fields {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return string(ids[i]) < string(ids[j])
	})
	return ids
}

// FieldsByName returns a map of field names to their IDs and definitions
func (s *SchemaDefinition) FieldsByName() map[string]FieldEntry {
	if s.Fields == nil {
		return map[string]FieldEntry{}
	}
	result := make(map[string]FieldEntry, len(s.Fields))
	for id, field := range s.Fields {
		result[field.Name] = FieldEntry{
			ID:         id,
			Definition: field,
		}
	}
	return result
}

// ============================================================================
// NESTED SCHEMA NAVIGATION
// ============================================================================

// GetNestedSchema returns the nested schema definition if it exists
func (s *SchemaDefinition) GetNestedSchema(id string) (*NestedSchemaDefinition, bool) {
	if s.NestedSchemas == nil {
		return nil, false
	}
	schema, ok := s.NestedSchemas[id]
	return schema, ok
}

// GetNestedSchemaByName returns the ID and definition of the nested schema with the given name
func (s *SchemaDefinition) GetNestedSchemaByName(name string) (string, *NestedSchemaDefinition, bool) {
	if s.NestedSchemas == nil {
		return "", nil, false
	}
	for id, schema := range s.NestedSchemas {
		if schema.Name == name {
			return id, schema, true
		}
	}
	return "", nil, false
}

// HasNestedSchema checks if a nested schema with this ID exists
func (s *SchemaDefinition) HasNestedSchema(id string) bool {
	_, ok := s.GetNestedSchema(id)
	return ok
}

// HasNestedSchemaWithName checks if any nested schema has this name
func (s *SchemaDefinition) HasNestedSchemaWithName(name string) bool {
	_, _, ok := s.GetNestedSchemaByName(name)
	return ok
}

// NestedSchemaIDs returns all nested schema IDs, sorted alphabetically
func (s *SchemaDefinition) NestedSchemaIDs() []string {
	if s.NestedSchemas == nil {
		return []string{}
	}
	ids := make([]string, 0, len(s.NestedSchemas))
	for id := range s.NestedSchemas {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return string(ids[i]) < string(ids[j])
	})
	return ids
}

// FindFieldsReferencingNestedSchema returns all fields that reference this nested schema
func (s *SchemaDefinition) FindFieldsReferencingNestedSchema(id string) map[string]*FieldDefinition {
	result := make(map[string]*FieldDefinition)
	if s.Fields == nil {
		return result
	}

	for fieldID, field := range s.Fields {
		if field.Schema == nil {
			continue
		}

		// Check single schema reference
		if ref, ok := field.Schema.(NestedSchemaReference); ok {
			if string(ref.ID) == id {
				result[fieldID] = field
			}
			continue
		}

		// Check array of schema references (for unions)
		if refs, ok := field.Schema.([]NestedSchemaReference); ok {
			for _, ref := range refs {
				if string(ref.ID) == id {
					result[fieldID] = field
					break
				}
			}
		}
	}

	return result
}

// FindOrphanedNestedSchemas returns nested schemas not referenced by any field
func (s *SchemaDefinition) FindOrphanedNestedSchemas() []string {
	if s.NestedSchemas == nil {
		return []string{}
	}

	orphaned := []string{}
	for id := range s.NestedSchemas {
		refs := s.FindFieldsReferencingNestedSchema(id)
		if len(refs) == 0 {
			orphaned = append(orphaned, id)
		}
	}

	sort.Slice(orphaned, func(i, j int) bool {
		return string(orphaned[i]) < string(orphaned[j])
	})

	return orphaned
}

// ============================================================================
// INDEX NAVIGATION
// ============================================================================

// GetIndex returns the index definition and its position if found
func (s *SchemaDefinition) GetIndex(name string) (index *IndexDefinition, position int, found bool) {
	if s.Indexes == nil {
		return nil, -1, false
	}
	for i, ior := range s.Indexes {
		if ior.IsIndex() && ior.Index.Name == name {
			return ior.Index, i, true
		}
	}
	return nil, -1, false
}

// GetIndexAt returns the index at the specified position
func (s *SchemaDefinition) GetIndexAt(position int) (*IndexDefinition, bool) {
	if s.Indexes == nil || position < 0 || position >= len(s.Indexes) {
		return nil, false
	}
	ior := s.Indexes[position]
	if !ior.IsIndex() {
		return nil, false
	}
	return ior.Index, true
}

// HasIndex checks if an index with this name exists
func (s *SchemaDefinition) HasIndex(name string) bool {
	_, _, found := s.GetIndex(name)
	return found
}

// IndexCount returns the number of indexes
func (s *SchemaDefinition) IndexCount() int {
	if s.Indexes == nil {
		return 0
	}
	return len(s.Indexes)
}

// FindIndexesReferencingField returns all indexes that reference the field
func (s *SchemaDefinition) FindIndexesReferencingField(fieldName string) []IndexWithPosition {
	if s.Indexes == nil {
		return []IndexWithPosition{}
	}

	result := []IndexWithPosition{}
	for i, ior := range s.Indexes {
		if !ior.IsIndex() {
			continue
		}
		if ior.Index.ReferencesField(fieldName) {
			result = append(result, IndexWithPosition{
				Index:    ior.Index,
				Position: i,
			})
		}
	}
	return result
}

// FindIndexesReferencingAnyField returns indexes referencing any of the given fields
func (s *SchemaDefinition) FindIndexesReferencingAnyField(fieldNames []string) []IndexWithPosition {
	if s.Indexes == nil {
		return []IndexWithPosition{}
	}

	result := []IndexWithPosition{}
	for i, ior := range s.Indexes {
		if !ior.IsIndex() {
			continue
		}
		if ior.Index.ReferencesAnyField(fieldNames) {
			result = append(result, IndexWithPosition{
				Index:    ior.Index,
				Position: i,
			})
		}
	}
	return result
}

// FindIndexesByType returns all indexes of the specified type
func (s *SchemaDefinition) FindIndexesByType(indexType IndexType) []IndexWithPosition {
	if s.Indexes == nil {
		return []IndexWithPosition{}
	}

	result := []IndexWithPosition{}
	for i, ior := range s.Indexes {
		if !ior.IsIndex() {
			continue
		}
		if ior.Index.Type == indexType {
			result = append(result, IndexWithPosition{
				Index:    ior.Index,
				Position: i,
			})
		}
	}
	return result
}

// FindPrimaryIndex returns the primary key index if it exists
func (s *SchemaDefinition) FindPrimaryIndex() (*IndexDefinition, int, bool) {
	indexes := s.FindIndexesByType(IndexTypePrimary)
	if len(indexes) == 0 {
		return nil, -1, false
	}
	return indexes[0].Index, indexes[0].Position, true
}

// FindUniqueIndexes returns all unique indexes
func (s *SchemaDefinition) FindUniqueIndexes() []IndexWithPosition {
	if s.Indexes == nil {
		return []IndexWithPosition{}
	}

	result := []IndexWithPosition{}
	for i, ior := range s.Indexes {
		if !ior.IsIndex() {
			continue
		}
		if ior.Index.IsUnique() {
			result = append(result, IndexWithPosition{
				Index:    ior.Index,
				Position: i,
			})
		}
	}
	return result
}

// ============================================================================
// CONSTRAINT NAVIGATION
// ============================================================================

// GetConstraint returns the constraint rule and its location if found
func (s *SchemaDefinition) GetConstraint(name string) (*ConstraintRule, *ConstraintLocation, bool) {
	if s.Constraints == nil {
		return nil, nil, false
	}

	return findConstraintInRules(s.Constraints, name, "/constraints", 0)
}

// findConstraintInRules recursively searches for a constraint by name
func findConstraintInRules(rules SchemaConstraint, name string, basePath string, depth int) (*ConstraintRule, *ConstraintLocation, bool) {
	for i, rule := range rules {
		ruleName := getConstraintRuleName(&rule)
		currentPath := fmt.Sprintf("%s/%d", basePath, i)

		if ruleName == name {
			return &rule, &ConstraintLocation{
				JSONPath: currentPath,
				Position: i,
				Depth:    depth,
			}, true
		}

		if rule.IsConstraintGroup() {
			nestedPath := fmt.Sprintf("%s/rules", currentPath)
			if found, loc, ok := findConstraintInRules(rule.ConstraintGroup.Rules, name, nestedPath, depth+1); ok {
				loc.Parent = ruleName
				return found, loc, true
			}
		}
	}
	return nil, nil, false
}

// GetConstraintAt returns the constraint at the specified top-level position
func (s *SchemaDefinition) GetConstraintAt(position int) (*ConstraintRule, bool) {
	if s.Constraints == nil || position < 0 || position >= len(s.Constraints) {
		return nil, false
	}
	return &s.Constraints[position], true
}

// GetConstraintByPath returns constraint at hierarchical path (e.g., "group1/group2/constraint")
func (s *SchemaDefinition) GetConstraintByPath(path string) (*ConstraintRule, *ConstraintLocation, bool) {
	if s.Constraints == nil {
		return nil, nil, false
	}

	parts := ParseHierarchicalName(path)
	if len(parts) == 0 {
		return nil, nil, false
	}

	return findConstraintByPathParts(s.Constraints, parts, "/constraints", 0)
}

// findConstraintByPathParts recursively finds a constraint by hierarchical path parts
func findConstraintByPathParts(rules SchemaConstraint, parts []string, basePath string, depth int) (*ConstraintRule, *ConstraintLocation, bool) {
	if depth >= len(parts) {
		return nil, nil, false
	}

	targetName := parts[depth]

	for i, rule := range rules {
		ruleName := getConstraintRuleName(&rule)
		currentPath := fmt.Sprintf("%s/%d", basePath, i)

		if ruleName == targetName {
			if depth == len(parts)-1 {
				// Found the target
				return &rule, &ConstraintLocation{
					JSONPath: currentPath,
					Position: i,
					Depth:    depth,
				}, true
			}

			// Need to go deeper
			if rule.IsConstraintGroup() {
				nestedPath := fmt.Sprintf("%s/rules", currentPath)
				if found, loc, ok := findConstraintByPathParts(rule.ConstraintGroup.Rules, parts, nestedPath, depth+1); ok {
					loc.Parent = ruleName
					return found, loc, true
				}
			}
		}
	}

	return nil, nil, false
}

// HasConstraint checks if a constraint with this name exists (searches entire tree)
func (s *SchemaDefinition) HasConstraint(name string) bool {
	_, _, ok := s.GetConstraint(name)
	return ok
}

// HasConstraintAtPath checks if a constraint exists at the hierarchical path
func (s *SchemaDefinition) HasConstraintAtPath(path string) bool {
	_, _, ok := s.GetConstraintByPath(path)
	return ok
}

// ConstraintCount returns the number of top-level constraints
func (s *SchemaDefinition) ConstraintCount() int {
	if s.Constraints == nil {
		return 0
	}
	return len(s.Constraints)
}

// TotalConstraintCount returns total number of constraints including nested ones
func (s *SchemaDefinition) TotalConstraintCount() int {
	if s.Constraints == nil {
		return 0
	}
	return countConstraintsRecursive(s.Constraints)
}

// countConstraintsRecursive counts all constraints recursively
func countConstraintsRecursive(rules SchemaConstraint) int {
	count := 0
	for i := range rules {
		count++
		if rules[i].IsConstraintGroup() {
			count += countConstraintsRecursive(rules[i].ConstraintGroup.Rules)
		}
	}
	return count
}

// FindConstraintsReferencingField returns all constraints that reference the field
func (s *SchemaDefinition) FindConstraintsReferencingField(fieldName string) []ConstraintWithLocation {
	if s.Constraints == nil {
		return []ConstraintWithLocation{}
	}

	result := []ConstraintWithLocation{}
	collectConstraintsReferencingField(s.Constraints, fieldName, "/constraints", 0, &result)
	return result
}

// collectConstraintsReferencingField recursively collects constraints referencing a field
func collectConstraintsReferencingField(rules SchemaConstraint, fieldName string, basePath string, depth int, result *[]ConstraintWithLocation) {
	for i, rule := range rules {
		currentPath := fmt.Sprintf("%s/%d", basePath, i)

		if rule.ReferencesField(fieldName) {
			*result = append(*result, ConstraintWithLocation{
				Rule: &rule,
				Location: &ConstraintLocation{
					JSONPath: currentPath,
					Position: i,
					Depth:    depth,
				},
			})
		}

		if rule.IsConstraintGroup() {
			nestedPath := fmt.Sprintf("%s/rules", currentPath)
			collectConstraintsReferencingField(rule.ConstraintGroup.Rules, fieldName, nestedPath, depth+1, result)
		}
	}
}

// FindConstraintsReferencingAnyField returns constraints referencing any of the given fields
func (s *SchemaDefinition) FindConstraintsReferencingAnyField(fieldNames []string) []ConstraintWithLocation {
	if s.Constraints == nil {
		return []ConstraintWithLocation{}
	}

	result := []ConstraintWithLocation{}
	collectConstraintsReferencingAnyField(s.Constraints, fieldNames, "/constraints", 0, &result)
	return result
}

// collectConstraintsReferencingAnyField recursively collects constraints referencing any field
func collectConstraintsReferencingAnyField(rules SchemaConstraint, fieldNames []string, basePath string, depth int, result *[]ConstraintWithLocation) {
	for i, rule := range rules {
		currentPath := fmt.Sprintf("%s/%d", basePath, i)

		if rule.ReferencesAnyField(fieldNames) {
			*result = append(*result, ConstraintWithLocation{
				Rule: &rule,
				Location: &ConstraintLocation{
					JSONPath: currentPath,
					Position: i,
					Depth:    depth,
				},
			})
		}

		if rule.IsConstraintGroup() {
			nestedPath := fmt.Sprintf("%s/rules", currentPath)
			collectConstraintsReferencingAnyField(rule.ConstraintGroup.Rules, fieldNames, nestedPath, depth+1, result)
		}
	}
}

// ListAllConstraintPaths returns all hierarchical paths to constraints
func (s *SchemaDefinition) ListAllConstraintPaths() []string {
	if s.Constraints == nil {
		return []string{}
	}

	paths := []string{}
	collectAllConstraintPaths(s.Constraints, []string{}, &paths)
	sort.Strings(paths)
	return paths
}

// collectAllConstraintPaths recursively collects all constraint paths
func collectAllConstraintPaths(rules SchemaConstraint, currentPath []string, paths *[]string) {
	for i := range rules {
		ruleName := getConstraintRuleName(&rules[i])
		newPath := append(currentPath, ruleName)
		*paths = append(*paths, BuildHierarchicalName(newPath))

		if rules[i].IsConstraintGroup() {
			collectAllConstraintPaths(rules[i].ConstraintGroup.Rules, newPath, paths)
		}
	}
}

// ListAllConstraintNames returns all constraint names (flattened from groups)
func (s *SchemaDefinition) ListAllConstraintNames() []string {
	if s.Constraints == nil {
		return []string{}
	}

	names := []string{}
	collectAllConstraintNames(s.Constraints, &names)
	sort.Strings(names)
	return names
}

// collectAllConstraintNames recursively collects all constraint names
func collectAllConstraintNames(rules SchemaConstraint, names *[]string) {
	for i := range rules {
		ruleName := getConstraintRuleName(&rules[i])
		*names = append(*names, ruleName)

		if rules[i].IsConstraintGroup() {
			collectAllConstraintNames(rules[i].ConstraintGroup.Rules, names)
		}
	}
}

// getConstraintRuleName returns the name of a constraint rule
func getConstraintRuleName(rule *ConstraintRule) string {
	if rule.IsConstraint() {
		return rule.Constraint.Name
	}
	if rule.IsConstraintGroup() {
		return rule.ConstraintGroup.Name
	}
	return ""
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================


// containsAny checks if a string slice contains any of the values
func containsAny(slice []string, values []string) bool {
	for _, value := range values {
		if slices.Contains(slice, value) {
			return true
		}
	}
	return false
}
