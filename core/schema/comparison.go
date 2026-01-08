package schema

import (
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// ============================================================================
// SCHEMA COMPARISON
// ============================================================================

// Equals returns true if this schema is identical to the other schema
func (s *SchemaDefinition) Equals(other *SchemaDefinition) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}

	// Use google-cmp for deep equality comparison
	// Ignore the Mock function field as it's not comparable
	return cmp.Equal(s, other, 
		cmpopts.IgnoreUnexported(SchemaDefinition{}),
		cmp.Comparer(func(a, b func(faker any) (any, error)) bool {
			// Mock functions are not comparable
			return true
		}),
	)
}

// FieldsEqual returns true if fields are identical
func (s *SchemaDefinition) FieldsEqual(other *SchemaDefinition) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}

	return cmp.Equal(s.Fields, other.Fields, cmpopts.IgnoreUnexported(FieldDefinition{}))
}

// IndexesEqual returns true if indexes are identical
func (s *SchemaDefinition) IndexesEqual(other *SchemaDefinition) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}

	return cmp.Equal(s.Indexes, other.Indexes, cmpopts.IgnoreUnexported(IndexDefinition{}))
}

// ConstraintsEqual returns true if constraints are identical
func (s *SchemaDefinition) ConstraintsEqual(other *SchemaDefinition) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}

	return cmp.Equal(s.Constraints, other.Constraints, cmpopts.IgnoreUnexported(Constraint{}, ConstraintGroup{}))
}

// IsFieldIdentical checks if the field in schema exactly matches the provided field
func (s *SchemaDefinition) IsFieldIdentical(id string, field *FieldDefinition) bool {
	existingField, ok := s.GetField(id)
	if !ok {
		return false
	}

	return existingField.Equals(field)
}

// CompareField returns detailed comparison between schema field and provided field
func (s *SchemaDefinition) CompareField(id string, field *FieldDefinition) (*FieldComparison, error) {
	existingField, exists := s.GetField(id)

	comparison := &FieldComparison{
		Exists:      exists,
		Differences: []string{},
	}

	if !exists {
		comparison.Differences = append(comparison.Differences, "field does not exist in schema")
		return comparison, nil
	}

	// Check if fields are identical
	if existingField.Equals(field) {
		comparison.Identical = true
		return comparison, nil
	}

	// Build detailed differences
	if existingField.Name != field.Name {
		comparison.NameDifferent = true
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("name differs: '%s' vs '%s'", existingField.Name, field.Name))
	}

	if existingField.Type != field.Type {
		comparison.TypeDifferent = true
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("type differs: '%s' vs '%s'", existingField.Type, field.Type))
	}

	if !cmp.Equal(existingField.Required, field.Required) {
		comparison.RequiredDifferent = true
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("required differs: %v vs %v", existingField.Required, field.Required))
	}

	if !cmp.Equal(existingField.Unique, field.Unique) {
		comparison.UniqueDifferent = true
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("unique differs: %v vs %v", existingField.Unique, field.Unique))
	}

	if !cmp.Equal(existingField.Schema, field.Schema) {
		comparison.SchemaDifferent = true
		comparison.Differences = append(comparison.Differences, "schema reference differs")
	}

	if !cmp.Equal(existingField.Default, field.Default) {
		comparison.Differences = append(comparison.Differences, "default value differs")
	}

	if !cmp.Equal(existingField.Constraints, field.Constraints) {
		comparison.Differences = append(comparison.Differences, "constraints differ")
	}

	if !cmp.Equal(existingField.ItemsType, field.ItemsType) {
		comparison.Differences = append(comparison.Differences, "itemsType differs")
	}

	if !cmp.Equal(existingField.Values, field.Values) {
		comparison.Differences = append(comparison.Differences, "enum values differ")
	}

	return comparison, nil
}

