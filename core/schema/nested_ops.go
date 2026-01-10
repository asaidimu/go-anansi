package schema

import (
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// IsMap returns true if fields are represented as a map.
func (nsf *NestedSchemaFields) IsMap() bool {
	return nsf.FieldsMap != nil
}

// IsLegacyFieldsArray returns true if fields are represented as a conditional array using the deprecated FieldsArray.
func (nsf *NestedSchemaFields) IsLegacyFieldsArray() bool {
	return nsf.FieldsArray != nil
}

// IsFieldSets returns true if fields are represented as a map of conditional field sets.
func (nsf *NestedSchemaFields) IsFieldSets() bool {
	return nsf.FieldSets != nil
}

// IsConditionalSets returns true if the NestedSchemaFields contains any conditional field sets (either as map or array).
func (nsf *NestedSchemaFields) IsConditionalSets() bool {
	return nsf.FieldSets != nil || nsf.FieldsArray != nil
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
		for _, fielDef := range nsd.Fields.FieldsMap {
			if fielDef.Name == name {
				return fielDef, true
			}
		}
		return nil, false
	}

	// Prefer FieldSets if present
	if nsd.Fields.IsFieldSets() {
		for _, conditionalSet := range nsd.Fields.FieldSets {
			for _, fielDef := range conditionalSet.Fields {
				if fielDef.Name == name {
					return fielDef, true
				}
			}
		}
	} else if nsd.Fields.IsLegacyFieldsArray() {
		for _, conditionalSet := range nsd.Fields.FieldsArray {
			for _, fielDef := range conditionalSet.Fields {
				if fielDef.Name == name {
					return fielDef, true
				}
			}
		}
	}

	return nil, false
}

// GetFieldByID retrieves a FieldDefinition by its ID from the nested schema's fields.
// This method correctly handles the different internal representations of NestedSchemaFields
// by performing a direct map lookup using the provided ID.
func (nsd *NestedSchemaDefinition) GetFieldByID(fieldID string) (*FieldDefinition, bool) {
	if nsd.Fields == nil {
		return nil, false
	}

	if nsd.Fields.FieldsMap != nil {
		if field, found := nsd.Fields.FieldsMap[fieldID]; found {
			return field, true
		}
	}

	// Iterate FieldSets (prefer FieldSets over FieldsArray as per deprecation)
	if nsd.Fields.FieldSets != nil {
		for _, conditionalSet := range nsd.Fields.FieldSets {
			if field, found := conditionalSet.Fields[fieldID]; found {
				return field, true
			}
		}
	}

	// Fallback to FieldsArray (deprecated)
	if nsd.Fields.FieldsArray != nil {
		for _, conditionalSet := range nsd.Fields.FieldsArray {
			if field, found := conditionalSet.Fields[fieldID]; found {
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
	seen := make(map[string]bool)

	if nsd.Fields.IsMap() {
		// FIX: Don't use the map key; use the Name property from the value
		for _, fielDef := range nsd.Fields.FieldsMap {
			if !seen[fielDef.Name] {
				seen[fielDef.Name] = true
				names = append(names, fielDef.Name)
			}
		}
	}

	if nsd.Fields.IsFieldSets() {
		for _, conditionalSet := range nsd.Fields.FieldSets {
			for _, fielDef := range conditionalSet.Fields {
				if !seen[fielDef.Name] {
					seen[fielDef.Name] = true
					names = append(names, fielDef.Name)
				}
			}
		}
	} else if nsd.Fields.IsLegacyFieldsArray() {
		for _, conditionalSet := range nsd.Fields.FieldsArray {
			for _, fielDef := range conditionalSet.Fields {
				if !seen[fielDef.Name] {
					seen[fielDef.Name] = true
					names = append(names, fielDef.Name)
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

// GetConditionalFieldSets returns conditional field sets as a slice, prioritizing FieldSets over FieldsArray.
func (nsd *NestedSchemaDefinition) GetConditionalFieldSets() []ConditionalFieldSet {
	if !nsd.IsStructured() {
		return []ConditionalFieldSet{}
	}

	if nsd.Fields.IsFieldSets() {
		sets := make([]ConditionalFieldSet, 0, len(nsd.Fields.FieldSets))
		for _, set := range nsd.Fields.FieldSets {
			sets = append(sets, set)
		}
		return sets
	} else if nsd.Fields.IsLegacyFieldsArray() {
		return nsd.Fields.FieldsArray
	}

	return []ConditionalFieldSet{}
}

// GetBaseFields returns a map of FieldID to FieldDefinition for all fields directly defined
// in nsd.Fields.FieldsMap. This excludes fields that are part of any conditional sets.
func (nsd *NestedSchemaDefinition) GetBaseFields() map[string]*FieldDefinition {
	if !nsd.IsStructured() || nsd.Fields.FieldsMap == nil {
		return map[string]*FieldDefinition{}
	}
	// Return a copy to prevent external modification
	baseFields := make(map[string]*FieldDefinition, len(nsd.Fields.FieldsMap))
	for id, field := range nsd.Fields.FieldsMap {
		baseFields[id] = field
	}
	return baseFields
}

// FindFieldsForCondition returns fields that apply for the given data
func (nsd *NestedSchemaDefinition) FindFieldsForCondition(data map[string]any) map[string]*FieldDefinition {
	result := make(map[string]*FieldDefinition)

	if !nsd.IsStructured() {
		return result
	}

	if nsd.Fields.IsMap() {
		// All fields apply for map-based schemas
		for key, field := range nsd.Fields.FieldsMap {
			result[key] = field
		}
		return result
	}

	// Prefer FieldSets if present
	if nsd.Fields.IsFieldSets() {
		// Check each conditional set in FieldSets
		for _, conditionalSet := range nsd.Fields.FieldSets {
			if conditionalSet.When == nil || conditionalSet.When.Evaluate(data) {
				for key, field := range conditionalSet.Fields {
					result[key] = field
				}
			}
		}
	} else if nsd.Fields.IsLegacyFieldsArray() { // Fallback to deprecated FieldsArray
		// Check each conditional set in FieldsArray
		for _, conditionalSet := range nsd.Fields.FieldsArray {
			if conditionalSet.When == nil || conditionalSet.When.Evaluate(data) {
				for key, field := range conditionalSet.Fields {
					result[key] = field
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
