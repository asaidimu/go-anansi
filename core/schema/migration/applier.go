package migration

import (
	"encoding/json"
	"slices"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// ApplierOptions configures the migration applier behavior.
type ApplierOptions struct {
	ValidateResult bool
	CleanupOrphans bool
	StrictMode     bool
}

// MigrationApplier applies migrations to schemas, producing new versioned schemas.
type MigrationApplier struct {
	options ApplierOptions
}

// NewMigrationApplier creates a new migration applier.
func NewMigrationApplier(options ApplierOptions) *MigrationApplier {
	return &MigrationApplier{
		options: options,
	}
}

// ApplyMigration creates a new schema by applying a migration to a source schema.
// The source schema remains unchanged.
func (m *MigrationApplier) ApplyMigration(
	source *schema.SchemaDefinition,
	migration *schema.Migration,
) (*schema.SchemaDefinition, error) {
	if err := m.validateMigration(source, migration); err != nil {
		return nil, err
	}

	target, err := deepCloneSchema(source)
	if err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_CLONE_FAILED").
			WithMessage("failed to clone schema").
			WithCause(err).
			WithOperation("MigrationApplier.ApplyMigration")
	}

	target.Version = *migration.Version.Target

	if err := m.applyChanges(target, migration.Changes); err != nil {
		return nil, err
	}

	if m.options.CleanupOrphans {
		if err := m.cleanupOrphanedReferences(target); err != nil {
			return nil, common.NewSystemError("ERR_CLEANUP_ORPHANS_FAILED").
				WithMessage("failed to cleanup orphaned references").
				WithCause(err).
				WithOperation("MigrationApplier.ApplyMigration")
		}
	}

	if m.options.ValidateResult {
		if err := m.validateResultingSchema(target); err != nil {
			return nil, common.NewSystemError("ERR_RESULTING_SCHEMA_INVALID").
				WithMessage("resulting schema is invalid").
				WithCause(err).
				WithOperation("MigrationApplier.ApplyMigration")
		}
	}

	return target, nil
}

// validateMigration ensures the migration can be applied to the source schema.
func (m *MigrationApplier) validateMigration(source *schema.SchemaDefinition, migration *schema.Migration) error {
	op := "MigrationApplier.validateMigration"

	if source.Version != migration.Version.Source {
		return common.NewSystemError("ERR_MIGRATION_VERSION_MISMATCH").
			WithMessagef("migration source version %s does not match schema version %s",
				migration.Version.Source, source.Version).
			WithOperation(op)
	}

	if migration.Version.Target == nil {
		return common.NewSystemError("ERR_MIGRATION_INVALID_TARGET_VERSION").
			WithMessage("migration target version is nil").
			WithOperation(op)
	}

	return nil
}

// applyChanges applies all changes sequentially to the target schema.
func (m *MigrationApplier) applyChanges(target *schema.SchemaDefinition, changes []schema.SchemaChange) error {
	for i, change := range changes {
		if err := m.applyChange(target, change); err != nil {
			return common.NewSystemError("ERR_APPLY_CHANGE_FAILED").
				WithMessagef("failed to apply change %d (%s)", i, change.Type).
				WithCause(err).
				WithOperation("MigrationApplier.applyChanges")
		}
	}
	return nil
}

// applyChange applies a single schema change to the target schema.
func (m *MigrationApplier) applyChange(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	switch change.Type {
	case schema.SchemaChangeTypeModifyProperty:
		return m.applyModifyProperty(target, change)
	case schema.SchemaChangeTypeAddField:
		return m.applyAddField(target, change)
	case schema.SchemaChangeTypeRemoveField:
		return m.applyRemoveField(target, change)
	case schema.SchemaChangeTypeModifyField:
		return m.applyModifyField(target, change)
	case schema.SchemaChangeTypeAddIndex:
		return m.applyAddIndex(target, change)
	case schema.SchemaChangeTypeRemoveIndex:
		return m.applyRemoveIndex(target, change)
	case schema.SchemaChangeTypeModifyIndex:
		return m.applyModifyIndex(target, change)
	case schema.SchemaChangeTypeAddConstraint:
		return m.applyAddConstraint(target, change)
	case schema.SchemaChangeTypeRemoveConstraint:
		return m.applyRemoveConstraint(target, change)
	case schema.SchemaChangeTypeModifyConstraint:
		return m.applyModifyConstraint(target, change)
	case schema.SchemaChangeTypeAddSchema:
		return m.applyAddSchema(target, change)
	case schema.SchemaChangeTypeRemoveSchema:
		return m.applyRemoveSchema(target, change)
	case schema.SchemaChangeTypeModifySchema:
		return m.applyModifySchema(target, change)
	case schema.SchemaChangeTypeModifySchemaReference:
		return m.applyModifySchemaReference(target, change)
	default:
		return common.NewSystemError("ERR_UNKNOWN_SCHEMA_CHANGE_TYPE").
			WithMessagef("unknown schema change type: %s", change.Type).
			WithOperation("MigrationApplier.applyChange")
	}
}

