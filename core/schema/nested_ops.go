package schema

import (
	"github.com/asaidimu/go-anansi/v6/core/utils"
)


// IsMap returns true if fields are represented as a map.
func (nsf *NestedSchemaFields) IsMap() bool {
	return nsf.FieldsMap != nil
}

// IsArray returns true if fields are represented as a conditional array.
func (nsf *NestedSchemaFields) IsArray() bool {
	return nsf.FieldsArray != nil
}

// IsStructured returns true if this is a structured schema (has fields).
func (nsd *NestedSchemaDefinition) IsStructured() bool {
	return nsd.Fields != nil
}

// IsTyped returns true if this is a typed schema (has type).
func (nsd *NestedSchemaDefinition) IsTyped() bool {
	return nsd.Type != nil
}
// ============================================================================
// NESTED SCHEMA DEFINITION QUERY OPERATIONS
// ============================================================================

// GetField returns the field with the given name (if structured)
func (nsd *NestedSchemaDefinition) GetField(name string) (*FieldDefinition, bool) {
	if !nsd.IsStructured() {
		return nil, false
	}

	if nsd.Fields.IsMap() {
		field, ok := nsd.Fields.FieldsMap[name]
		return field, ok
	}

	if nsd.Fields.IsArray() {
		for _, conditionalSet := range nsd.Fields.FieldsArray {
			if field, ok := conditionalSet.Fields[name]; ok {
				return field, true
			}
		}
	}

	return nil, false
}

// HasField returns true if a field with the given name exists
func (nsd *NestedSchemaDefinition) HasField(name string) bool {
	_, ok := nsd.GetField(name)
	return ok
}

// FieldNames returns all field names
func (nsd *NestedSchemaDefinition) FieldNames() []string {
	if !nsd.IsStructured() {
		return []string{}
	}

	names := []string{}

	if nsd.Fields.IsMap() {
		for name := range nsd.Fields.FieldsMap {
			names = append(names, name)
		}
	}

	if nsd.Fields.IsArray() {
		seen := make(map[string]bool)
		for _, conditionalSet := range nsd.Fields.FieldsArray {
			for name := range conditionalSet.Fields {
				if !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}

	return names
}

// FieldCount returns the number of fields
func (nsd *NestedSchemaDefinition) FieldCount() int {
	return len(nsd.FieldNames())
}

// GetConditionalFieldSets returns conditional field sets (if array-based)
func (nsd *NestedSchemaDefinition) GetConditionalFieldSets() []ConditionalFieldSet {
	if !nsd.IsStructured() || !nsd.Fields.IsArray() {
		return []ConditionalFieldSet{}
	}

	return nsd.Fields.FieldsArray
}

// FindFieldsForCondition returns fields that apply for the given data
func (nsd *NestedSchemaDefinition) FindFieldsForCondition(data map[string]any) map[string]*FieldDefinition {
	result := make(map[string]*FieldDefinition)

	if !nsd.IsStructured() {
		return result
	}

	if nsd.Fields.IsMap() {
		// All fields apply for map-based schemas
		for name, field := range nsd.Fields.FieldsMap {
			result[name] = field
		}
		return result
	}

	if nsd.Fields.IsArray() {
		// Check each conditional set
		for _, conditionalSet := range nsd.Fields.FieldsArray {
			if conditionalSet.When == nil || conditionalSet.When.Evaluate(data) {
				for name, field := range conditionalSet.Fields {
					result[name] = field
				}
			}
		}
	}

	return result
}

// GetIndex returns the index with the given name
func (nsd *NestedSchemaDefinition) GetIndex(name string) (*IndexDefinition, int, bool) {
	if nsd.Indexes == nil {
		return nil, -1, false
	}

	for i, ior := range nsd.Indexes {
		if ior.IsIndex() && ior.Index.Name == name {
			return ior.Index, i, true
		}
	}

	return nil, -1, false
}

// HasIndex returns true if an index with the given name exists
func (nsd *NestedSchemaDefinition) HasIndex(name string) bool {
	_, _, ok := nsd.GetIndex(name)
	return ok
}

// GetConstraint returns the constraint with the given name
func (nsd *NestedSchemaDefinition) GetConstraint(name string) (*ConstraintRule, *ConstraintLocation, bool) {
	if nsd.Constraints == nil {
		return nil, nil, false
	}

	return findConstraintInRules(nsd.Constraints, name, "/constraints", 0)
}

// HasConstraint returns true if a constraint with the given name exists
func (nsd *NestedSchemaDefinition) HasConstraint(name string) bool {
	_, _, ok := nsd.GetConstraint(name)
	return ok
}

// DeepClone returns a deep copy of the nested schema definition
func (nsd *NestedSchemaDefinition) DeepClone() (*NestedSchemaDefinition, error) {
	var clone NestedSchemaDefinition
	if err := utils.Clone(*nsd, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}
