package migration

import (
	"slices"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
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

	target, err := source.DeepClone()
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
		target, _, err = target.WithAllOrphansRemoved()
		if err != nil {
			return nil, common.NewSystemError("ERR_CLEANUP_ORPHANS_FAILED").
				WithMessage("failed to cleanup orphaned references").
				WithCause(err).
				WithOperation("MigrationApplier.ApplyMigration")
		}
	}

	if m.options.ValidateResult {
		if issues := target.ValidateAll(); len(issues) > 0 {
			return nil, common.NewSystemError("ERR_RESULTING_SCHEMA_INVALID").
				WithMessage("resulting schema is invalid").
				WithOperation("MigrationApplier.ApplyMigration").WithIssues(issues)
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
	case "hint":
		if hint, ok := change.SchemaChangeModifyPropertyPayload.Value.(*schema.SchemaHint); ok {
			target.Hint = hint
		} else {
			target.Hint = nil
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

	fieldId := *change.ID
	fieldDef := change.SchemaChangeAddFieldPayload.Definition
	fieldName := fieldDef.Name

	if _, exists := target.Fields[fieldId]; exists && m.options.StrictMode {
		return common.NewSystemError("ERR_FIELD_ALREADY_EXISTS").
			WithMessagef("field with ID %s already exists", fieldId).
			WithOperation(op)
	}

	// Validate nested schema references
	if fieldDef.Schema != nil {
		switch s := fieldDef.Schema.(type) {
		case schema.NestedSchemaReference:
			if _, exists := target.NestedSchemas[s.ID]; !exists {
				return common.NewSystemError("ERR_SCHEMA_NOT_FOUND").
					WithMessagef("nested schema '%s' referenced by new field '%s' not found", s.ID, fieldName).
					WithOperation(op)
			}
		case []schema.NestedSchemaReference:
			for _, ref := range s {
				if _, exists := target.NestedSchemas[ref.ID]; !exists {
					return common.NewSystemError("ERR_SCHEMA_NOT_FOUND").
						WithMessagef("nested schema '%s' referenced by new field '%s' not found", ref.ID, fieldName).
						WithOperation(op)
				}
			}
		}
	}

	if target.Fields == nil {
		target.Fields = make(map[string]*schema.FieldDefinition)
	}

	clonedField, err := fieldDef.DeepClone()
	if err != nil {
		return common.NewSystemError("ERR_FIELD_CLONE_FAILED").
			WithMessage("failed to clone field definition").
			WithCause(err).
			WithOperation(op)
	}

	target.Fields[fieldId] = clonedField
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

	fieldId := *change.ID

	field, exists := target.Fields[fieldId]
	if !exists {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_FIELD_NOT_FOUND").
				WithMessagef("field with ID %s does not exist", fieldId).
				WithOperation(op)
		}
		return nil
	}

	delete(target.Fields, fieldId)

	// Clean up indexes referencing this field
	// Assuming field.Name is the string used in index definitions.
	target.Indexes = filterIndexesByField(target.Indexes, field.Name)

	// Clean up constraints referencing this field
	// Assuming field.Name is the string used in constraint definitions.
	target.Constraints = filterConstraintsByField(target.Constraints, field.Name)

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

	fieldId := *change.ID
	field, exists := target.Fields[fieldId]
	if !exists {
		return common.NewSystemError("ERR_FIELD_NOT_FOUND").
			WithMessagef("field with ID %s does not exist", fieldId).
			WithOperation(op)
	}

	return field.ApplyPartialChanges(&change.SchemaChangeModifyFieldPayload.Changes)
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

	// Validate that fields for the index exist by Name
	for _, indexFieldName := range indexDef.Fields {
		found := false
		for _, schemaField := range target.Fields { // target.Fields is map[fieldId]*FieldDefinition
			if schemaField.Name == indexFieldName {
				found = true
				break
			}
		}
		if !found {
			return common.NewSystemError("ERR_FIELD_NOT_FOUND").
				WithMessagef("field '%s' specified in index '%s' does not exist by name in the schema", indexFieldName, indexDef.Name).
				WithOperation(op)
		}
	}

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

	clonedIndex, err := indexDef.DeepClone()
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

	return targetIndex.ApplyPartialChanges(&change.SchemaChangeModifyIndexPayload.Changes)
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

	// Validate that fields for the constraint exist
	if err := m.validateConstraintFields(&constraintRule, target.Fields, op); err != nil {
		return err
	}

	constraintName := constraintRule.GetName()

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

	clonedRule, err := constraintRule.DeepClone()
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

	nameParts := parseHierarchicalName(constraintName)
	targetRule := m.findConstraintByNameParts(target.Constraints, nameParts, 0)

	if targetRule == nil {
		return common.NewSystemError("ERR_CONSTRAINT_NOT_FOUND").
			WithMessagef("constraint %s does not exist", constraintName).
			WithOperation(op)
	}

	return targetRule.ApplyPartialChanges(&change.SchemaChangeModifyConstraintPayload.Changes)
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
	clonedSchema, err := nestedSchema.DeepClone()
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

	changes := change.SchemaChangeModifySchemaPayload.Changes

	if nestedSchema.IsStructured() {
		// Handle properties unique to NestedSchemaDefinition before applying other changes.
		remainingChanges := make([]schema.SchemaChange, 0, len(changes))
		for _, change := range changes {
			if change.Type == schema.SchemaChangeTypeModifyProperty && change.ID != nil {
				propID := *change.ID
				payload := change.SchemaChangeModifyPropertyPayload
				if payload == nil {
					return common.NewSystemError("ERR_MISSING_PAYLOAD").
						WithMessage("missing modifyProperty payload for structured nested schema").
						WithOperation(op)
				}

				switch propID {
				case "concrete":
					if concrete, ok := payload.Value.(bool); ok {
						nestedSchema.Concrete = &concrete
					}
					continue // Change handled, do not add to remainingChanges
				case "id":
					if id, ok := payload.Value.(string); ok {
						nestedSchema.ID = &id
					}
					continue // Change handled, do not add to remainingChanges
				}
			}
			remainingChanges = append(remainingChanges, change)
		}

		if len(remainingChanges) == 0 {
			return nil // All changes were handled
		}

		if nestedSchema.Fields != nil && nestedSchema.Fields.IsMap() {
			tempSchema := nestedSchemaToTempSchema(nestedSchema)
			if err := m.applyChanges(tempSchema, remainingChanges); err != nil {
				return err
			}
			tempSchemaToNestedSchema(tempSchema, nestedSchema)
		} else if nestedSchema.Fields != nil && nestedSchema.Fields.FieldSets != nil {
			if err := m.applyChangesToNestedSchemaWithFieldSets(nestedSchema, remainingChanges); err != nil {
				return err
			}
		} else if nestedSchema.Fields != nil && nestedSchema.Fields.IsLegacyFieldsArray() {
			if err := m.applyChangesToNestedSchemaWithArrayFields(nestedSchema, remainingChanges); err != nil {
				return err
			}
		} else {
			// Structured schema but no fields defined (should not happen if IsStructured() is true)
			return common.NewSystemError("ERR_INVALID_STRUCTURED_SCHEMA").
				WithMessage("structured nested schema has no fields defined").
				WithOperation(op)
		}
	} else {
		// Logic for typed (primitive-like) nested schemas
		for _, subChange := range changes {
			switch subChange.Type {
			case schema.SchemaChangeTypeModifyProperty:
				payload := subChange.SchemaChangeModifyPropertyPayload
				if payload == nil || subChange.ID == nil {
					return common.NewSystemError("ERR_MISSING_PAYLOAD").
						WithMessage("missing modifyProperty payload for typed nested schema").
						WithOperation(op)
				}
				propID := *subChange.ID
				switch propID {
				case "description":
					nestedSchema.Description = convertToStringPtr(payload.Value)
				case "metadata":
					if metadata, ok := payload.Value.(map[string]any); ok {
						nestedSchema.Metadata = metadata
					} else {
						nestedSchema.Metadata = nil
					}
				case "type":
					if typeStr, ok := payload.Value.(string); ok {
						*nestedSchema.Type = schema.FieldType(typeStr)
					}
				case "default":
					nestedSchema.Default = payload.Value
				case "itemsType":
					if typeStr, ok := payload.Value.(string); ok {
						*nestedSchema.ItemsType = schema.FieldType(typeStr)
					}
				case "values":
					if values, ok := payload.Value.([]any); ok {
						nestedSchema.Values = values
					} else {
						nestedSchema.Values = nil
					}
				case "concrete":
					if concrete, ok := payload.Value.(bool); ok {
						nestedSchema.Concrete = utils.BoolPtr(concrete)
					}
				case "id":
					if id, ok := payload.Value.(string); ok {
						nestedSchema.ID = utils.StringPtr(id)
					}
				default:
					return common.NewSystemError("ERR_UNKNOWN_PROPERTY").
						WithMessagef("unsupported property change on typed nested schema: %s", propID).
						WithOperation(op)
				}
			case schema.SchemaChangeTypeAddConstraint, schema.SchemaChangeTypeRemoveConstraint, schema.SchemaChangeTypeModifyConstraint,
				schema.SchemaChangeTypeAddIndex, schema.SchemaChangeTypeRemoveIndex, schema.SchemaChangeTypeModifyIndex:
				// Reuse existing applier logic by creating a temporary schema with only relevant fields
				tempSchemaForSlices := &schema.SchemaDefinition{
					Name:        nestedSchema.Name,
					Indexes:     nestedSchema.Indexes,
					Constraints: nestedSchema.Constraints,
				}
				if err := m.applyChange(tempSchemaForSlices, subChange); err != nil {
					return err
				}
				nestedSchema.Indexes = tempSchemaForSlices.Indexes
				nestedSchema.Constraints = tempSchemaForSlices.Constraints
			default:
				return common.NewSystemError("ERR_INVALID_CHANGE_ON_TYPED_SCHEMA").
					WithMessagef("unsupported change type '%s' on typed nested schema", subChange.Type).
					WithOperation(op)
			}
		}
	}
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

	fieldPath := change.SchemaChangeModifySchemaReferencePayload.Field
	field, _, found := m.findFieldInSchemaRecursive(target, fieldPath) // The parentSchema is not directly used here after field is found
	if !found {
		return common.NewSystemError("ERR_FIELD_NOT_FOUND").
			WithMessagef("field %s not found for schema reference modification", fieldPath).
			WithOperation(op)
	}

	// Validate field type
	switch field.Type {
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeUnion, schema.FieldTypeRecord:
		// These types can have schema references
	default:
		return common.NewSystemError("ERR_INVALID_FIELD_TYPE_FOR_SCHEMA_REF").
			WithMessagef("field '%s' of type '%s' cannot have a schema reference modified", field.Name, field.Type).
			WithOperation(op)
	}

	switch s := field.Schema.(type) {
	case schema.NestedSchemaReference:
		// Create a mutable copy to modify, then assign back to field.Schema
		tempRef := s
		schemaRefToModify := &tempRef

		// Apply nested changes to the schema reference
		tempSchemaDef := &schema.SchemaDefinition{
			Name:        schemaRefToModify.ID,
			Indexes:     schemaRefToModify.Indexes,
			Constraints: schemaRefToModify.Constraints,
		}

		for i, nestedChange := range change.SchemaChangeModifySchemaReferencePayload.Changes {
			if err := m.applyChange(tempSchemaDef, nestedChange); err != nil {
				return common.NewSystemError("ERR_APPLY_NESTED_SCHEMA_REFERENCE_CHANGE_FAILED").
					WithMessagef("failed to apply nested change %d to schema reference of field %s", i, fieldPath).
					WithCause(err).
					WithOperation(op)
			}
		}

		schemaRefToModify.Indexes = tempSchemaDef.Indexes
		schemaRefToModify.Constraints = tempSchemaDef.Constraints
		field.Schema = *schemaRefToModify // Assign the modified copy back

	case []schema.NestedSchemaReference:
		payloadID := ""
		if change.SchemaChangeModifySchemaReferencePayload.ID != nil {
			payloadID = *change.SchemaChangeModifySchemaReferencePayload.ID
		}

		found := false
		for i := range s {
			if s[i].ID == payloadID {
				schemaRefToModify := &s[i] // Pointer to slice element - this directly modifies the slice element
				found = true

				// Apply nested changes to the schema reference
				tempSchemaDef := &schema.SchemaDefinition{
					Name:        schemaRefToModify.ID,
					Indexes:     schemaRefToModify.Indexes,
					Constraints: schemaRefToModify.Constraints,
				}

				for j, nestedChange := range change.SchemaChangeModifySchemaReferencePayload.Changes {
					if err := m.applyChange(tempSchemaDef, nestedChange); err != nil {
						return common.NewSystemError("ERR_APPLY_NESTED_SCHEMA_REFERENCE_CHANGE_FAILED").
							WithMessagef("failed to apply nested change %d to schema reference of field %s", j, fieldPath).
							WithCause(err).
							WithOperation(op)
					}
				}
				schemaRefToModify.Indexes = tempSchemaDef.Indexes
				schemaRefToModify.Constraints = tempSchemaDef.Constraints
				break
			}
		}
		if !found {
			return common.NewSystemError("ERR_SCHEMA_REF_NOT_FOUND").WithMessage("could not resolve schema reference in slice").WithOperation(op)
		}
		// No need to reassign field.Schema here, as s[i] was modified directly.

	default:
		return common.NewSystemError("ERR_INVALID_SCHEMA_REFERENCE_TYPE").WithMessagef("unsupported schema reference type: %T", field.Schema).WithOperation(op)
	}

	return nil
}

// applyAddConditionalSet adds a new conditional set to the schema.
func (m *MigrationApplier) applyAddConditionalSet(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyAddConditionalSet"

	if change.SchemaChangeAddConditionalSetPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing addConditionalSet payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_CONDITIONAL_SET_ID").
			WithMessage("missing conditional set ID").
			WithOperation(op)
	}

	conditionalSetID := *change.ID
	nestedSchema, err := m.findNestedSchemaForConditionalSetChanges(target, conditionalSetID, op)
	if err != nil {
		return err
	}

	if nestedSchema.Fields == nil {
		nestedSchema.Fields = &schema.NestedSchemaFields{}
	}
	if nestedSchema.Fields.FieldSets == nil {
		nestedSchema.Fields.FieldSets = make(map[string]schema.ConditionalFieldSet)
	}

	if _, exists := nestedSchema.Fields.FieldSets[conditionalSetID]; exists && m.options.StrictMode {
		return common.NewSystemError("ERR_CONDITIONAL_SET_ALREADY_EXISTS").
			WithMessagef("conditional set with ID %s already exists", conditionalSetID).
			WithOperation(op)
	}

	nestedSchema.Fields.FieldSets[conditionalSetID] = change.SchemaChangeAddConditionalSetPayload.Definition
	return nil
}

