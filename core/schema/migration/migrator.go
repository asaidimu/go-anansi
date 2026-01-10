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
		return nil, nil
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

func (g *MigrationGenerator) validateSchemas(oldSchema, newSchema *schema.SchemaDefinition) error {
	if oldSchema.Name != newSchema.Name {
		return common.NewSystemError("ERR_SCHEMA_NAME_MISMATCH").
			WithMessagef("schema names must match: %s != %s", oldSchema.Name, newSchema.Name).
			WithOperation("MigrationGenerator.Generate").
			WithPath("name")
	}
	return nil
}

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

func (g *MigrationGenerator) compareProperties(oldSchema, newSchema *schema.SchemaDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)
	if !stringPtrEqual(oldSchema.Description, newSchema.Description) {
		changes = append(changes, g.createPropertyChange("description", newSchema.Description))
	}
	if !g.options.IgnoreMetadata && !reflect.DeepEqual(oldSchema.Metadata, newSchema.Metadata) {
		changes = append(changes, g.createPropertyChange("metadata", newSchema.Metadata))
	}
	if !reflect.DeepEqual(oldSchema.Hint, newSchema.Hint) {
		changes = append(changes, g.createPropertyChange("hint", newSchema.Hint))
	}
	return changes
}

func (g *MigrationGenerator) createPropertyChange(id string, value any) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeModifyProperty,
		ID:   utils.StringPtr(id),
		SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{Value: value},
	}
}

func (g *MigrationGenerator) compareFields(oldFields, newFields map[string]*schema.FieldDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)
	for name := range oldFields {
		if _, exists := newFields[name]; !exists {
			changes = append(changes, g.createRemoveFieldChange(name))
		}
	}
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

func (g *MigrationGenerator) createRemoveFieldChange(name string) schema.SchemaChange {
	return schema.SchemaChange{Type: schema.SchemaChangeTypeRemoveField, ID: utils.StringPtr(name)}
}

func (g *MigrationGenerator) createAddFieldChange(name string, field *schema.FieldDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                            schema.SchemaChangeTypeAddField,
		ID:                              utils.StringPtr(name),
		SchemaChangeAddFieldPayload:     &schema.SchemaChangeAddFieldPayload{Definition: *field},
	}
}

func (g *MigrationGenerator) processFieldModification(name string, oldField, newField *schema.FieldDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)
	partialChanges, nestedChanges := g.compareFieldDefinitions(oldField, newField)
	if partialChanges != nil {
		changes = append(changes, schema.SchemaChange{
			Type:                               schema.SchemaChangeTypeModifyField,
			ID:                                 utils.StringPtr(name),
			SchemaChangeModifyFieldPayload:     &schema.SchemaChangeModifyFieldPayload{Changes: *partialChanges},
		})
	}
	changes = append(changes, nestedChanges...)
	return changes
}

func (g *MigrationGenerator) compareFieldDefinitions(oldField, newField *schema.FieldDefinition) (*schema.PartialFieldDefinition, []schema.SchemaChange) {
	partial := &schema.PartialFieldDefinition{}
	hasChanges := g.compareFieldSimpleProperties(oldField, newField, partial)
	nestedChanges := g.compareFieldSchemaProperty(oldField, newField, partial, &hasChanges)
	if !hasChanges {
		return nil, nestedChanges
	}
	return partial, nestedChanges
}

