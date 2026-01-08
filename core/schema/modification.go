package schema

import (
	"github.com/google/uuid"
)

// ============================================================================
// FIELD MODIFICATION OPERATIONS
// ============================================================================

// WithField returns a new schema with the field added or replaced (by ID)
func (s *SchemaDefinition) WithField(id string, field *FieldDefinition, provider SchemaProvider) (*SchemaDefinition, error) {
	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	if clone.Fields == nil {
		clone.Fields = make(map[string]*FieldDefinition)
	}

	fieldClone, err := field.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Fields[id] = fieldClone

	// Add nested schemas if provider is given
	if provider != nil {
		if err := addNestedSchemasFromProvider(clone, provider); err != nil {
			return nil, err
		}
	}

	return clone, nil
}

// WithFieldAdded returns a new schema with the field added (generates new ID, fails if name exists)
func (s *SchemaDefinition) WithFieldAdded(field *FieldDefinition, provider SchemaProvider) (*SchemaDefinition, string, error) {
	// Check if field name already exists
	if s.HasFieldWithName(field.Name) {
		return nil, "", NewFieldNameAlreadyExistsError(field.Name)
	}

	// Generate new stable ID
	newID := string(uuid.Must(uuid.NewV7()).String())

	clone, err := s.WithField(newID, field, provider)
	if err != nil {
		return nil, "", err
	}

	return clone, newID, nil
}

// WithFieldEnsured returns a new schema ensuring the field exists with exact properties
// If field with same name exists but different properties, it's replaced
// If field doesn't exist, it's added
// Returns the ID (existing or new) and whether it was modified
func (s *SchemaDefinition) WithFieldEnsured(field *FieldDefinition, provider SchemaProvider) (*SchemaDefinition, string, bool, error) {
	// Check if field with this name already exists
	existingID, existingField, exists := s.GetFieldByName(field.Name)

	if exists {
		// Check if it's identical
		if existingField.Equals(field) {
			// Already exists with correct properties
			return s, existingID, false, nil
		}

		// Replace with new properties
		clone, err := s.WithField(existingID, field, provider)
		if err != nil {
			return nil, "", false, err
		}
		return clone, existingID, true, nil
	}

	// Add new field
	clone, newID, err := s.WithFieldAdded(field, provider)
	if err != nil {
		return nil, "", false, err
	}

	return clone, newID, true, nil
}

