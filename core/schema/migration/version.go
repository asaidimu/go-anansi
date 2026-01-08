package migration

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// VersioningUtil provides functionality for calculating schema version bumps.
type VersioningUtil struct{}

// NewVersioningUtil creates a new versioning utility.
func NewVersioningUtil() *VersioningUtil {
	return &VersioningUtil{}
}

// CalculateNextVersion calculates the next version number based on schema changes.
// It analyzes all changes and determines whether a major, minor, or patch bump is needed.
func (vu *VersioningUtil) CalculateNextVersion(currentVersion string, changes []schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if len(changes) == 0 {
		return "", common.NewSystemError("ERR_NO_CHANGES").WithMessage("no changes provided to calculate next version")
	}

	version, err := common.NewVersion(currentVersion)
	if err != nil {
		return "", common.SystemErrorFrom(err).WithOperation("CalculateNextVersion")
	}

	highestImpact, err := vu.calculateEffectiveVersion(changes, oldSchema)
	if err != nil {
		return "", common.SystemErrorFrom(err).WithOperation("CalculateNextVersion")
	}

	switch highestImpact {
	case "major":
		return fmt.Sprintf("%d.0.0", version.Major+1), nil
	case "minor":
		return fmt.Sprintf("%d.%d.0", version.Major, version.Minor+1), nil
	case "patch":
		return fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch+1), nil
	default:
		return currentVersion, nil
	}
}

// calculateEffectiveVersion determines the highest impact level from a list of schema changes.
func (vu *VersioningUtil) calculateEffectiveVersion(changes []schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if len(changes) == 0 {
		return "none", nil
	}

	highestImpact := "patch"

	for _, change := range changes {
		impact, err := vu.getChangeImpact(change, oldSchema)
		if err != nil {
			return "", err
		}

		if impact == "major" {
			return "major", nil // Early exit optimization
		}

		if impact == "minor" && highestImpact == "patch" {
			highestImpact = "minor"
		}
	}

	return highestImpact, nil
}

// getChangeImpact determines the impact level of a single schema change.
func (vu *VersioningUtil) getChangeImpact(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	switch change.Type {
	case schema.SchemaChangeTypeRemoveField:
		return vu.handleRemoveField(change, oldSchema)

	case schema.SchemaChangeTypeAddConstraint:
		return vu.handleAddConstraint(change)

	case schema.SchemaChangeTypeRemoveConstraint:
		return vu.handleRemoveConstraint(change)

	case schema.SchemaChangeTypeAddSchema:
		return vu.handleAddSchema(change)

	case schema.SchemaChangeTypeModifyProperty:
		return vu.handleModifyProperty(change)

	case schema.SchemaChangeTypeAddField:
		return vu.handleAddField(change)

	case schema.SchemaChangeTypeAddIndex:
		return vu.handleAddIndex(change)

	case schema.SchemaChangeTypeRemoveIndex:
		return vu.handleRemoveIndex(change, oldSchema)

	case schema.SchemaChangeTypeModifyField:
		return vu.handleModifyField(change, oldSchema)

	case schema.SchemaChangeTypeModifyIndex:
		return vu.handleModifyIndex(change, oldSchema)

	case schema.SchemaChangeTypeModifyConstraint:
		return vu.handleModifyConstraint(change, oldSchema)

	case schema.SchemaChangeTypeRemoveSchema:
		return vu.handleRemoveSchema(change)

	case schema.SchemaChangeTypeModifySchema:
		return vu.handleModifySchema(change, oldSchema)

	case schema.SchemaChangeTypeModifySchemaReference:
		return vu.handleModifySchemaReference(change, oldSchema)

	default:
		return "patch", nil
	}
}

// handleRemoveField processes field removal changes (MAJOR).
func (vu *VersioningUtil) handleRemoveField(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if change.ID == nil || *change.ID == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("removeField change must specify an ID").
			WithPath("id")
	}

	if _, ok := oldSchema.Fields[*change.ID]; !ok {
		return "", common.NewSystemError("ERR_SCHEMA_FIELD_NOT_FOUND").
			WithMessagef("field '%s' not found in old schema", *change.ID).
			WithPath("id")
	}

	return "major", nil
}

