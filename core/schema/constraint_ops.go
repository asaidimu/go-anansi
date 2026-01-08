package schema

import (
	"slices"
	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// IsConstraint returns true if this rule is a Constraint.
func (cr *ConstraintRule) IsConstraint() bool {
	return cr.Constraint != nil
}

// IsConstraintGroup returns true if this rule is a ConstraintGroup.
func (cr *ConstraintRule) IsConstraintGroup() bool {
	return cr.ConstraintGroup != nil
}

// IsReference returns true if this rule is a ResourceReference.
func (cr *ConstraintRule) IsReference() bool {
	return cr.Reference != nil
}

// IsConstraint returns true if this is a Constraint.
func (cog *ConstraintOrGroup) IsConstraint() bool {
	return cog.Constraint != nil
}

// IsConstraintGroup returns true if this is a ConstraintGroup.
func (cog *ConstraintOrGroup) IsConstraintGroup() bool {
	return cog.ConstraintGroup != nil
}

// ============================================================================
// CONSTRAINT RULE QUERY OPERATIONS
// ============================================================================

// GetName returns the name of this constraint or group
func (cr *ConstraintRule) GetName() string {
	if cr.IsConstraint() {
		return cr.Constraint.Name
	}
	if cr.IsConstraintGroup() {
		return cr.ConstraintGroup.Name
	}
	return ""
}

// ReferencesField returns true if this constraint references the given field
func (cr *ConstraintRule) ReferencesField(fieldName string) bool {
	if cr.IsConstraint() {
		if cr.Constraint.Field != nil && *cr.Constraint.Field == fieldName {
			return true
		}
		return slices.Contains(cr.Constraint.Fields, fieldName)
	}

	if cr.IsConstraintGroup() {
		for i := range cr.ConstraintGroup.Rules {
			if cr.ConstraintGroup.Rules[i].ReferencesField(fieldName) {
				return true
			}
		}
	}

	return false
}

// ReferencesAnyField returns true if this constraint references any of the given fields
func (cr *ConstraintRule) ReferencesAnyField(fieldNames []string) bool {
	return slices.ContainsFunc(fieldNames, cr.ReferencesField)
}

// GetReferencedFields returns all fields referenced by this constraint
func (cr *ConstraintRule) GetReferencedFields() []string {
	fields := []string{}

	if cr.IsConstraint() {
		if cr.Constraint.Field != nil {
			fields = append(fields, *cr.Constraint.Field)
		}
		fields = append(fields, cr.Constraint.Fields...)
	}

	if cr.IsConstraintGroup() {
		for i := range cr.ConstraintGroup.Rules {
			fields = append(fields, cr.ConstraintGroup.Rules[i].GetReferencedFields()...)
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := []string{}
	for _, field := range fields {
		if !seen[field] {
			seen[field] = true
			unique = append(unique, field)
		}
	}

	return unique
}

// GetRules returns the rules if this is a group (nil otherwise)
func (cr *ConstraintRule) GetRules() []ConstraintRule {
	if cr.IsConstraintGroup() {
		return cr.ConstraintGroup.Rules
	}
	return nil
}

// GetOperator returns the operator if this is a group
func (cr *ConstraintRule) GetOperator() (common.LogicalOperator, bool) {
	if cr.IsConstraintGroup() {
		return cr.ConstraintGroup.Operator, true
	}
	return "", false
}

// RuleCount returns the number of rules if this is a group (0 otherwise)
func (cr *ConstraintRule) RuleCount() int {
	if cr.IsConstraintGroup() {
		return len(cr.ConstraintGroup.Rules)
	}
	return 0
}

// FindRule recursively finds a rule by name within this constraint/group
func (cr *ConstraintRule) FindRule(name string) (*ConstraintRule, bool) {
	if cr.GetName() == name {
		return cr, true
	}

	if cr.IsConstraintGroup() {
		for i := range cr.ConstraintGroup.Rules {
			if found, ok := cr.ConstraintGroup.Rules[i].FindRule(name); ok {
				return found, true
			}
		}
	}

	return nil, false
}

// GetPredicate returns the predicate if this is a constraint
func (cr *ConstraintRule) GetPredicate() (string, bool) {
	if cr.IsConstraint() {
		return cr.Constraint.Predicate, true
	}
	return "", false
}

// GetParameters returns the parameters if this is a constraint
func (cr *ConstraintRule) GetParameters() (any, bool) {
	if cr.IsConstraint() {
		return cr.Constraint.Parameters, true
	}
	return nil, false
}

// GetErrorMessage returns the error message if set
func (cr *ConstraintRule) GetErrorMessage() (string, bool) {
	if cr.IsConstraint() && cr.Constraint.ErrorMessage != nil {
		return *cr.Constraint.ErrorMessage, true
	}
	return "", false
}

// ============================================================================
// CONSTRAINT RULE VALIDATION
// ============================================================================

// ValidateFieldReferences checks if all referenced fields exist in the schema
func (cr *ConstraintRule) ValidateFieldReferences(schema *SchemaDefinition) error {
	fields := cr.GetReferencedFields()
	for _, fieldName := range fields {
		if !schema.HasFieldWithName(fieldName) {
			return NewFieldNameNotFoundError(fieldName)
		}
	}
	return nil
}

// ValidateStructure checks internal consistency of the constraint rule
func (cr *ConstraintRule) ValidateStructure() error {
	if cr.IsConstraint() {
		if cr.Constraint.Name == "" {
			return common.NewSystemError("ERR_INVALID_CONSTRAINT").
				WithMessage("constraint must have a name")
		}
		if cr.Constraint.Predicate == "" {
			return common.NewSystemError("ERR_INVALID_CONSTRAINT").
				WithMessage("constraint must have a predicate")
		}
	}

	if cr.IsConstraintGroup() {
		if cr.ConstraintGroup.Name == "" {
			return common.NewSystemError("ERR_INVALID_CONSTRAINT_GROUP").
				WithMessage("constraint group must have a name")
		}
		if len(cr.ConstraintGroup.Rules) == 0 {
			return common.NewSystemError("ERR_INVALID_CONSTRAINT_GROUP").
				WithMessage("constraint group must have at least one rule")
		}
		// Validate nested rules
		for i := range cr.ConstraintGroup.Rules {
			if err := cr.ConstraintGroup.Rules[i].ValidateStructure(); err != nil {
				return err
			}
		}
	}

	return nil
}

// ============================================================================
// CONSTRAINT RULE TRAVERSAL
// ============================================================================

// ForEachRule recursively iterates over all rules in this constraint/group
func (cr *ConstraintRule) ForEachRule(visitor func(rule *ConstraintRule, depth int) error) error {
	return cr.forEachRuleWithDepth(visitor, 0)
}

func (cr *ConstraintRule) forEachRuleWithDepth(visitor func(rule *ConstraintRule, depth int) error, depth int) error {
	if err := visitor(cr, depth); err != nil {
		return err
	}

	if cr.IsConstraintGroup() {
		for i := range cr.ConstraintGroup.Rules {
			if err := cr.ConstraintGroup.Rules[i].forEachRuleWithDepth(visitor, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// ============================================================================
// CONSTRAINT RULE IMMUTABLE OPERATIONS (for groups)
// ============================================================================

// WithRule returns a new constraint group with the rule added
func (cr *ConstraintRule) WithRule(rule ConstraintRule) (*ConstraintRule, error) {
	if !cr.IsConstraintGroup() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT_GROUP").
			WithMessage("can only add rules to constraint groups")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.ConstraintGroup.Rules = append(clone.ConstraintGroup.Rules, rule)
	return clone, nil
}

// WithoutRule returns a new constraint group with the rule removed
func (cr *ConstraintRule) WithoutRule(name string) (*ConstraintRule, error) {
	if !cr.IsConstraintGroup() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT_GROUP").
			WithMessage("can only remove rules from constraint groups")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	newRules := []ConstraintRule{}
	for i := range clone.ConstraintGroup.Rules {
		if clone.ConstraintGroup.Rules[i].GetName() != name {
			newRules = append(newRules, clone.ConstraintGroup.Rules[i])
		}
	}

	clone.ConstraintGroup.Rules = newRules
	return clone, nil
}

// WithOperator returns a new constraint group with operator changed
func (cr *ConstraintRule) WithOperator(op common.LogicalOperator) (*ConstraintRule, error) {
	if !cr.IsConstraintGroup() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT_GROUP").
			WithMessage("can only set operator on constraint groups")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.ConstraintGroup.Operator = op
	return clone, nil
}

// ============================================================================
// CONSTRAINT RULE IMMUTABLE OPERATIONS (for constraints)
// ============================================================================

// WithPredicate returns a new constraint with predicate changed
func (cr *ConstraintRule) WithPredicate(predicate string) (*ConstraintRule, error) {
	if !cr.IsConstraint() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT").
			WithMessage("can only set predicate on constraints")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraint.Predicate = predicate
	return clone, nil
}

// WithParameters returns a new constraint with parameters changed
func (cr *ConstraintRule) WithParameters(params any) (*ConstraintRule, error) {
	if !cr.IsConstraint() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT").
			WithMessage("can only set parameters on constraints")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraint.Parameters = params
	return clone, nil
}

// WithErrorMessage returns a new constraint with error message changed
func (cr *ConstraintRule) WithErrorMessage(message string) (*ConstraintRule, error) {
	if !cr.IsConstraint() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT").
			WithMessage("can only set error message on constraints")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraint.ErrorMessage = &message
	return clone, nil
}

// WithField returns a new constraint with field changed
func (cr *ConstraintRule) WithField(fieldName string) (*ConstraintRule, error) {
	if !cr.IsConstraint() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT").
			WithMessage("can only set field on constraints")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraint.Field = &fieldName
	return clone, nil
}

// WithFields returns a new constraint with fields changed
func (cr *ConstraintRule) WithFields(fieldNames []string) (*ConstraintRule, error) {
	if !cr.IsConstraint() {
		return nil, common.NewSystemError("ERR_NOT_CONSTRAINT").
			WithMessage("can only set fields on constraints")
	}

	clone, err := cr.DeepClone()
	if err != nil {
		return nil, err
	}

	clone.Constraint.Fields = fieldNames
	return clone, nil
}

// ============================================================================
// CONSTRAINT RULE CLONING
// ============================================================================

// DeepClone returns a deep copy of the constraint rule
func (cr *ConstraintRule) DeepClone() (*ConstraintRule, error) {
	var clone ConstraintRule
	if err := utils.Clone(*cr, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

// ============================================================================
// CONSTRAINT RULE PARTIAL CHANGES
// ============================================================================

// ApplyPartialChanges applies partial changes to the constraint rule (mutates)
func (cr *ConstraintRule) ApplyPartialChanges(partial *PartialConstraint) error {
	if partial == nil {
		return nil
	}

	if cr.IsConstraint() {
		return applyPartialConstraintChanges(cr.Constraint, partial)
	}

	if cr.IsConstraintGroup() {
		if partial.Operator != nil {
			cr.ConstraintGroup.Operator = *partial.Operator
		}
		if partial.Name != nil {
			cr.ConstraintGroup.Name = *partial.Name
		}
	}

	return nil
}

// applyPartialConstraintChanges applies partial changes to a constraint
func applyPartialConstraintChanges(constraint *Constraint, partial *PartialConstraint) error {
	if partial.Predicate != nil {
		constraint.Predicate = *partial.Predicate
	}
	if partial.Field != nil {
		constraint.Field = partial.Field
	}
	if partial.Fields != nil {
		constraint.Fields = partial.Fields
	}
	if partial.Parameters != nil {
		constraint.Parameters = partial.Parameters
	}
	if partial.Description != nil {
		constraint.Description = partial.Description
	}
	if partial.ErrorMessage != nil {
		constraint.ErrorMessage = partial.ErrorMessage
	}
	if partial.Name != nil {
		constraint.Name = *partial.Name
	}

	// Handle unset operations
	for _, unsetField := range partial.Unset {
		switch unsetField {
		case "field":
			constraint.Field = nil
		case "fields":
			constraint.Fields = nil
		case "parameters":
			constraint.Parameters = nil
		case "description":
			constraint.Description = nil
		case "errorMessage":
			constraint.ErrorMessage = nil
		}
	}

	return nil
}

