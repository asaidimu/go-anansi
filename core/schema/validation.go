package schema

import (
	"fmt"
	"slices"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// ============================================================================
// COMPREHENSIVE SEMANTIC VALIDATION
// ============================================================================

// ValidateAll runs all validation checks to ensure schema is semantically correct
// Based on the TypeScript schema definition semantics
func (s *SchemaDefinition) ValidateAll() []common.Issue {
	errors := []common.Issue{}

	// Core schema validation
	errors = append(errors, s.validateSchemaStructure()...)

	// Field validation
	errors = append(errors, s.validateFields()...)

	// Reference validation
	errors = append(errors, s.ValidateIndexReferences()...)
	errors = append(errors, s.ValidateConstraintReferences()...)
	errors = append(errors, s.ValidateNestedSchemaReferences()...)

	// Semantic validation
	errors = append(errors, s.validateFieldSemantics()...)
	errors = append(errors, s.validateIndexSemantics()...)
	errors = append(errors, s.validateConstraintSemantics()...)
	errors = append(errors, s.validateNestedSchemaSemantics()...)

	return errors
}

// ============================================================================
// SCHEMA STRUCTURE VALIDATION
// ============================================================================

// validateSchemaStructure validates the basic schema structure
func (s *SchemaDefinition) validateSchemaStructure() []common.Issue {
	errors := []common.Issue{}

	// Name is required
	if s.Name == "" {
		errors = append(errors, common.Issue{
			Path:    "/",
			Message: "schema name is required",
		})
	}

	// Name must be valid identifier
	if s.Name != "" && !IsValidIdentifier(s.Name) {
		errors = append(errors, common.Issue{
			Path:    "/name",
			Message: "schema name must be a valid identifier (alphanumeric and underscores only)",
		})
	}

	// Version is required
	if s.Version == "" {
		errors = append(errors, common.Issue{
			Path:    "/version",
			Message: "schema version is required",
		})
	}

	// Version must be valid (semantic versioning recommended)
	if _, err := common.NewVersion(s.Version); err != nil {
		errors = append(errors, common.Issue{
			Path:    "/version",
			Message: "schema version should follow semantic versioning (e.g., 1.0.0)",
		})
	}

	// Fields map must not be nil
	if s.Fields == nil {
		errors = append(errors, common.Issue{
			Path:    "/fields",
			Message: "fields map cannot be nil",
		})
	}

	return errors
}

// ============================================================================
// FIELD VALIDATION
// ============================================================================

// validateFields validates all field definitions
func (s *SchemaDefinition) validateFields() []common.Issue {
	errors := []common.Issue{}

	if s.Fields == nil {
		return errors
	}

	// Check for duplicate field names
	nameMap := make(map[string]string)
	for id, field := range s.Fields {
		if existingID, exists := nameMap[field.Name]; exists {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/fields/%s", id),
				Message: fmt.Sprintf("duplicate field name '%s' (also defined with ID '%s')", field.Name, existingID),
			})
		}
		nameMap[field.Name] = id

		// Validate individual field
		errors = append(errors, s.validateField(id, field)...)
	}

	return errors
}

// validateField validates a single field definition
func (s *SchemaDefinition) validateField(id string, field *FieldDefinition) []common.Issue {
	errors := []common.Issue{}
	basePath := fmt.Sprintf("/fields/%s", id)

	// Name is required
	if field.Name == "" {
		errors = append(errors, common.Issue{
			Path:    basePath + "/name",
			Message: "field name is required",
		})
	}

	// Name must be valid identifier
	if field.Name != "" && !IsValidIdentifier(field.Name) {
		errors = append(errors, common.Issue{
			Path:    basePath + "/name",
			Message: "field name must be a valid identifier (alphanumeric and underscores only)",
		})
	}

	// Type is required (in Go it's always set, but check for unknown)
	if field.Type == FieldTypeUnknown || field.Type == "" {
		errors = append(errors, common.Issue{
			Path:    basePath + "/type",
			Message: "field type is required and must be valid",
		})
	}

	// Validate type-specific requirements
	errors = append(errors, validateFieldTypeSemantics(basePath, field)...)

	return errors
}

