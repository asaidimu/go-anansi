package schema

// ============================================================================
// TRAVERSAL OPERATIONS
// ============================================================================

// ForEachField iterates over all fields
func (s *SchemaDefinition) ForEachField(fn FieldVisitor) error {
	if s.Fields == nil {
		return nil
	}

	for id, field := range s.Fields {
		if err := fn(id, field); err != nil {
			return err
		}
	}

	return nil
}

// ForEachNestedSchema iterates over all nested schemas
func (s *SchemaDefinition) ForEachNestedSchema(fn NestedSchemaVisitor) error {
	if s.NestedSchemas == nil {
		return nil
	}

	for id, schema := range s.NestedSchemas {
		if err := fn(id, schema); err != nil {
			return err
		}
	}

	return nil
}

// ForEachIndex iterates over all indexes
func (s *SchemaDefinition) ForEachIndex(fn IndexVisitor) error {
	if s.Indexes == nil {
		return nil
	}

	for idx, ior := range s.Indexes {
		if ior.IsIndex() {
			if err := fn(idx, ior.Index); err != nil {
				return err
			}
		}
	}

	return nil
}

// ForEachConstraint iterates over all top-level constraints
func (s *SchemaDefinition) ForEachConstraint(fn ConstraintVisitor) error {
	if s.Constraints == nil {
		return nil
	}

	for idx := range s.Constraints {
		if err := fn(idx, &s.Constraints[idx]); err != nil {
			return err
		}
	}

	return nil
}

// WalkConstraints recursively walks all constraints including nested groups
func (s *SchemaDefinition) WalkConstraints(visitor ConstraintWalker) error {
	if s.Constraints == nil {
		return nil
	}

	return walkConstraintRules(s.Constraints, visitor, 0)
}

// walkConstraintRules recursively walks constraint rules
func walkConstraintRules(rules SchemaConstraint, visitor ConstraintWalker, depth int) error {
	for i := range rules {
		if err := visitor(&rules[i], depth); err != nil {
			return err
		}

		if rules[i].IsConstraintGroup() {
			if err := walkConstraintRules(rules[i].ConstraintGroup.Rules, visitor, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// ============================================================================
// FILTERING OPERATIONS
// ============================================================================

// FilterFields returns fields matching the predicate
func (s *SchemaDefinition) FilterFields(predicate FieldPredicate) map[string]*FieldDefinition {
	result := make(map[string]*FieldDefinition)

	if s.Fields == nil {
		return result
	}

	for id, field := range s.Fields {
		if predicate(id, field) {
			result[id] = field
		}
	}

	return result
}

// FilterIndexes returns indexes matching the predicate
func (s *SchemaDefinition) FilterIndexes(predicate IndexPredicate) []*IndexDefinition {
	result := []*IndexDefinition{}

	if s.Indexes == nil {
		return result
	}

	for _, ior := range s.Indexes {
		if ior.IsIndex() && predicate(ior.Index) {
			result = append(result, ior.Index)
		}
	}

	return result
}

// FilterConstraints returns constraints matching the predicate
func (s *SchemaDefinition) FilterConstraints(predicate ConstraintPredicate) []ConstraintRule {
	result := []ConstraintRule{}

	if s.Constraints == nil {
		return result
	}

	filterConstraintRules(s.Constraints, predicate, &result)
	return result
}

// filterConstraintRules recursively filters constraints
func filterConstraintRules(rules SchemaConstraint, predicate ConstraintPredicate, result *[]ConstraintRule) {
	for i := range rules {
		if predicate(&rules[i]) {
			*result = append(*result, rules[i])
		}

		if rules[i].IsConstraintGroup() {
			filterConstraintRules(rules[i].ConstraintGroup.Rules, predicate, result)
		}
	}
}

// GetRequiredFields returns all required fields
func (s *SchemaDefinition) GetRequiredFields() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.IsRequired()
	})
}

// GetOptionalFields returns all optional fields
func (s *SchemaDefinition) GetOptionalFields() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return !field.IsRequired()
	})
}

// GetUniqueFields returns all unique fields
func (s *SchemaDefinition) GetUniqueFields() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.IsUnique()
	})
}

// GetDeprecatedFields returns all deprecated fields
func (s *SchemaDefinition) GetDeprecatedFields() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.IsDeprecated()
	})
}

// GetFieldsByType returns all fields of the specified type
func (s *SchemaDefinition) GetFieldsByType(fieldType FieldType) map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.Type == fieldType
	})
}

// GetComplexFields returns all fields with complex types (object, array, union, etc.)
func (s *SchemaDefinition) GetComplexFields() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.Type.IsComplex()
	})
}

// GetFieldsWithDefaults returns all fields that have default values
func (s *SchemaDefinition) GetFieldsWithDefaults() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.HasDefault()
	})
}

// GetFieldsWithConstraints returns all fields that have constraints
func (s *SchemaDefinition) GetFieldsWithConstraints() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.ConstraintCount() > 0
	})
}

// GetFieldsWithSchemaReferences returns all fields that reference nested schemas
func (s *SchemaDefinition) GetFieldsWithSchemaReferences() map[string]*FieldDefinition {
	return s.FilterFields(func(id string, field *FieldDefinition) bool {
		return field.HasSchemaReference()
	})
}

// ============================================================================
// MAPPING OPERATIONS
// ============================================================================

// MapFields transforms fields using the mapper function and returns a new schema
func (s *SchemaDefinition) MapFields(mapper FieldMapper) *SchemaDefinition {
	clone, _ := s.DeepClone()

	if clone.Fields == nil {
		return clone
	}

	for id, field := range clone.Fields {
		clone.Fields[id] = mapper(id, field)
	}

	return clone
}

// ============================================================================
// CONVERSION OPERATIONS
// ============================================================================

// ToFieldSlice returns all fields as a slice
func (s *SchemaDefinition) ToFieldSlice() []*FieldDefinition {
	if s.Fields == nil {
		return []*FieldDefinition{}
	}

	result := make([]*FieldDefinition, 0, len(s.Fields))
	for _, field := range s.Fields {
		result = append(result, field)
	}

	return result
}

// ToIndexSlice returns all indexes as a slice
func (s *SchemaDefinition) ToIndexSlice() []*IndexDefinition {
	if s.Indexes == nil {
		return []*IndexDefinition{}
	}

	result := make([]*IndexDefinition, 0, len(s.Indexes))
	for _, ior := range s.Indexes {
		if ior.IsIndex() {
			result = append(result, ior.Index)
		}
	}

	return result
}

// ToConstraintSlice returns all constraints as a slice
func (s *SchemaDefinition) ToConstraintSlice() []ConstraintRule {
	if s.Constraints == nil {
		return []ConstraintRule{}
	}

	result := make([]ConstraintRule, len(s.Constraints))
	copy(result, s.Constraints)
	return result
}

// ToNestedSchemaSlice returns all nested schemas as a slice
func (s *SchemaDefinition) ToNestedSchemaSlice() []*NestedSchemaDefinition {
	if s.NestedSchemas == nil {
		return []*NestedSchemaDefinition{}
	}

	result := make([]*NestedSchemaDefinition, 0, len(s.NestedSchemas))
	for _, schema := range s.NestedSchemas {
		result = append(result, schema)
	}

	return result
}