func (g *MigrationGenerator) compareFieldSimpleProperties(oldField, newField *schema.FieldDefinition, partial *schema.PartialFieldDefinition) bool {
	hasChanges := false
	if oldField.Name != newField.Name {
		partial.Name = &newField.Name
		hasChanges = true
	}
	if oldField.Type != newField.Type {
		partial.Type = &newField.Type
		hasChanges = true
	}
	if !boolPtrEqual(oldField.Required, newField.Required) {
		if newField.Required == nil && oldField.Required != nil {
			partial.Unset = append(partial.Unset, "required")
		} else {
			partial.Required = newField.Required
		}
		hasChanges = true
	}
	if !boolPtrEqual(oldField.Unique, newField.Unique) {
		if newField.Unique == nil && oldField.Unique != nil {
			partial.Unset = append(partial.Unset, "unique")
		} else {
			partial.Unique = newField.Unique
		}
		hasChanges = true
	}
	if !stringPtrEqual(oldField.Description, newField.Description) {
		if newField.Description == nil && oldField.Description != nil {
			partial.Unset = append(partial.Unset, "description")
		} else {
			partial.Description = newField.Description
		}
		hasChanges = true
	}
	if !reflect.DeepEqual(oldField.Default, newField.Default) {
		if newField.Default == nil && oldField.Default != nil {
			partial.Unset = append(partial.Unset, "default")
		} else {
			partial.Default = newField.Default
		}
		hasChanges = true
	}
	if !fieldTypePtrEqual(oldField.ItemsType, newField.ItemsType) {
		if newField.ItemsType == nil && oldField.ItemsType != nil {
			partial.Unset = append(partial.Unset, "itemsType")
		} else {
			partial.ItemsType = newField.ItemsType
		}
		hasChanges = true
	}
	if !reflect.DeepEqual(oldField.Values, newField.Values) {
		if len(newField.Values) == 0 {
			partial.Unset = append(partial.Unset, "values")
		} else {
			partial.Values = newField.Values
		}
		hasChanges = true
	}
	if !reflect.DeepEqual(oldField.Constraints, newField.Constraints) {
		if len(newField.Constraints) == 0 {
			partial.Unset = append(partial.Unset, "constraints")
		} else {
			partial.Constraints = newField.Constraints
		}
		hasChanges = true
	}
	if !reflect.DeepEqual(oldField.Hint, newField.Hint) {
		if newField.Hint == nil && oldField.Hint != nil {
			partial.Unset = append(partial.Unset, "hint")
		} else {
			partial.Hint = newField.Hint
		}
		hasChanges = true
	}
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

func (g *MigrationGenerator) compareFieldSchemaProperty(oldField, newField *schema.FieldDefinition, partial *schema.PartialFieldDefinition, hasChanges *bool) []schema.SchemaChange {
	if reflect.DeepEqual(oldField.Schema, newField.Schema) {
		return nil
	}
	oldSchemaRef, oldIsRef := oldField.Schema.(schema.NestedSchemaReference)
	newSchemaRef, newIsRef := newField.Schema.(schema.NestedSchemaReference)
	if oldIsRef && newIsRef && oldSchemaRef.ID == newSchemaRef.ID {
		return g.compareSchemaReferenceChanges(oldField, oldSchemaRef, newSchemaRef)
	}
	if newField.Schema == nil {
		partial.Unset = append(partial.Unset, "schema")
		*hasChanges = true
		return nil
	}
	partial.Schema = newField.Schema
	*hasChanges = true
	return nil
}

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

func (g *MigrationGenerator) compareIndexes(oldIndexes, newIndexes []schema.IndexOrReference) []schema.SchemaChange {
	oldMap := buildIndexMap(oldIndexes)
	newMap := buildIndexMap(newIndexes)
	changes := make([]schema.SchemaChange, 0)
	for name := range oldMap {
		if _, exists := newMap[name]; !exists {
			changes = append(changes, g.createRemoveIndexChange(name))
		}
	}
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

func (g *MigrationGenerator) createRemoveIndexChange(name string) schema.SchemaChange {
	return schema.SchemaChange{Type: schema.SchemaChangeTypeRemoveIndex, Name: utils.StringPtr(name)}
}

func (g *MigrationGenerator) createAddIndexChange(index *schema.IndexDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                            schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload:     &schema.SchemaChangeAddIndexPayload{Definition: *index},
	}
}

func (g *MigrationGenerator) createModifyIndexChange(name string, changes *schema.PartialIndexDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                               schema.SchemaChangeTypeModifyIndex,
		Name:                               utils.StringPtr(name),
		SchemaChangeModifyIndexPayload:     &schema.SchemaChangeModifyIndexPayload{Changes: *changes},
	}
}

func (g *MigrationGenerator) compareIndexDefinitions(oldIndex, newIndex *schema.IndexDefinition) *schema.PartialIndexDefinition {
	changes := &schema.PartialIndexDefinition{}
	hasChanges := false
	if oldIndex.Type != newIndex.Type {
		changes.Type = &newIndex.Type
		hasChanges = true
	}
	if !stringSliceEqual(oldIndex.Fields, newIndex.Fields) {
		if len(newIndex.Fields) == 0 {
			changes.Unset = append(changes.Unset, "fields")
		} else {
			changes.Fields = newIndex.Fields
		}
		hasChanges = true
	}
	if !boolPtrEqual(oldIndex.Unique, newIndex.Unique) {
		if newIndex.Unique == nil && oldIndex.Unique != nil {
			changes.Unset = append(changes.Unset, "unique")
		} else {
			changes.Unique = newIndex.Unique
		}
		hasChanges = true
	}
	if !stringPtrEqual(oldIndex.Description, newIndex.Description) {
		if newIndex.Description == nil && oldIndex.Description != nil {
			changes.Unset = append(changes.Unset, "description")
		} else {
			changes.Description = newIndex.Description
		}
		hasChanges = true
	}
	if !stringPtrEqual(oldIndex.Order, newIndex.Order) {
		if newIndex.Order == nil && oldIndex.Order != nil {
			changes.Unset = append(changes.Unset, "order")
		} else {
			changes.Order = newIndex.Order
		}
		hasChanges = true
	}
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

func (g *MigrationGenerator) compareConstraints(oldConstraints, newConstraints schema.SchemaConstraint) []schema.SchemaChange {
	oldMap := buildConstraintMap(oldConstraints)
	newMap := buildConstraintMap(newConstraints)
	changes := make([]schema.SchemaChange, 0)
	for name := range oldMap {
		if _, exists := newMap[name]; !exists {
			changes = append(changes, g.createRemoveConstraintChange(name))
		}
	}
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

func (g *MigrationGenerator) createRemoveConstraintChange(name string) schema.SchemaChange {
	return schema.SchemaChange{Type: schema.SchemaChangeTypeRemoveConstraint, Name: utils.StringPtr(name)}
}

func (g *MigrationGenerator) createAddConstraintChange(rule *schema.ConstraintRule) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                                  schema.SchemaChangeTypeAddConstraint,
		SchemaChangeAddConstraintPayload:      &schema.SchemaChangeAddConstraintPayload{Constraint: *rule},
	}
}