// handleAddConstraint processes constraint addition changes (MAJOR).
func (vu *VersioningUtil) handleAddConstraint(change schema.SchemaChange) (string, error) {
	if change.SchemaChangeAddConstraintPayload == nil || change.SchemaChangeAddConstraintPayload.Constraint.Constraint == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("addConstraint change must have a constraint payload").
			WithPath("constraint")
	}

	return "major", nil
}

// handleRemoveConstraint processes constraint removal changes (MINOR).
func (vu *VersioningUtil) handleRemoveConstraint(change schema.SchemaChange) (string, error) {
	if change.Name == nil || *change.Name == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("removeConstraint change must specify a name").
			WithPath("name")
	}

	return "minor", nil
}

// handleAddSchema processes schema addition changes (MINOR).
func (vu *VersioningUtil) handleAddSchema(change schema.SchemaChange) (string, error) {
	if change.ID == nil || *change.ID == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessagef("%s change must specify an ID", change.Type).
			WithPath("id")
	}

	return "minor", nil
}

// handleModifyProperty processes property modification changes (PATCH).
func (vu *VersioningUtil) handleModifyProperty(change schema.SchemaChange) (string, error) {
	if change.ID == nil || *change.ID == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifyProperty change must specify an ID").
			WithPath("id")
	}

	return "patch", nil
}

// handleAddField processes field addition changes (MINOR or MAJOR).
// Adding a required field without a default is MAJOR, otherwise MINOR.
func (vu *VersioningUtil) handleAddField(change schema.SchemaChange) (string, error) {
	if change.SchemaChangeAddFieldPayload == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("addField change must have a definition payload").
			WithPath("definition")
	}

	field := change.SchemaChangeAddFieldPayload.Definition
	if field.Required != nil && *field.Required && field.Default == nil {
		return "major", nil
	}

	return "minor", nil
}

// handleAddIndex processes index addition changes (MAJOR for unique, PATCH for non-unique).
func (vu *VersioningUtil) handleAddIndex(change schema.SchemaChange) (string, error) {
	if change.SchemaChangeAddIndexPayload == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("addIndex change must have a definition payload").
			WithPath("definition")
	}

	if change.SchemaChangeAddIndexPayload.Definition.Unique != nil && *change.SchemaChangeAddIndexPayload.Definition.Unique {
		return "major", nil
	}

	return "patch", nil
}

// handleRemoveIndex processes index removal changes (MINOR for unique, PATCH for non-unique).
func (vu *VersioningUtil) handleRemoveIndex(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if change.Name == nil || *change.Name == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("removeIndex change must specify a name").
			WithPath("name")
	}

	oldIndex := vu.findIndexByName(oldSchema, *change.Name)
	if oldIndex == nil {
		return "", common.NewSystemError("ERR_SCHEMA_INDEX_NOT_FOUND").
			WithMessagef("index '%s' not found in old schema", *change.Name).
			WithPath("name")
	}

	if oldIndex.Unique != nil && *oldIndex.Unique {
		return "minor", nil
	}

	return "patch", nil
}

// handleModifyField processes field modification changes.
func (vu *VersioningUtil) handleModifyField(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if change.ID == nil || *change.ID == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifyField change must specify an ID").
			WithPath("id")
	}

	if change.SchemaChangeModifyFieldPayload == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifyField change must have a changes payload").
			WithPath("changes")
	}

	oldField, ok := oldSchema.Fields[*change.ID]
	if !ok {
		return "", common.NewSystemError("ERR_SCHEMA_FIELD_NOT_FOUND").
			WithMessagef("field '%s' not found in old schema", *change.ID).
			WithPath("id")
	}

	return vu.determineModifyFieldImpact(oldField, change.SchemaChangeModifyFieldPayload.Changes)
}

