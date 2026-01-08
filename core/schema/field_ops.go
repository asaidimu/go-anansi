package schema

import (
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// ============================================================================
// FIELD DEFINITION QUERY OPERATIONS
// ============================================================================

// IsRequired returns true if the field is required
func (fd *FieldDefinition) IsRequired() bool {
	return fd.Required != nil && *fd.Required
}

// IsUnique returns true if the field is unique
func (fd *FieldDefinition) IsUnique() bool {
	return fd.Unique != nil && *fd.Unique
}

// IsDeprecated returns true if the field is deprecated
func (fd *FieldDefinition) IsDeprecated() bool {
	return fd.Deprecated != nil && *fd.Deprecated
}

// HasDefault returns true if the field has a default value
func (fd *FieldDefinition) HasDefault() bool {
	return fd.Default != nil
}

// HasSchemaReference returns true if the field references a nested schema
func (fd *FieldDefinition) HasSchemaReference() bool {
	return fd.Schema != nil
}

// GetSchemaReference returns the schema reference if the field type supports it
func (fd *FieldDefinition) GetSchemaReference() (*NestedSchemaReference, bool) {
	if fd.Schema == nil {
		return nil, false
	}

	// Try direct NestedSchemaReference
	if ref, ok := fd.Schema.(NestedSchemaReference); ok {
		return &ref, true
	}

	// Try pointer to NestedSchemaReference
	if ref, ok := fd.Schema.(*NestedSchemaReference); ok {
		return ref, true
	}

	return nil, false
}

// GetSchemaReferences returns schema references for union types
func (fd *FieldDefinition) GetSchemaReferences() ([]NestedSchemaReference, bool) {
	if fd.Schema == nil {
		return nil, false
	}

	// Try slice of NestedSchemaReference
	if refs, ok := fd.Schema.([]NestedSchemaReference); ok {
		return refs, true
	}

	// Try slice of pointers
	if refsPtr, ok := fd.Schema.([]*NestedSchemaReference); ok {
		refs := make([]NestedSchemaReference, 0, len(refsPtr))
		for _, refPtr := range refsPtr {
			if refPtr != nil {
				refs = append(refs, *refPtr)
			}
		}
		return refs, true
	}

	return nil, false
}

// GetConstraint returns the constraint with the given name
func (fd *FieldDefinition) GetConstraint(name string) (*ConstraintRule, int, bool) {
	if fd.Constraints == nil {
		return nil, -1, false
	}

	for i := range fd.Constraints {
		ruleName := getConstraintRuleName(&fd.Constraints[i])
		if ruleName == name {
			return &fd.Constraints[i], i, true
		}
	}

	return nil, -1, false
}

// HasConstraint returns true if the field has a constraint with the given name
func (fd *FieldDefinition) HasConstraint(name string) bool {
	_, _, ok := fd.GetConstraint(name)
	return ok
}

// ConstraintCount returns the number of constraints on the field
func (fd *FieldDefinition) ConstraintCount() int {
	if fd.Constraints == nil {
		return 0
	}
	return len(fd.Constraints)
}

// ============================================================================
// FIELD DEFINITION IMMUTABLE OPERATIONS
// ============================================================================

// WithRequired returns a new field with required flag set
func (fd *FieldDefinition) WithRequired(required bool) *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Required = &required
	return clone
}

// WithUnique returns a new field with unique flag set
func (fd *FieldDefinition) WithUnique(unique bool) *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Unique = &unique
	return clone
}

// WithDeprecated returns a new field with deprecated flag set
func (fd *FieldDefinition) WithDeprecated(deprecated bool) *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Deprecated = &deprecated
	return clone
}

// WithDefault returns a new field with default value set
func (fd *FieldDefinition) WithDefault(value any) *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Default = value
	return clone
}

// WithoutDefault returns a new field with default value removed
func (fd *FieldDefinition) WithoutDefault() *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Default = nil
	return clone
}

// WithDescription returns a new field with description set
func (fd *FieldDefinition) WithDescription(description string) *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Description = &description
	return clone
}

// WithoutDescription returns a new field with description removed
func (fd *FieldDefinition) WithoutDescription() *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Description = nil
	return clone
}

// WithType returns a new field with type changed
func (fd *FieldDefinition) WithType(fieldType FieldType) *FieldDefinition {
	clone, _ := fd.DeepClone()
	clone.Type = fieldType
	return clone
}

// WithConstraint returns a new field with constraint added
func (fd *FieldDefinition) WithConstraint(constraint ConstraintRule) (*FieldDefinition, error) {
	clone, err := fd.DeepClone()
	if err != nil {
		return nil, err
	}

	if clone.Constraints == nil {
		clone.Constraints = SchemaConstraint{}
	}

	clone.Constraints = append(clone.Constraints, constraint)
	return clone, nil
}

// WithoutConstraint returns a new field with constraint removed
func (fd *FieldDefinition) WithoutConstraint(name string) (*FieldDefinition, error) {
	clone, err := fd.DeepClone()
	if err != nil {
		return nil, err
	}

	if clone.Constraints == nil {
		return clone, nil
	}

	newConstraints := SchemaConstraint{}
	for i := range clone.Constraints {
		ruleName := getConstraintRuleName(&clone.Constraints[i])
		if ruleName != name {
			newConstraints = append(newConstraints, clone.Constraints[i])
		}
	}

	clone.Constraints = newConstraints
	return clone, nil
}

// ============================================================================
// FIELD DEFINITION CLONING
// ============================================================================

// DeepClone returns a deep copy of the field definition
func (fd *FieldDefinition) DeepClone() (*FieldDefinition, error) {
	var clone FieldDefinition
	if err := utils.Clone(*fd, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

// ============================================================================
// FIELD DEFINITION PARTIAL CHANGES
// ============================================================================

// ApplyPartialChanges applies partial changes to the field definition (mutates)
func (fd *FieldDefinition) ApplyPartialChanges(partial *PartialFieldDefinition) error {
	if partial == nil {
		return nil
	}

	// Apply non-nil changes
	if partial.Type != nil {
		fd.Type = *partial.Type
	}
	if partial.Required != nil {
		fd.Required = partial.Required
	}
	if partial.Unique != nil {
		fd.Unique = partial.Unique
	}
	if partial.Description != nil {
		fd.Description = partial.Description
	}
	if partial.Default != nil {
		fd.Default = partial.Default
	}
	if partial.ItemsType != nil {
		fd.ItemsType = partial.ItemsType
	}
	if partial.Values != nil {
		fd.Values = partial.Values
	}
	if partial.Schema != nil {
		fd.Schema = partial.Schema
	}
	if partial.Constraints != nil {
		fd.Constraints = partial.Constraints
	}
	if partial.Hint != nil {
		fd.Hint = partial.Hint
	}
	if partial.Deprecated != nil {
		fd.Deprecated = partial.Deprecated
	}
	if partial.Name != nil {
		fd.Name = *partial.Name
	}

	// Handle unset operations
	for _, unsetField := range partial.Unset {
		switch unsetField {
		case "required":
			fd.Required = nil
		case "unique":
			fd.Unique = nil
		case "description":
			fd.Description = nil
		case "default":
			fd.Default = nil
		case "itemsType":
			fd.ItemsType = nil
		case "values":
			fd.Values = nil
		case "schema":
			fd.Schema = nil
		case "constraints":
			fd.Constraints = nil
		case "hint":
			fd.Hint = nil
		case "deprecated":
			fd.Deprecated = nil
		}
	}

	return nil
}