// validateFieldTypeSemantics validates type-specific field semantics
func validateFieldTypeSemantics(basePath string, field *FieldDefinition) []common.Issue {
	errors := []common.Issue{}

	switch field.Type {
	case FieldTypeEnum:
		// Enum must have values
		if len(field.Values) == 0 {
			errors = append(errors, common.Issue{
				Path:    basePath + "/values",
				Message: "enum field must define values",
			})
		}

	case FieldTypeArray, FieldTypeSet:
		// Array/Set should have itemsType
		if field.ItemsType == nil {
			errors = append(errors, common.Issue{
				Path:    basePath + "/itemsType",
				Message: fmt.Sprintf("%s field should specify itemsType", field.Type),
			})
		}

		// If itemsType is object/union, must have schema
		if field.ItemsType != nil && (*field.ItemsType == FieldTypeObject || *field.ItemsType == FieldTypeUnion) {
			if field.Schema == nil {
				errors = append(errors, common.Issue{
					Path:    basePath + "/schema",
					Message: fmt.Sprintf("%s field with itemsType '%s' must have schema reference", field.Type, *field.ItemsType),
				})
			}
		}

	case FieldTypeObject:
		// Object must have schema reference
		if field.Schema == nil {
			errors = append(errors, common.Issue{
				Path:    basePath + "/schema",
				Message: "object field must have schema reference",
			})
		}

		// Object should have single schema, not array
		if field.Schema != nil {
			if _, ok := field.Schema.([]NestedSchemaReference); ok {
				errors = append(errors, common.Issue{
					Path:    basePath + "/schema",
					Message: "object field should have single schema reference, not array (use union for multiple schemas)",
				})
			}
		}

	case FieldTypeUnion:
		// Union must have schema array
		if field.Schema == nil {
			errors = append(errors, common.Issue{
				Path:    basePath + "/schema",
				Message: "union field must have schema references",
			})
		}

		// Union schema must be array
		if field.Schema != nil {
			if _, ok := field.Schema.(NestedSchemaReference); ok {
				errors = append(errors, common.Issue{
					Path:    basePath + "/schema",
					Message: "union field must have array of schema references, not single reference",
				})
			}

			// Must have at least 2 schemas for union
			if refs, ok := field.Schema.([]NestedSchemaReference); ok {
				if len(refs) < 2 {
					errors = append(errors, common.Issue{
						Path:    basePath + "/schema",
						Message: "union field must have at least 2 schema references",
					})
				}
			}
		}

	case FieldTypeRecord: // records are very permissive
	case FieldTypeDynamic:
		//Dynamic should not have schema
		if field.Schema != nil {
			errors = append(errors, common.Issue{
				Path:    basePath + "/schema",
				Message: fmt.Sprintf("%s field should not have schema reference (it's unstructured)", field.Type),
			})
		}

	case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeDecimal, FieldTypeBoolean:
		// Primitives should not have schema
		if field.Schema != nil {
			errors = append(errors, common.Issue{
				Path:    basePath + "/schema",
				Message: fmt.Sprintf("primitive field type '%s' should not have schema reference", field.Type),
			})
		}

		// Primitives should not have itemsType
		if field.ItemsType != nil {
			errors = append(errors, common.Issue{
				Path:    basePath + "/itemsType",
				Message: fmt.Sprintf("primitive field type '%s' should not have itemsType", field.Type),
			})
		}
	}

	// Validate default value type compatibility (basic check)
	if field.Default != nil {
		errors = append(errors, validateDefaultValue(basePath, field)...)
	}

	return errors
}

// validateDefaultValue validates that default value is compatible with field type
func validateDefaultValue(basePath string, field *FieldDefinition) []common.Issue {
	errors := []common.Issue{}

	// This is a basic type check - full validation would require runtime type checking
	if !field.ValidateType(field.Default) {
		errors = append(errors, common.Issue{
			Path:    basePath + "/default",
			Message: fmt.Sprintf("default value type is incompatible with field type '%s'", field.Type),
		})
	}

	return errors
}