// applyModifyProperty modifies a schema-level property.
func (m *MigrationApplier) applyModifyProperty(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifyProperty"

	if change.SchemaChangeModifyPropertyPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifyProperty payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_PROPERTY_ID").
			WithMessage("missing property ID").
			WithOperation(op)
	}

	switch *change.ID {
	case "description":
		target.Description = convertToStringPtr(change.SchemaChangeModifyPropertyPayload.Value)
	case "metadata":
		if metadata, ok := change.SchemaChangeModifyPropertyPayload.Value.(map[string]any); ok {
			target.Metadata = metadata
		} else {
			target.Metadata = nil
		}
	default:
		return common.NewSystemError("ERR_UNKNOWN_PROPERTY").
			WithMessagef("unknown property: %s", *change.ID).
			WithOperation(op)
	}

	return nil
}

// applyAddField adds a new field to the schema.
func (m *MigrationApplier) applyAddField(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyAddField"

	if change.SchemaChangeAddFieldPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing addField payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_FIELD_ID").
			WithMessage("missing field ID").
			WithOperation(op)
	}

	fieldName := *change.ID

	if _, exists := target.Fields[fieldName]; exists && m.options.StrictMode {
		return common.NewSystemError("ERR_FIELD_ALREADY_EXISTS").
			WithMessagef("field %s already exists", fieldName).
			WithOperation(op)
	}

	if target.Fields == nil {
		target.Fields = make(map[string]*schema.FieldDefinition)
	}

	fieldDef := change.SchemaChangeAddFieldPayload.Definition
	clonedField, err := deepCloneFieldDefinition(&fieldDef)
	if err != nil {
		return common.NewSystemError("ERR_FIELD_CLONE_FAILED").
			WithMessage("failed to clone field definition").
			WithCause(err).
			WithOperation(op)
	}

	target.Fields[fieldName] = clonedField
	return nil
}

// applyRemoveField removes a field from the schema.
func (m *MigrationApplier) applyRemoveField(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyRemoveField"

	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_FIELD_ID").
			WithMessage("missing field ID").
			WithOperation(op)
	}

	fieldName := *change.ID

	if _, exists := target.Fields[fieldName]; !exists {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_FIELD_NOT_FOUND").
				WithMessagef("field %s does not exist", fieldName).
				WithOperation(op)
		}
		return nil
	}

	delete(target.Fields, fieldName)

	// Clean up indexes referencing this field
	target.Indexes = filterIndexesByField(target.Indexes, fieldName)

	// Clean up constraints referencing this field
	target.Constraints = filterConstraintsByField(target.Constraints, fieldName)

	return nil
}

// applyModifyField modifies an existing field.
func (m *MigrationApplier) applyModifyField(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifyField"

	if change.SchemaChangeModifyFieldPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifyField payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_FIELD_ID").
			WithMessage("missing field ID").
			WithOperation(op)
	}

	fieldName := *change.ID
	field, exists := target.Fields[fieldName]
	if !exists {
		return common.NewSystemError("ERR_FIELD_NOT_FOUND").
			WithMessagef("field %s does not exist", fieldName).
			WithOperation(op)
	}

	applyPartialFieldChanges(field, &change.SchemaChangeModifyFieldPayload.Changes)
	return nil
}

// applyAddIndex adds a new index to the schema.
func (m *MigrationApplier) applyAddIndex(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyAddIndex"

	if change.SchemaChangeAddIndexPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing addIndex payload").
			WithOperation(op)
	}

	indexDef := change.SchemaChangeAddIndexPayload.Definition

	// Check for existing index with same name
	if indexExists(target.Indexes, indexDef.Name) {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_INDEX_ALREADY_EXISTS").
				WithMessagef("index %s already exists", indexDef.Name).
				WithOperation(op)
		}
		// Remove existing index first
		if err := m.removeIndexByName(target, indexDef.Name); err != nil {
			return common.SystemErrorFrom(err).WithOperation(op)
		}
	}

	if target.Indexes == nil {
		target.Indexes = make([]schema.IndexOrReference, 0)
	}

	clonedIndex, err := deepCloneIndexDefinition(&indexDef)
	if err != nil {
		return common.NewSystemError("ERR_INDEX_CLONE_FAILED").
			WithMessage("failed to clone index definition").
			WithCause(err).
			WithOperation(op)
	}

	target.Indexes = append(target.Indexes, schema.IndexOrReference{Index: clonedIndex})
	return nil
}

