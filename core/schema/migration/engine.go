package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	scjson "github.com/asaidimu/go-anansi/v6/core/json"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// MigrationEngine defines the interface for generating, applying, and transforming schema migrations.
type MigrationEngine interface {
	// Diff generates the structural delta between two states.
	Diff(oldSchema, newSchema schema.SchemaDefinition) (*schema.Migration, error)

	// Apply projects a Migration onto a SchemaDefinition to produce the next version.
	Apply(base *schema.SchemaDefinition, m *schema.Migration) (*schema.SchemaDefinition, error)

	// Patch applies RFC6902 JSON Patches to a SchemaDefinition.
	Patch(base *schema.SchemaDefinition, patches []scjson.PatchOperation) (*schema.SchemaDefinition, error)

	// Transform executes Wasm-based logic on a data stream.
	Transform(ctx context.Context, input io.Reader, m *schema.Migration, direction string) (io.Reader, error)

	// Plan generates an execution plan from a migration.
	Plan(
		ctx context.Context,
		sourceSchema *schema.SchemaDefinition,
		targetSchema *schema.SchemaDefinition,
		migration *schema.Migration,
	) (*MigrationPlan, error)
}

// DefaultMigrationEngine is a concrete implementation of the MigrationEngine interface.
type DefaultMigrationEngine struct {
	generator *MigrationGenerator
	applier   *MigrationApplier
}

// NewDefaultMigrationEngine creates a new instance of DefaultMigrationEngine.
func NewDefaultMigrationEngine(genOptions GeneratorOptions, applierOptions ApplierOptions) *DefaultMigrationEngine {
	return &DefaultMigrationEngine{
		generator: NewMigrationGenerator(genOptions),
		applier:   NewMigrationApplier(applierOptions),
	}
}

// Diff generates the structural delta between two states using the internal MigrationGenerator.
func (e *DefaultMigrationEngine) Diff(oldSchema, newSchema schema.SchemaDefinition) (*schema.Migration, error) {
	op := "DefaultMigrationEngine.Diff"
	mig, err := e.generator.Generate(&oldSchema, &newSchema)
	if err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation(op)
	}
	return mig, nil
}

// Apply projects a Migration onto a SchemaDefinition using the internal MigrationApplier.
func (e *DefaultMigrationEngine) Apply(base *schema.SchemaDefinition, m *schema.Migration) (*schema.SchemaDefinition, error) {
	op := "DefaultMigrationEngine.Apply"
	result, err := e.applier.ApplyMigration(base, m)
	if err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation(op)
	}
	return result, nil
}

// Patch applies RFC6902 JSON Patches to a SchemaDefinition.
func (e *DefaultMigrationEngine) Patch(base *schema.SchemaDefinition, patches []scjson.PatchOperation) (*schema.SchemaDefinition, error) {
	op := "DefaultMigrationEngine.Patch"

	// Convert base schema to map for JSON patching
	sourceBytes, err := json.Marshal(base)
	if err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_MARSHAL").
			WithMessage("failed to marshal base schema for JSON patch").
			WithCause(err).
			WithOperation(op)
	}

	var sourceMap map[string]any
	if err := json.Unmarshal(sourceBytes, &sourceMap); err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_UNMARSHAL").
			WithMessage("failed to unmarshal base schema into map for JSON patch").
			WithCause(err).
			WithOperation(op)
	}

	// Apply JSON patches
	patcher := scjson.NewPatcher()
	patchedMapAny, err := patcher.Apply(sourceMap, patches)
	if err != nil {
		return nil, common.NewSystemError("ERR_JSON_PATCH_APPLY").
			WithMessage("failed to apply JSON patches").
			WithCause(err).
			WithOperation(op)
	}

	patchedMap := patchedMapAny.(map[string]any)

	// Clean up empty slices/maps to match omitempty behavior
	cleanupEmptyCollections(patchedMap)

	// Convert patched map back to SchemaDefinition
	patchedBytes, err := json.Marshal(patchedMap)
	if err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_MARSHAL").
			WithMessage("failed to marshal patched map back to bytes").
			WithCause(err).
			WithOperation(op)
	}

	result, err := schema.From(patchedBytes)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithCode("ERR_SCHEMA_FROM").
			WithMessage("failed to create schema from patched bytes").
			WithOperation(op)
	}

	return result, nil
}