// ============================================================================
// FIELD SEMANTICS VALIDATION
// ============================================================================

// validateFieldSemantics validates semantic rules for fields
func (s *SchemaDefinition) validateFieldSemantics() []common.Issue {
	errors := []common.Issue{}

	if s.Fields == nil {
		return errors
	}

	// Check for required fields with default values (warning)
	for id, field := range s.Fields {
		if field.IsRequired() && field.HasDefault() {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/fields/%s", id),
				Message: "required field has default value (default will never be used)",
			})
		}

		// Check for deprecated required fields
		if field.IsDeprecated() && field.IsRequired() {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/fields/%s", id),
				Message: "deprecated field should not be required",
			})
		}

		// Check for unique non-required fields without default
		if field.IsUnique() && !field.IsRequired() && !field.HasDefault() {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/fields/%s", id),
				Message: "unique optional field should have default value or be required",
			})
		}
	}

	return errors
}

// ============================================================================
// INDEX SEMANTICS VALIDATION
// ============================================================================

// validateIndexSemantics validates semantic rules for indexes
func (s *SchemaDefinition) validateIndexSemantics() []common.Issue {
	errors := []common.Issue{}

	if s.Indexes == nil {
		return errors
	}

	// Check for duplicate index names
	nameMap := make(map[string]int)
	for i, ior := range s.Indexes {
		if !ior.IsIndex() {
			continue
		}

		index := ior.Index

		// Name is required
		if index.Name == "" {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/indexes/%d", i),
				Message: "index name is required",
			})
			continue
		}

		// Check for duplicates
		if existingIdx, exists := nameMap[index.Name]; exists {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/indexes/%d", i),
				Message: fmt.Sprintf("duplicate index name (also defined at position %d)", existingIdx),
			})
		}
		nameMap[index.Name] = i

		// Fields must not be empty
		if len(index.Fields) == 0 {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/indexes/%d/fields", i),
				Message: "index must reference at least one field",
			})
		}

		// Check for duplicate fields in index
		fieldSet := make(map[string]bool)
		for _, fieldName := range index.Fields {
			if fieldSet[fieldName] {
				errors = append(errors, common.Issue{
					Path:    fmt.Sprintf("/indexes/%d/fields", i),
					Message: fmt.Sprintf("duplicate field '%s' in index", fieldName),
				})
			}
			fieldSet[fieldName] = true
		}

		// Primary key must be unique
		if index.IsPrimary() && !index.IsUnique() {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/indexes/%d", i),
				Message: "primary key index must be unique",
			})
		}

		// Spatial indexes should only have one field
		if index.Type == IndexTypeSpatial && len(index.Fields) > 1 {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/indexes/%d/fields", i),
				Message: "spatial index should only reference one field",
			})
		}
	}

	// Check for multiple primary keys
	primaryCount := 0
	for _, ior := range s.Indexes {
		if ior.IsIndex() && ior.Index.IsPrimary() {
			primaryCount++
		}
	}
	if primaryCount > 1 {
		errors = append(errors, common.Issue{
			Path:    "/indexes",
			Message: fmt.Sprintf("schema has %d primary key indexes, should have at most 1", primaryCount),
		})
	}

	return errors
}

// ============================================================================
// CONSTRAINT SEMANTICS VALIDATION
// ============================================================================

// validateConstraintSemantics validates semantic rules for constraints
func (s *SchemaDefinition) validateConstraintSemantics() []common.Issue {
	errors := []common.Issue{}

	if s.Constraints == nil {
		return errors
	}

	// Check for duplicate constraint names
	nameMap := make(map[string]string)
	validateConstraintNames(s.Constraints, "/constraints", nameMap, &errors)

	// Validate constraint structure
	validateConstraintStructures(s.Constraints, "/constraints", &errors)

	return errors
}