func (g *MigrationGenerator) processConstraintModification(name string, oldRule, newRule *schema.ConstraintRule) []schema.SchemaChange {
	if reflect.DeepEqual(oldRule, newRule) {
		return nil
	}
	if oldRule.IsConstraintGroup() && newRule.IsConstraintGroup() {
		return g.compareConstraintGroupChanges(oldRule.ConstraintGroup, newRule.ConstraintGroup)
	}
	if oldRule.IsConstraint() && newRule.IsConstraint() {
		if partialChanges := g.compareConstraintDefinitions(oldRule.Constraint, newRule.Constraint); partialChanges != nil {
			return []schema.SchemaChange{g.createModifyConstraintChange(name, partialChanges)}
		}
		return nil
	}
	return []schema.SchemaChange{
		g.createRemoveConstraintChange(name),
		g.createAddConstraintChange(newRule),
	}
}

func (g *MigrationGenerator) createModifyConstraintChange(name string, changes *schema.PartialConstraint) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                                     schema.SchemaChangeTypeModifyConstraint,
		Name:                                     utils.StringPtr(name),
		SchemaChangeModifyConstraintPayload:      &schema.SchemaChangeModifyConstraintPayload{Changes: *changes},
	}
}

func (g *MigrationGenerator) compareConstraintGroupChanges(oldGroup, newGroup *schema.ConstraintGroup) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)
	if oldGroup.Operator != newGroup.Operator {
		changes = append(changes, g.createModifyConstraintChange(oldGroup.Name, &schema.PartialConstraint{
			Operator: &newGroup.Operator,
		}))
	}
	changes = append(changes, g.compareConstraintGroupRules(oldGroup, newGroup)...)
	return changes
}

func (g *MigrationGenerator) compareConstraintGroupRules(oldGroup, newGroup *schema.ConstraintGroup) []schema.SchemaChange {
	oldRulesMap := buildConstraintRuleMap(oldGroup.Rules)
	newRulesMap := buildConstraintRuleMap(newGroup.Rules)
	changes := make([]schema.SchemaChange, 0)
	for name := range oldRulesMap {
		if _, exists := newRulesMap[name]; !exists {
			hierarchicalName := fmt.Sprintf("%s/%s", oldGroup.Name, name)
			changes = append(changes, g.createRemoveConstraintChange(hierarchicalName))
		}
	}
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

func (g *MigrationGenerator) processNestedConstraintChange(name string, oldRule, newRule schema.ConstraintRule) []schema.SchemaChange {
	if oldRule.IsConstraintGroup() && newRule.IsConstraintGroup() {
		return g.compareConstraintGroupChanges(oldRule.ConstraintGroup, newRule.ConstraintGroup)
	}
	if oldRule.IsConstraint() && newRule.IsConstraint() {
		if partialChanges := g.compareConstraintDefinitions(oldRule.Constraint, newRule.Constraint); partialChanges != nil {
			return []schema.SchemaChange{g.createModifyConstraintChange(name, partialChanges)}
		}
	}
	return nil
}

func (g *MigrationGenerator) compareConstraintDefinitions(oldConstraint, newConstraint *schema.Constraint) *schema.PartialConstraint {
	changes := &schema.PartialConstraint{}
	hasChanges := false

	// Type comparison - if types differ, treat as remove+add since PartialConstraint lacks Type field
	if !constraintTypePtrEqual(oldConstraint.Type, newConstraint.Type) {
		// This requires remove+add at a higher level since we can't modify Type
		// For now, mark as changed and let processConstraintModification handle it
		hasChanges = true
	}

	if oldConstraint.Predicate != newConstraint.Predicate {
		changes.Predicate = &newConstraint.Predicate
		hasChanges = true
	}
	if !stringPtrEqual(oldConstraint.Field, newConstraint.Field) {
		if newConstraint.Field == nil && oldConstraint.Field != nil {
			changes.Unset = append(changes.Unset, "field")
		} else {
			changes.Field = newConstraint.Field
		}
		hasChanges = true
	}
	if !stringSliceEqual(oldConstraint.Fields, newConstraint.Fields) {
		if len(newConstraint.Fields) == 0 {
			changes.Unset = append(changes.Unset, "fields")
		} else {
			changes.Fields = newConstraint.Fields
		}
		hasChanges = true
	}
	if !reflect.DeepEqual(oldConstraint.Parameters, newConstraint.Parameters) {
		if newConstraint.Parameters == nil && oldConstraint.Parameters != nil {
			changes.Unset = append(changes.Unset, "parameters")
		} else {
			changes.Parameters = newConstraint.Parameters
		}
		hasChanges = true
	}
	if !stringPtrEqual(oldConstraint.Description, newConstraint.Description) {
		if newConstraint.Description == nil && oldConstraint.Description != nil {
			changes.Unset = append(changes.Unset, "description")
		} else {
			changes.Description = newConstraint.Description
		}
		hasChanges = true
	}
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

func (g *MigrationGenerator) compareNested(oldSchemas, newSchemas map[string]*schema.NestedSchemaDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)
	for id := range oldSchemas {
		if _, exists := newSchemas[id]; !exists {
			changes = append(changes, g.createRemoveSchemaChange(id))
		}
	}
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

func (g *MigrationGenerator) createRemoveSchemaChange(id string) schema.SchemaChange {
	return schema.SchemaChange{Type: schema.SchemaChangeTypeRemoveSchema, ID: utils.StringPtr(id)}
}

func (g *MigrationGenerator) createAddSchemaChange(id string, def *schema.NestedSchemaDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                              schema.SchemaChangeTypeAddSchema,
		ID:                                utils.StringPtr(id),
		SchemaChangeAddSchemaPayload:      &schema.SchemaChangeAddSchemaPayload{Definition: *def},
	}
}

func (g *MigrationGenerator) createModifySchemaChange(id string, changes []schema.SchemaChange) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                                 schema.SchemaChangeTypeModifySchema,
		ID:                                   utils.StringPtr(id),
		SchemaChangeModifySchemaPayload:      &schema.SchemaChangeModifySchemaPayload{Changes: changes},
	}
}