// DiffFields returns fields that differ between schemas
func (s *SchemaDefinition) DiffFields(other *SchemaDefinition) *FieldDiff {
	diff := &FieldDiff{
		Added:    make(map[string]*FieldDefinition),
		Removed:  make(map[string]*FieldDefinition),
		Modified: make(map[string]*FieldComparison),
	}

	// Find added and modified fields
	for id, otherField := range other.Fields {
		existingField, exists := s.Fields[id]
		if !exists {
			diff.Added[id] = otherField
		} else if !existingField.Equals(otherField) {
			comparison, _ := s.CompareField(id, otherField)
			diff.Modified[id] = comparison
		}
	}

	// Find removed fields
	for id, field := range s.Fields {
		if _, exists := other.Fields[id]; !exists {
			diff.Removed[id] = field
		}
	}

	return diff
}

// DiffIndexes returns indexes that differ between schemas
func (s *SchemaDefinition) DiffIndexes(other *SchemaDefinition) *IndexDiff {
	diff := &IndexDiff{
		Added:    []*IndexDefinition{},
		Removed:  []*IndexDefinition{},
		Modified: make(map[string]*IndexDefinition),
	}

	// Build maps for easier comparison
	sIndexes := make(map[string]*IndexDefinition)
	for _, ior := range s.Indexes {
		if ior.IsIndex() {
			sIndexes[ior.Index.Name] = ior.Index
		}
	}

	otherIndexes := make(map[string]*IndexDefinition)
	for _, ior := range other.Indexes {
		if ior.IsIndex() {
			otherIndexes[ior.Index.Name] = ior.Index
		}
	}

	// Find added and modified indexes
	for name, otherIndex := range otherIndexes {
		existingIndex, exists := sIndexes[name]
		if !exists {
			diff.Added = append(diff.Added, otherIndex)
		} else if !cmp.Equal(existingIndex, otherIndex) {
			diff.Modified[name] = otherIndex
		}
	}

	// Find removed indexes
	for name, index := range sIndexes {
		if _, exists := otherIndexes[name]; !exists {
			diff.Removed = append(diff.Removed, index)
		}
	}

	return diff
}

// DiffConstraints returns constraints that differ between schemas
func (s *SchemaDefinition) DiffConstraints(other *SchemaDefinition) *ConstraintDiff {
	diff := &ConstraintDiff{
		Added:    []ConstraintRule{},
		Removed:  []ConstraintRule{},
		Modified: make(map[string]*ConstraintRule),
	}

	// Build maps for easier comparison
	sConstraints := make(map[string]*ConstraintRule)
	for i := range s.Constraints {
		name := getConstraintRuleName(&s.Constraints[i])
		if name != "" {
			sConstraints[name] = &s.Constraints[i]
		}
	}

	otherConstraints := make(map[string]*ConstraintRule)
	for i := range other.Constraints {
		name := getConstraintRuleName(&other.Constraints[i])
		if name != "" {
			otherConstraints[name] = &other.Constraints[i]
		}
	}

	// Find added and modified constraints
	for name, otherConstraint := range otherConstraints {
		existingConstraint, exists := sConstraints[name]
		if !exists {
			diff.Added = append(diff.Added, *otherConstraint)
		} else if !cmp.Equal(existingConstraint, otherConstraint) {
			diff.Modified[name] = otherConstraint
		}
	}

	// Find removed constraints
	for name, constraint := range sConstraints {
		if _, exists := otherConstraints[name]; !exists {
			diff.Removed = append(diff.Removed, *constraint)
		}
	}

	return diff
}

// ============================================================================
// FIELD DEFINITION COMPARISON
// ============================================================================

// Equals returns true if this field is identical to the other field
func (fd *FieldDefinition) Equals(other *FieldDefinition) bool {
	if fd == nil && other == nil {
		return true
	}
	if fd == nil || other == nil {
		return false
	}

	return cmp.Equal(fd, other, cmpopts.IgnoreUnexported(FieldDefinition{}))
}

// TypeEquals returns true if this field's type matches the other field's type
func (fd *FieldDefinition) TypeEquals(other *FieldDefinition) bool {
	if fd == nil && other == nil {
		return true
	}
	if fd == nil || other == nil {
		return false
	}

	return fd.Type == other.Type
}

