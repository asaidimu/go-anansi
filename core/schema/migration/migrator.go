// Package migration provides automatic migration generation from schema differences.
package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// GeneratorOptions configures the migration generator.
type GeneratorOptions struct {
	Description      string
	GenerateRollback bool
	IgnoreMetadata   bool
	StrictComparison bool
}

// MigrationGenerator generates migrations from schema differences.
type MigrationGenerator struct {
	options        GeneratorOptions
	versioningUtil *VersioningUtil
	rollbackGen    *RollbackGenerator
}

// NewMigrationGenerator creates a new migration generator.
func NewMigrationGenerator(options GeneratorOptions) *MigrationGenerator {
	return &MigrationGenerator{
		options:        options,
		versioningUtil: NewVersioningUtil(),
		rollbackGen:    NewRollbackGenerator(),
	}
}

// Generate generates a migration from two schema versions.
func (g *MigrationGenerator) Generate(oldSchema, newSchema *schema.SchemaDefinition) (*schema.Migration, error) {
	if err := g.validateSchemas(oldSchema, newSchema); err != nil {
		return nil, err
	}

	changes := g.collectAllChanges(oldSchema, newSchema)
	if len(changes) == 0 {
		return nil, nil // No changes detected
	}

	nextVersion, err := g.versioningUtil.CalculateNextVersion(oldSchema.Version, changes, oldSchema)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithMessage("failed to calculate next version").
			WithOperation("MigrationGenerator.Generate")
	}

	migration := g.buildMigration(oldSchema, newSchema, nextVersion, changes)

	if err := g.addChecksum(migration); err != nil {
		return nil, err
	}

	return migration, nil
}

// validateSchemas ensures the schemas can be compared.
func (g *MigrationGenerator) validateSchemas(oldSchema, newSchema *schema.SchemaDefinition) error {
	if oldSchema.Name != newSchema.Name {
		return common.NewSystemError("ERR_SCHEMA_NAME_MISMATCH").
			WithMessagef("schema names must match: %s != %s", oldSchema.Name, newSchema.Name).
			WithOperation("MigrationGenerator.Generate").
			WithPath("name")
	}
	return nil
}

// collectAllChanges gathers all changes between two schemas.
func (g *MigrationGenerator) collectAllChanges(oldSchema, newSchema *schema.SchemaDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	changes = append(changes, g.compareProperties(oldSchema, newSchema)...)
	changes = append(changes, g.compareFields(oldSchema.Fields, newSchema.Fields)...)
	changes = append(changes, g.compareIndexes(oldSchema.Indexes, newSchema.Indexes)...)
	changes = append(changes, g.compareConstraints(oldSchema.Constraints, newSchema.Constraints)...)

	if oldSchema.NestedSchemas != nil || newSchema.NestedSchemas != nil {
		changes = append(changes, g.compareNested(oldSchema.NestedSchemas, newSchema.NestedSchemas)...)
	}

	return changes
}

// buildMigration constructs the migration object.
func (g *MigrationGenerator) buildMigration(oldSchema, newSchema *schema.SchemaDefinition, nextVersion string, changes []schema.SchemaChange) *schema.Migration {
	migration := &schema.Migration{
		ID: fmt.Sprintf("%d", time.Now().UnixNano()),
		Version: schema.MigrationVersion{
			Source: oldSchema.Version,
			Target: &nextVersion,
		},
		Changes:     changes,
		Description: g.options.Description,
		Status:      "pending",
		Transform:   "",
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	if g.options.GenerateRollback {
		migration.Rollback = g.rollbackGen.Generate(changes, oldSchema, newSchema)
	}

	return migration
}

// addChecksum generates and adds a checksum to the migration.
func (g *MigrationGenerator) addChecksum(migration *schema.Migration) error {
	checksum, err := generateChecksum(migration)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithMessage("failed to generate checksum").
			WithOperation("MigrationGenerator.Generate")
	}
	migration.Checksum = checksum
	return nil
}

// compareProperties compares schema-level properties.
func (g *MigrationGenerator) compareProperties(oldSchema, newSchema *schema.SchemaDefinition) []schema.SchemaChange {
	if g.options.IgnoreMetadata {
		return nil
	}

	changes := make([]schema.SchemaChange, 0, 2)

	if !stringPtrEqual(oldSchema.Description, newSchema.Description) {
		changes = append(changes, g.createPropertyChange("description", newSchema.Description))
	}

	if !reflect.DeepEqual(oldSchema.Metadata, newSchema.Metadata) {
		changes = append(changes, g.createPropertyChange("metadata", newSchema.Metadata))
	}

	return changes
}

// createPropertyChange creates a property modification change.
func (g *MigrationGenerator) createPropertyChange(id string, value any) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyProperty,
		ID:   utils.StringPtr(id),
		SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
			Value: value,
		},
	}
}