// Transform executes Wasm-based logic on a data stream. (Placeholder)
func (e *DefaultMigrationEngine) Transform(ctx context.Context, input io.Reader, m *schema.Migration, direction string) (io.Reader, error) {
	return nil, common.NewSystemError("ERR_NOT_IMPLEMENTED").
		WithMessage("Wasm-based transformation not yet implemented").
		WithOperation("DefaultMigrationEngine.Transform")
}

// MigrationPlan represents the execution plan for a schema migration.
type MigrationPlan struct {
	SourceVersion     string
	TargetVersion     string
	SourceSchema      *schema.SchemaDefinition
	TargetSchema      *schema.SchemaDefinition
	Strategy          MigrationStrategy
	Changes           []schema.SchemaChange
	TransformFunction string
}

// MigrationStrategy determines how the migration is executed.
type MigrationStrategy string

const (
	// MigrationStrategyInPlace applies changes directly to the existing collection.
	MigrationStrategyInPlace MigrationStrategy = "in_place"

	// MigrationStrategyBlueGreen creates a new collection version and migrates data.
	MigrationStrategyBlueGreen MigrationStrategy = "blue_green"
)

// Plan generates an execution plan from a migration.
func (e *DefaultMigrationEngine) Plan(
	ctx context.Context,
	sourceSchema *schema.SchemaDefinition,
	targetSchema *schema.SchemaDefinition,
	migration *schema.Migration,
) (*MigrationPlan, error) {
	op := "DefaultMigrationEngine.Plan"

	if err := e.validatePlanInputs(sourceSchema, targetSchema, migration); err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation(op)
	}

	strategy := e.determineStrategy(sourceSchema, migration.Changes)

	plan := &MigrationPlan{
		SourceVersion:     migration.Version.Source,
		TargetVersion:     getTargetVersion(migration.Version),
		SourceSchema:      sourceSchema,
		TargetSchema:      targetSchema,
		Strategy:          strategy,
		TransformFunction: migration.Transform,
	}

	if strategy == MigrationStrategyInPlace {
		plan.Changes = e.filterInPlaceChanges(sourceSchema, migration.Changes)
	} else {
		plan.Changes = []schema.SchemaChange{}
	}

	return plan, nil
}

// validatePlanInputs validates the inputs for plan generation.
func (e *DefaultMigrationEngine) validatePlanInputs(
	sourceSchema *schema.SchemaDefinition,
	targetSchema *schema.SchemaDefinition,
	migration *schema.Migration,
) error {
	if sourceSchema == nil {
		return common.NewSystemError("ERR_PLAN_NIL_SOURCE_SCHEMA").
			WithMessage("source schema cannot be nil")
	}
	if targetSchema == nil {
		return common.NewSystemError("ERR_PLAN_NIL_TARGET_SCHEMA").
			WithMessage("target schema cannot be nil")
	}
	if migration == nil {
		return common.NewSystemError("ERR_PLAN_NIL_MIGRATION").
			WithMessage("migration cannot be nil")
	}
	return nil
}

// determineStrategy analyzes changes and decides the execution strategy.
func (e *DefaultMigrationEngine) determineStrategy(source *schema.SchemaDefinition, changes []schema.SchemaChange) MigrationStrategy {
	for _, change := range changes {
		if e.requiresBlueGreen(source, change) {
			return MigrationStrategyBlueGreen
		}
	}
	return MigrationStrategyInPlace
}

// requiresBlueGreen checks if a change necessitates creating a new collection.
func (e *DefaultMigrationEngine) requiresBlueGreen(source *schema.SchemaDefinition, change schema.SchemaChange) bool {
	switch change.Type {
	case schema.SchemaChangeTypeRemoveField:
		return true

	case schema.SchemaChangeTypeAddField:
		return e.isAddFieldDestructive(change)

	case schema.SchemaChangeTypeModifyField:
		return e.isModifyFieldDestructive(source, change)

	case schema.SchemaChangeTypeModifyIndex:
		return e.isModifyIndexDestructive(change)

	case schema.SchemaChangeTypeModifySchema:
		return e.isModifySchemaDestructive(source, change)

	default:
		// All other changes can be done in-place:
		// - AddIndex, RemoveIndex (index operations)
		// - AddConstraint, RemoveConstraint, ModifyConstraint (WASM-enforced)
		// - ModifyProperty (metadata)
		// - AddSchema, RemoveSchema (nested schema registry)
		// - ModifySchemaReference (field-level schema changes)
		return false
	}
}