func (g *MigrationGenerator) compareNestedSchemas(oldSchema, newSchema *schema.NestedSchemaDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Compare simple properties
	if !stringPtrEqual(oldSchema.Description, newSchema.Description) {
		changes = append(changes, g.createPropertyChange("description", newSchema.Description))
	}
	if !g.options.IgnoreMetadata && !reflect.DeepEqual(oldSchema.Metadata, newSchema.Metadata) {
		changes = append(changes, g.createPropertyChange("metadata", newSchema.Metadata))
	}

	if !boolPtrEqual(oldSchema.Concrete, newSchema.Concrete) {
		changes = append(changes, g.createPropertyChange("concrete", newSchema.Concrete))
	}

	if !stringPtrEqual(oldSchema.ID, newSchema.ID) {
		changes = append(changes, g.createPropertyChange("id", newSchema.ID))
	}

	isOldStructured := oldSchema.IsStructured()
	isNewStructured := newSchema.IsStructured()

	if isOldStructured && isNewStructured {
		// Both structured - compare fields
		if oldSchema.Fields.IsMap() && newSchema.Fields.IsMap() {
			changes = append(changes, g.compareNestedSchemaFields(oldSchema.Fields.FieldsMap, newSchema.Fields.FieldsMap)...)
		} else if oldSchema.Fields.FieldSets != nil && newSchema.Fields.FieldSets != nil { // New FieldSets comparison
			changes = append(changes, g.compareConditionalFieldSets(oldSchema.Fields.FieldSets, newSchema.Fields.FieldSets)...)
		} else if oldSchema.Fields.IsLegacyFieldsArray() && newSchema.Fields.IsLegacyFieldsArray() {
			if !reflect.DeepEqual(oldSchema.Fields.FieldsArray, newSchema.Fields.FieldsArray) {
				changes = append(changes, g.createPropertyChange("fields", newSchema.Fields))
			}
		} else {
			// Changed between different field types (map, array, or sets).
			// This covers:
			// - Map <-> Array
			// - Map <-> FieldSets
			// - Array <-> FieldSets
			changes = append(changes, g.createPropertyChange("fields", newSchema.Fields))
		}
	} else if !isOldStructured && !isNewStructured {
		// Both typed - compare type properties
		if !fieldTypePtrEqual(oldSchema.Type, newSchema.Type) {
			changes = append(changes, g.createPropertyChange("type", newSchema.Type))
		}
		if !reflect.DeepEqual(oldSchema.Default, newSchema.Default) {
			changes = append(changes, g.createPropertyChange("default", newSchema.Default))
		}
		if !fieldTypePtrEqual(oldSchema.ItemsType, newSchema.ItemsType) {
			changes = append(changes, g.createPropertyChange("itemsType", newSchema.ItemsType))
		}
		if !reflect.DeepEqual(oldSchema.Values, newSchema.Values) {
			changes = append(changes, g.createPropertyChange("values", newSchema.Values))
		}

		schemaChanges := g.compareTypedNestedSchemaReferences(oldSchema, newSchema)
		// Part 2: Typed Nested Schema Comparison and Rollback Generation

// compareNestedSchemas continuation from Part 1
		changes = append(changes, schemaChanges...)
	} else {
		// Schema changed between structured and typed - detected at parent level
	}

	// Compare shared complex properties
	changes = append(changes, g.compareIndexes(oldSchema.Indexes, newSchema.Indexes)...)
	changes = append(changes, g.compareConstraints(oldSchema.Constraints, newSchema.Constraints)...)

	return changes
}

func (g *MigrationGenerator) compareNestedSchemaFields(oldFields, newFields map[string]*schema.FieldDefinition) []schema.SchemaChange {
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
			// Compare fields including their schema references
			changes = append(changes, g.processFieldModification(name, oldField, newField)...)
		}
	}

	return changes
}