// compareFields compares field definitions between schemas.
func (g *MigrationGenerator) compareFields(oldFields, newFields map[string]*schema.FieldDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Process removed fields
	for name := range oldFields {
		if _, exists := newFields[name]; !exists {
			changes = append(changes, g.createRemoveFieldChange(name))
		}
	}

	// Process added and modified fields
	for name, newField := range newFields {
		oldField, exists := oldFields[name]

		if !exists {
			changes = append(changes, g.createAddFieldChange(name, newField))
		} else {
			changes = append(changes, g.processFieldModification(name, oldField, newField)...)
		}
	}

	return changes
}

// createRemoveFieldChange creates a field removal change.
func (g *MigrationGenerator) createRemoveFieldChange(name string) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveField,
		ID:   utils.StringPtr(name),
	}
}

// createAddFieldChange creates a field addition change.
func (g *MigrationGenerator) createAddFieldChange(name string, field *schema.FieldDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddField,
		ID:   utils.StringPtr(name),
		SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
			Definition: *field,
		},
	}
}

// processFieldModification handles field modification detection.
func (g *MigrationGenerator) processFieldModification(name string, oldField, newField *schema.FieldDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	partialChanges, nestedChanges := g.compareFieldDefinitions(oldField, newField)

	if partialChanges != nil {
		changes = append(changes, schema.SchemaChange{
			Type: schema.SchemaChangeTypeModifyField,
			ID:   utils.StringPtr(name),
			SchemaChangeModifyFieldPayload: &schema.SchemaChangeModifyFieldPayload{
				Changes: *partialChanges,
			},
		})
	}

	changes = append(changes, nestedChanges...)
	return changes
}

// compareFieldDefinitions compares two field definitions.
func (g *MigrationGenerator) compareFieldDefinitions(oldField, newField *schema.FieldDefinition) (*schema.PartialFieldDefinition, []schema.SchemaChange) {
	partial := &schema.PartialFieldDefinition{}
	hasChanges := false

	// Compare simple fields
	hasChanges = g.compareFieldSimpleProperties(oldField, newField, partial) || hasChanges

	// Compare schema references and generate nested changes
	nestedChanges := g.compareFieldSchemaProperty(oldField, newField, partial, &hasChanges)

	if !hasChanges {
		return nil, nestedChanges
	}

	return partial, nestedChanges
}

// compareFieldSimpleProperties compares simple field properties.
func (g *MigrationGenerator) compareFieldSimpleProperties(oldField, newField *schema.FieldDefinition, partial *schema.PartialFieldDefinition) bool {
	hasChanges := false

	// Name
	if oldField.Name != newField.Name {
		partial.Name = &newField.Name
		hasChanges = true
	}

	// Type
	if oldField.Type != newField.Type {
		partial.Type = &newField.Type
		hasChanges = true
	}

	// Required
	if !boolPtrEqual(oldField.Required, newField.Required) {
		if newField.Required == nil && oldField.Required != nil {
			partial.Unset = append(partial.Unset, "required")
		} else {
			partial.Required = newField.Required
		}
		hasChanges = true
	}

	// Unique
	if !boolPtrEqual(oldField.Unique, newField.Unique) {
		if newField.Unique == nil && oldField.Unique != nil {
			partial.Unset = append(partial.Unset, "unique")
		} else {
			partial.Unique = newField.Unique
		}
		hasChanges = true
	}

	// Description
	if !stringPtrEqual(oldField.Description, newField.Description) {
		if newField.Description == nil && oldField.Description != nil {
			partial.Unset = append(partial.Unset, "description")
		} else {
			partial.Description = newField.Description
		}
		hasChanges = true
	}

	// Default
	if !reflect.DeepEqual(oldField.Default, newField.Default) {
		if newField.Default == nil && oldField.Default != nil {
			partial.Unset = append(partial.Unset, "default")
		} else {
			partial.Default = newField.Default
		}
		hasChanges = true
	}

	// ItemsType
	if !fieldTypePtrEqual(oldField.ItemsType, newField.ItemsType) {
		if newField.ItemsType == nil && oldField.ItemsType != nil {
			partial.Unset = append(partial.Unset, "itemsType")
		} else {
			partial.ItemsType = newField.ItemsType
		}
		hasChanges = true
	}

	// Values (enums)
	if !reflect.DeepEqual(oldField.Values, newField.Values) {
		if len(newField.Values) == 0 {
			partial.Unset = append(partial.Unset, "values")
		} else {
			partial.Values = newField.Values
		}
		hasChanges = true
	}

	// Constraints
	if !reflect.DeepEqual(oldField.Constraints, newField.Constraints) {
		if len(newField.Constraints) == 0 {
			partial.Unset = append(partial.Unset, "constraints")
		} else {
			partial.Constraints = newField.Constraints
		}
		hasChanges = true
	}

	// Hint
	if !reflect.DeepEqual(oldField.Hint, newField.Hint) {
		if newField.Hint == nil && oldField.Hint != nil {
			partial.Unset = append(partial.Unset, "hint")
		} else {
			partial.Hint = newField.Hint
		}
		hasChanges = true
	}

	// Deprecated
	if !boolPtrEqual(oldField.Deprecated, newField.Deprecated) {
		if newField.Deprecated == nil && oldField.Deprecated != nil {
			partial.Unset = append(partial.Unset, "deprecated")
		} else {
			partial.Deprecated = newField.Deprecated
		}
		hasChanges = true
	}

	return hasChanges
}