// handleModifyIndex processes index modification changes.
func (vu *VersioningUtil) handleModifyIndex(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if change.Name == nil || *change.Name == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifyIndex change must specify a name").
			WithPath("name")
	}

	if change.SchemaChangeModifyIndexPayload == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifyIndex change must have a changes payload").
			WithPath("changes")
	}

	oldIndex := vu.findIndexByName(oldSchema, *change.Name)
	if oldIndex == nil {
		return "", common.NewSystemError("ERR_SCHEMA_INDEX_NOT_FOUND").
			WithMessagef("index '%s' not found in old schema", *change.Name).
			WithPath("name")
	}

	return vu.determineModifyIndexImpact(oldIndex, change.SchemaChangeModifyIndexPayload.Changes)
}

// handleModifyConstraint processes constraint modification changes.
func (vu *VersioningUtil) handleModifyConstraint(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if change.Name == nil || *change.Name == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifyConstraint change must specify a name").
			WithPath("name")
	}

	if change.SchemaChangeModifyConstraintPayload == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifyConstraint change must have a changes payload").
			WithPath("changes")
	}

	oldRule, err := vu.findConstraintRuleByName(oldSchema, *change.Name)
	if err != nil {
		return "", common.SystemErrorFrom(err).WithOperation("handleModifyConstraint")
	}

	return vu.determineModifyConstraintImpact(oldRule, change.SchemaChangeModifyConstraintPayload.Changes)
}

// handleRemoveSchema processes schema removal changes (MAJOR).
func (vu *VersioningUtil) handleRemoveSchema(change schema.SchemaChange) (string, error) {
	if change.ID == nil || *change.ID == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessagef("%s change must specify an ID", change.Type).
			WithPath("id")
	}

	return "major", nil
}

// handleModifySchema processes nested schema modification changes.
func (vu *VersioningUtil) handleModifySchema(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if change.ID == nil || *change.ID == "" {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessagef("%s change must specify an ID", change.Type).
			WithPath("id")
	}

	if change.SchemaChangeModifySchemaPayload == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifySchema change must have a payload").
			WithPath("payload")
	}

	oldNestedSchema, ok := oldSchema.NestedSchemas[*change.ID]
	if !ok {
		return "", common.NewSystemError("ERR_SCHEMA_NOT_FOUND").
			WithMessagef("nested schema '%s' not found in old schema", *change.ID).
			WithPath("id")
	}

	tempOldSchemaDef := vu.buildTempSchemaDefinition(oldNestedSchema, oldSchema)
	return vu.calculateEffectiveVersion(change.SchemaChangeModifySchemaPayload.Changes, tempOldSchemaDef)
}

// handleModifySchemaReference processes schema reference modification changes.
func (vu *VersioningUtil) handleModifySchemaReference(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) (string, error) {
	if change.SchemaChangeModifySchemaReferencePayload == nil {
		return "", common.NewSystemError("ERR_SCHEMA_CHANGE_INVALID").
			WithMessage("modifySchemaReference change must have a payload").
			WithPath("payload")
	}

	highestImpact := "patch"
	for _, subChange := range change.SchemaChangeModifySchemaReferencePayload.Changes {
		impact, err := vu.getChangeImpact(subChange, oldSchema)
		if err != nil {
			return "", err
		}

		if impact == "major" {
			return "major", nil
		}

		if impact == "minor" && highestImpact == "patch" {
			highestImpact = "minor"
		}
	}

	return highestImpact, nil
}

// determineModifyFieldImpact analyzes field modification changes and returns impact level.
func (vu *VersioningUtil) determineModifyFieldImpact(oldField *schema.FieldDefinition, changes schema.PartialFieldDefinition) (string, error) {
	// Check for MAJOR changes first
	if majorImpact := vu.checkMajorFieldChanges(oldField, changes); majorImpact != "" {
		return majorImpact, nil
	}

	// Check for MINOR changes
	if minorImpact := vu.checkMinorFieldChanges(oldField, changes); minorImpact != "" {
		return minorImpact, nil
	}

	// Check constraint changes
	if changes.Constraints != nil {
		impact, err := vu.determineConstraintChangesImpact(oldField.Constraints, changes.Constraints)
		if err != nil {
			return "", err
		}
		if impact != "patch" {
			return impact, nil
		}
	}

	// Default to PATCH for all other changes
	return "patch", nil
}