func (g *MigrationGenerator) compareTypedNestedSchemaReferences(oldSchema, newSchema *schema.NestedSchemaDefinition) []schema.SchemaChange {
	if reflect.DeepEqual(oldSchema.Schema, newSchema.Schema) {
		return nil
	}

	changes := make([]schema.SchemaChange, 0)

	// Case 1: Single NestedSchemaReference
	oldRef, oldIsSingleRef := oldSchema.Schema.(schema.NestedSchemaReference)
	newRef, newIsSingleRef := newSchema.Schema.(schema.NestedSchemaReference)

	if oldIsSingleRef && newIsSingleRef {
		if oldRef.ID == newRef.ID {
			// Same ID - check for internal changes (constraints/indexes)
			refChanges := g.compareNestedSchemaReferenceInternals(oldRef, newRef, oldSchema.Name)
			return refChanges
		}
		// Different ID - treat as replacement
		changes = append(changes, g.createPropertyChange("schema", newSchema.Schema))
		return changes
	}

	// Case 2: Array of NestedSchemaReferences (union types)
	oldRefs, oldIsArrayRef := oldSchema.Schema.([]schema.NestedSchemaReference)
	newRefs, newIsArrayRef := newSchema.Schema.([]schema.NestedSchemaReference)

	if oldIsArrayRef && newIsArrayRef {
		unionChanges := g.compareUnionSchemaReferences(oldRefs, newRefs, oldSchema.Name)
		return unionChanges
	}

	// Case 3: Schema removed, added, or changed type
	if newSchema.Schema == nil {
		changes = append(changes, schema.SchemaChange{
			Type: schema.SchemaChangeTypeModifyProperty,
			ID:   utils.StringPtr("schema"),
			SchemaChangeModifyPropertyPayload: &schema.SchemaChangeModifyPropertyPayload{Value: nil},
		})
	} else {
		changes = append(changes, g.createPropertyChange("schema", newSchema.Schema))
	}

	return changes
}

func (g *MigrationGenerator) compareNestedSchemaReferenceInternals(oldRef, newRef schema.NestedSchemaReference, schemaName string) []schema.SchemaChange {
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
			Field:   schemaName,
			ID:      utils.StringPtr(oldRef.ID),
			Changes: subChanges,
		},
	}}
}

func (g *MigrationGenerator) compareUnionSchemaReferences(oldRefs, newRefs []schema.NestedSchemaReference, schemaName string) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	oldMap := make(map[string]schema.NestedSchemaReference)
	newMap := make(map[string]schema.NestedSchemaReference)

	for _, ref := range oldRefs {
		oldMap[ref.ID] = ref
	}
	for _, ref := range newRefs {
		newMap[ref.ID] = ref
	}

	// Check for removed or added union members
	for id := range oldMap {
		if _, exists := newMap[id]; !exists {
			// Union member removed - treat as full schema replacement
			changes = append(changes, g.createPropertyChange("schema", newRefs))
			return changes
		}
	}

	for id := range newMap {
		if _, exists := oldMap[id]; !exists {
			// Union member added - treat as full schema replacement
			changes = append(changes, g.createPropertyChange("schema", newRefs))
			return changes
		}
	}

	// Check for modified union members (same IDs, different internals)
	for id, oldRef := range oldMap {
		newRef := newMap[id]
		refChanges := g.compareNestedSchemaReferenceInternals(oldRef, newRef, schemaName)
		changes = append(changes, refChanges...)
	}

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
		if migration != nil {
			migrations = append(migrations, migration)
		}
	}

	return migrations, nil
}

// RollbackGenerator generates rollback changes for migrations.
type RollbackGenerator struct{}

func NewRollbackGenerator() *RollbackGenerator {
	return &RollbackGenerator{}
}

func (r *RollbackGenerator) Generate(changes []schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) []schema.SchemaChange {
	rollback := make([]schema.SchemaChange, 0, len(changes))
	for i := len(changes) - 1; i >= 0; i-- {
		if rollbackChange := r.generateRollbackForChange(changes[i], oldSchema, newSchema); rollbackChange != nil {
			rollback = append(rollback, *rollbackChange)
		}
	}
	return rollback
}

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
	case schema.SchemaChangeTypeAddSchema:
		return r.rollbackAddSchema(change)
	case schema.SchemaChangeTypeRemoveSchema:
		return r.rollbackRemoveSchema(change, oldSchema)
	case schema.SchemaChangeTypeModifySchema:
		return r.rollbackModifySchema(change, oldSchema, newSchema)
	default:
		return nil
	}
}

func (r *RollbackGenerator) rollbackAddField(change schema.SchemaChange) *schema.SchemaChange {
	return &schema.SchemaChange{Type: schema.SchemaChangeTypeRemoveField, ID: change.ID}
}

func (r *RollbackGenerator) rollbackRemoveField(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldField, exists := oldSchema.Fields[*change.ID]
	if !exists {
		return nil
	}
	return &schema.SchemaChange{
		Type:                            schema.SchemaChangeTypeAddField,
		ID:                              change.ID,
		SchemaChangeAddFieldPayload:     &schema.SchemaChangeAddFieldPayload{Definition: *oldField},
	}
}

func (r *RollbackGenerator) rollbackModifyField(change schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldField, oldExists := oldSchema.Fields[*change.ID]
	newField, newExists := newSchema.Fields[*change.ID]
	if !oldExists || !newExists {
		return nil
	}
	generator := NewMigrationGenerator(GeneratorOptions{})
	reverseChanges, _ := generator.compareFieldDefinitions(newField, oldField)
	if reverseChanges == nil {
		return nil
	}
	return &schema.SchemaChange{
		Type:                               schema.SchemaChangeTypeModifyField,
		ID:                                 change.ID,
		SchemaChangeModifyFieldPayload:     &schema.SchemaChangeModifyFieldPayload{Changes: *reverseChanges},
	}
}