// compareFieldSchemaProperty handles schema reference comparisons in fields.
func (g *MigrationGenerator) compareFieldSchemaProperty(oldField, newField *schema.FieldDefinition, partial *schema.PartialFieldDefinition, hasChanges *bool) []schema.SchemaChange {
	if reflect.DeepEqual(oldField.Schema, newField.Schema) {
		return nil
	}

	oldSchemaRef, oldIsRef := oldField.Schema.(schema.NestedSchemaReference)
	newSchemaRef, newIsRef := newField.Schema.(schema.NestedSchemaReference)

	// Same reference ID - check for internal changes
	if oldIsRef && newIsRef && oldSchemaRef.ID == newSchemaRef.ID {
		return g.compareSchemaReferenceChanges(oldField, oldSchemaRef, newSchemaRef)
	}

	// Schema removed
	if newField.Schema == nil {
		partial.Unset = append(partial.Unset, "schema")
		*hasChanges = true
		return nil
	}

	// Schema added or replaced
	partial.Schema = newField.Schema
	*hasChanges = true
	return nil
}

// compareSchemaReferenceChanges compares internal changes in schema references.
func (g *MigrationGenerator) compareSchemaReferenceChanges(oldField *schema.FieldDefinition, oldRef, newRef schema.NestedSchemaReference) []schema.SchemaChange {
	indexChanges := g.compareIndexes(oldRef.Indexes, newRef.Indexes)
	constraintChanges := g.compareConstraints(oldRef.Constraints, newRef.Constraints)

	if len(indexChanges) == 0 && len(constraintChanges) == 0 {
		return nil
	}

	subChanges := make([]schema.SchemaChange, 0, len(indexChanges)+len(constraintChanges))
	subChanges = append(subChanges, indexChanges...)
	subChanges = append(subChanges, constraintChanges...)

	return []schema.SchemaChange{{
		Type: schema.SchemaChangeTypeModifySchemaReference,
		ID:   utils.StringPtr(oldRef.ID),
		SchemaChangeModifySchemaReferencePayload: &schema.SchemaChangeModifySchemaReferencePayload{
			Field:   oldField.Name,
			ID:      utils.StringPtr(oldRef.ID),
			Changes: subChanges,
		},
	}}
}

// compareIndexes compares index definitions between schemas.
func (g *MigrationGenerator) compareIndexes(oldIndexes, newIndexes []schema.IndexOrReference) []schema.SchemaChange {
	oldMap := buildIndexMap(oldIndexes)
	newMap := buildIndexMap(newIndexes)

	changes := make([]schema.SchemaChange, 0)

	// Process removed indexes
	for name := range oldMap {
		if _, exists := newMap[name]; !exists {
			changes = append(changes, g.createRemoveIndexChange(name))
		}
	}

	// Process added and modified indexes
	for name, newIndex := range newMap {
		oldIndex, exists := oldMap[name]

		if !exists {
			changes = append(changes, g.createAddIndexChange(newIndex))
		} else if indexChanges := g.compareIndexDefinitions(oldIndex, newIndex); indexChanges != nil {
			changes = append(changes, g.createModifyIndexChange(name, indexChanges))
		}
	}

	return changes
}

// createRemoveIndexChange creates an index removal change.
func (g *MigrationGenerator) createRemoveIndexChange(name string) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveIndex,
		Name: utils.StringPtr(name),
	}
}

// createAddIndexChange creates an index addition change.
func (g *MigrationGenerator) createAddIndexChange(index *schema.IndexDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
			Definition: *index,
		},
	}
}

// createModifyIndexChange creates an index modification change.
func (g *MigrationGenerator) createModifyIndexChange(name string, changes *schema.PartialIndexDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyIndex,
		Name: utils.StringPtr(name),
		SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
			Changes: *changes,
		},
	}
}

