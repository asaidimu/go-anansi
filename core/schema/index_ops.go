package schema

import (
	"slices"

	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// IsIndex returns true if this is an IndexDefinition.
func (ior *IndexOrReference) IsIndex() bool {
	return ior.Index != nil
}

// IsReference returns true if this is a ResourceReference.
func (ior *IndexOrReference) IsReference() bool {
	return ior.Reference != nil
}

// ============================================================================
// INDEX DEFINITION QUERY OPERATIONS
// ============================================================================

// ReferencesField returns true if the index references the given field
func (id *IndexDefinition) ReferencesField(fieldName string) bool {
	return slices.Contains(id.Fields, fieldName)
}

// ReferencesAllFields returns true if the index references all given fields
func (id *IndexDefinition) ReferencesAllFields(fieldNames []string) bool {
	for _, fieldName := range fieldNames {
		if !slices.Contains(id.Fields, fieldName) {
			return false
		}
	}
	return true
}

// ReferencesAnyField returns true if the index references any of the given fields
func (id *IndexDefinition) ReferencesAnyField(fieldNames []string) bool {
	return containsAny(id.Fields, fieldNames)
}

// IsPrimary returns true if this is a primary key index
func (id *IndexDefinition) IsPrimary() bool {
	return id.Type == IndexTypePrimary
}

// IsUnique returns true if this is a unique index
func (id *IndexDefinition) IsUnique() bool {
	return id.Type == IndexTypeUnique || (id.Unique != nil && *id.Unique) || id.IsPrimary()
}

// IsPartial returns true if this is a partial index
func (id *IndexDefinition) IsPartial() bool {
	return id.Partial != nil
}

// IsSpatial returns true if this is a spatial index
func (id *IndexDefinition) IsSpatial() bool {
	return id.Type == IndexTypeSpatial
}

// IsFullText returns true if this is a full-text index
func (id *IndexDefinition) IsFullText() bool {
	return id.Type == IndexTypeFullText
}

// ============================================================================
// INDEX DEFINITION VALIDATION
// ============================================================================

// ValidateFieldReferences checks if all referenced fields exist in the schema
func (id *IndexDefinition) ValidateFieldReferences(schema *SchemaDefinition) error {
	for _, fieldName := range id.Fields {
		if !schema.HasFieldWithName(fieldName) {
			return NewFieldNameNotFoundError(fieldName)
		}
	}
	return nil
}

// ============================================================================
// INDEX DEFINITION IMMUTABLE OPERATIONS
// ============================================================================

// WithField returns a new index with the field added
func (id *IndexDefinition) WithField(fieldName string) *IndexDefinition {
	clone, _ := id.DeepClone()

	// Don't add if already present
	if !slices.Contains(clone.Fields, fieldName) {
		clone.Fields = append(clone.Fields, fieldName)
	}

	return clone
}

// WithoutField returns a new index with the field removed
func (id *IndexDefinition) WithoutField(fieldName string) *IndexDefinition {
	clone, _ := id.DeepClone()

	newFields := make([]string, 0, len(clone.Fields))
	for _, field := range clone.Fields {
		if field != fieldName {
			newFields = append(newFields, field)
		}
	}
	clone.Fields = newFields

	return clone
}

// WithUnique returns a new index with unique flag set
func (id *IndexDefinition) WithUnique(unique bool) *IndexDefinition {
	clone, _ := id.DeepClone()
	clone.Unique = &unique
	return clone
}

// WithPartialCondition returns a new index with partial condition set
func (id *IndexDefinition) WithPartialCondition(condition *PartialIndexCondition) *IndexDefinition {
	clone, _ := id.DeepClone()
	clone.Partial = condition
	return clone
}

// WithoutPartialCondition returns a new index with partial condition removed
func (id *IndexDefinition) WithoutPartialCondition() *IndexDefinition {
	clone, _ := id.DeepClone()
	clone.Partial = nil
	return clone
}

// WithDescription returns a new index with description set
func (id *IndexDefinition) WithDescription(description string) *IndexDefinition {
	clone, _ := id.DeepClone()
	clone.Description = &description
	return clone
}

// WithoutDescription returns a new index with description removed
func (id *IndexDefinition) WithoutDescription() *IndexDefinition {
	clone, _ := id.DeepClone()
	clone.Description = nil
	return clone
}

// WithType returns a new index with type changed
func (id *IndexDefinition) WithType(indexType IndexType) *IndexDefinition {
	clone, _ := id.DeepClone()
	clone.Type = indexType
	return clone
}

// ============================================================================
// INDEX DEFINITION CLONING
// ============================================================================

// DeepClone returns a deep copy of the index definition
func (id *IndexDefinition) DeepClone() (*IndexDefinition, error) {
	var clone IndexDefinition
	if err := utils.Clone(*id, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

// ============================================================================
// INDEX DEFINITION PARTIAL CHANGES
// ============================================================================

// ApplyPartialChanges applies partial changes to the index definition (mutates)
func (id *IndexDefinition) ApplyPartialChanges(partial *PartialIndexDefinition) error {
	if partial == nil {
		return nil
	}

	// Apply non-nil changes
	if partial.Type != nil {
		id.Type = *partial.Type
	}
	if partial.Fields != nil {
		id.Fields = partial.Fields
	}
	if partial.Unique != nil {
		id.Unique = partial.Unique
	}
	if partial.Description != nil {
		id.Description = partial.Description
	}
	if partial.Order != nil {
		id.Order = partial.Order
	}
	if partial.Partial != nil {
		id.Partial = partial.Partial
	}
	if partial.Name != nil {
		id.Name = *partial.Name
	}

	// Handle unset operations
	for _, unsetField := range partial.Unset {
		switch unsetField {
		case "fields":
			id.Fields = nil
		case "unique":
			id.Unique = nil
		case "description":
			id.Description = nil
		case "order":
			id.Order = nil
		case "partial":
			id.Partial = nil
		}
	}

	return nil
}