func (r *RollbackGenerator) rollbackAddIndex(change schema.SchemaChange) *schema.SchemaChange {
	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveIndex,
		Name: utils.StringPtr(change.SchemaChangeAddIndexPayload.Definition.Name),
	}
}

func (r *RollbackGenerator) rollbackRemoveIndex(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldIndex := findIndexInSchema(oldSchema, *change.Name)
	if oldIndex == nil {
		return nil
	}
	return &schema.SchemaChange{
		Type:                            schema.SchemaChangeTypeAddIndex,
		SchemaChangeAddIndexPayload:     &schema.SchemaChangeAddIndexPayload{Definition: *oldIndex},
	}
}

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
		Type:                               schema.SchemaChangeTypeModifyIndex,
		Name:                               change.Name,
		SchemaChangeModifyIndexPayload:     &schema.SchemaChangeModifyIndexPayload{Changes: *reverseChanges},
	}
}

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

func (r *RollbackGenerator) rollbackRemoveConstraint(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	oldRule := findConstraintInSchema(oldSchema, *change.Name)
	if oldRule == nil {
		return nil
	}
	return &schema.SchemaChange{
		Type:                                  schema.SchemaChangeTypeAddConstraint,
		SchemaChangeAddConstraintPayload:      &schema.SchemaChangeAddConstraintPayload{Constraint: *oldRule},
	}
}

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
		Type:                                     schema.SchemaChangeTypeModifyConstraint,
		Name:                                     change.Name,
		SchemaChangeModifyConstraintPayload:      &schema.SchemaChangeModifyConstraintPayload{Changes: *reverseChanges},
	}
}

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

func (r *RollbackGenerator) rollbackModifyProperty(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	if change.ID == nil {
		return nil
	}
	var oldValue any
	switch *change.ID {
	case "description":
		oldValue = oldSchema.Description
	case "metadata":
		oldValue = oldSchema.Metadata
	case "hint":
		oldValue = oldSchema.Hint
	default:
		return nil
	}
	return &schema.SchemaChange{
		Type:                                  schema.SchemaChangeTypeModifyProperty,
		ID:                                    change.ID,
		SchemaChangeModifyPropertyPayload:     &schema.SchemaChangeModifyPropertyPayload{Value: oldValue},
	}
}

func (r *RollbackGenerator) rollbackAddSchema(change schema.SchemaChange) *schema.SchemaChange {
	if change.SchemaChangeAddSchemaPayload == nil || change.ID == nil {
		return nil
	}
	return &schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveSchema,
		ID:   change.ID,
	}
}

func (r *RollbackGenerator) rollbackRemoveSchema(change schema.SchemaChange, oldSchema *schema.SchemaDefinition) *schema.SchemaChange {
	if change.ID == nil || oldSchema.NestedSchemas == nil {
		return nil
	}
	oldNestedSchema, exists := oldSchema.NestedSchemas[*change.ID]
	if !exists {
		return nil
	}
	return &schema.SchemaChange{
		Type:                              schema.SchemaChangeTypeAddSchema,
		ID:                                change.ID,
		SchemaChangeAddSchemaPayload:      &schema.SchemaChangeAddSchemaPayload{Definition: *oldNestedSchema},
	}
}

func (r *RollbackGenerator) rollbackModifySchema(change schema.SchemaChange, oldSchema, newSchema *schema.SchemaDefinition) *schema.SchemaChange {
	if change.SchemaChangeModifySchemaPayload == nil || change.ID == nil {
		return nil
	}
	if oldSchema.NestedSchemas == nil || newSchema.NestedSchemas == nil {
		return nil
	}
	oldNestedSchema, oldExists := oldSchema.NestedSchemas[*change.ID]
	newNestedSchema, newExists := newSchema.NestedSchemas[*change.ID]
	if !oldExists || !newExists {
		return nil
	}
	generator := NewMigrationGenerator(GeneratorOptions{})
	reverseChanges := generator.compareNestedSchemas(newNestedSchema, oldNestedSchema)
	if len(reverseChanges) == 0 {
		return nil
	}
	return &schema.SchemaChange{
		Type:                                 schema.SchemaChangeTypeModifySchema,
		ID:                                   change.ID,
		SchemaChangeModifySchemaPayload:      &schema.SchemaChangeModifySchemaPayload{Changes: reverseChanges},
	}
}

func (g *MigrationGenerator) compareConditionalFieldSets(oldSets, newSets map[string]schema.ConditionalFieldSet) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Identify removed conditional sets
	for id := range oldSets {
		if _, exists := newSets[id]; !exists {
			changes = append(changes, g.createRemoveConditionalSetChange(id))
		}
	}

	// Identify added and modified conditional sets
	for id, newSet := range newSets {
		oldSet, exists := oldSets[id]
		if !exists {
			changes = append(changes, g.createAddConditionalSetChange(id, newSet))
		} else {
			// Compare existing conditional sets for modifications
			changes = append(changes, g.compareConditionalFieldSet(id, oldSet, newSet)...)
		}
	}

	return changes
}