// compareIndexDefinitions compares two index definitions.
func (g *MigrationGenerator) compareIndexDefinitions(oldIndex, newIndex *schema.IndexDefinition) *schema.PartialIndexDefinition {
	changes := &schema.PartialIndexDefinition{}
	hasChanges := false

	// Type
	if oldIndex.Type != newIndex.Type {
		changes.Type = &newIndex.Type
		hasChanges = true
	}

	// Fields
	if !stringSliceEqual(oldIndex.Fields, newIndex.Fields) {
		if len(newIndex.Fields) == 0 {
			changes.Unset = append(changes.Unset, "fields")
		} else {
			changes.Fields = newIndex.Fields
		}
		hasChanges = true
	}

	// Unique
	if !boolPtrEqual(oldIndex.Unique, newIndex.Unique) {
		if newIndex.Unique == nil && oldIndex.Unique != nil {
			changes.Unset = append(changes.Unset, "unique")
		} else {
			changes.Unique = newIndex.Unique
		}
		hasChanges = true
	}

	// Description
	if !stringPtrEqual(oldIndex.Description, newIndex.Description) {
		if newIndex.Description == nil && oldIndex.Description != nil {
			changes.Unset = append(changes.Unset, "description")
		} else {
			changes.Description = newIndex.Description
		}
		hasChanges = true
	}

	// Order
	if !stringPtrEqual(oldIndex.Order, newIndex.Order) {
		if newIndex.Order == nil && oldIndex.Order != nil {
			changes.Unset = append(changes.Unset, "order")
		} else {
			changes.Order = newIndex.Order
		}
		hasChanges = true
	}

	// Partial
	if !reflect.DeepEqual(oldIndex.Partial, newIndex.Partial) {
		if newIndex.Partial == nil && oldIndex.Partial != nil {
			changes.Unset = append(changes.Unset, "partial")
		} else {
			changes.Partial = newIndex.Partial
		}
		hasChanges = true
	}

	if !hasChanges {
		return nil
	}

	return changes
}

// compareConstraints compares constraint definitions between schemas.
func (g *MigrationGenerator) compareConstraints(oldConstraints, newConstraints schema.SchemaConstraint) []schema.SchemaChange {
	oldMap := buildConstraintMap(oldConstraints)
	newMap := buildConstraintMap(newConstraints)

	changes := make([]schema.SchemaChange, 0)

	// Process removed constraints
	for name := range oldMap {
		if _, exists := newMap[name]; !exists {
			changes = append(changes, g.createRemoveConstraintChange(name))
		}
	}

	// Process added and modified constraints
	for name, newRule := range newMap {
		oldRule, exists := oldMap[name]

		if !exists {
			changes = append(changes, g.createAddConstraintChange(newRule))
		} else {
			changes = append(changes, g.processConstraintModification(name, oldRule, newRule)...)
		}
	}

	return changes
}

// createRemoveConstraintChange creates a constraint removal change.
func (g *MigrationGenerator) createRemoveConstraintChange(name string) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveConstraint,
		Name: utils.StringPtr(name),
	}
}

// createAddConstraintChange creates a constraint addition change.
func (g *MigrationGenerator) createAddConstraintChange(rule *schema.ConstraintRule) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddConstraint,
		SchemaChangeAddConstraintPayload: &schema.SchemaChangeAddConstraintPayload{
			Constraint: *rule,
		},
	}
}

// processConstraintModification handles constraint modification detection.
func (g *MigrationGenerator) processConstraintModification(name string, oldRule, newRule *schema.ConstraintRule) []schema.SchemaChange {
	if reflect.DeepEqual(oldRule, newRule) {
		return nil
	}

	// Both are constraint groups - deep compare
	if oldRule.IsConstraintGroup() && newRule.IsConstraintGroup() {
		return g.compareConstraintGroupChanges(oldRule.ConstraintGroup, newRule.ConstraintGroup)
	}

	// Both are simple constraints - compare
	if oldRule.IsConstraint() && newRule.IsConstraint() {
		if partialChanges := g.compareConstraintDefinitions(oldRule.Constraint, newRule.Constraint); partialChanges != nil {
			return []schema.SchemaChange{g.createModifyConstraintChange(name, partialChanges)}
		}
		return nil
	}

	// Type changed - remove and add
	return []schema.SchemaChange{
		g.createRemoveConstraintChange(name),
		g.createAddConstraintChange(newRule),
	}
}

// createModifyConstraintChange creates a constraint modification change.
func (g *MigrationGenerator) createModifyConstraintChange(name string, changes *schema.PartialConstraint) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyConstraint,
		Name: utils.StringPtr(name),
		SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
			Changes: *changes,
		},
	}
}

// compareConstraintGroupChanges performs deep diff on constraint groups.
func (g *MigrationGenerator) compareConstraintGroupChanges(oldGroup, newGroup *schema.ConstraintGroup) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Check operator change
	if oldGroup.Operator != newGroup.Operator {
		changes = append(changes, g.createModifyConstraintChange(oldGroup.Name, &schema.PartialConstraint{
			Operator: &newGroup.Operator,
		}))
	}

	// Compare rules within the group
	changes = append(changes, g.compareConstraintGroupRules(oldGroup, newGroup)...)

	return changes
}