// applyRemoveIndex removes an index from the schema.
func (m *MigrationApplier) applyRemoveIndex(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyRemoveIndex"

	if change.Name == nil {
		return common.NewSystemError("ERR_MISSING_INDEX_NAME").
			WithMessage("missing index name").
			WithOperation(op)
	}

	return m.removeIndexByName(target, *change.Name)
}

// applyModifyIndex modifies an existing index.
func (m *MigrationApplier) applyModifyIndex(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifyIndex"

	if change.SchemaChangeModifyIndexPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifyIndex payload").
			WithOperation(op)
	}
	if change.Name == nil {
		return common.NewSystemError("ERR_MISSING_INDEX_NAME").
			WithMessage("missing index name").
			WithOperation(op)
	}

	indexName := *change.Name
	targetIndex := findIndexInSchema(target, indexName)
	if targetIndex == nil {
		return common.NewSystemError("ERR_INDEX_NOT_FOUND").
			WithMessagef("index %s does not exist", indexName).
			WithOperation(op)
	}

	applyPartialIndexChanges(targetIndex, &change.SchemaChangeModifyIndexPayload.Changes)
	return nil
}

// applyAddConstraint adds a new constraint to the schema.
func (m *MigrationApplier) applyAddConstraint(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyAddConstraint"

	if change.SchemaChangeAddConstraintPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing addConstraint payload").
			WithOperation(op)
	}

	constraintRule := change.SchemaChangeAddConstraintPayload.Constraint
	constraintName := getConstraintRuleName(&constraintRule)

	if constraintName == "" {
		return common.NewSystemError("ERR_INVALID_CONSTRAINT_RULE").
			WithMessage("invalid constraint rule").
			WithOperation(op)
	}

	// Check if constraint already exists
	if constraintExists(target.Constraints, constraintName) {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_CONSTRAINT_ALREADY_EXISTS").
				WithMessagef("constraint %s already exists", constraintName).
				WithOperation(op)
		}
		// Remove existing constraint first
		if err := m.removeConstraintByName(target, constraintName); err != nil {
			return common.SystemErrorFrom(err).WithOperation(op)
		}
	}

	if target.Constraints == nil {
		target.Constraints = make(schema.SchemaConstraint, 0)
	}

	clonedRule, err := deepCloneConstraintRule(&constraintRule)
	if err != nil {
		return common.NewSystemError("ERR_CONSTRAINT_CLONE_FAILED").
			WithMessage("failed to clone constraint rule").
			WithCause(err).
			WithOperation(op)
	}

	target.Constraints = append(target.Constraints, *clonedRule)
	return nil
}

// applyRemoveConstraint removes a constraint from the schema.
func (m *MigrationApplier) applyRemoveConstraint(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyRemoveConstraint"

	if change.Name == nil {
		return common.NewSystemError("ERR_MISSING_CONSTRAINT_NAME").
			WithMessage("missing constraint name").
			WithOperation(op)
	}

	return m.removeConstraintByName(target, *change.Name)
}

// applyModifyConstraint modifies an existing constraint.
func (m *MigrationApplier) applyModifyConstraint(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifyConstraint"

	if change.SchemaChangeModifyConstraintPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifyConstraint payload").
			WithOperation(op)
	}
	if change.Name == nil {
		return common.NewSystemError("ERR_MISSING_CONSTRAINT_NAME").
			WithMessage("missing constraint name").
			WithOperation(op)
	}

	constraintName := *change.Name

	// FIX: Parse hierarchical constraint names
	nameParts := parseHierarchicalName(constraintName)
	targetRule := m.findConstraintByNameParts(target.Constraints, nameParts, 0)

	if targetRule == nil {
		return common.NewSystemError("ERR_CONSTRAINT_NOT_FOUND").
			WithMessagef("constraint %s does not exist", constraintName).
			WithOperation(op)
	}

	return m.applyPartialConstraintChanges(targetRule, &change.SchemaChangeModifyConstraintPayload.Changes)
}