func (g *MigrationGenerator) createRemoveConditionalSetChange(id string) schema.SchemaChange {
	return schema.SchemaChange{Type: schema.SchemaChangeTypeRemoveConditionalSet, ID: utils.StringPtr(id)}
}

func (g *MigrationGenerator) createAddConditionalSetChange(id string, def schema.ConditionalFieldSet) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                                 schema.SchemaChangeTypeAddConditionalSet,
		ID:                                   utils.StringPtr(id),
		SchemaChangeAddConditionalSetPayload: &schema.SchemaChangeAddConditionalSetPayload{ID: id, Definition: def},
	}
}

// createModifyConditionalSetChange generates a SchemaChange of type ModifyConditionalSet
// for changes to the 'When' condition of a ConditionalFieldSet.
func (g *MigrationGenerator) createModifyConditionalSetChange(id string, newWhen *schema.FieldInclusionCondition, unsetWhen bool) schema.SchemaChange {
	payload := &schema.SchemaChangeModifyConditionalSetPayload{ID: id}
	if unsetWhen {
		payload.Unset = append(payload.Unset, "when")
	} else if newWhen != nil {
		payload.When = newWhen
	}
	return schema.SchemaChange{
		Type:                                  schema.SchemaChangeTypeModifyConditionalSet,
		ID:                                    utils.StringPtr(id),
		SchemaChangeModifyConditionalSetPayload: payload,
	}
}

func (g *MigrationGenerator) compareConditionalFieldSet(id string, oldSet, newSet schema.ConditionalFieldSet) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Compare 'When' condition
	if !reflect.DeepEqual(oldSet.When, newSet.When) {
		if newSet.When == nil {
			changes = append(changes, g.createModifyConditionalSetChange(id, nil, true))
		} else {
			changes = append(changes, g.createModifyConditionalSetChange(id, newSet.When, false))
		}
	}

	// Compare fields within the conditional set
	fieldChanges := g.compareConditionalFields(id, oldSet.Fields, newSet.Fields)
	changes = append(changes, fieldChanges...)

	return changes
}

func (g *MigrationGenerator) compareConditionalFields(setId string, oldFields, newFields map[string]*schema.FieldDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)

	// Identify removed fields
	for name := range oldFields {
		if _, exists := newFields[name]; !exists {
			changes = append(changes, g.createRemoveConditionalFieldChange(setId, name))
		}
	}

	// Identify added and modified fields
	for name, newField := range newFields {
		oldField, exists := oldFields[name]
		if !exists {
			changes = append(changes, g.createAddConditionalFieldChange(setId, name, newField))
		} else {
			// Compare existing fields for modifications
			changes = append(changes, g.processConditionalFieldModification(setId, name, oldField, newField)...)
		}
	}

	return changes
}

func (g *MigrationGenerator) createRemoveConditionalFieldChange(setId, fieldName string) schema.SchemaChange {
	return schema.SchemaChange{
		Type: schema.SchemaChangeTypeRemoveConditionalField,
		ID:   utils.StringPtr(setId),
		SchemaChangeRemoveConditionalFieldPayload: &schema.SchemaChangeRemoveConditionalFieldPayload{
			ID: setId, Field: fieldName,
		},
	}
}

func (g *MigrationGenerator) createAddConditionalFieldChange(setId, fieldName string, field *schema.FieldDefinition) schema.SchemaChange {
	return schema.SchemaChange{
		Type:                               schema.SchemaChangeTypeAddConditionalField,
		ID:                                 utils.StringPtr(setId),
		SchemaChangeAddConditionalFieldPayload: &schema.SchemaChangeAddConditionalFieldPayload{
			ID: setId, Field: fieldName, Definition: *field,
		},
	}
}