// applyRemoveConditionalSet removes a conditional set from the schema.
func (m *MigrationApplier) applyRemoveConditionalSet(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyRemoveConditionalSet"

	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_CONDITIONAL_SET_ID").
			WithMessage("missing conditional set ID").
			WithOperation(op)
	}

	conditionalSetID := *change.ID
	nestedSchema, err := m.findNestedSchemaForConditionalSetChanges(target, conditionalSetID, op)
	if err != nil {
		return err
	}

	if nestedSchema.Fields == nil || nestedSchema.Fields.FieldSets == nil {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
				WithMessagef("conditional set with ID %s not found", conditionalSetID).
				WithOperation(op)
		}
		return nil
	}

	if _, exists := nestedSchema.Fields.FieldSets[conditionalSetID]; !exists {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
				WithMessagef("conditional set with ID %s not found", conditionalSetID).
				WithOperation(op)
		}
		return nil
	}

	// Clean up indexes and constraints if needed (fields within this set are gone)
	// This might require iterating over the fields in the set before deleting.
	// For now, assume top-level schema will handle general cleanup.

	delete(nestedSchema.Fields.FieldSets, conditionalSetID)
	return nil
}

// applyModifyConditionalSet modifies an existing conditional set (e.g., its 'When' condition).
func (m *MigrationApplier) applyModifyConditionalSet(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifyConditionalSet"

	if change.SchemaChangeModifyConditionalSetPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifyConditionalSet payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_CONDITIONAL_SET_ID").
			WithMessage("missing conditional set ID").
			WithOperation(op)
	}

	conditionalSetID := *change.ID
	nestedSchema, err := m.findNestedSchemaForConditionalSetChanges(target, conditionalSetID, op)
	if err != nil {
		return err
	}

	if nestedSchema.Fields == nil || nestedSchema.Fields.FieldSets == nil {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	conditionalSet, exists := nestedSchema.Fields.FieldSets[conditionalSetID]
	if !exists {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	// Apply changes from payload
	payload := change.SchemaChangeModifyConditionalSetPayload
	for _, unsetProp := range payload.Unset {
		if unsetProp == "when" {
			conditionalSet.When = nil
		}
	}
	if payload.When != nil {
		conditionalSet.When = payload.When
	}

	// Update the map entry (important for structs in maps)
	nestedSchema.Fields.FieldSets[conditionalSetID] = conditionalSet
	return nil
}

// applyAddConditionalField adds a field to a specific conditional set.
func (m *MigrationApplier) applyAddConditionalField(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyAddConditionalField"

	if change.SchemaChangeAddConditionalFieldPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing addConditionalField payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_CONDITIONAL_SET_ID").
			WithMessage("missing conditional set ID").
			WithOperation(op)
	}

	conditionalSetID := *change.ID
	fieldName := change.SchemaChangeAddConditionalFieldPayload.Field
	fieldDef := change.SchemaChangeAddConditionalFieldPayload.Definition

	nestedSchema, err := m.findNestedSchemaForConditionalSetChanges(target, conditionalSetID, op)
	if err != nil {
		return err
	}

	if nestedSchema.Fields == nil || nestedSchema.Fields.FieldSets == nil {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	conditionalSet, exists := nestedSchema.Fields.FieldSets[conditionalSetID]
	if !exists {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	if conditionalSet.Fields == nil {
		conditionalSet.Fields = make(map[string]*schema.FieldDefinition)
	}

	if _, fieldExists := conditionalSet.Fields[fieldName]; fieldExists && m.options.StrictMode {
		return common.NewSystemError("ERR_FIELD_ALREADY_EXISTS").
			WithMessagef("field %s already exists in conditional set %s", fieldName, conditionalSetID).
			WithOperation(op)
	}

	clonedField, err := fieldDef.DeepClone()
	if err != nil {
		return common.NewSystemError("ERR_FIELD_CLONE_FAILED").
			WithMessage("failed to clone field definition").
			WithCause(err).
			WithOperation(op)
	}

	conditionalSet.Fields[fieldName] = clonedField
	nestedSchema.Fields.FieldSets[conditionalSetID] = conditionalSet
	return nil
}

// applyRemoveConditionalField removes a field from a specific conditional set.
func (m *MigrationApplier) applyRemoveConditionalField(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyRemoveConditionalField"

	if change.SchemaChangeRemoveConditionalFieldPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing removeConditionalField payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_CONDITIONAL_SET_ID").
			WithMessage("missing conditional set ID").
			WithOperation(op)
	}

	conditionalSetID := *change.ID
	fieldName := change.SchemaChangeRemoveConditionalFieldPayload.Field

	nestedSchema, err := m.findNestedSchemaForConditionalSetChanges(target, conditionalSetID, op)
	if err != nil {
		return err
	}

	if nestedSchema.Fields == nil || nestedSchema.Fields.FieldSets == nil {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	conditionalSet, exists := nestedSchema.Fields.FieldSets[conditionalSetID]
	if !exists {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	if conditionalSet.Fields == nil {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_FIELD_NOT_FOUND").
				WithMessagef("field %s not found in conditional set %s", fieldName, conditionalSetID).
				WithOperation(op)
		}
		return nil
	}

	if _, fieldExists := conditionalSet.Fields[fieldName]; !fieldExists {
		if m.options.StrictMode {
			return common.NewSystemError("ERR_FIELD_NOT_FOUND").
				WithMessagef("field %s not found in conditional set %s", fieldName, conditionalSetID).
				WithOperation(op)
		}
		return nil
	}

	// Clean up indexes and constraints referencing this field if it's the actual field name
	// This is a bit tricky since field names can be non-unique across sets.
	// We'll rely on the FieldDefinition's actual Name for cleanup, not the map key.
	if field, exists := conditionalSet.Fields[fieldName]; exists {
		if field.Name != "" { // Only cleanup if the FieldDefinition has a name
			nestedSchema.Indexes = filterIndexesByField(nestedSchema.Indexes, field.Name)
			nestedSchema.Constraints = filterConstraintsByField(nestedSchema.Constraints, field.Name)
		}
	}

	delete(conditionalSet.Fields, fieldName)
	nestedSchema.Fields.FieldSets[conditionalSetID] = conditionalSet
	return nil
}

// applyModifyConditionalField modifies a field within a specific conditional set.
func (m *MigrationApplier) applyModifyConditionalField(target *schema.SchemaDefinition, change schema.SchemaChange) error {
	op := "MigrationApplier.applyModifyConditionalField"

	if change.SchemaChangeModifyConditionalFieldPayload == nil {
		return common.NewSystemError("ERR_MISSING_PAYLOAD").
			WithMessage("missing modifyConditionalField payload").
			WithOperation(op)
	}
	if change.ID == nil {
		return common.NewSystemError("ERR_MISSING_CONDITIONAL_SET_ID").
			WithMessage("missing conditional set ID").
			WithOperation(op)
	}

	conditionalSetID := *change.ID
	fieldName := change.SchemaChangeModifyConditionalFieldPayload.Field

	nestedSchema, err := m.findNestedSchemaForConditionalSetChanges(target, conditionalSetID, op)
	if err != nil {
		return err
	}

	if nestedSchema.Fields == nil || nestedSchema.Fields.FieldSets == nil {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	conditionalSet, exists := nestedSchema.Fields.FieldSets[conditionalSetID]
	if !exists {
		return common.NewSystemError("ERR_CONDITIONAL_SET_NOT_FOUND").
			WithMessagef("conditional set with ID %s not found", conditionalSetID).
			WithOperation(op)
	}

	if conditionalSet.Fields == nil {
		return common.NewSystemError("ERR_FIELD_NOT_FOUND").
			WithMessagef("field %s not found in conditional set %s", fieldName, conditionalSetID).
			WithOperation(op)
	}

	field, fieldExists := conditionalSet.Fields[fieldName]
	if !fieldExists {
		return common.NewSystemError("ERR_FIELD_NOT_FOUND").
			WithMessagef("field %s not found in conditional set %s", fieldName, conditionalSetID).
			WithOperation(op)
	}

	if err := field.ApplyPartialChanges(&change.SchemaChangeModifyConditionalFieldPayload.Changes); err != nil {
		return common.NewSystemError("ERR_APPLY_PARTIAL_CHANGES_FAILED").
			WithMessagef("failed to apply partial changes to field %s in conditional set %s", fieldName, conditionalSetID).
			WithCause(err).
			WithOperation(op)
	}

	nestedSchema.Fields.FieldSets[conditionalSetID] = conditionalSet
	return nil
}

// findParentNestedSchemaForFieldSets attempts to find the nested schema definition that
// contains the FieldSets we are trying to modify. This assumes the change ID
// directly refers to the NestedSchemaDefinition ID.
func (m *MigrationApplier) findNestedSchemaForConditionalSetChanges(
	target *schema.SchemaDefinition,
	nestedSchemaID string,
	op string,
) (*schema.NestedSchemaDefinition, error) {
	if target.NestedSchemas == nil {
		return nil, common.NewSystemError("ERR_NESTED_SCHEMAS_NOT_FOUND").
			WithMessagef("no nested schemas defined in target for ID %s", nestedSchemaID).
			WithOperation(op)
	}

	nestedSchema, exists := target.NestedSchemas[nestedSchemaID]
	if !exists {
		return nil, common.NewSystemError("ERR_NESTED_SCHEMA_NOT_FOUND").
			WithMessagef("nested schema with ID %s not found", nestedSchemaID).
			WithOperation(op)
	}
	return nestedSchema, nil
}

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
		ruleName := rule.GetName()

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
		if rule.GetName() == name {
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
		ruleName := rule.GetName()

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

// findFieldInSchemaRecursive attempts to find a FieldDefinition given a path like "field" or "nestedSchemaID.field".
// It returns the found FieldDefinition, the parent SchemaDefinition (where the field resides), and a boolean indicating if it was found.
func (m *MigrationApplier) findFieldInSchemaRecursive(
	targetSchema *schema.SchemaDefinition,
	fieldPath string,
) (*schema.FieldDefinition, *schema.SchemaDefinition, bool) {
	parts := strings.Split(fieldPath, ".")
	currentSchema := targetSchema
	var foundField *schema.FieldDefinition
	var parentSchema *schema.SchemaDefinition

	for i, part := range parts {
		if i == len(parts)-1 { // Last part is the field name
			if currentSchema.Fields == nil {
				return nil, nil, false
			}
			field, exists := currentSchema.Fields[part]
			if !exists {
				return nil, nil, false
			}
			foundField = field
			parentSchema = currentSchema
			break
		} else { // Intermediate part is a nested schema ID
			if currentSchema.NestedSchemas == nil {
				return nil, nil, false
			}
			nestedDef, exists := currentSchema.NestedSchemas[part]
			if !exists {
				return nil, nil, false
			}
			if !nestedDef.IsStructured() {
				// Cannot traverse into non-structured nested schemas for fields
				return nil, nil, false
			}
			// Convert NestedSchemaDefinition to SchemaDefinition for traversal
			currentSchema = nestedSchemaToTempSchema(nestedDef)
		}
	}

	return foundField, parentSchema, foundField != nil
}

// findFieldInConditionalSet finds a field within a ConditionalFieldSet.
func findFieldInConditionalSet(conditionalSet *schema.ConditionalFieldSet, fieldName string) (*schema.FieldDefinition, bool) {
	if conditionalSet == nil || conditionalSet.Fields == nil {
		return nil, false
	}
	field, exists := conditionalSet.Fields[fieldName]
	return field, exists
}

// validateConstraintFields recursively validates that fields in a constraint rule exist.
func (m *MigrationApplier) validateConstraintFields(rule *schema.ConstraintRule, fields map[string]*schema.FieldDefinition, op string) error {
	if rule.Constraint != nil {
		fieldNamesToCheck := []string{}
		if rule.Constraint.Field != nil {
			fieldNamesToCheck = append(fieldNamesToCheck, *rule.Constraint.Field)
		}
		fieldNamesToCheck = append(fieldNamesToCheck, rule.Constraint.Fields...)

		for _, constraintFieldName := range fieldNamesToCheck {
			found := false
			for _, schemaField := range fields { // fields map is keyed by fieldId
				if schemaField.Name == constraintFieldName {
					found = true
					break
				}
			}
			if !found {
				return common.NewSystemError("ERR_FIELD_NOT_FOUND").
					WithMessagef("field '%s' referenced in constraint '%s' does not exist by name in the schema", constraintFieldName, rule.Constraint.Name).
					WithOperation(op)
			}
		}
	} else if rule.ConstraintGroup != nil {
		for i := range rule.ConstraintGroup.Rules {
			if err := m.validateConstraintFields(&rule.ConstraintGroup.Rules[i], fields, op); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyChangesToNestedSchemaWithFieldSets applies changes directly to a NestedSchemaDefinition
// that uses map-based fields (FieldSets).
func (m *MigrationApplier) applyChangesToNestedSchemaWithFieldSets(
	nestedSchema *schema.NestedSchemaDefinition,
	changes []schema.SchemaChange,
) error {
	op := "MigrationApplier.applyChangesToNestedSchemaWithFieldSets"

	if nestedSchema.Fields == nil || nestedSchema.Fields.FieldSets == nil {
		return common.NewSystemError("ERR_INVALID_SCHEMA_FIELDS_TYPE").
			WithMessage("nested schema does not have map-based conditional fields (FieldSets)").
			WithOperation(op)
	}

	for _, change := range changes {
		// The change.ID is assumed to be the ID of the NestedSchemaDefinition itself
		// for these types of changes.
		// The payloads contain the specific ConditionalSet ID or Field Name.
		tempTargetSchema := nestedSchemaToTempSchema(nestedSchema) // Used for routing.

		switch change.Type {
		case schema.SchemaChangeTypeAddConditionalSet:
			if err := m.applyAddConditionalSet(tempTargetSchema, change); err != nil {
				return err
			}
		case schema.SchemaChangeTypeRemoveConditionalSet:
			if err := m.applyRemoveConditionalSet(tempTargetSchema, change); err != nil {
				return err
			}
		case schema.SchemaChangeTypeModifyConditionalSet:
			if err := m.applyModifyConditionalSet(tempTargetSchema, change); err != nil {
				return err
			}
		case schema.SchemaChangeTypeAddConditionalField:
			if err := m.applyAddConditionalField(tempTargetSchema, change); err != nil {
				return err
			}
		case schema.SchemaChangeTypeRemoveConditionalField:
			if err := m.applyRemoveConditionalField(tempTargetSchema, change); err != nil {
				return err
			}
		case schema.SchemaChangeTypeModifyConditionalField:
			if err := m.applyModifyConditionalField(tempTargetSchema, change); err != nil {
				return err
			}
		case schema.SchemaChangeTypeAddIndex, schema.SchemaChangeTypeRemoveIndex, schema.SchemaChangeTypeModifyIndex:
			// These changes apply to the overall nested schema's indexes.
			// Re-use existing applyChange by creating a temporary SchemaDefinition.
			// Apply the change, then copy the modified indexes back.
			tempSchemaForIndexes := &schema.SchemaDefinition{
				Name:    nestedSchema.Name,
				Indexes: nestedSchema.Indexes,
			}
			if err := m.applyChange(tempSchemaForIndexes, change); err != nil {
				return err
			}
			nestedSchema.Indexes = tempSchemaForIndexes.Indexes
		case schema.SchemaChangeTypeAddConstraint, schema.SchemaChangeTypeRemoveConstraint, schema.SchemaChangeTypeModifyConstraint:
			// These changes apply to the overall nested schema's constraints.
			// Re-use existing applyChange by creating a temporary SchemaDefinition.
			// Apply the change, then copy the modified constraints back.
			tempSchemaForConstraints := &schema.SchemaDefinition{
				Name:        nestedSchema.Name,
				Constraints: nestedSchema.Constraints,
			}
			if err := m.applyChange(tempSchemaForConstraints, change); err != nil {
				return err
			}
			nestedSchema.Constraints = tempSchemaForConstraints.Constraints
		default:
			return common.NewSystemError("ERR_UNSUPPORTED_CHANGE_TYPE_FOR_FIELD_SETS").
				WithMessagef("unsupported change type '%s' for map-based conditional fields (FieldSets)", change.Type).
				WithOperation(op)
		}
	}
	return nil
}

// applyChangesToNestedSchemaWithArrayFields applies changes directly to a NestedSchemaDefinition
// that uses array-based fields (ConditionalFieldSet).
func (m *MigrationApplier) applyChangesToNestedSchemaWithArrayFields(
	nestedSchema *schema.NestedSchemaDefinition,
	changes []schema.SchemaChange,
) error {
	op := "MigrationApplier.applyChangesToNestedSchemaWithArrayFields"

	if nestedSchema.Fields == nil || !nestedSchema.Fields.IsLegacyFieldsArray() {
		return common.NewSystemError("ERR_INVALID_SCHEMA_FIELDS_TYPE").
			WithMessage("nested schema does not have array-based fields").
			WithOperation(op)
	}

	for i, change := range changes {
		switch change.Type {
		case schema.SchemaChangeTypeAddField:
			if change.SchemaChangeAddFieldPayload == nil {
				return common.NewSystemError("ERR_MISSING_PAYLOAD").
					WithMessage("missing addField payload for array-based nested schema").
					WithOperation(op)
			}
			if change.ID == nil {
				return common.NewSystemError("ERR_MISSING_FIELD_ID").
					WithMessage("missing field ID for addField change in array-based nested schema").
					WithOperation(op)
			}

			fieldId := *change.ID
			fieldDef := change.SchemaChangeAddFieldPayload.Definition

			// Find the base case (ConditionalFieldSet with no 'When' condition)
			var baseCaseSet *schema.ConditionalFieldSet
			for i := range nestedSchema.Fields.FieldsArray {
				if nestedSchema.Fields.FieldsArray[i].When == nil {
					baseCaseSet = &nestedSchema.Fields.FieldsArray[i]
					break
				}
			}

			if baseCaseSet == nil {
				// No base case found, create a new one
				newSet := schema.ConditionalFieldSet{
					Fields: make(map[string]*schema.FieldDefinition),
					When:   nil,
				}
				nestedSchema.Fields.FieldsArray = append(nestedSchema.Fields.FieldsArray, newSet)
				baseCaseSet = &nestedSchema.Fields.FieldsArray[len(nestedSchema.Fields.FieldsArray)-1]
			}

			if _, exists := baseCaseSet.Fields[fieldId]; exists && m.options.StrictMode {
				return common.NewSystemError("ERR_FIELD_ALREADY_EXISTS").
					WithMessagef("field with ID %s already exists in the base case of array-based nested schema", fieldId).
					WithOperation(op)
			}

			clonedField, err := fieldDef.DeepClone()
			if err != nil {
				return common.NewSystemError("ERR_FIELD_CLONE_FAILED").
					WithMessage("failed to clone field definition for array-based nested schema").
					WithCause(err).
					WithOperation(op)
			}
			if baseCaseSet.Fields == nil {
				baseCaseSet.Fields = make(map[string]*schema.FieldDefinition)
			}
			baseCaseSet.Fields[fieldId] = clonedField

		case schema.SchemaChangeTypeRemoveField:
			if change.ID == nil {
				return common.NewSystemError("ERR_MISSING_FIELD_ID").
					WithMessage("missing field ID for removeField change in array-based nested schema").
					WithOperation(op)
			}
			fieldId := *change.ID
			found := false
			removedFieldNames := []string{}

			for i := range nestedSchema.Fields.FieldsArray {
				conditionalSet := &nestedSchema.Fields.FieldsArray[i]
				if field, exists := conditionalSet.Fields[fieldId]; exists {
					removedFieldNames = append(removedFieldNames, field.Name)
					delete(conditionalSet.Fields, fieldId)
					found = true
				}
			}

			if !found && m.options.StrictMode {
				return common.NewSystemError("ERR_FIELD_NOT_FOUND").
					WithMessagef("field with ID %s not found in any conditional set of array-based nested schema", fieldId).
					WithOperation(op)
			}

			// Clean up indexes and constraints referencing the removed field names
			for _, fieldName := range removedFieldNames {
				nestedSchema.Indexes = filterIndexesByField(nestedSchema.Indexes, fieldName)
				nestedSchema.Constraints = filterConstraintsByField(nestedSchema.Constraints, fieldName)
			}

		case schema.SchemaChangeTypeModifyField:
			if change.SchemaChangeModifyFieldPayload == nil {
				return common.NewSystemError("ERR_MISSING_PAYLOAD").
					WithMessage("missing modifyField payload for array-based nested schema").
					WithOperation(op)
			}
			if change.ID == nil {
				return common.NewSystemError("ERR_MISSING_FIELD_ID").
					WithMessage("missing field ID for modifyField change in array-based nested schema").
					WithOperation(op)
			}
			fieldId := *change.ID
			found := false

			for i := range nestedSchema.Fields.FieldsArray {
				conditionalSet := &nestedSchema.Fields.FieldsArray[i]
				if field, exists := conditionalSet.Fields[fieldId]; exists {
					if err := field.ApplyPartialChanges(&change.SchemaChangeModifyFieldPayload.Changes); err != nil {
						return common.NewSystemError("ERR_APPLY_PARTIAL_CHANGES_FAILED").
							WithMessagef("failed to apply partial changes to field with ID %s", fieldId).
							WithCause(err).
							WithOperation(op)
					}
					found = true
				}
			}

			if !found {
				return common.NewSystemError("ERR_FIELD_NOT_FOUND").
					WithMessagef("field with ID %s not found in any conditional set of array-based nested schema for modification", fieldId).
					WithOperation(op)
			}

		case schema.SchemaChangeTypeAddIndex, schema.SchemaChangeTypeRemoveIndex, schema.SchemaChangeTypeModifyIndex:
			// These changes apply to the overall nested schema's indexes, not fields within conditional sets.
			// Temporarily convert to SchemaDefinition for index application.
			tempSchema := &schema.SchemaDefinition{
				Name:    nestedSchema.Name,
				Indexes: nestedSchema.Indexes,
			}
			if err := m.applyChange(tempSchema, change); err != nil {
				return err
			}
			nestedSchema.Indexes = tempSchema.Indexes
		case schema.SchemaChangeTypeAddConstraint, schema.SchemaChangeTypeRemoveConstraint, schema.SchemaChangeTypeModifyConstraint:
			// These changes apply to the overall nested schema's constraints, not fields within conditional sets.
			// Temporarily convert to SchemaDefinition for constraint application.
			tempSchema := &schema.SchemaDefinition{
				Name:        nestedSchema.Name,
				Constraints: nestedSchema.Constraints,
			}
			if err := m.applyChange(tempSchema, change); err != nil {
				return err
			}
			nestedSchema.Constraints = tempSchema.Constraints
		default:
			return common.NewSystemError("ERR_UNSUPPORTED_CHANGE_TYPE_FOR_ARRAY_FIELDS").
				WithMessagef("unsupported change type '%s' for array-based nested schema (change %d)", change.Type, i).
				WithOperation(op)
		}
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