// validateConstraintNames recursively checks for duplicate constraint names
func validateConstraintNames(rules SchemaConstraint, basePath string, nameMap map[string]string, errors *[]common.Issue) {
	for i := range rules {
		currentPath := fmt.Sprintf("%s/%d", basePath, i)
		rule := &rules[i]
		name := rule.GetName()

		if name == "" {
			*errors = append(*errors, common.Issue{
				Path:    currentPath + "/name",
				Message: "constraint name is required",
			})
			continue
		}

		// Check for duplicate names
		if existingPath, exists := nameMap[name]; exists {
			*errors = append(*errors, common.Issue{
				Path:    currentPath,
				Message: fmt.Sprintf("duplicate constraint name (also defined at %s)", existingPath),
			})
		}
		nameMap[name] = currentPath

		// Recursively check nested groups
		if rule.IsConstraintGroup() {
			validateConstraintNames(rule.ConstraintGroup.Rules, currentPath+"/rules", nameMap, errors)
		}
	}
}

// validateConstraintStructures validates constraint internal structure
func validateConstraintStructures(rules SchemaConstraint, basePath string, errors *[]common.Issue) {
	for i := range rules {
		currentPath := fmt.Sprintf("%s/%d", basePath, i)
		rule := &rules[i]

		if rule.IsConstraint() {
			// Predicate is required
			if rule.Constraint.Predicate == "" {
				*errors = append(*errors, common.Issue{
					Path:    currentPath + "/predicate",
					Message: "constraint predicate is required",
				})
			}

			// Must have either field or fields (not both)
			hasField := rule.Constraint.Field != nil && *rule.Constraint.Field != ""
			hasFields := len(rule.Constraint.Fields) > 0

			if !hasField && !hasFields {
				*errors = append(*errors, common.Issue{
					Path:    currentPath,
					Message: "constraint must reference at least one field (either 'field' or 'fields')",
				})
			}

			if hasField && hasFields {
				*errors = append(*errors, common.Issue{
					Path:    currentPath,
					Message: "constraint should use either 'field' or 'fields', not both",
				})
			}
		}

		if rule.IsConstraintGroup() {
			// Group must have rules
			if len(rule.ConstraintGroup.Rules) == 0 {
				*errors = append(*errors, common.Issue{
					Path:    currentPath + "/rules",
					Message: "constraint group must have at least one rule",
				})
			}

			// Operator is required
			if rule.ConstraintGroup.Operator == "" {
				*errors = append(*errors, common.Issue{
					Path:    currentPath + "/operator",
					Message: "constraint group operator is required",
				})
			}

			// Recursively validate nested rules
			validateConstraintStructures(rule.ConstraintGroup.Rules, currentPath+"/rules", errors)
		}
	}
}

// ============================================================================
// NESTED SCHEMA SEMANTICS VALIDATION
// ============================================================================

// validateNestedSchemaSemantics validates semantic rules for nested schemas
func (s *SchemaDefinition) validateNestedSchemaSemantics() []common.Issue {
	errors := []common.Issue{}

	if s.NestedSchemas == nil {
		return errors
	}

	// Check for duplicate nested schema names
	nameMap := make(map[string]string)
	for id, nestedSchema := range s.NestedSchemas {
		if existingID, exists := nameMap[nestedSchema.Name]; exists {
			errors = append(errors, common.Issue{
				Path:    fmt.Sprintf("/nestedSchemas/%s", id),
				Message: fmt.Sprintf("duplicate nested schema name (also defined with ID '%s')", existingID),
			})
		}
		nameMap[nestedSchema.Name] = id

		// Validate nested schema structure
		errors = append(errors, validateNestedSchemaStructure(s, id, nestedSchema)...)
	}

	return errors
}