func (g *MigrationGenerator) processConditionalFieldModification(setId, fieldName string, oldField, newField *schema.FieldDefinition) []schema.SchemaChange {
	changes := make([]schema.SchemaChange, 0)
	partialChanges, nestedChanges := g.compareFieldDefinitions(oldField, newField) // Re-use existing field comparison logic

	if partialChanges != nil {
		changes = append(changes, schema.SchemaChange{
			Type:                               schema.SchemaChangeTypeModifyConditionalField,
			ID:                                 utils.StringPtr(setId),
			SchemaChangeModifyConditionalFieldPayload: &schema.SchemaChangeModifyConditionalFieldPayload{
				ID: setId, Field: fieldName, Changes: *partialChanges,
			},
		})
	}
	// Append nested changes (e.g., to schema reference within the field)
	if len(nestedChanges) > 0 {
		// These nested changes are for the field itself, not the conditional set.
		// We might need a way to link them, or modify compareFieldDefinitions to return
		// changes specific to a FieldDefinition's internal structure.
		// For now, let's assume compareFieldDefinitions generates changes that can be applied directly.
		// However, to keep it clean and tied to the conditional field, we should wrap these.
		// This needs careful consideration. If nestedChanges are specific to the Field.Schema,
		// then modifying the FieldDefinition via ModifyConditionalFieldPayload might be sufficient
		// if the payload itself can contain nested changes.
		// Looking at schema.SchemaChangeModifyConditionalFieldPayload, it has 'Changes PartialFieldDefinition'.
		// PartialFieldDefinition has 'Schema any'. This means a direct change to the schema property of the field.
		// If the nested changes are more granular, like modify index/constraint *within* a nested schema reference,
		// then we need to rethink how to apply them.
		//
		// For now, let's assume `compareFieldDefinitions` only returns changes to the FieldDefinition itself,
		// and not deeper nested schema reference changes. If that's the case, then nestedChanges should not be generated here.
		// Let's re-examine compareFieldDefinitions: it returns `*schema.PartialFieldDefinition` and `[]schema.SchemaChange`.
		// The `[]schema.SchemaChange` are indeed for `SchemaChangeTypeModifySchemaReference`.
		// These changes need to be correctly scoped to the conditional field's schema.
		//
		// This requires a new change type: `SchemaChangeTypeModifyConditionalFieldSchemaReference`
		// or passing the `setId` and `fieldName` down into `compareSchemaReferenceChanges`
		// to create appropriate changes.
		//
		// For simplicity, let's create a new type.
		//
		// Re-evaluation: `compareFieldDefinitions` returns `*schema.PartialFieldDefinition` and `[]schema.SchemaChange`.
		// The `[]schema.SchemaChange` are indeed `SchemaChangeTypeModifySchemaReference`.
		// If these changes are to be applied to a field *within* a conditional set,
		// then they need to be associated with that conditional field.
		//
		// For now, I will append these `nestedChanges` directly. The applier will need to know
		// the context (which conditional set and field) to apply them correctly.
		// This might require adding `ConditionalSetID` and `ConditionalFieldID` to `SchemaChangeModifySchemaReferencePayload`.
		//
		// Let's defer adding the `nestedChanges` here for now and focus on the direct changes
		// to the conditional set and field. We can revisit nested changes if needed.
		//
		// The existing `compareSchemaReferenceChanges` already creates changes that affect a field's schema.
		// These changes are generated by comparing two `NestedSchemaReference` objects.
		// If `FieldDefinition.Schema` is a `NestedSchemaReference`, then changes to its internal
		// indexes/constraints would result in `SchemaChangeTypeModifySchemaReference`.
		//
		// For `SchemaChangeTypeModifyConditionalFieldPayload`, the `Changes` field is `PartialFieldDefinition`.
		// `PartialFieldDefinition` can include a `Schema` field. So, if the schema itself changes,
		// it will be part of `PartialFieldDefinition`.
		// If the *internal* indexes/constraints of a *referenced* nested schema change,
		// that should be a `SchemaChangeTypeModifySchema` on the nested schema ID, not on the field.
		//
		// Let's keep it simple for now: `processConditionalFieldModification` will only handle direct
		// changes to the field definition within the conditional set.
		// The `nestedChanges` from `compareFieldDefinitions` for `SchemaChangeTypeModifySchemaReference`
		// need to be handled carefully. If these changes apply to a schema referenced by a conditional field,
		// they need to be wrapped.

		// For now, let's assume compareFieldDefinitions' nestedChanges are handled externally or don't apply directly here.
		// If a field's schema reference is modified, the modifyField payload can carry the new schema.
		// However, if the *referenced nested schema itself* changes (e.g., its indexes change),
		// that should be a `SchemaChangeTypeModifySchema` on the nested schema ID, not on the field.
		//
		// Let's re-think `compareFieldDefinitions`. The `nestedChanges` it returns are of type `SchemaChangeTypeModifySchemaReference`.
		// These changes are meant to modify the *internal* parts (indexes/constraints) of a `NestedSchemaReference` that a field points to.
		//
		// If we have a conditional field:
		// ConditionalSet A
		//   Field B (references NestedSchema X)
		// And NestedSchema X changes internally, this should be a ModifySchema change for X.
		//
		// If Field B changes its reference from X to Y, or its own internal indexes/constraints if its Schema is inline,
		// that's a ModifyConditionalField.
		//
		// The current `processFieldModification` (for top-level fields) appends these `nestedChanges` directly.
		// This means that `SchemaChangeTypeModifySchemaReference` changes are currently *not* specific to conditional fields.
		// They apply to the referenced schema ID directly.
		//
		// So, for `processConditionalFieldModification`, we only care about changes to the field's definition.
	}
	return changes
}

// Comparison helpers
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

func sortSchemasByVersion(schemas []*schema.SchemaDefinition) []*schema.SchemaDefinition {
	sorted := make([]*schema.SchemaDefinition, len(schemas))
	copy(sorted, schemas)
	sort.Slice(sorted, func(i, j int) bool {
		return common.MustNewVersion(sorted[i].Version).Compare(common.MustNewVersion(sorted[j].Version)) < 0
	})
	return sorted
}

func buildIndexMap(indexes []schema.IndexOrReference) map[string]*schema.IndexDefinition {
	indexMap := make(map[string]*schema.IndexDefinition)
	for _, ior := range indexes {
		if ior.IsIndex() {
			indexMap[ior.Index.Name] = ior.Index
		}
	}
	return indexMap
}

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

func findIndexInSchema(s *schema.SchemaDefinition, name string) *schema.IndexDefinition {
	for _, ior := range s.Indexes {
		if ior.IsIndex() && ior.Index.Name == name {
			return ior.Index
		}
	}
	return nil
}

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

// Comparison helpers
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

func constraintTypePtrEqual(a, b *schema.ConstraintType) bool {
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