// applyAddSchema adds a new schema to the registry.
func (m *MigrationApplier) applyAddSchema(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyAddSchema"

	if change.SchemaChangeAddSchemaPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing addSchema payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_SCHEMA_ID").
			WithMessage("missing schema ID").
			WithOperation(op)
	}

	schemaID := *change.ID

	if target.NestedSchemas == nil {
		target.NestedSchemas = make(map[string]*schema.NestedSchemaDefinition)
	}

	if _, exists := target.NestedSchemas[schemaID]; exists && m.options.StrictMode {
		return common.NewSystemError("ERR_SCHEMA_ALREADY_EXISTS").
			WithMessagef("schema %s already exists in registry", schemaID).
			WithOperation(op)
	}

	nestedSchema := change.SchemaChangeAddSchemaPayload.Definition
	clonedSchema, err := deepCloneNestedSchemaDefinition(&nestedSchema)
	if err != nil {
		return common.NewSystemError("ERR_SCHEMA_CLONE_FAILED").
			WithMessage("failed to clone nested schema definition").
			WithCause(err).
			WithOperation(op)
	}

	target.NestedSchemas[schemaID] = clonedSchema
	return nil
}

// applyRemoveSchema removes a schema from the registry.
func (m *MigrationApplier) applyRemoveSchema(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyRemoveSchema"

	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_SCHEMA_ID").
			WithMessage("missing schema ID").
			WithOperation(op)
	}

	schemaID := *change.ID

	if target.NestedSchemas == nil {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_SCHEMA_NOT_FOUND").
				WithMessagef("schema %s does not exist in registry", schemaID).
				WithOperation(op)
		}
		return nil
	}

	if _, exists := target.NestedSchemas[schemaID]; !exists && m.options.StrictMode {
		return common.NewSystemError("ERR_SCHEMA_NOT_FOUND").
			WithMessagef("schema %s does not exist in registry", schemaID).
			WithOperation(op)
	}

	delete(target.NestedSchemas, schemaID)
	return nil
}

// applyModifySchema modifies a schema in the registry.
func (m *MigrationApplier) applyModifySchema(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifySchema"

	if change.SchemaChangeModifySchemaPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifySchema payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_SCHEMA_ID").
			WithMessage("missing schema ID").
			WithOperation(op)
	}

	schemaID := *change.ID

	if target.NestedSchemas == nil {
		return common.NewSystemError("ERR_SCHEMA_NOT_FOUND").
			WithMessagef("schema %s does not exist in registry", schemaID).
			WithOperation(op)
	}

	nestedSchema, exists := target.NestedSchemas[schemaID]
	if !exists {
		return common.NewSystemError("ERR_SCHEMA_NOT_FOUND").
			WithMessagef("schema %s does not exist in registry", schemaID).
			WithOperation(op)
	}

	// Convert to temporary schema for change application
	tempSchema := nestedSchemaToTempSchema(nestedSchema)

	for i, nestedChange := range change.SchemaChangeModifySchemaPayload.Changes {
		if err := m.applyChange(tempSchema, nestedChange); err != nil {
			return common.NewSystemError("ERR_APPLY_NESTED_CHANGE_FAILED").
				WithMessagef("failed to apply nested change %d", i).
				WithCause(err).
				WithOperation(op)
		}
	}

	// Convert back
	tempSchemaToNestedSchema(tempSchema, nestedSchema)
	return nil
}

// applyModifySchemaReference modifies a NestedSchemaReference within a FieldDefinition.
func (m *MigrationApplier) applyModifySchemaReference(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifySchemaReference"

	if change.SchemaChangeModifySchemaReferencePayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifySchemaReference payload").
			WithOperation(op)
	}
	if change.SchemaChangeModifySchemaReferencePayload.Field == "" {
		return common.NewSystemError("ERR_MISSING_FIELD_NAME").
			WithMessage("missing field name for modifySchemaReference").
			WithOperation(op)
	}

	fieldName := change.SchemaChangeModifySchemaReferencePayload.Field
	field, exists := target.Fields[fieldName]
	if !exists {
		return common.NewSystemError("ERR_FIELD_NOT_FOUND").
			WithMessagef("field %s not found for schema reference modification", fieldName).
			WithOperation(op)
	}

	// Ensure the field's schema is a NestedSchemaReference
	currentSchema, ok := field.Schema.(schema.NestedSchemaReference)
	if !ok {
		return common.NewSystemError("ERR_INVALID_SCHEMA_REFERENCE_TYPE").
			WithMessagef("field %s schema is not a NestedSchemaReference", fieldName).
			WithOperation(op)
	}

	if change.SchemaChangeModifySchemaReferencePayload.ID != nil &&
		*change.SchemaChangeModifySchemaReferencePayload.ID != currentSchema.ID {
		return common.NewSystemError("ERR_SCHEMA_REFERENCE_ID_MISMATCH").
			WithMessagef("schema reference ID mismatch for field %s: expected %s, got %s",
				fieldName, currentSchema.ID, *change.SchemaChangeModifySchemaReferencePayload.ID).
			WithOperation(op)
	}

	// Apply nested changes to the schema reference
	tempSchema := &schema.SchemaDefinition{
		Name:        currentSchema.ID,
		Indexes:     currentSchema.Indexes,
		Constraints: currentSchema.Constraints,
	}

	for i, nestedChange := range change.SchemaChangeModifySchemaReferencePayload.Changes {
		if err := m.applyChange(tempSchema, nestedChange); err != nil {
			return common.NewSystemError("ERR_APPLY_NESTED_SCHEMA_REFERENCE_CHANGE_FAILED").
				WithMessagef("failed to apply nested change %d to schema reference of field %s", i, fieldName).
				WithCause(err).
				WithOperation(op)
		}
	}

	// Update the field's schema with modified reference
	currentSchema.Indexes = tempSchema.Indexes
	currentSchema.Constraints = tempSchema.Constraints
	field.Schema = currentSchema

	return nil
}