// validateNestedSchemaStructure validates a nested schema's structure
func validateNestedSchemaStructure(s *SchemaDefinition, id string, nsd *NestedSchemaDefinition) []common.Issue {
	errors := []common.Issue{}
	basePath := fmt.Sprintf("/nestedSchemas/%s", id)

	// Name is required
	if nsd.Name == "" {
		errors = append(errors, common.Issue{
			Path:    basePath + "/name",
			Message: "nested schema name is required",
		})
	}

	// Name must be valid identifier
	if nsd.Name != "" && !IsValidIdentifier(nsd.Name) {
		errors = append(errors, common.Issue{
			Path:    basePath + "/name",
			Message: "nested schema name must be a valid identifier",
		})
	}

	// Must be either structured (has fields) or typed (has type)
	hasFields := nsd.Fields != nil
	hasType := nsd.Type != nil

	if !hasFields && !hasType {
		errors = append(errors, common.Issue{
			Path:    basePath,
			Message: "nested schema must have either 'fields' (structured) or 'type' (typed)",
		})
	}

	if hasFields && hasType {
		errors = append(errors, common.Issue{
			Path:    basePath,
			Message: "nested schema cannot have both 'fields' and 'type'",
		})
	}

	// If structured, validate fields
	if hasFields {
		sc, err := s.ResolveNestedSchema(id)
		if err != nil {
			errors = append(errors, common.SystemErrorFrom(err).ToIssue())
		}
		errors = append(errors, sc.validateSchemaStructure()...)
		if nsd.Fields.IsMap() {
			if len(nsd.Fields.FieldsMap) == 0 {
				errors = append(errors, common.Issue{
					Path:    basePath + "/fields",
					Message: "structured nested schema must have at least one field",
				})
			}
		}

		if nsd.Fields.IsArray() {
			if len(nsd.Fields.FieldsArray) == 0 {
				errors = append(errors, common.Issue{
					Path:    basePath + "/fields",
					Message: "conditional field array must have at least one entry",
				})
			}

			// Check for discriminator field consistency
			discriminatorField := ""
			for i, cfs := range nsd.Fields.FieldsArray {
				if cfs.When != nil {
					if discriminatorField == "" {
						discriminatorField = cfs.When.Field
					} else if cfs.When.Field != discriminatorField {
						errors = append(errors, common.Issue{
							Path:    fmt.Sprintf("%s/fields/%d/when", basePath, i),
							Message: fmt.Sprintf("all 'when' conditions must use the same discriminator field (found '%s' and '%s')", discriminatorField, cfs.When.Field),
						})
					}
				}
			}
		}
	}

	// If typed, validate type
	if hasType {
		if *nsd.Type == FieldTypeUnknown || *nsd.Type == "" {
			errors = append(errors, common.Issue{
				Path:    basePath + "/type",
				Message: "nested schema type must be valid",
			})
		}

		// Typed nested schemas shouldn't be 'object' or 'union' (those need fields)
		if *nsd.Type == FieldTypeObject || *nsd.Type == FieldTypeUnion {
			errors = append(errors, common.Issue{
				Path:    basePath + "/type",
				Message: fmt.Sprintf("typed nested schema should not use type '%s' (use structured with fields instead)", *nsd.Type),
			})
		}
	}

	return errors
}

// ============================================================================
// REFERENCE VALIDATION (from original validation.go)
// ============================================================================

// ValidateIndexReferences checks if all index field references are valid
func (s *SchemaDefinition) ValidateIndexReferences() []common.Issue {
	errors := []common.Issue{}

	if s.Indexes == nil {
		return errors
	}

	for i, ior := range s.Indexes {
		if !ior.IsIndex() {
			continue
		}

		index := ior.Index
		for _, fieldName := range index.Fields {
			if !s.HasFieldWithName(fieldName) {
				errors = append(errors, common.Issue{
					Path:    fmt.Sprintf("/indexes/%d", i),
					Message: fmt.Sprintf("references non-existent field '%s'", fieldName),
				})
			}
		}
	}

	return errors
}

// ValidateConstraintReferences checks if all constraint field references are valid
func (s *SchemaDefinition) ValidateConstraintReferences() []common.Issue {
	errors := []common.Issue{}

	if s.Constraints == nil {
		return errors
	}

	validateConstraintRulesReferences(s.Constraints, s, "/constraints", &errors)
	return errors
}