// compareConstraintGroupRules compares rules within constraint groups.
func (g *MigrationGenerator) compareConstraintGroupRules(oldGroup, newGroup *schema.ConstraintGroup) []schema.SchemaChange {
	oldRulesMap := buildConstraintRuleMap(oldGroup.Rules)
	newRulesMap := buildConstraintRuleMap(newGroup.Rules)

	changes := make([]schema.SchemaChange, 0)

	// Process removed rules
	for name := range oldRulesMap {
		if _, exists := newRulesMap[name]; !exists {
			hierarchicalName := fmt.Sprintf("%s/%s", oldGroup.Name, name)
			changes = append(changes, g.createRemoveConstraintChange(hierarchicalName))
		}
	}

	// Process added and modified rules
	for name, newRule := range newRulesMap {
		hierarchicalName := fmt.Sprintf("%s/%s", oldGroup.Name, name)
		oldRule, exists := oldRulesMap[name]

		if !exists {
			change := g.createAddConstraintChange(&newRule)
			change.Name = utils.StringPtr(hierarchicalName)
			changes = append(changes, change)
		} else if !reflect.DeepEqual(oldRule, newRule) {
			changes = append(changes, g.processNestedConstraintChange(hierarchicalName, oldRule, newRule)...)
		}
	}

	return changes
}

// processNestedConstraintChange handles nested constraint modifications.
func (g *MigrationGenerator) processNestedConstraintChange(name string, oldRule, newRule schema.ConstraintRule) []schema.SchemaChange {
	// Both are groups - recurse
	if oldRule.IsConstraintGroup() && newRule.IsConstraintGroup() {
		return g.compareConstraintGroupChanges(oldRule.ConstraintGroup, newRule.ConstraintGroup)
	}

	// Simple constraints - compare
	if oldRule.IsConstraint() && newRule.IsConstraint() {
		if partialChanges := g.compareConstraintDefinitions(oldRule.Constraint, newRule.Constraint); partialChanges != nil {
			return []schema.SchemaChange{g.createModifyConstraintChange(name, partialChanges)}
		}
	}

	return nil
}

// compareConstraintDefinitions compares two constraint definitions.
func (g *MigrationGenerator) compareConstraintDefinitions(oldConstraint, newConstraint *schema.Constraint) *schema.PartialConstraint {
	changes := &schema.PartialConstraint{}
	hasChanges := false

	// Predicate
	if oldConstraint.Predicate != newConstraint.Predicate {
		changes.Predicate = &newConstraint.Predicate
		hasChanges = true
	}

	// Field
	if !stringPtrEqual(oldConstraint.Field, newConstraint.Field) {
		if newConstraint.Field == nil && oldConstraint.Field != nil {
			changes.Unset = append(changes.Unset, "field")
		} else {
			changes.Field = newConstraint.Field
		}
		hasChanges = true
	}

	// Fields
	if !stringSliceEqual(oldConstraint.Fields, newConstraint.Fields) {
		if len(newConstraint.Fields) == 0 {
			changes.Unset = append(changes.Unset, "fields")
		} else {
			changes.Fields = newConstraint.Fields
		}
		hasChanges = true
	}

	// Parameters
	if !reflect.DeepEqual(oldConstraint.Parameters, newConstraint.Parameters) {
		if newConstraint.Parameters == nil && oldConstraint.Parameters != nil {
			changes.Unset = append(changes.Unset, "parameters")
		} else {
			changes.Parameters = newConstraint.Parameters
		}
		hasChanges = true
	}

	// Description
	if !stringPtrEqual(oldConstraint.Description, newConstraint.Description) {
		if newConstraint.Description == nil && oldConstraint.Description != nil {
			changes.Unset = append(changes.Unset, "description")
		} else {
			changes.Description = newConstraint.Description
		}
		hasChanges = true
	}

	// ErrorMessage
	if !stringPtrEqual(oldConstraint.ErrorMessage, newConstraint.ErrorMessage) {
		if newConstraint.ErrorMessage == nil && oldConstraint.ErrorMessage != nil {
			changes.Unset = append(changes.Unset, "errorMessage")
		} else {
			changes.ErrorMessage = newConstraint.ErrorMessage
		}
		hasChanges = true
	}

	if !hasChanges {
		return nil
	}

	return changes
}

// compareNested compares nested schema definitions.
func (g *MigrationGenerator) compareNested(oldSchemas, newSchemas map[string]*schema.NestedSchemaDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Process removed schemas
	for id := range oldSchemas {
		if _, exists := newSchemas[id]; !exists {
			changes = append(changes, g.createRemoveSchemaChange(id))
		}
	}

	// Process added and modified schemas
	for id, newSchema := range newSchemas {
		oldSchema, exists := oldSchemas[id]

		if !exists {
			changes = append(changes, g.createAddSchemaChange(id, newSchema))
		} else if nestedChanges := g.compareNestedSchemas(oldSchema, newSchema); len(nestedChanges) > 0 {
			changes = append(changes, g.createModifySchemaChange(id, nestedChanges))
		}
	}

	return changes
}