// isAddFieldDestructive checks if adding a field requires BlueGreen.
func (e *DefaultMigrationEngine) isAddFieldDestructive(change schema.SchemaChange) bool {
	if change.SchemaChangeAddFieldPayload == nil {
		return false
	}

	field := change.SchemaChangeAddFieldPayload.Definition
	// Destructive if required field without default (needs data backfill)
	return field.Required != nil && *field.Required && field.Default == nil
}

// isModifyFieldDestructive checks if modifying a field requires BlueGreen.
func (e *DefaultMigrationEngine) isModifyFieldDestructive(source *schema.SchemaDefinition, change schema.SchemaChange) bool {
	if change.ID == nil {
		return true // Unknown field - conservative
	}

	field, exists := source.Fields[*change.ID]
	if !exists {
		return true // Field not found - conservative
	}

	if change.SchemaChangeModifyFieldPayload == nil {
		return false
	}

	mods := change.SchemaChangeModifyFieldPayload.Changes

	// Type change is always destructive
	if mods.Type != nil && *mods.Type != field.Type {
		return true
	}

	// Making field required without default is destructive
	if mods.Required != nil && *mods.Required {
		if field.Required == nil || !*field.Required {
			// Field becoming required - check if it has default
			if mods.Default == nil && field.Default == nil {
				return true
			}
		}
	}

	// Renaming field is destructive (requires data migration)
	if mods.Name != nil && *mods.Name != field.Name {
		return true
	}

	// Changing array/set element type is destructive
	if mods.ItemsType != nil {
		if field.ItemsType == nil || *mods.ItemsType != *field.ItemsType {
			return true
		}
	}

	// Changing nested schema structure is destructive
	if mods.Schema != nil && !e.isSchemaSafeChange(field.Schema, mods.Schema) {
		return true
	}

	// Removing enum values is destructive
	if len(mods.Values) > 0 && e.hasRemovedEnumValues(field.Values, mods.Values) {
		return true
	}

	return false
}

// isModifyIndexDestructive checks if modifying an index requires BlueGreen.
func (e *DefaultMigrationEngine) isModifyIndexDestructive(change schema.SchemaChange) bool {
	if change.SchemaChangeModifyIndexPayload == nil {
		return false
	}

	changes := change.SchemaChangeModifyIndexPayload.Changes

	// Modifying to primary key type is destructive
	if changes.Type != nil && *changes.Type == schema.IndexTypePrimary {
		return true
	}

	// Modifying primary key fields is destructive
	if change.Name != nil && len(changes.Fields) > 0 {
		if isPrimaryKeyName(*change.Name) {
			return true
		}
	}

	return false
}

// isModifySchemaDestructive checks if modifying a nested schema requires BlueGreen.
func (e *DefaultMigrationEngine) isModifySchemaDestructive(source *schema.SchemaDefinition, change schema.SchemaChange) bool {
	if change.SchemaChangeModifySchemaPayload == nil {
		return false
	}

	if change.ID == nil {
		return true // Unknown schema - conservative
	}

	// Get the nested schema definition
	nestedSchemaDef, ok := source.FindNestedSchemaById(*change.ID)
	if !ok {
		return true // Schema not found - conservative
	}

	// Create temporary schema for nested context
	tempSchema := &schema.SchemaDefinition{
		Name:          nestedSchemaDef.Name,
		Fields:        extractFieldsFromNested(nestedSchemaDef),
		NestedSchemas: source.NestedSchemas,
		Constraints:   nestedSchemaDef.Constraints,
		Indexes:       nestedSchemaDef.Indexes,
	}

	// Recursively check nested changes
	for _, nestedChange := range change.SchemaChangeModifySchemaPayload.Changes {
		if e.requiresBlueGreen(tempSchema, nestedChange) {
			return true
		}
	}

	return false
}

// isSchemaSafeChange checks if a schema modification is safe.
func (e *DefaultMigrationEngine) isSchemaSafeChange(oldSchema, newSchema any) bool {
	if oldSchema == nil {
		return newSchema == nil // Adding schema to untyped field is destructive
	}

	switch old := oldSchema.(type) {
	case schema.NestedSchemaReference:
		if new, ok := newSchema.(schema.NestedSchemaReference); ok {
			return compareNestedSchemaRef(old, new)
		}
		return false

	case []schema.NestedSchemaReference:
		if new, ok := newSchema.([]schema.NestedSchemaReference); ok {
			return compareSchemaRefSlices(old, new)
		}
		return false

	default:
		return reflect.DeepEqual(oldSchema, newSchema)
	}
}