// ValidateNestedSchemaReferences checks if all nested schema references are valid
func (s *SchemaDefinition) ValidateNestedSchemaReferences() []common.Issue {
	errors := []common.Issue{}

	if s.Fields == nil {
		return errors
	}

	for id, field := range s.Fields {
		// Check single schema reference
		if ref, ok := field.GetSchemaReference(); ok {
			schemaID := string(ref.ID)
			if !s.HasNestedSchema(schemaID) {
				errors = append(errors, common.Issue{
					Path:    fmt.Sprintf("/fields/%s/schema", id),
					Message: fmt.Sprintf("references non-existent nested schema '%s'", ref.ID),
				})
			}
		}

		// Check array of schema references (for unions)
		if refs, ok := field.GetSchemaReferences(); ok {
			for j, ref := range refs {
				schemaID := string(ref.ID)
				if !s.HasNestedSchema(schemaID) {
					errors = append(errors, common.Issue{
						Path:    fmt.Sprintf("/fields/%s/schema/%d", id, j),
						Message: fmt.Sprintf("references non-existent nested schema '%s'", ref.ID),
					})
				}
			}
		}
	}

	return errors
}

// ============================================================================
// ORPHANED REFERENCE DETECTION
// ============================================================================

// GetOrphanedIndexes returns indexes that reference non-existent fields
func (s *SchemaDefinition) GetOrphanedIndexes() []string {
	orphaned := []string{}

	if s.Indexes == nil {
		return orphaned
	}

	for _, ior := range s.Indexes {
		if !ior.IsIndex() {
			continue
		}

		index := ior.Index
		for _, fieldName := range index.Fields {
			if !s.HasFieldWithName(fieldName) {
				orphaned = append(orphaned, index.Name)
				break
			}
		}
	}

	return orphaned
}

// GetOrphanedConstraints returns constraints that reference non-existent fields
func (s *SchemaDefinition) GetOrphanedConstraints() []string {
	orphaned := []string{}

	if s.Constraints == nil {
		return orphaned
	}

	collectOrphanedConstraints(s.Constraints, s, &orphaned)
	return orphaned
}

// collectOrphanedConstraints recursively collects orphaned constraints
func collectOrphanedConstraints(rules SchemaConstraint, schema *SchemaDefinition, orphaned *[]string) {
	for i := range rules {
		rule := &rules[i]

		fields := rule.GetReferencedFields()
		hasOrphaned := false
		for _, fieldName := range fields {
			if !schema.HasFieldWithName(fieldName) {
				hasOrphaned = true
				break
			}
		}

		if hasOrphaned {
			*orphaned = append(*orphaned, rule.GetName())
		}

		if rule.IsConstraintGroup() {
			collectOrphanedConstraints(rule.ConstraintGroup.Rules, schema, orphaned)
		}
	}
}

// ============================================================================
// VALIDATION OPERATIONS
// ============================================================================

// validateConstraintRulesReferences recursively validates constraint field references
func validateConstraintRulesReferences(rules SchemaConstraint, schema *SchemaDefinition, basePath string, errors *[]common.Issue) {
	for i := range rules {
		currentPath := fmt.Sprintf("%s/%d", basePath, i)
		rule := &rules[i]

		fields := rule.GetReferencedFields()
		for _, fieldName := range fields {
			if !schema.HasFieldWithName(fieldName) {
				*errors = append(*errors, common.Issue{
					Path:    currentPath,
					Message: fmt.Sprintf("references non-existent field '%s'", fieldName),
				})
			}
		}

		if rule.IsConstraintGroup() {
			nestedPath := fmt.Sprintf("%s/rules", currentPath)
			validateConstraintRulesReferences(rule.ConstraintGroup.Rules, schema, nestedPath, errors)
		}
	}
}

// ============================================================================
// CLEANUP OPERATIONS
// ============================================================================