// WithoutField returns a new schema without the specified field (by ID)
func (s *SchemaDefinition) WithoutField(id string) (*SchemaDefinition, error) {
	if !s.HasField(id) {
		return s, nil // Already doesn't exist
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	// Get field name before deleting
	field, _ := clone.Fields[id]
	fieldName := field.Name

	// Delete the field
	delete(clone.Fields, id)

	// Clean up indexes and constraints referencing this field
	clone, _, _ = clone.WithoutIndexesReferencingField(fieldName)
	clone, _, _ = clone.WithoutConstraintsReferencingField(fieldName)

	return clone, nil
}

// WithoutFieldByName returns a new schema without the field with the given name
func (s *SchemaDefinition) WithoutFieldByName(name string) (*SchemaDefinition, error) {
	id, _, ok := s.GetFieldByName(name)
	if !ok {
		return s, nil // Already doesn't exist
	}

	return s.WithoutField(id)
}

// WithFieldUpdated returns a new schema with the field modified via updater function
func (s *SchemaDefinition) WithFieldUpdated(id string, updater FieldUpdater) (*SchemaDefinition, error) {
	field, ok := s.GetField(id)
	if !ok {
		return nil, NewFieldNotFoundError(id)
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	fieldClone, err := field.DeepClone()
	if err != nil {
		return nil, err
	}

	if err := updater(fieldClone); err != nil {
		return nil, err
	}

	clone.Fields[id] = fieldClone
	return clone, nil
}

// WithFieldRenamed returns a new schema with the field's name changed
func (s *SchemaDefinition) WithFieldRenamed(id string, newName string) (*SchemaDefinition, error) {
	// Check if new name already exists
	if existingID, _, ok := s.GetFieldByName(newName); ok && existingID != id {
		return nil, NewFieldNameAlreadyExistsError(newName)
	}

	return s.WithFieldUpdated(id, func(fd *FieldDefinition) error {
		fd.Name = newName
		return nil
	})
}

// WithFields returns a new schema with multiple fields added/replaced
func (s *SchemaDefinition) WithFields(fields map[string]*FieldDefinition, provider SchemaProvider) (*SchemaDefinition, error) {
	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	if clone.Fields == nil {
		clone.Fields = make(map[string]*FieldDefinition)
	}

	for id, field := range fields {
		fieldClone, err := field.DeepClone()
		if err != nil {
			return nil, err
		}
		clone.Fields[id] = fieldClone
	}

	// Add nested schemas if provider is given
	if provider != nil {
		if err := addNestedSchemasFromProvider(clone, provider); err != nil {
			return nil, err
		}
	}

	return clone, nil
}

// WithoutFields returns a new schema without the specified fields
func (s *SchemaDefinition) WithoutFields(ids []string) (*SchemaDefinition, error) {
	clone := s
	var err error

	for _, id := range ids {
		clone, err = clone.WithoutField(id)
		if err != nil {
			return nil, err
		}
	}

	return clone, nil
}

// ============================================================================
// NESTED SCHEMA MODIFICATION OPERATIONS
// ============================================================================

// WithNestedSchema returns a new schema with the nested schema added or replaced (by ID)
func (s *SchemaDefinition) WithNestedSchema(id string, schema *NestedSchemaDefinition) (*SchemaDefinition, error) {
	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	if clone.NestedSchemas == nil {
		clone.NestedSchemas = make(map[string]*NestedSchemaDefinition)
	}

	schemaClone, err := schema.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.NestedSchemas[id] = schemaClone
	return clone, nil
}

// WithNestedSchemaAdded returns a new schema with the nested schema added (generates new ID)
func (s *SchemaDefinition) WithNestedSchemaAdded(schema *NestedSchemaDefinition) (*SchemaDefinition, string, error) {
	// Check if schema name already exists
	if s.HasNestedSchemaWithName(schema.Name) {
		return nil, "", NewNestedSchemaNameNotFoundError(schema.Name)
	}

	// Generate new stable ID or use existing
	var newID string
	if schema.ID != nil && *schema.ID != "" {
		newID = string(*schema.ID)
	} else {
		newID = string(uuid.Must(uuid.NewV7()).String())
	}

	clone, err := s.WithNestedSchema(newID, schema)
	if err != nil {
		return nil, "", err
	}

	return clone, newID, nil
}

// WithNestedSchemaEnsured ensures the nested schema exists with exact properties
func (s *SchemaDefinition) WithNestedSchemaEnsured(schema *NestedSchemaDefinition) (*SchemaDefinition, string, bool, error) {
	// Check if schema with this name already exists
	existingID, existingSchema, exists := s.GetNestedSchemaByName(schema.Name)

	if exists {
		// Check if it's identical
		if existingSchema.Equals(schema) {
			// Already exists with correct properties
			return s, existingID, false, nil
		}

		// Replace with new properties
		clone, err := s.WithNestedSchema(existingID, schema)
		if err != nil {
			return nil, "", false, err
		}
		return clone, existingID, true, nil
	}

	// Add new schema
	clone, newID, err := s.WithNestedSchemaAdded(schema)
	if err != nil {
		return nil, "", false, err
	}

	return clone, newID, true, nil
}

// WithoutNestedSchema returns a new schema without the specified nested schema
func (s *SchemaDefinition) WithoutNestedSchema(id string) (*SchemaDefinition, error) {
	if !s.HasNestedSchema(id) {
		return s, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	delete(clone.NestedSchemas, id)
	return clone, nil
}

// WithoutNestedSchemaByName returns a new schema without the nested schema with the given name
func (s *SchemaDefinition) WithoutNestedSchemaByName(name string) (*SchemaDefinition, error) {
	id, _, ok := s.GetNestedSchemaByName(name)
	if !ok {
		return s, nil
	}

	return s.WithoutNestedSchema(id)
}

// WithNestedSchemaUpdated returns a new schema with the nested schema modified
func (s *SchemaDefinition) WithNestedSchemaUpdated(id string, updater NestedSchemaUpdater) (*SchemaDefinition, error) {
	schema, ok := s.GetNestedSchema(id)
	if !ok {
		return nil, NewNestedSchemaNotFoundError(id)
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	schemaClone, err := schema.DeepClone()
	if err != nil {
		return nil, err
	}

	if err := updater(schemaClone); err != nil {
		return nil, err
	}

	clone.NestedSchemas[id] = schemaClone
	return clone, nil
}

// ============================================================================
// INDEX MODIFICATION OPERATIONS
// ============================================================================

// WithIndex returns a new schema with the index added (fails if name exists)
func (s *SchemaDefinition) WithIndex(index *IndexDefinition) (*SchemaDefinition, error) {
	if s.HasIndex(index.Name) {
		return nil, NewIndexAlreadyExistsError(index.Name)
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	if clone.Indexes == nil {
		clone.Indexes = []IndexOrReference{}
	}

	indexClone, err := index.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Indexes = append(clone.Indexes, IndexOrReference{Index: indexClone})
	return clone, nil
}

// WithIndexReplaced returns a new schema with the index replaced (by name)
func (s *SchemaDefinition) WithIndexReplaced(index *IndexDefinition) (*SchemaDefinition, error) {
	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	// Find and replace the index
	found := false
	for i, ior := range clone.Indexes {
		if ior.IsIndex() && ior.Index.Name == index.Name {
			indexClone, err := index.DeepClone()
			if err != nil {
				return nil, err
			}
			clone.Indexes[i] = IndexOrReference{Index: indexClone}
			found = true
			break
		}
	}

	if !found {
		return nil, NewIndexNotFoundError(index.Name)
	}

	return clone, nil
}

// WithIndexEnsured ensures the index exists (adds if missing, replaces if exists)
func (s *SchemaDefinition) WithIndexEnsured(index *IndexDefinition) (*SchemaDefinition, bool, error) {
	if s.HasIndex(index.Name) {
		clone, err := s.WithIndexReplaced(index)
		return clone, true, err
	}

	clone, err := s.WithIndex(index)
	return clone, true, err
}

// WithoutIndex returns a new schema without the specified index (by name)
func (s *SchemaDefinition) WithoutIndex(name string) (*SchemaDefinition, error) {
	if !s.HasIndex(name) {
		return s, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	newIndexes := []IndexOrReference{}
	for _, ior := range clone.Indexes {
		if !ior.IsIndex() || ior.Index.Name != name {
			newIndexes = append(newIndexes, ior)
		}
	}

	clone.Indexes = newIndexes
	return clone, nil
}

// WithoutIndexAt returns a new schema without the index at the specified position
func (s *SchemaDefinition) WithoutIndexAt(position int) (*SchemaDefinition, error) {
	if position < 0 || position >= s.IndexCount() {
		return s, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Indexes = append(clone.Indexes[:position], clone.Indexes[position+1:]...)
	return clone, nil
}

// WithoutIndexesReferencingField returns a new schema without indexes referencing the field
func (s *SchemaDefinition) WithoutIndexesReferencingField(fieldName string) (*SchemaDefinition, *IndexRemovalResult, error) {
	indexes := s.FindIndexesReferencingField(fieldName)
	if len(indexes) == 0 {
		return s, &IndexRemovalResult{RemovedNames: []string{}, Count: 0}, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, nil, err
	}

	removedNames := []string{}
	newIndexes := []IndexOrReference{}

	for _, ior := range clone.Indexes {
		if ior.IsIndex() && ior.Index.ReferencesField(fieldName) {
			removedNames = append(removedNames, ior.Index.Name)
		} else {
			newIndexes = append(newIndexes, ior)
		}
	}

	clone.Indexes = newIndexes
	result := &IndexRemovalResult{
		RemovedNames: removedNames,
		Count:        len(removedNames),
	}

	return clone, result, nil
}

// WithoutIndexesReferencingFields returns a new schema without indexes referencing any of the fields
func (s *SchemaDefinition) WithoutIndexesReferencingFields(fieldNames []string) (*SchemaDefinition, *IndexRemovalResult, error) {
	indexes := s.FindIndexesReferencingAnyField(fieldNames)
	if len(indexes) == 0 {
		return s, &IndexRemovalResult{RemovedNames: []string{}, Count: 0}, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, nil, err
	}

	removedNames := []string{}
	newIndexes := []IndexOrReference{}

	for _, ior := range clone.Indexes {
		if ior.IsIndex() && ior.Index.ReferencesAnyField(fieldNames) {
			removedNames = append(removedNames, ior.Index.Name)
		} else {
			newIndexes = append(newIndexes, ior)
		}
	}

	clone.Indexes = newIndexes
	result := &IndexRemovalResult{
		RemovedNames: removedNames,
		Count:        len(removedNames),
	}

	return clone, result, nil
}

// WithIndexUpdated returns a new schema with the index modified
func (s *SchemaDefinition) WithIndexUpdated(name string, updater IndexUpdater) (*SchemaDefinition, error) {
	index, position, ok := s.GetIndex(name)
	if !ok {
		return nil, NewIndexNotFoundError(name)
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	indexClone, err := index.DeepClone()
	if err != nil {
		return nil, err
	}

	if err := updater(indexClone); err != nil {
		return nil, err
	}

	clone.Indexes[position] = IndexOrReference{Index: indexClone}
	return clone, nil
}

// WithIndexes returns a new schema with multiple indexes added
func (s *SchemaDefinition) WithIndexes(indexes []*IndexDefinition) (*SchemaDefinition, error) {
	clone := s
	var err error

	for _, index := range indexes {
		clone, err = clone.WithIndex(index)
		if err != nil {
			return nil, err
		}
	}

	return clone, nil
}

// WithoutIndexes returns a new schema without the specified indexes
func (s *SchemaDefinition) WithoutIndexes(names []string) (*SchemaDefinition, error) {
	clone := s
	var err error

	for _, name := range names {
		clone, err = clone.WithoutIndex(name)
		if err != nil {
			return nil, err
		}
	}

	return clone, nil
}

// ============================================================================
// CONSTRAINT MODIFICATION OPERATIONS
// ============================================================================

// WithConstraint returns a new schema with the constraint added (fails if name exists)
func (s *SchemaDefinition) WithConstraint(constraint ConstraintRule) (*SchemaDefinition, error) {
	name := constraint.GetName()
	if s.HasConstraint(name) {
		return nil, NewConstraintAlreadyExistsError(name)
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	if clone.Constraints == nil {
		clone.Constraints = SchemaConstraint{}
	}

	constraintClone, err := constraint.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraints = append(clone.Constraints, *constraintClone)
	return clone, nil
}

// WithConstraintReplaced returns a new schema with the constraint replaced (by name)
func (s *SchemaDefinition) WithConstraintReplaced(constraint ConstraintRule) (*SchemaDefinition, error) {
	name := constraint.GetName()

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	// Find and replace the constraint
	replaced := false
	for i := range clone.Constraints {
		if clone.Constraints[i].GetName() == name {
			constraintClone, err := constraint.DeepClone()
			if err != nil {
				return nil, err
			}
			clone.Constraints[i] = *constraintClone
			replaced = true
			break
		}
	}

	if !replaced {
		return nil, NewConstraintNotFoundError(name)
	}

	return clone, nil
}

// WithConstraintEnsured ensures the constraint exists (adds if missing, replaces if exists)
func (s *SchemaDefinition) WithConstraintEnsured(constraint ConstraintRule) (*SchemaDefinition, bool, error) {
	name := constraint.GetName()

	if s.HasConstraint(name) {
		clone, err := s.WithConstraintReplaced(constraint)
		return clone, true, err
	}

	clone, err := s.WithConstraint(constraint)
	return clone, true, err
}

// WithoutConstraint returns a new schema without the specified constraint (by name)
func (s *SchemaDefinition) WithoutConstraint(name string) (*SchemaDefinition, error) {
	if !s.HasConstraint(name) {
		return s, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraints, _ = removeConstraintByName(clone.Constraints, name)
	return clone, nil
}

// WithoutConstraintAt returns a new schema without the constraint at the specified position
func (s *SchemaDefinition) WithoutConstraintAt(position int) (*SchemaDefinition, error) {
	if position < 0 || position >= s.ConstraintCount() {
		return s, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraints = append(clone.Constraints[:position], clone.Constraints[position+1:]...)
	return clone, nil
}

// WithoutConstraintByPath returns a new schema without the constraint at the hierarchical path
func (s *SchemaDefinition) WithoutConstraintByPath(path string) (*SchemaDefinition, error) {
	if !s.HasConstraintAtPath(path) {
		return s, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	parts := ParseHierarchicalName(path)
	clone.Constraints, _ = removeConstraintByPathParts(clone.Constraints, parts, 0)
	return clone, nil
}

// WithoutConstraintsReferencingField returns a new schema without constraints referencing the field
func (s *SchemaDefinition) WithoutConstraintsReferencingField(fieldName string) (*SchemaDefinition, *ConstraintRemovalResult, error) {
	constraints := s.FindConstraintsReferencingField(fieldName)
	if len(constraints) == 0 {
		return s, &ConstraintRemovalResult{RemovedPaths: []string{}, RemovedNames: []string{}, Count: 0}, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, nil, err
	}

	removedNames := []string{}
	removedPaths := []string{}

	clone.Constraints = filterConstraintsByField(clone.Constraints, fieldName, &removedNames, &removedPaths)

	result := &ConstraintRemovalResult{
		RemovedPaths: removedPaths,
		RemovedNames: removedNames,
		Count:        len(removedNames),
	}

	return clone, result, nil
}

// WithoutConstraintsReferencingFields returns a new schema without constraints referencing any of the fields
func (s *SchemaDefinition) WithoutConstraintsReferencingFields(fieldNames []string) (*SchemaDefinition, *ConstraintRemovalResult, error) {
	constraints := s.FindConstraintsReferencingAnyField(fieldNames)
	if len(constraints) == 0 {
		return s, &ConstraintRemovalResult{RemovedPaths: []string{}, RemovedNames: []string{}, Count: 0}, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, nil, err
	}

	removedNames := []string{}
	removedPaths := []string{}

	for _, fieldName := range fieldNames {
		clone.Constraints = filterConstraintsByField(clone.Constraints, fieldName, &removedNames, &removedPaths)
	}

	result := &ConstraintRemovalResult{
		RemovedPaths: removedPaths,
		RemovedNames: removedNames,
		Count:        len(removedNames),
	}

	return clone, result, nil
}

// WithConstraintUpdated returns a new schema with the constraint modified
func (s *SchemaDefinition) WithConstraintUpdated(name string, updater ConstraintUpdater) (*SchemaDefinition, error) {
	if _, _, ok := s.GetConstraint(name); !ok {
		return nil, NewConstraintNotFoundError(name)
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, err
	}

	// Find and update the constraint
	if err := updateConstraintInRules(clone.Constraints, name, updater); err != nil {
		return nil, err
	}

	return clone, nil
}

// WithConstraints returns a new schema with multiple constraints added
func (s *SchemaDefinition) WithConstraints(constraints []ConstraintRule) (*SchemaDefinition, error) {
	clone := s
	var err error

	for _, constraint := range constraints {
		clone, err = clone.WithConstraint(constraint)
		if err != nil {
			return nil, err
		}
	}

	return clone, nil
}

// WithoutConstraints returns a new schema without the specified constraints
func (s *SchemaDefinition) WithoutConstraints(names []string) (*SchemaDefinition, error) {
	clone := s
	var err error

	for _, name := range names {
		clone, err = clone.WithoutConstraint(name)
		if err != nil {
			return nil, err
		}
	}

	return clone, nil
}

// ============================================================================
// PROPERTY MODIFICATION OPERATIONS
// ============================================================================

// WithDescription returns a new schema with the description updated
func (s *SchemaDefinition) WithDescription(description string) *SchemaDefinition {
	clone, _ := s.DeepClone()
	clone.Description = &description
	return clone
}

// WithoutDescription returns a new schema with the description removed
func (s *SchemaDefinition) WithoutDescription() *SchemaDefinition {
	clone, _ := s.DeepClone()
	clone.Description = nil
	return clone
}

// WithMetadata returns a new schema with metadata added/updated
func (s *SchemaDefinition) WithMetadata(key string, value any) *SchemaDefinition {
	clone, _ := s.DeepClone()
	if clone.Metadata == nil {
		clone.Metadata = make(map[string]any)
	}
	clone.Metadata[key] = value
	return clone
}

// WithoutMetadata returns a new schema with metadata key removed
func (s *SchemaDefinition) WithoutMetadata(key string) *SchemaDefinition {
	clone, _ := s.DeepClone()
	if clone.Metadata != nil {
		delete(clone.Metadata, key)
	}
	return clone
}

// WithMetadataMap returns a new schema with entire metadata replaced
func (s *SchemaDefinition) WithMetadataMap(metadata map[string]any) *SchemaDefinition {
	clone, _ := s.DeepClone()
	clone.Metadata = metadata
	return clone
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// addNestedSchemasFromProvider adds nested schemas from a provider
func addNestedSchemasFromProvider(s *SchemaDefinition, provider SchemaProvider) error {
	primary, dependencies := provider(s)

	if s.NestedSchemas == nil {
		s.NestedSchemas = make(map[string]*NestedSchemaDefinition)
	}

	// Add primary schema
	if primary != nil {
		var id string
		if primary.ID != nil {
			id = string(*primary.ID)
		} else {
			id = string(primary.Name)
		}
		s.NestedSchemas[id] = primary
	}

	// Add dependencies
	for _, dep := range dependencies {
		if dep != nil {
			var id string
			if dep.ID != nil {
				id = string(*dep.ID)
			} else {
				id = string(dep.Name)
			}
			s.NestedSchemas[id] = dep
		}
	}

	return nil
}

// removeConstraintByName removes a constraint by name from a list
func removeConstraintByName(rules SchemaConstraint, name string) (SchemaConstraint, bool) {
	result := SchemaConstraint{}
	found := false

	for i := range rules {
		if rules[i].GetName() == name {
			found = true
			continue
		}
		result = append(result, rules[i])
	}

	return result, found
}

// removeConstraintByPathParts removes a constraint by hierarchical path
func removeConstraintByPathParts(rules SchemaConstraint, parts []string, depth int) (SchemaConstraint, bool) {
	if depth >= len(parts) {
		return rules, false
	}

	targetName := parts[depth]
	result := SchemaConstraint{}
	found := false

	for i := range rules {
		ruleName := rules[i].GetName()

		if ruleName == targetName {
			if depth == len(parts)-1 {
				// This is the target to remove
				found = true
				continue
			}

			// Need to go deeper
			if rules[i].IsConstraintGroup() {
				newRules, foundNested := removeConstraintByPathParts(rules[i].ConstraintGroup.Rules, parts, depth+1)
				if foundNested {
					found = true
					if len(newRules) > 0 {
						rules[i].ConstraintGroup.Rules = newRules
						result = append(result, rules[i])
					}
				} else {
					result = append(result, rules[i])
				}
			} else {
				result = append(result, rules[i])
			}
		} else {
			result = append(result, rules[i])
		}
	}

	return result, found
}

// filterConstraintsByField filters out constraints referencing a field
func filterConstraintsByField(rules SchemaConstraint, fieldName string, removedNames *[]string, removedPaths *[]string) SchemaConstraint {
	result := SchemaConstraint{}

	for i := range rules {
		if rules[i].ReferencesField(fieldName) {
			*removedNames = append(*removedNames, rules[i].GetName())
			continue
		}

		if rules[i].IsConstraintGroup() {
			filteredRules := filterConstraintsByField(rules[i].ConstraintGroup.Rules, fieldName, removedNames, removedPaths)
			if len(filteredRules) > 0 {
				rules[i].ConstraintGroup.Rules = filteredRules
				result = append(result, rules[i])
			}
		} else {
			result = append(result, rules[i])
		}
	}

	return result
}

// updateConstraintInRules finds and updates a constraint by name
func updateConstraintInRules(rules SchemaConstraint, name string, updater ConstraintUpdater) error {
	for i := range rules {
		if rules[i].GetName() == name {
			return updater(&rules[i])
		}

		if rules[i].IsConstraintGroup() {
			if err := updateConstraintInRules(rules[i].ConstraintGroup.Rules, name, updater); err == nil {
				return nil
			}
		}
	}

	return NewConstraintNotFoundError(name)
}