// checkMajorFieldChanges checks for breaking field changes that require a major version bump.
func (vu *VersioningUtil) checkMajorFieldChanges(oldField *schema.FieldDefinition, changes schema.PartialFieldDefinition) string {
	// Name change
	if changes.Name != nil && *changes.Name != oldField.Name {
		return "major"
	}

	// Type change
	if changes.Type != nil && *changes.Type != oldField.Type {
		return "major"
	}

	// ItemsType change or removal
	if changes.ItemsType != nil {
		if oldField.ItemsType == nil || *changes.ItemsType != *oldField.ItemsType {
			return "major"
		}
	} else if oldField.ItemsType != nil && containsString(changes.Unset, "itemsType") {
		return "major"
	}

	// Default value removal
	if oldField.Default != nil && changes.Default == nil && containsString(changes.Unset, "default") {
		return "major"
	}

	// Field became required
	if (oldField.Required == nil || !*oldField.Required) && (changes.Required != nil && *changes.Required) {
		return "major"
	}

	// Uniqueness added
	if (oldField.Unique == nil || !*oldField.Unique) && (changes.Unique != nil && *changes.Unique) {
		return "major"
	}

	// Enum values removed
	if changes.Values != nil && len(changes.Values) < len(oldField.Values) {
		return "major"
	}
	if changes.Values == nil && containsString(changes.Unset, "values") && len(oldField.Values) > 0 {
		return "major"
	}

	return ""
}

// checkMinorFieldChanges checks for non-breaking field changes that require a minor version bump.
func (vu *VersioningUtil) checkMinorFieldChanges(oldField *schema.FieldDefinition, changes schema.PartialFieldDefinition) string {
	// Enum values added
	if changes.Values != nil && len(changes.Values) > len(oldField.Values) {
		return "minor"
	}

	// Field made optional
	if (oldField.Required != nil && *oldField.Required) &&
		((changes.Required != nil && !*changes.Required) || containsString(changes.Unset, "required")) {
		return "minor"
	}

	// Uniqueness removed
	if (oldField.Unique != nil && *oldField.Unique) && (changes.Unique != nil && !*changes.Unique) {
		return "minor"
	}

	// Deprecated status changed
	if changes.Deprecated != nil && *changes.Deprecated != (oldField.Deprecated != nil && *oldField.Deprecated) {
		return "minor"
	}

	return ""
}

// determineModifyIndexImpact analyzes index modification changes and returns impact level.
func (vu *VersioningUtil) determineModifyIndexImpact(oldIndex *schema.IndexDefinition, mods schema.PartialIndexDefinition) (string, error) {
	// Check for MAJOR changes
	wasUnique := oldIndex.Unique != nil && *oldIndex.Unique

	// If index was unique, changes to fields or partial condition are MAJOR
	if wasUnique {
		if mods.Fields != nil || mods.Partial != nil || containsString(mods.Unset, "partial") {
			return "major", nil
		}
	}

	// Becoming unique is MAJOR
	if mods.Unique != nil && *mods.Unique && !wasUnique {
		return "major", nil
	}

	// Changing index type is MAJOR
	if mods.Type != nil && *mods.Type != oldIndex.Type {
		return "major", nil
	}

	// Changing fields is MAJOR
	if mods.Fields != nil && !stringSliceEqual(mods.Fields, oldIndex.Fields) {
		return "major", nil
	}

	// Check for MINOR changes
	// Becoming non-unique is MINOR (relaxes a constraint)
	if mods.Unique != nil && !*mods.Unique && wasUnique {
		return "minor", nil
	}

	// All other changes are PATCH
	return "patch", nil
}

// determineModifyConstraintImpact analyzes constraint modification changes and returns impact level.
func (vu *VersioningUtil) determineModifyConstraintImpact(oldRule *schema.ConstraintRule, mods schema.PartialConstraint) (string, error) {
	if oldRule.IsConstraint() {
		return vu.determineSimpleConstraintImpact(oldRule.Constraint, mods)
	}

	if oldRule.IsConstraintGroup() {
		return vu.determineConstraintGroupImpact(oldRule.ConstraintGroup, mods)
	}

	return "patch", nil
}