// hasRemovedEnumValues checks if any enum values were removed.
func (e *DefaultMigrationEngine) hasRemovedEnumValues(oldValues, newValues []any) bool {
	if len(oldValues) == 0 {
		return false // No old values to remove
	}

	// Create set of new values for quick lookup
	newValueSet := make(map[string]bool)
	for _, newVal := range newValues {
		newValueSet[formatValue(newVal)] = true
	}

	// Check if any old values are missing
	for _, oldVal := range oldValues {
		if !newValueSet[formatValue(oldVal)] {
			return true // Value was removed
		}
	}

	return false
}

// filterInPlaceChanges filters changes that can be applied in-place.
func (e *DefaultMigrationEngine) filterInPlaceChanges(
	sourceSchema *schema.SchemaDefinition,
	changes []schema.SchemaChange,
) []schema.SchemaChange {
	safe := make([]schema.SchemaChange, 0, len(changes))

	for _, change := range changes {
		if !e.requiresBlueGreen(sourceSchema, change) && e.isDriverActionRequired(change) {
			safe = append(safe, change)
		}
	}

	return safe
}

// isDriverActionRequired returns true if the change affects DB structure.
func (e *DefaultMigrationEngine) isDriverActionRequired(change schema.SchemaChange) bool {
	switch change.Type {
	case schema.SchemaChangeTypeAddIndex,
		schema.SchemaChangeTypeRemoveIndex,
		schema.SchemaChangeTypeModifyIndex,
		schema.SchemaChangeTypeAddField,
		schema.SchemaChangeTypeModifyField:
		return true
	default:
		// Constraints, deprecations, and metadata are app/WASM layer concerns
		return false
	}
}

// Helper functions

// cleanupEmptyCollections removes empty slices/maps to match omitempty behavior.
func cleanupEmptyCollections(m map[string]any) {
	keysToCleanup := []string{"indexes", "nestedSchemas", "migrations", "constraints"}

	for _, key := range keysToCleanup {
		val, ok := m[key]
		if !ok || val == nil {
			continue
		}

		v := reflect.ValueOf(val)
		kind := v.Kind()

		if (kind == reflect.Slice || kind == reflect.Map) && v.Len() == 0 {
			delete(m, key)
		}
	}
}

// getTargetVersion extracts the target version from a migration version.
func getTargetVersion(version schema.MigrationVersion) string {
	if version.Target != nil {
		return *version.Target
	}
	return ""
}

// extractFieldsFromNested extracts fields from a nested schema definition.
func extractFieldsFromNested(nested *schema.NestedSchemaDefinition) map[string]*schema.FieldDefinition {
	if nested.Fields == nil {
		return make(map[string]*schema.FieldDefinition)
	}
	if nested.Fields.IsMap() {
		return nested.Fields.FieldsMap
	}
	return make(map[string]*schema.FieldDefinition)
}

// isPrimaryKeyName checks if a name indicates a primary key.
func isPrimaryKeyName(name string) bool {
	lower := strings.ToLower(name)
	return lower == "primary" || lower == "pk" || strings.Contains(lower, "primary")
}

// compareNestedSchemaRef compares two nested schema references.
func compareNestedSchemaRef(source, target schema.NestedSchemaReference) bool {
	if source.ID != target.ID {
		return false
	}

	if !reflect.DeepEqual(source.Constraints, target.Constraints) {
		return false
	}

	if len(source.Indexes) != len(target.Indexes) {
		return false
	}

	return reflect.DeepEqual(source.Indexes, target.Indexes)
}

// compareSchemaRefSlices compares two slices of nested schema references.
func compareSchemaRefSlices(source, target []schema.NestedSchemaReference) bool {
	if len(source) != len(target) {
		return false
	}

	for i := range source {
		if !compareNestedSchemaRef(source[i], target[i]) {
			return false
		}
	}

	return true
}

// formatValue converts any value to a string for comparison.
func formatValue(v any) string {
	return fmt.Sprintf("%v", v)
}