// Helper methods

// removeIndexByName removes an index by name.
func (m *MigrationApplier) removeIndexByName(target *schema.SchemaDefinition, name string) error {
	op := "MigrationApplier.removeIndexByName"

	if target.Indexes == nil {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_INDEX_NOT_FOUND").
				WithMessagef("index %s does not exist", name).
				WithOperation(op)
		}
		return nil
	}

	newIndexes, found := removeIndexFromList(target.Indexes, name)
	if !found && m.options.StrictMode {
		return common.NewSystemError("ERR_INDEX_NOT_FOUND").
			WithMessagef("index %s does not exist", name).
			WithOperation(op)
	}

	target.Indexes = newIndexes
	return nil
}

// removeConstraintByName removes a constraint by name (including hierarchical names).
func (m *MigrationApplier) removeConstraintByName(target *schema.SchemaDefinition, name string) error {
	op := "MigrationApplier.removeConstraintByName"

	if target.Constraints == nil {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_CONSTRAINT_NOT_FOUND").
				WithMessagef("constraint %s does not exist", name).
				WithOperation(op)
		}
		return nil
	}

	// FIX: Handle hierarchical names
	nameParts := parseHierarchicalName(name)
	newConstraints, found := removeConstraintByNameParts(target.Constraints, nameParts, 0)

	if !found && m.options.StrictMode {
		return common.NewSystemError("ERR_CONSTRAINT_NOT_FOUND").
			WithMessagef("constraint %s does not exist", name).
			WithOperation(op)
	}

	target.Constraints = newConstraints
	return nil
}

// findConstraintByNameParts finds a constraint by hierarchical name parts.
func (m *MigrationApplier) findConstraintByNameParts(
	rules schema.SchemaConstraint,
	nameParts []string,
	depth int,
) *schema.ConstraintRule {
	if depth >= len(nameParts) {
		return nil
	}

	targetName := nameParts[depth]

	for i := range rules {
		rule := &rules[i]
		ruleName := getConstraintRuleName(rule)

		if ruleName == targetName {
			// Found matching name at this level
			if depth == len(nameParts)-1 {
				// This is the final part - return this rule
				return rule
			}
			// Not final - need to go deeper
			if rule.IsConstraintGroup() {
				return m.findConstraintByNameParts(rule.ConstraintGroup.Rules, nameParts, depth+1)
			}
		}
	}

	return nil
}

// applyPartialConstraintChanges applies partial changes to a constraint rule.
func (m *MigrationApplier) applyPartialConstraintChanges(
	rule *schema.ConstraintRule,
	partial *schema.PartialConstraint,
) error {
	op := "MigrationApplier.applyPartialConstraintChanges"

	if rule.IsConstraint() {
		applyPartialConstraintDefinitionChanges(rule.Constraint, partial)
		return nil
	}

	if rule.IsConstraintGroup() {
		// FIX: Handle operator changes correctly - use partial.Operator directly
		if partial.Operator != nil {
			rule.ConstraintGroup.Operator = *partial.Operator
		}
		return nil
	}

	return common.NewSystemError("ERR_INVALID_CONSTRAINT_RULE_TYPE").
		WithMessage("invalid constraint rule type").
		WithOperation(op)
}

// cleanupOrphanedReferences removes references that point to non-existent resources.
func (m *MigrationApplier) cleanupOrphanedReferences(_ *schema.SchemaDefinition) error {
	// Placeholder for future implementation
	return nil
}