// determineSimpleConstraintImpact analyzes simple constraint modifications.
func (vu *VersioningUtil) determineSimpleConstraintImpact(oldConstraint *schema.Constraint, mods schema.PartialConstraint) (string, error) {
	// MAJOR changes: changes to core constraint logic
	if mods.Predicate != nil && *mods.Predicate != oldConstraint.Predicate {
		return "major", nil
	}

	if mods.Field != nil && (oldConstraint.Field == nil || *mods.Field != *oldConstraint.Field) {
		return "major", nil
	}

	if mods.Fields != nil && !stringSliceEqual(mods.Fields, oldConstraint.Fields) {
		return "major", nil
	}

	if mods.Parameters != nil {
		return "major", nil
	}

	// Unsetting core properties is MAJOR
	if containsString(mods.Unset, "predicate") ||
		containsString(mods.Unset, "field") ||
		containsString(mods.Unset, "fields") ||
		containsString(mods.Unset, "parameters") {
		return "major", nil
	}

	// PATCH changes: metadata updates
	if (mods.Description != nil && (oldConstraint.Description == nil || *mods.Description != *oldConstraint.Description)) ||
		(mods.ErrorMessage != nil && (oldConstraint.ErrorMessage == nil || *mods.ErrorMessage != *oldConstraint.ErrorMessage)) ||
		containsString(mods.Unset, "description") ||
		containsString(mods.Unset, "errorMessage") {
		return "patch", nil
	}

	return "patch", nil
}

// determineConstraintGroupImpact analyzes constraint group modifications.
func (vu *VersioningUtil) determineConstraintGroupImpact(oldGroup *schema.ConstraintGroup, mods schema.PartialConstraint) (string, error) {
	// Operator changes affect constraint strictness
	if mods.Operator != nil && *mods.Operator != oldGroup.Operator {
		// AND is more restrictive than OR
		if *mods.Operator == common.LogicalAnd && oldGroup.Operator == common.LogicalOr {
			return "major", nil
		}
		// OR is less restrictive than AND
		if *mods.Operator == common.LogicalOr && oldGroup.Operator == common.LogicalAnd {
			return "minor", nil
		}
		// Any other operator change is MAJOR
		return "major", nil
	}

	// Metadata changes are PATCH
	if containsString(mods.Unset, "description") || containsString(mods.Unset, "errorMessage") {
		return "patch", nil
	}

	return "patch", nil
}

// determineConstraintChangesImpact compares two SchemaConstraint slices and determines the highest impact.
func (vu *VersioningUtil) determineConstraintChangesImpact(oldConstraints, newConstraints schema.SchemaConstraint) (string, error) {
	oldMap := vu.buildConstraintMap(oldConstraints)
	newMap := vu.buildConstraintMap(newConstraints)

	highestImpact := "patch"

	// Check for modifications and removals
	for name, oldRule := range oldMap {
		if newRule, exists := newMap[name]; exists {
			// Rule exists in both, check for modifications
			if !reflect.DeepEqual(oldRule, newRule) {
				impact, err := vu.compareConstraintRules(oldRule, newRule)
				if err != nil {
					return "", err
				}

				if impact == "major" {
					return "major", nil
				}
				if impact == "minor" && highestImpact == "patch" {
					highestImpact = "minor"
				}
			}
		} else {
			// Constraint removed - MINOR
			if highestImpact == "patch" {
				highestImpact = "minor"
			}
		}
	}

	// Check for additions
	for name := range newMap {
		if _, exists := oldMap[name]; !exists {
			// Constraint added - MAJOR
			return "major", nil
		}
	}

	return highestImpact, nil
}

// compareConstraintRules compares two constraint rules and determines impact.
func (vu *VersioningUtil) compareConstraintRules(oldRule, newRule *schema.ConstraintRule) (string, error) {
	// Type of constraint rule changed (Constraint to Group or vice versa)
	if oldRule.IsConstraint() != newRule.IsConstraint() {
		return "major", nil
	}

	if oldRule.IsConstraint() && newRule.IsConstraint() {
		return vu.compareSimpleConstraints(oldRule.Constraint, newRule.Constraint)
	}

	if oldRule.IsConstraintGroup() && newRule.IsConstraintGroup() {
		return vu.compareConstraintGroups(oldRule.ConstraintGroup, newRule.ConstraintGroup)
	}

	return "patch", nil
}