// createRemoveSchemaChange creates a schema removal change.
func (g *MigrationGenerator) createRemoveSchemaChange(id string) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveSchema,
		ID:   utils.StringPtr(id),
	}
}

// createAddSchemaChange creates a schema addition change.
func (g *MigrationGenerator) createAddSchemaChange(id string, def *schema.NestedSchemaDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddSchema,
		ID:   utils.StringPtr(id),
		SchemaChangeAddSchemaPayload: &schema.SchemaChangeAddSchemaPayload{
			Definition: *def,
		},
	}
}

// createModifySchemaChange creates a schema modification change.
func (g *MigrationGenerator) createModifySchemaChange(id string, changes []schema.SchemaChange) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifySchema,
		ID:   utils.StringPtr(id),
		SchemaChangeModifySchemaPayload: &schema.SchemaChangeModifySchemaPayload{
			Changes: changes,
		},
	}
}

// compareNestedSchemas compares nested schema definitions.
func (g *MigrationGenerator) compareNestedSchemas(oldSchema, newSchema *schema.NestedSchemaDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Compare fields for structured schemas
	if oldSchema.Fields != nil && newSchema.Fields != nil {
		if oldSchema.Fields.IsMap() && newSchema.Fields.IsMap() {
			changes = append(changes, g.compareFields(oldSchema.Fields.FieldsMap, newSchema.Fields.FieldsMap)...)
		}
	}

	// Compare indexes
	changes = append(changes, g.compareIndexes(oldSchema.Indexes, newSchema.Indexes)...)

	// Compare constraints
	changes = append(changes, g.compareConstraints(oldSchema.Constraints, newSchema.Constraints)...)

	return changes
}

// GenerateMigrationSequence generates a sequence of migrations from a slice of schemas.
func GenerateMigrationSequence(schemas []*schema.SchemaDefinition, options GeneratorOptions) ([]*schema.Migration, error) {
	if len(schemas) < 2 {
		return nil, common.NewSystemError("ERR_INSUFFICIENT_SCHEMAS").
			WithMessage("at least 2 schemas required to generate migration sequence").
			WithOperation("GenerateMigrationSequence")
	}

	sortedSchemas := sortSchemasByVersion(schemas)
	generator := NewMigrationGenerator(options)
	migrations := make([]*schema.Migration, 0, len(sortedSchemas)-1)

	for i := 0; i < len(sortedSchemas)-1; i++ {
		migration, err := generator.Generate(sortedSchemas[i], sortedSchemas[i+1])
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithMessagef("failed to generate migration for schema pair %d", i).
				WithOperation("GenerateMigrationSequence")
		}

		// Skip if no changes detected
		if migration != nil {
			migrations = append(migrations, migration)
		}
	}

	return migrations, nil
}

// RollbackGenerator generates rollback changes for migrations.
type RollbackGenerator struct{}

// NewRollbackGenerator creates a new rollback generator.
func NewRollbackGenerator() *RollbackGenerator {
	return &RollbackGenerator{}
}

// Generate generates rollback changes for a migration.
func (r *RollbackGenerator) Generate(changes []schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) []schema.SchemaChange {
	rollback := make([]schema.SchemaChange, 0, len(changes))

	// Process in reverse order
	for i := len(changes) - 1; i >= 0; i-- {
		if rollbackChange := r.generateRollbackForChange(changes[i], oldSchema, newSchema); rollbackChange != nil {
			rollback = append(rollback, *rollbackChange)
		}
	}

	return rollback
}

// generateRollbackForChange generates a rollback for a single change.
func (r *RollbackGenerator) generateRollbackForChange(change schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) *schema.SchemaChange {
	switch change.Type {
	case schema.SchemaChangeTypeAddField:
		return r.rollbackAddField(change)

	case schema.SchemaChangeTypeRemoveField:
		return r.rollbackRemoveField(change, oldSchema)

	case schema.SchemaChangeTypeModifyField:
		return r.rollbackModifyField(change, oldSchema, newSchema)

	case schema.SchemaChangeTypeAddIndex:
		return r.rollbackAddIndex(change)

	case schema.SchemaChangeTypeRemoveIndex:
		return r.rollbackRemoveIndex(change, oldSchema)

	case schema.SchemaChangeTypeModifyIndex:
		return r.rollbackModifyIndex(change, oldSchema, newSchema)

	case schema.SchemaChangeTypeAddConstraint:
		return r.rollbackAddConstraint(change)

	case schema.SchemaChangeTypeRemoveConstraint:
		return r.rollbackRemoveConstraint(change, oldSchema)

	case schema.SchemaChangeTypeModifyConstraint:
		return r.rollbackModifyConstraint(change, oldSchema, newSchema)

	case schema.SchemaChangeTypeModifySchemaReference:
		return r.rollbackModifySchemaReference(change, oldSchema, newSchema)

	case schema.SchemaChangeTypeModifyProperty:
		return r.rollbackModifyProperty(change, oldSchema)

	default:
		return nil
	}
}