// validateResultingSchema validates the resulting schema for consistency.
func (m *MigrationApplier) validateResultingSchema(target *schema.SchemaDefinition) error {
	op := "MigrationApplier.validateResultingSchema"

	// Check that all index fields exist
	for _, ior := range target.Indexes {
		if ior.IsIndex() {
			for _, fieldName := range ior.Index.Fields {
				if _, exists := target.Fields[fieldName]; !exists {
					return common.NewSystemError("ERR_INDEX_REFERENCES_NON_EXISTENT_FIELD").
						WithMessagef("index %s references non-existent field %s", ior.Index.Name, fieldName).
						WithOperation(op)
				}
			}
		}
	}

	// Check that all constraint fields exist
	if err := validateConstraintFields(target.Constraints, target.Fields); err != nil {
		return common.SystemErrorFrom(err).WithOperation(op)
	}

	return nil
}

// Deep cloning functions

func deepCloneSchema(source *schema.SchemaDefinition) (*schema.SchemaDefinition, error) {
	op := "deepCloneSchema"
	data, err := json.Marshal(source)
	if err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_MARSHAL").
			WithMessage("failed to marshal schema for cloning").
			WithCause(err).
			WithOperation(op)
	}

	var target schema.SchemaDefinition
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_UNMARSHAL").
			WithMessage("failed to unmarshal schema for cloning").
			WithCause(err).
			WithOperation(op)
	}

	return &target, nil
}

func deepCloneFieldDefinition(source *schema.FieldDefinition) (*schema.FieldDefinition, error) {
	op := "deepCloneFieldDefinition"
	data, err := json.Marshal(source)
	if err != nil {
		return nil, common.NewSystemError("ERR_FIELD_MARSHAL").
			WithMessage("failed to marshal field definition for cloning").
			WithCause(err).
			WithOperation(op)
	}

	var target schema.FieldDefinition
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, common.NewSystemError("ERR_FIELD_UNMARSHAL").
			WithMessage("failed to unmarshal field definition for cloning").
			WithCause(err).
			WithOperation(op)
	}

	return &target, nil
}

func deepCloneIndexDefinition(source *schema.IndexDefinition) (*schema.IndexDefinition, error) {
	op := "deepCloneIndexDefinition"
	data, err := json.Marshal(source)
	if err != nil {
		return nil, common.NewSystemError("ERR_INDEX_MARSHAL").
			WithMessage("failed to marshal index definition for cloning").
			WithCause(err).
			WithOperation(op)
	}

	var target schema.IndexDefinition
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, common.NewSystemError("ERR_INDEX_UNMARSHAL").
			WithMessage("failed to unmarshal index definition for cloning").
			WithCause(err).
			WithOperation(op)
	}

	return &target, nil
}

func deepCloneConstraintRule(source *schema.ConstraintRule) (*schema.ConstraintRule, error) {
	op := "deepCloneConstraintRule"
	data, err := json.Marshal(source)
	if err != nil {
		return nil, common.NewSystemError("ERR_CONSTRAINT_MARSHAL").
			WithMessage("failed to marshal constraint rule for cloning").
			WithCause(err).
			WithOperation(op)
	}

	var target schema.ConstraintRule
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, common.NewSystemError("ERR_CONSTRAINT_UNMARSHAL").
			WithMessage("failed to unmarshal constraint rule for cloning").
			WithCause(err).
			WithOperation(op)
	}

	return &target, nil
}

func deepCloneNestedSchemaDefinition(source *schema.NestedSchemaDefinition) (*schema.NestedSchemaDefinition, error) {
	op := "deepCloneNestedSchemaDefinition"
	data, err := json.Marshal(source)
	if err != nil {
		return nil, common.NewSystemError("ERR_NESTED_SCHEMA_MARSHAL").
			WithMessage("failed to marshal nested schema definition for cloning").
			WithCause(err).
			WithOperation(op)
	}

	var target schema.NestedSchemaDefinition
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, common.NewSystemError("ERR_NESTED_SCHEMA_UNMARSHAL").
			WithMessage("failed to unmarshal nested schema definition for cloning").
			WithCause(err).
			WithOperation(op)
	}

	return &target, nil
}

// Partial application functions