// WithOrphanedIndexesRemoved returns a new schema with all orphaned indexes removed
func (s *SchemaDefinition) WithOrphanedIndexesRemoved() (*SchemaDefinition, *IndexRemovalResult, error) {
	orphaned := s.GetOrphanedIndexes()
	if len(orphaned) == 0 {
		return s, &IndexRemovalResult{RemovedNames: []string{}, Count: 0}, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, nil, err
	}

	newIndexes := []IndexOrReference{}
	for _, ior := range clone.Indexes {
		if !ior.IsIndex() {
			newIndexes = append(newIndexes, ior)
			continue
		}

		if !slices.Contains(orphaned, ior.Index.Name) {
			newIndexes = append(newIndexes, ior)
		}
	}

	clone.Indexes = newIndexes

	result := &IndexRemovalResult{
		RemovedNames: orphaned,
		Count:        len(orphaned),
	}

	return clone, result, nil
}

// WithOrphanedConstraintsRemoved returns a new schema with all orphaned constraints removed
func (s *SchemaDefinition) WithOrphanedConstraintsRemoved() (*SchemaDefinition, *ConstraintRemovalResult, error) {
	orphaned := s.GetOrphanedConstraints()
	if len(orphaned) == 0 {
		return s, &ConstraintRemovalResult{RemovedPaths: []string{}, RemovedNames: []string{}, Count: 0}, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, nil, err
	}

	removedNames := []string{}
	removedPaths := []string{}
	clone.Constraints = removeOrphanedConstraintRules(clone.Constraints, orphaned, &removedNames, &removedPaths)

	result := &ConstraintRemovalResult{
		RemovedPaths: removedPaths,
		RemovedNames: removedNames,
		Count:        len(removedNames),
	}

	return clone, result, nil
}

// removeOrphanedConstraintRules recursively removes orphaned constraints
func removeOrphanedConstraintRules(rules SchemaConstraint, orphaned []string, removedNames *[]string, removedPaths *[]string) SchemaConstraint {
	result := SchemaConstraint{}

	for i := range rules {
		ruleName := rules[i].GetName()

		if slices.Contains(orphaned, ruleName) {
			*removedNames = append(*removedNames, ruleName)
			continue
		}

		if rules[i].IsConstraintGroup() {
			filteredRules := removeOrphanedConstraintRules(rules[i].ConstraintGroup.Rules, orphaned, removedNames, removedPaths)
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

// WithOrphanedNestedSchemasRemoved returns a new schema with all orphaned nested schemas removed
func (s *SchemaDefinition) WithOrphanedNestedSchemasRemoved() (*SchemaDefinition, *NestedSchemaRemovalResult, error) {
	orphaned := s.FindOrphanedNestedSchemas()
	if len(orphaned) == 0 {
		return s, &NestedSchemaRemovalResult{RemovedIDs: []string{}, Count: 0}, nil
	}

	clone, err := s.DeepClone()
	if err != nil {
		return nil, nil, err
	}

	for _, id := range orphaned {
		delete(clone.NestedSchemas, id)
	}

	result := &NestedSchemaRemovalResult{
		RemovedIDs: orphaned,
		Count:      len(orphaned),
	}

	return clone, result, nil
}

// WithAllOrphansRemoved returns a new schema with all orphaned references removed


func (s *SchemaDefinition) WithAllOrphansRemoved() (*SchemaDefinition, *CleanupResult, error) {
	result := &CleanupResult{}

	// Remove orphaned indexes
	clone, indexResult, err := s.WithOrphanedIndexesRemoved()
	if err != nil {
		return nil, nil, err
	}
	result.Indexes = indexResult

	// Remove orphaned constraints
	clone, constraintResult, err := clone.WithOrphanedConstraintsRemoved()
	if err != nil {
		return nil, nil, err
	}
	result.Constraints = constraintResult

	// Remove orphaned nested schemas
	clone, nestedSchemaResult, err := clone.WithOrphanedNestedSchemasRemoved()
	if err != nil {
		return nil, nil, err
	}
	result.NestedSchemas = nestedSchemaResult

	return clone, result, nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================