// ConstraintsEqual returns true if this field's constraints match the other field's constraints
func (fd *FieldDefinition) ConstraintsEqual(other *FieldDefinition) bool {
	if fd == nil && other == nil {
		return true
	}
	if fd == nil || other == nil {
		return false
	}

	return cmp.Equal(fd.Constraints, other.Constraints, cmpopts.IgnoreUnexported(Constraint{}, ConstraintGroup{}))
}

// ============================================================================
// INDEX DEFINITION COMPARISON
// ============================================================================

// Equals returns true if this index is identical to the other index
func (id *IndexDefinition) Equals(other *IndexDefinition) bool {
	if id == nil && other == nil {
		return true
	}
	if id == nil || other == nil {
		return false
	}

	return cmp.Equal(id, other, cmpopts.IgnoreUnexported(IndexDefinition{}))
}

// ============================================================================
// CONSTRAINT RULE COMPARISON
// ============================================================================

// Equals returns true if this constraint rule is identical to the other constraint rule
func (cr *ConstraintRule) Equals(other *ConstraintRule) bool {
	if cr == nil && other == nil {
		return true
	}
	if cr == nil || other == nil {
		return false
	}

	return cmp.Equal(cr, other, cmpopts.IgnoreUnexported(Constraint{}, ConstraintGroup{}))
}

// ============================================================================
// NESTED SCHEMA DEFINITION COMPARISON
// ============================================================================

// Equals returns true if this nested schema is identical to the other nested schema
func (nsd *NestedSchemaDefinition) Equals(other *NestedSchemaDefinition) bool {
	if nsd == nil && other == nil {
		return true
	}
	if nsd == nil || other == nil {
		return false
	}

	return cmp.Equal(nsd, other, cmpopts.IgnoreUnexported(NestedSchemaDefinition{}))
}

// ============================================================================
// DIFF SUMMARY UTILITIES
// ============================================================================

// Summary returns a human-readable summary of field differences
func (fd *FieldDiff) Summary() string {
	parts := []string{}

	if len(fd.Added) > 0 {
		parts = append(parts, fmt.Sprintf("%d fields added", len(fd.Added)))
	}
	if len(fd.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("%d fields removed", len(fd.Removed)))
	}
	if len(fd.Modified) > 0 {
		parts = append(parts, fmt.Sprintf("%d fields modified", len(fd.Modified)))
	}

	if len(parts) == 0 {
		return "no field differences"
	}

	return strings.Join(parts, ", ")
}

// HasChanges returns true if there are any field differences
func (fd *FieldDiff) HasChanges() bool {
	return len(fd.Added) > 0 || len(fd.Removed) > 0 || len(fd.Modified) > 0
}

// Summary returns a human-readable summary of index differences
func (id *IndexDiff) Summary() string {
	parts := []string{}

	if len(id.Added) > 0 {
		parts = append(parts, fmt.Sprintf("%d indexes added", len(id.Added)))
	}
	if len(id.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("%d indexes removed", len(id.Removed)))
	}
	if len(id.Modified) > 0 {
		parts = append(parts, fmt.Sprintf("%d indexes modified", len(id.Modified)))
	}

	if len(parts) == 0 {
		return "no index differences"
	}

	return strings.Join(parts, ", ")
}

// HasChanges returns true if there are any index differences
func (id *IndexDiff) HasChanges() bool {
	return len(id.Added) > 0 || len(id.Removed) > 0 || len(id.Modified) > 0
}

// Summary returns a human-readable summary of constraint differences
func (cd *ConstraintDiff) Summary() string {
	parts := []string{}

	if len(cd.Added) > 0 {
		parts = append(parts, fmt.Sprintf("%d constraints added", len(cd.Added)))
	}
	if len(cd.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("%d constraints removed", len(cd.Removed)))
	}
	if len(cd.Modified) > 0 {
		parts = append(parts, fmt.Sprintf("%d constraints modified", len(cd.Modified)))
	}

	if len(parts) == 0 {
		return "no constraint differences"
	}

	return strings.Join(parts, ", ")
}

// HasChanges returns true if there are any constraint differences
func (cd *ConstraintDiff) HasChanges() bool {
	return len(cd.Added) > 0 || len(cd.Removed) > 0 || len(cd.Modified) > 0
}