func applyPartialFieldChanges(field *schema.FieldDefinition, partial *schema.PartialFieldDefinition) {
	if partial.Type != nil {
		field.Type = *partial.Type
	}
	if partial.Required != nil {
		field.Required = partial.Required
	}
	if partial.Unique != nil {
		field.Unique = partial.Unique
	}
	if partial.Description != nil {
		field.Description = partial.Description
	}
	if partial.Default != nil {
		field.Default = partial.Default
	}
	if partial.ItemsType != nil {
		field.ItemsType = partial.ItemsType
	}
	if partial.Values != nil {
		field.Values = partial.Values
	}
	if partial.Schema != nil {
		field.Schema = partial.Schema
	}
	if partial.Constraints != nil {
		field.Constraints = partial.Constraints
	}
	if partial.Hint != nil {
		field.Hint = partial.Hint
	}
	if partial.Deprecated != nil {
		field.Deprecated = partial.Deprecated
	}
	if partial.Name != nil {
		field.Name = *partial.Name
	}

	// Handle unset operations
	for _, unsetField := range partial.Unset {
		switch unsetField {
		case "required":
			field.Required = nil
		case "unique":
			field.Unique = nil
		case "description":
			field.Description = nil
		case "default":
			field.Default = nil
		case "itemsType":
			field.ItemsType = nil
		case "values":
			field.Values = nil
		case "schema":
			field.Schema = nil
		case "constraints":
			field.Constraints = nil
		case "hint":
			field.Hint = nil
		case "deprecated":
			field.Deprecated = nil
		}
	}
}

func applyPartialIndexChanges(index *schema.IndexDefinition, partial *schema.PartialIndexDefinition) {
	if partial.Type != nil {
		index.Type = *partial.Type
	}
	if partial.Fields != nil {
		index.Fields = partial.Fields
	}
	if partial.Unique != nil {
		index.Unique = partial.Unique
	}
	if partial.Description != nil {
		index.Description = partial.Description
	}
	if partial.Order != nil {
		index.Order = partial.Order
	}
	if partial.Partial != nil {
		index.Partial = partial.Partial
	}
	if partial.Name != nil {
		index.Name = *partial.Name
	}

	// Handle unset operations
	for _, unsetField := range partial.Unset {
		switch unsetField {
		case "fields":
			index.Fields = nil
		case "unique":
			index.Unique = nil
		case "description":
			index.Description = nil
		case "order":
			index.Order = nil
		case "partial":
			index.Partial = nil
		}
	}
}

func applyPartialConstraintDefinitionChanges(constraint *schema.Constraint, partial *schema.PartialConstraint) {
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
}

// Utility functions

// getConstraintRuleName returns the name of a constraint rule.
func getConstraintRuleName(rule *schema.ConstraintRule) string {
	if rule.IsConstraint() {
		return rule.Constraint.Name
	}
	if rule.IsConstraintGroup() {
		return rule.ConstraintGroup.Name
	}
	return ""
}

// indexExists checks if an index with the given name exists.
func indexExists(indexes []schema.IndexOrReference, name string) bool {
	for _, ior := range indexes {
		if ior.IsIndex() && ior.Index.Name == name {
			return true
		}
	}
	return false
}

// constraintExists checks if a constraint with the given name exists.
func constraintExists(constraints schema.SchemaConstraint, name string) bool {
	for i := range constraints {
		rule := &constraints[i]
		if getConstraintRuleName(rule) == name {
			return true
		}
		if rule.IsConstraintGroup() {
			if constraintExists(rule.ConstraintGroup.Rules, name) {
				return true
			}
		}
	}
	return false
}

// removeIndexFromList removes an index from a list by name.
func removeIndexFromList(indexes []schema.IndexOrReference, name string) ([]schema.IndexOrReference, bool) {
	newIndexes := make([]schema.IndexOrReference, 0, len(indexes))
	found := false

	for _, ior := range indexes {
		if ior.IsIndex() && ior.Index.Name == name {
			found = true
			continue
		}
		newIndexes = append(newIndexes, ior)
	}

	return newIndexes, found
}

// removeConstraintByNameParts removes a constraint by hierarchical name parts.
func removeConstraintByNameParts(
	rules schema.SchemaConstraint,
	nameParts []string,
	depth int,
) (schema.SchemaConstraint, bool) {
	if depth >= len(nameParts) {
		return rules, false
	}

	targetName := nameParts[depth]
	result := make(schema.SchemaConstraint, 0, len(rules))
	found := false

	for i := range rules {
		rule := &rules[i]
		ruleName := getConstraintRuleName(rule)

		if ruleName == targetName {
			// Found matching name at this level
			if depth == len(nameParts)-1 {
				// This is the final part - remove this rule
				found = true
				continue
			}
			// Not final - need to go deeper into the group
			if rule.IsConstraintGroup() {
				newRules, foundNested := removeConstraintByNameParts(
					rule.ConstraintGroup.Rules,
					nameParts,
					depth+1,
				)
				if foundNested {
					found = true
					if len(newRules) > 0 {
						newGroup := *rule.ConstraintGroup
						newGroup.Rules = newRules
						result = append(result, schema.ConstraintRule{
							ConstraintGroup: &newGroup,
						})
					}
				} else {
					result = append(result, *rule)
				}
			} else {
				result = append(result, *rule)
			}
		} else {
			result = append(result, *rule)
		}
	}

	return result, found
}