// compareSimpleConstraints compares two simple constraints.
func (vu *VersioningUtil) compareSimpleConstraints(old, new *schema.Constraint) (string, error) {
	// Core property changes are MAJOR
	if old.Predicate != new.Predicate ||
		old.Field != new.Field ||
		!stringSliceEqual(old.Fields, new.Fields) ||
		!reflect.DeepEqual(old.Parameters, new.Parameters) {
		return "major", nil
	}

	return "patch", nil
}

// compareConstraintGroups compares two constraint groups.
func (vu *VersioningUtil) compareConstraintGroups(old, new *schema.ConstraintGroup) (string, error) {
	// Operator changes affect strictness
	if old.Operator != new.Operator {
		if new.Operator == common.LogicalAnd && old.Operator == common.LogicalOr {
			return "major", nil
		}
		if new.Operator == common.LogicalOr && old.Operator == common.LogicalAnd {
			return "minor", nil
		}
		return "major", nil
	}

	// Recursively check rules within the group
	return vu.determineConstraintChangesImpact(old.Rules, new.Rules)
}

// findConstraintRuleByName searches for a ConstraintRule by its name in the schema.
func (vu *VersioningUtil) findConstraintRuleByName(s *schema.SchemaDefinition, name string) (*schema.ConstraintRule, error) {
	// Search top-level constraints
	for i := range s.Constraints {
		cr := &s.Constraints[i]
		if (cr.IsConstraint() && cr.Constraint.Name == name) ||
			(cr.IsConstraintGroup() && cr.ConstraintGroup.Name == name) {
			return cr, nil
		}
	}

	// Search nested schema constraints
	for _, nsd := range s.NestedSchemas {
		for i := range nsd.Constraints {
			cr := &nsd.Constraints[i]
			if (cr.IsConstraint() && cr.Constraint.Name == name) ||
				(cr.IsConstraintGroup() && cr.ConstraintGroup.Name == name) {
				return cr, nil
			}
		}
	}

	return nil, common.NewSystemError("ERR_SCHEMA_CONSTRAINT_NOT_FOUND").
		WithMessagef("constraint or constraint group '%s' not found", name)
}

// findIndexByName searches for an index by its name in the schema.
func (vu *VersioningUtil) findIndexByName(s *schema.SchemaDefinition, name string) *schema.IndexDefinition {
	for _, ior := range s.Indexes {
		if ior.IsIndex() && ior.Index.Name == name {
			return ior.Index
		}
	}
	return nil
}

// buildConstraintMap creates a map of constraint names to rules for easy lookup.
func (vu *VersioningUtil) buildConstraintMap(constraints schema.SchemaConstraint) map[string]*schema.ConstraintRule {
	constraintMap := make(map[string]*schema.ConstraintRule)

	for i := range constraints {
		rule := &constraints[i]
		if rule.IsConstraint() {
			constraintMap[rule.Constraint.Name] = rule
		} else if rule.IsConstraintGroup() {
			constraintMap[rule.ConstraintGroup.Name] = rule
		}
	}

	return constraintMap
}

// buildTempSchemaDefinition creates a temporary schema definition for nested schema processing.
func (vu *VersioningUtil) buildTempSchemaDefinition(nested *schema.NestedSchemaDefinition, parent *schema.SchemaDefinition) *schema.SchemaDefinition {
	temp := &schema.SchemaDefinition{
		Name:          nested.Name,
		Description:   nested.Description,
		Fields:        make(map[string]*schema.FieldDefinition),
		NestedSchemas: parent.NestedSchemas,
		Constraints:   nested.Constraints,
		Indexes:       nested.Indexes,
	}

	// Populate fields if nested schema is structured
	if nested.IsStructured() && nested.Fields.IsMap() {
		temp.Fields = nested.Fields.FieldsMap
	}

	return temp
}

// containsString checks if a string exists in a slice of strings.
func containsString(slice []string, s string) bool {
	return slices.Contains(slice, s)
}