// rollbackAddField generates rollback for field addition.
func (r *RollbackGenerator) rollbackAddField(change schema.SchemaChange) *schema.SchemaChange {
	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveField,
		ID:   change.ID,
	}
}

// rollbackRemoveField generates rollback for field removal.
func (r *RollbackGenerator) rollbackRemoveField(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldField, exists := oldSchema.Fields[*change.ID]
	if !exists {
		return nil
	}

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddField,
		ID:   change.ID,
		SchemaChangeAddFieldPayload: &schema.SchemaChangeAddFieldPayload{
			Definition: *oldField,
		},
	}
}

// rollbackModifyField generates rollback for field modification.
func (r *RollbackGenerator) rollbackModifyField(change schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldField, oldExists := oldSchema.Fields[*change.ID]
	newField, newExists := newSchema.Fields[*change.ID]

	if !oldExists || !newExists {
		return nil
	}

	generator := NewMigrationGenerator(GeneratorOptions{})
	reverseChanges, nestedRollback := generator.compareFieldDefinitions(newField, oldField)

	if reverseChanges == nil && len(nestedRollback) == 0 {
		return nil
	}

	result := &schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyField,
		ID:   change.ID,
	}

	if reverseChanges != nil {
		result.SchemaChangeModifyFieldPayload = &schema.SchemaChangeModifyFieldPayload{
			Changes: *reverseChanges,
		}
	}

	return result
}

// rollbackAddIndex generates rollback for index addition.
func (r *RollbackGenerator) rollbackAddIndex(change schema.SchemaChange) *schema.SchemaChange {
	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveIndex,
		Name: utils.StringPtr(change.SchemaChangeAddIndexPayload.Definition.Name),
	}
}

// rollbackRemoveIndex generates rollback for index removal.
func (r *RollbackGenerator) rollbackRemoveIndex(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldIndex := findIndexInSchema(oldSchema, *change.Name)
	if oldIndex == nil {
		return nil
	}

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload: &schema.SchemaChangeAddIndexPayload{
			Definition: *oldIndex,
		},
	}
}

// rollbackModifyIndex generates rollback for index modification.
func (r *RollbackGenerator) rollbackModifyIndex(change schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldIndex := findIndexInSchema(oldSchema, *change.Name)
	newIndex := findIndexInSchema(newSchema, *change.Name)

	if oldIndex == nil || newIndex == nil {
		return nil
	}

	generator := NewMigrationGenerator(GeneratorOptions{})
	reverseChanges := generator.compareIndexDefinitions(newIndex, oldIndex)

	if reverseChanges == nil {
		return nil
	}

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyIndex,
		Name: change.Name,
		SchemaChangeModifyIndexPayload: &schema.SchemaChangeModifyIndexPayload{
			Changes: *reverseChanges,
		},
	}
}

// rollbackAddConstraint generates rollback for constraint addition.
func (r *RollbackGenerator) rollbackAddConstraint(change schema.SchemaChange) *schema.SchemaChange {
	var name string
	if change.Constraint.IsConstraint() {
		name = change.Constraint.Constraint.Name
	} else if change.Constraint.IsConstraintGroup() {
		name = change.Constraint.ConstraintGroup.Name
	}

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveConstraint,
		Name: utils.StringPtr(name),
	}
}

// rollbackRemoveConstraint generates rollback for constraint removal.
func (r *RollbackGenerator) rollbackRemoveConstraint(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldRule := findConstraintInSchema(oldSchema, *change.Name)
	if oldRule == nil {
		return nil
	}

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeAddConstraint,
		SchemaChangeAddConstraintPayload: &schema.SchemaChangeAddConstraintPayload{
			Constraint: *oldRule,
		},
	}
}

// rollbackModifyConstraint generates rollback for constraint modification.
func (r *RollbackGenerator) rollbackModifyConstraint(change schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldRule := findConstraintInSchema(oldSchema, *change.Name)
	newRule := findConstraintInSchema(newSchema, *change.Name)

	if oldRule == nil || newRule == nil {
		return nil
	}

	generator := NewMigrationGenerator(GeneratorOptions{})

	var reverseChanges *schema.PartialConstraint
	if oldRule.IsConstraint() && newRule.IsConstraint() {
		reverseChanges = generator.compareConstraintDefinitions(newRule.Constraint, oldRule.Constraint)
	}

	if reverseChanges == nil {
		return nil
	}

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyConstraint,
		Name: change.Name,
		SchemaChangeModifyConstraintPayload: &schema.SchemaChangeModifyConstraintPayload{
			Changes: *reverseChanges,
		},
	}
}