// filterIndexesByField filters out indexes that reference a specific field.
func filterIndexesByField(indexes []schema.IndexOrReference, fieldName string) []schema.IndexOrReference {
	result := make([]schema.IndexOrReference, 0, len(indexes))

	for _, ior := range indexes {
		if ior.IsIndex() {
			if !slices.Contains(ior.Index.Fields, fieldName) {
				result = append(result, ior)
			}
		} else {
			result = append(result, ior)
		}
	}

	return result
}

// filterConstraintsByField filters out constraints that reference a specific field.
func filterConstraintsByField(constraints schema.SchemaConstraint, fieldName string) schema.SchemaConstraint {
	result := make(schema.SchemaConstraint, 0, len(constraints))

	for i := range constraints {
		rule := &constraints[i]

		if rule.IsConstraint() {
			// Check if this constraint references the field
			if rule.Constraint.Field != nil && *rule.Constraint.Field == fieldName {
				continue
			}
			if slices.Contains(rule.Constraint.Fields, fieldName) {
				continue
			}
			result = append(result, *rule)
		} else if rule.IsConstraintGroup() {
			// Recursively filter the group's rules
			filteredRules := filterConstraintsByField(rule.ConstraintGroup.Rules, fieldName)
			if len(filteredRules) > 0 {
				newGroup := *rule.ConstraintGroup
				newGroup.Rules = filteredRules
				result = append(result, schema.ConstraintRule{
					ConstraintGroup: &newGroup,
				})
			}
		} else {
			// Keep references as-is
			result = append(result, *rule)
		}
	}

	return result
}

// validateConstraintFields validates that all constraint field references exist.
func validateConstraintFields(constraints schema.SchemaConstraint, fields map[string]*schema.FieldDefinition) error {
	op := "validateConstraintFields"

	for i := range constraints {
		rule := &constraints[i]

		if rule.IsConstraint() {
			if rule.Constraint.Field != nil {
				if _, exists := fields[*rule.Constraint.Field]; !exists {
					return common.NewSystemError("ERR_CONSTRAINT_REFERENCES_NON_EXISTENT_FIELD").
						WithMessagef("constraint %s references non-existent field %s",
							rule.Constraint.Name, *rule.Constraint.Field).
						WithOperation(op)
				}
			}

			for _, fieldName := range rule.Constraint.Fields {
				if _, exists := fields[fieldName]; !exists {
					return common.NewSystemError("ERR_CONSTRAINT_REFERENCES_NON_EXISTENT_FIELD").
						WithMessagef("constraint %s references non-existent field %s",
							rule.Constraint.Name, fieldName).
						WithOperation(op)
				}
			}
		} else if rule.IsConstraintGroup() {
			if err := validateConstraintFields(rule.ConstraintGroup.Rules, fields); err != nil {
				return common.SystemErrorFrom(err).WithOperation(op)
			}
		}
	}

	return nil
}

// convertToStringPtr converts various types to *string.
func convertToStringPtr(value any) *string {
	if value == nil {
		return nil
	}
	if strPtr, ok := value.(*string); ok {
		return strPtr
	}
	if str, ok := value.(string); ok {
		return &str
	}
	return nil
}

// Conversion helpers for nested schema modifications

func nestedSchemaToTempSchema(nested *schema.NestedSchemaDefinition) *schema.SchemaDefinition {
	temp := &schema.SchemaDefinition{
		Name:        nested.Name,
		Description: nested.Description,
		Metadata:    nested.Metadata,
		Indexes:     nested.Indexes,
		Constraints: nested.Constraints,
	}

	if nested.Fields != nil && nested.Fields.IsMap() {
		temp.Fields = nested.Fields.FieldsMap
	}

	return temp
}

func tempSchemaToNestedSchema(temp *schema.SchemaDefinition, nested *schema.NestedSchemaDefinition) {
	nested.Name = temp.Name
	nested.Description = temp.Description
	nested.Metadata = temp.Metadata
	nested.Indexes = temp.Indexes
	nested.Constraints = temp.Constraints

	if temp.Fields != nil {
		nested.Fields = &schema.NestedSchemaFields{
			FieldsMap: temp.Fields,
		}
	}
}