// rollbackModifySchemaReference generates rollback for schema reference modification.
func (r *RollbackGenerator) rollbackModifySchemaReference(change schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) *schema.SchemaChange {
	if change.SchemaChangeModifySchemaReferencePayload == nil || change.ID == nil {
		return nil
	}

	fieldName := change.SchemaChangeModifySchemaReferencePayload.Field
	nestedSchemaID := *change.ID

	oldField, oldExists := oldSchema.Fields[fieldName]
	newField, newExists := newSchema.Fields[fieldName]

	if !oldExists || !newExists {
		return nil
	}

	oldSchemaRef, oldIsRef := oldField.Schema.(schema.NestedSchemaReference)
	newSchemaRef, newIsRef := newField.Schema.(schema.NestedSchemaReference)

	if !oldIsRef || !newIsRef || oldSchemaRef.ID != nestedSchemaID {
		return nil
	}

	generator := NewMigrationGenerator(GeneratorOptions{})
	reverseIndexChanges := generator.compareIndexes(newSchemaRef.Indexes, oldSchemaRef.Indexes)
	reverseConstraintChanges := generator.compareConstraints(newSchemaRef.Constraints, oldSchemaRef.Constraints)

	if len(reverseIndexChanges) == 0 && len(reverseConstraintChanges) == 0 {
		return nil
	}

	reverseSubChanges := make([]schema.SchemaChange, 0, len(reverseIndexChanges)+len(reverseConstraintChanges))
	reverseSubChanges = append(reverseSubChanges, reverseIndexChanges...)
	reverseSubChanges = append(reverseSubChanges, reverseConstraintChanges...)

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifySchemaReference,
		ID:   utils.StringPtr(nestedSchemaID),
		SchemaChangeModifySchemaReferencePayload: &schema.SchemaChangeModifySchemaReferencePayload{
			Field:   fieldName,
			ID:      utils.StringPtr(nestedSchemaID),
			Changes: reverseSubChanges,
		},
	}
}

// rollbackModifyProperty generates rollback for property modification.
func (r *RollbackGenerator) rollbackModifyProperty(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	var oldValue any
	switch *change.ID {
	case "description":
		oldValue = oldSchema.Description
	case "metadata":
		oldValue = oldSchema.Metadata
	}

	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyProperty,
		ID:   change.ID,
		SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{
			Value: oldValue,
		},
	}
}

// Helper functions

// generateChecksum generates a checksum for a migration.
func generateChecksum(migration *schema.Migration) (string, error) {
	payload := map[string]any{
		"id":          migration.ID,
		"version":     migration.Version,
		"changes":     migration.Changes,
		"description": migration.Description,
		"rollback":    migration.Rollback,
		"createdAt":   migration.CreatedAt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// sortSchemasByVersion sorts schemas by version number.
func sortSchemasByVersion(schemas []*schema.SchemaDefinition) []*schema.SchemaDefinition {
	sorted := make([]*schema.SchemaDefinition, len(schemas))
	copy(sorted, schemas)

	sort.Slice(sorted, func(i, j int) bool {
		return common.MustNewVersion(sorted[i].Version).Compare(
			common.MustNewVersion(sorted[j].Version),
		) < 0
	})

	return sorted
}

// buildIndexMap creates a map of index names to definitions.
func buildIndexMap(indexes []schema.IndexOrReference) map[string]*schema.IndexDefinition {
	indexMap := make(map[string]*schema.IndexDefinition)
	for _, ior := range indexes {
		if ior.IsIndex() {
			indexMap[ior.Index.Name] = ior.Index
		}
	}
	return indexMap
}

// buildConstraintMap creates a map of constraint names to rules.
func buildConstraintMap(constraints schema.SchemaConstraint) map[string]*schema.ConstraintRule {
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

// buildConstraintRuleMap creates a map from constraint rules slice.
func buildConstraintRuleMap(rules schema.SchemaConstraint) map[string]schema.ConstraintRule {
	ruleMap := make(map[string]schema.ConstraintRule)
	for _, rule := range rules {
		if rule.IsConstraint() {
			ruleMap[rule.Constraint.Name] = rule
		} else if rule.IsConstraintGroup() {
			ruleMap[rule.ConstraintGroup.Name] = rule
		}
	}
	return ruleMap
}

// findIndexInSchema finds an index by name in a schema.
func findIndexInSchema(s *schema.SchemaDefinition, name string) *schema.IndexDefinition {
	for _, ior := range s.Indexes {
		if ior.IsIndex() && ior.Index.Name == name {
			return ior.Index
		}
	}
	return nil
}

// findConstraintInSchema finds a constraint by name in a schema.
func findConstraintInSchema(s *schema.SchemaDefinition, name string) *schema.ConstraintRule {
	for i := range s.Constraints {
		rule := &s.Constraints[i]
		var ruleName string
		if rule.IsConstraint() {
			ruleName = rule.Constraint.Name
		} else if rule.IsConstraintGroup() {
			ruleName = rule.ConstraintGroup.Name
		}
		if ruleName == name {
			return rule
		}
	}
	return nil
}

// Comparison helper functions

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func fieldTypePtrEqual(a, b *schema.FieldType) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
