package migration

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	scjson "github.com/asaidimu/go-anansi/v6/core/json"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// SchemaChangeToPatch converts a schema change to JSON Patch operations.
func SchemaChangeToPatch(change schema.SchemaChange, s *schema.SchemaDefinition) ([]scjson.PatchOperation, error) {
	converter := &PatchConverter{schema: s}
	return converter.Convert(change)
}

// PatchConverter handles conversion of schema changes to JSON patch operations.
type PatchConverter struct {
	schema *schema.SchemaDefinition
}

func NewPatchConverter(schema *schema.SchemaDefinition) *PatchConverter {
	return &PatchConverter{schema}
}

// Convert converts a schema change to patch operations.
func (pc *PatchConverter) Convert(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	switch change.Type {
	case schema.SchemaChangeTypeModifyProperty:
		return pc.convertModifyProperty(change)
	case schema.SchemaChangeTypeAddField:
		return pc.convertAddField(change)
	case schema.SchemaChangeTypeRemoveField:
		return pc.convertRemoveField(change)
	case schema.SchemaChangeTypeModifyField:
		return pc.convertModifyField(change)
	case schema.SchemaChangeTypeAddIndex:
		return pc.convertAddIndex(change)
	case schema.SchemaChangeTypeRemoveIndex:
		return pc.convertRemoveIndex(change)
	case schema.SchemaChangeTypeModifyIndex:
		return pc.convertModifyIndex(change)
	case schema.SchemaChangeTypeAddConstraint:
		return pc.convertAddConstraint(change)
	case schema.SchemaChangeTypeRemoveConstraint:
		return pc.convertRemoveConstraint(change)
	case schema.SchemaChangeTypeModifyConstraint:
		return pc.convertModifyConstraint(change)
	case schema.SchemaChangeTypeAddSchema:
		return pc.convertAddSchema(change)
	case schema.SchemaChangeTypeRemoveSchema:
		return pc.convertRemoveSchema(change)
	case schema.SchemaChangeTypeModifySchema:
		return pc.convertModifySchema(change)
	case schema.SchemaChangeTypeModifySchemaReference:
		return pc.convertModifySchemaReference(change)
	default:
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("unknown schema change type: %s", change.Type)
	}
}

// convertModifyProperty handles property modification patches.
func (pc *PatchConverter) convertModifyProperty(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeModifyPropertyPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing modifyProperty payload")
	}
	if change.ID == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing property ID")
	}

	return []scjson.PatchOperation{{
		Op:    "replace",
		Path:  fmt.Sprintf("/%s", *change.ID),
		Value: change.Value,
	}}, nil
}

// convertAddField handles field addition patches.
func (pc *PatchConverter) convertAddField(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeAddFieldPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing addField payload")
	}
	if change.ID == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing field ID")
	}

	return []scjson.PatchOperation{{
		Op:    "add",
		Path:  fmt.Sprintf("/fields/%s", *change.ID),
		Value: change.SchemaChangeAddFieldPayload.Definition,
	}}, nil
}

// convertRemoveField handles field removal patches.
func (pc *PatchConverter) convertRemoveField(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.ID == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing field ID")
	}

	return []scjson.PatchOperation{{
		Op:   "remove",
		Path: fmt.Sprintf("/fields/%s", *change.ID),
	}}, nil
}

// convertModifyField handles field modification patches.
func (pc *PatchConverter) convertModifyField(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeModifyFieldPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing modifyField payload")
	}
	if change.ID == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing field ID")
	}

	fieldPath := fmt.Sprintf("/fields/%s", *change.ID)
	changes := change.SchemaChangeModifyFieldPayload.Changes
	oldField, fieldExists := pc.schema.Fields[*change.ID]

	if !fieldExists {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("field %s not found in schema", *change.ID)
	}

	patches := make([]scjson.PatchOperation, 0)

	// Simple replacements - only for non-nil values
	if changes.Type != nil {
		patches = append(patches, pc.createReplacePatch(fieldPath, "type", *changes.Type))
	}
	if changes.Name != nil {
		patches = append(patches, pc.createReplacePatch(fieldPath, "name", *changes.Name))
	}
	if changes.Values != nil {
		patches = append(patches, pc.createReplacePatch(fieldPath, "values", changes.Values))
	}
	if changes.Schema != nil {
		patches = append(patches, pc.createReplacePatch(fieldPath, "schema", changes.Schema))
	}
	if changes.Constraints != nil {
		patches = append(patches, pc.createReplacePatch(fieldPath, "constraints", changes.Constraints))
	}

	// Handle optional pointer fields with proper add/remove/replace logic
	patches = append(patches, pc.handleOptionalFieldProperty(fieldPath, "required", changes.Required, oldField.Required)...)
	patches = append(patches, pc.handleOptionalFieldProperty(fieldPath, "unique", changes.Unique, oldField.Unique)...)
	patches = append(patches, pc.handleOptionalFieldProperty(fieldPath, "description", changes.Description, oldField.Description)...)
	patches = append(patches, pc.handleOptionalFieldProperty(fieldPath, "default", changes.Default, oldField.Default)...)
	patches = append(patches, pc.handleOptionalFieldProperty(fieldPath, "itemsType", changes.ItemsType, oldField.ItemsType)...)
	patches = append(patches, pc.handleOptionalFieldProperty(fieldPath, "deprecated", changes.Deprecated, oldField.Deprecated)...)
	patches = append(patches, pc.handleOptionalFieldProperty(fieldPath, "hint", changes.Hint, oldField.Hint)...)

	// Handle unset operations - only remove fields that exist in the old schema
	for _, unsetField := range changes.Unset {
		shouldRemove := false

		switch unsetField {
		case "required":
			shouldRemove = oldField.Required != nil
		case "unique":
			shouldRemove = oldField.Unique != nil
		case "description":
			shouldRemove = oldField.Description != nil
		case "hint":
			shouldRemove = oldField.Hint != nil
		case "default":
			shouldRemove = oldField.Default != nil
		case "itemsType":
			shouldRemove = oldField.ItemsType != nil
		case "deprecated":
			shouldRemove = oldField.Deprecated != nil
		case "values":
			shouldRemove = len(oldField.Values) > 0
		case "schema":
			shouldRemove = oldField.Schema != nil
		case "constraints":
			shouldRemove = len(oldField.Constraints) > 0
		}

		if shouldRemove {
			patches = append(patches, scjson.PatchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("%s/%s", fieldPath, unsetField),
			})
		}
	}

	return patches, nil
}

// convertAddIndex handles index addition patches.
func (pc *PatchConverter) convertAddIndex(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeAddIndexPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing addIndex payload")
	}

	patches := make([]scjson.PatchOperation, 0, 2)

	// Initialize indexes array if needed
	if pc.schema.Indexes == nil {
		patches = append(patches, scjson.PatchOperation{
			Op:    "add",
			Path:  "/indexes",
			Value: []any{},
		})
	}

	patches = append(patches, scjson.PatchOperation{
		Op:    "add",
		Path:  "/indexes/-",
		Value: change.SchemaChangeAddIndexPayload.Definition,
	})

	return patches, nil
}

// convertRemoveIndex handles index removal patches.
func (pc *PatchConverter) convertRemoveIndex(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.Name == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing index name")
	}

	indexIdx := findIndexByName(pc.schema, *change.Name)
	if indexIdx < 0 {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("index %s not found", *change.Name)
	}

	return []scjson.PatchOperation{{
		Op:   "remove",
		Path: fmt.Sprintf("/indexes/%d", indexIdx),
	}}, nil
}

// convertModifyIndex handles index modification patches.
func (pc *PatchConverter) convertModifyIndex(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeModifyIndexPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing modifyIndex payload")
	}
	if change.Name == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing index name")
	}

	indexIdx := findIndexByName(pc.schema, *change.Name)
	if indexIdx < 0 {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("index %s not found", *change.Name)
	}

	indexPath := fmt.Sprintf("/indexes/%d", indexIdx)
	changes := change.SchemaChangeModifyIndexPayload.Changes
	patches := make([]scjson.PatchOperation, 0)

	// Simple replacements
	if changes.Type != nil {
		patches = append(patches, pc.createReplacePatch(indexPath, "type", *changes.Type))
	}
	if changes.Fields != nil {
		patches = append(patches, pc.createReplacePatch(indexPath, "fields", changes.Fields))
	}
	if changes.Unique != nil {
		patches = append(patches, pc.createReplacePatch(indexPath, "unique", *changes.Unique))
	}
	if changes.Description != nil {
		patches = append(patches, pc.createReplacePatch(indexPath, "description", *changes.Description))
	}
	if changes.Order != nil {
		patches = append(patches, pc.createReplacePatch(indexPath, "order", *changes.Order))
	}
	if changes.Partial != nil {
		patches = append(patches, pc.createReplacePatch(indexPath, "partial", changes.Partial))
	}

	// Handle unset operations
	for _, unsetField := range changes.Unset {
		switch unsetField {
		case "fields", "unique", "description", "order", "partial":
			patches = append(patches, scjson.PatchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("%s/%s", indexPath, unsetField),
			})
		}
	}

	return patches, nil
}

// convertAddConstraint handles constraint addition patches.
func (pc *PatchConverter) convertAddConstraint(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeAddConstraintPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing addConstraint payload")
	}

	patches := make([]scjson.PatchOperation, 0, 2)

	// Initialize constraints array if needed
	if pc.schema.Constraints == nil {
		patches = append(patches, scjson.PatchOperation{
			Op:    "add",
			Path:  "/constraints",
			Value: []any{},
		})
	}

	patches = append(patches, scjson.PatchOperation{
		Op:    "add",
		Path:  "/constraints/-",
		Value: change.Constraint,
	})

	return patches, nil
}

// convertRemoveConstraint handles constraint removal patches.
func (pc *PatchConverter) convertRemoveConstraint(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.Name == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing constraint name")
	}

	constraintPath := findConstraintPath(pc.schema, *change.Name)
	if constraintPath == "" {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("constraint %s not found", *change.Name)
	}

	return []scjson.PatchOperation{{
		Op:   "remove",
		Path: constraintPath,
	}}, nil
}

// convertModifyConstraint handles constraint modification patches.
func (pc *PatchConverter) convertModifyConstraint(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeModifyConstraintPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing modifyConstraint payload")
	}
	if change.Name == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing constraint name")
	}

	constraintPath := findConstraintPath(pc.schema, *change.Name)
	if constraintPath == "" {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("constraint %s not found", *change.Name)
	}

	changes := change.SchemaChangeModifyConstraintPayload.Changes
	patches := make([]scjson.PatchOperation, 0)

	// Simple replacements
	if changes.Predicate != nil {
		patches = append(patches, pc.createReplacePatch(constraintPath, "predicate", *changes.Predicate))
	}
	if changes.Field != nil {
		patches = append(patches, pc.createReplacePatch(constraintPath, "field", *changes.Field))
	}
	if changes.Fields != nil {
		patches = append(patches, pc.createReplacePatch(constraintPath, "fields", changes.Fields))
	}
	if changes.Parameters != nil {
		patches = append(patches, pc.createReplacePatch(constraintPath, "parameters", changes.Parameters))
	}
	if changes.Description != nil {
		patches = append(patches, pc.createReplacePatch(constraintPath, "description", *changes.Description))
	}
	if changes.ErrorMessage != nil {
		patches = append(patches, pc.createReplacePatch(constraintPath, "errorMessage", *changes.ErrorMessage))
	}
	if changes.Operator != nil {
		patches = append(patches, pc.createReplacePatch(constraintPath, "operator", *changes.Operator))
	}

	// Handle unset operations
	for _, unsetField := range changes.Unset {
		switch unsetField {
		case "predicate", "field", "fields", "parameters", "description", "errorMessage", "operator":
			patches = append(patches, scjson.PatchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("%s/%s", constraintPath, unsetField),
			})
		}
	}

	return patches, nil
}

// convertAddSchema handles nested schema addition patches.
func (pc *PatchConverter) convertAddSchema(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeAddSchemaPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing addSchema payload")
	}
	if change.ID == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing schema ID")
	}

	patches := make([]scjson.PatchOperation, 0, 2)

	// Initialize nestedSchemas map if needed
	if pc.schema.NestedSchemas == nil {
		patches = append(patches, scjson.PatchOperation{
			Op:    "add",
			Path:  "/nestedSchemas",
			Value: map[string]any{},
		})
	}

	patches = append(patches, scjson.PatchOperation{
		Op:    "add",
		Path:  fmt.Sprintf("/nestedSchemas/%s", *change.ID),
		Value: change.SchemaChangeAddSchemaPayload.Definition,
	})

	return patches, nil
}

// convertRemoveSchema handles nested schema removal patches.
func (pc *PatchConverter) convertRemoveSchema(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.ID == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing schema ID")
	}

	return []scjson.PatchOperation{{
		Op:   "remove",
		Path: fmt.Sprintf("/nestedSchemas/%s", *change.ID),
	}}, nil
}

// convertModifySchema handles nested schema modification patches.
func (pc *PatchConverter) convertModifySchema(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeModifySchemaPayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing modifySchema payload")
	}
	if change.ID == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessage("missing schema ID")
	}

	schemaID := *change.ID

	// Get the nested schema to pass as context for nested changes
	nestedSchema, exists := pc.schema.NestedSchemas[schemaID]
	if !exists {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("nested schema %s not found in parent schema", schemaID)
	}

	// Create a temporary schema definition for the nested schema
	// to use as context for nested change conversions
	tempSchema := &schema.SchemaDefinition{
		Name:        nestedSchema.Name,
		Description: nestedSchema.Description,
		Indexes:     nestedSchema.Indexes,
		Constraints: nestedSchema.Constraints,
	}

	// If the nested schema has fields, populate them
	if nestedSchema.Fields != nil && nestedSchema.Fields.IsMap() {
		tempSchema.Fields = nestedSchema.Fields.FieldsMap
	} else {
		tempSchema.Fields = make(map[string]*schema.FieldDefinition)
	}

	// Create a temporary converter with the nested schema context
	nestedConverter := &PatchConverter{schema: tempSchema}

	patches := make([]scjson.PatchOperation, 0)

	// Recursively apply nested schema changes
	for _, nestedChange := range change.SchemaChangeModifySchemaPayload.Changes {
		nestedPatches, err := nestedConverter.Convert(nestedChange)
		if err != nil {
			return nil, err
		}

		// Prefix paths with the schema ID
		for _, p := range nestedPatches {
			p.Path = fmt.Sprintf("/nestedSchemas/%s%s", schemaID, p.Path)
			patches = append(patches, p)
		}
	}

	return patches, nil
}

// convertModifySchemaReference handles schema reference modification patches.
func (pc *PatchConverter) convertModifySchemaReference(change schema.SchemaChange) ([]scjson.PatchOperation, error) {
	if change.SchemaChangeModifySchemaReferencePayload == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").WithMessage("missing modifySchemaReference payload")
	}

	fieldName := change.SchemaChangeModifySchemaReferencePayload.Field
	field, exists := pc.schema.Fields[fieldName]
	if !exists {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").WithMessagef("field %s not found", fieldName)
	}

	var targetRef *schema.NestedSchemaReference
	schemaSubPath := "/schema" // Default for Object/Record

	// Handle the 'any' Schema type and resolve the correct path/reference
	switch s := field.Schema.(type) {
	case schema.NestedSchemaReference:
		targetRef = &s
	case []schema.NestedSchemaReference:
		// Logic for FieldTypeUnion
		payloadID := ""
		if change.SchemaChangeModifySchemaReferencePayload.ID != nil {
			payloadID = *change.SchemaChangeModifySchemaReferencePayload.ID
		}
		for i := range s {
			if s[i].ID == payloadID {
				targetRef = &s[i]
				schemaSubPath = fmt.Sprintf("/schema/%d", i)
				break
			}
		}
	}

	if targetRef == nil {
		return nil, common.NewSystemError("ERR_CREATE_MIGRATION_PATCH").
			WithMessagef("could not resolve schema reference for field %s", fieldName)
	}

	// Create context for the nested changes
	// Note: NestedSchemaReference doesn't have Fields, so we only provide Indexes/Constraints
	tempSchema := &schema.SchemaDefinition{
		Indexes:     targetRef.Indexes,
		Constraints: targetRef.Constraints,
		// If you expect to modify fields inside a reference, you would need to
		// resolve the actual definition from a Registry (not currently in your struct).
	}

	nestedConverter := &PatchConverter{schema: tempSchema}
	patches := make([]scjson.PatchOperation, 0)

	for _, nestedChange := range change.SchemaChangeModifySchemaReferencePayload.Changes {
		nestedPatches, err := nestedConverter.Convert(nestedChange)
		if err != nil {
			return nil, err
		}

		for _, p := range nestedPatches {
			// Combine: /fields/{name} + /schema[/index] + {nestedPath}
			p.Path = fmt.Sprintf("/fields/%s%s%s", fieldName, schemaSubPath, p.Path)
			patches = append(patches, p)
		}
	}

	return patches, nil
}

// Helper methods

// createReplacePatch creates a replace patch operation.
func (pc *PatchConverter) createReplacePatch(basePath, property string, value any) scjson.PatchOperation {
	return scjson.PatchOperation{
		Op:    "replace",
		Path:  fmt.Sprintf("%s/%s", basePath, property),
		Value: value,
	}
}

// handleOptionalFieldProperty handles add/remove/replace for optional field properties.
// Returns empty slice if both old and new are nil (no change needed).
func (pc *PatchConverter) handleOptionalFieldProperty(fieldPath, property string, newValue, oldValue any) []scjson.PatchOperation {
	path := fmt.Sprintf("%s/%s", fieldPath, property)

	// Determine if values are nil (checking for both actual nil and pointer-to-nil)
	hasOld := !isNilOrEmpty(oldValue)
	hasNew := !isNilOrEmpty(newValue)

	// No change needed if both are nil/empty
	if !hasOld && !hasNew {
		return nil
	}

	patches := make([]scjson.PatchOperation, 0, 1)

	// Dereference pointers to get actual values for patch operations
	actualNewValue := dereferenceValue(newValue)

	if hasNew && hasOld {
		// Replace existing value
		patches = append(patches, scjson.PatchOperation{
			Op:    "replace",
			Path:  path,
			Value: actualNewValue,
		})
	} else if hasNew && !hasOld {
		// Add new value
		patches = append(patches, scjson.PatchOperation{
			Op:    "add",
			Path:  path,
			Value: actualNewValue,
		})
	} else if !hasNew && hasOld {
		// Remove existing value - but only if it's explicitly marked for removal via Unset
		// Don't remove here, let the Unset handling do it
		return nil
	}

	return patches
}

// isNilOrEmpty checks if a value is nil or represents an empty/nil state.
func isNilOrEmpty(v any) bool {
	if v == nil {
		return true
	}

	// Use reflection to check for nil pointers
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		return val.IsNil()
	default:
		return false
	}
}

// dereferenceValue dereferences pointer values to get their actual value.
// Returns the original value if it's not a pointer.
func dereferenceValue(v any) any {
	if v == nil {
		return nil
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		return val.Elem().Interface()
	}

	return v
}

// findIndexByName finds the array index of an index definition by name.
func findIndexByName(s *schema.SchemaDefinition, name string) int {
	if s.Indexes == nil {
		return -1
	}

	for i, idxOrRef := range s.Indexes {
		if idxOrRef.IsIndex() && idxOrRef.Index.Name == name {
			return i
		}
	}
	return -1
}

// findConstraintPath recursively finds the JSON path to a constraint.
// Handles hierarchical names like "groupName/constraintName".
func findConstraintPath(s *schema.SchemaDefinition, name string) string {
	if s.Constraints == nil {
		return ""
	}

	// Parse hierarchical name
	nameParts := parseHierarchicalName(name)

	return findConstraintPathRecursive(s.Constraints, nameParts, "/constraints", 0)
}

// findConstraintPathRecursive recursively searches for a constraint path.
func findConstraintPathRecursive(rules schema.SchemaConstraint, nameParts []string, currentPath string, depth int) string {
	if depth >= len(nameParts) {
		return ""
	}

	targetName := nameParts[depth]

	for i := range rules {
		rule := &rules[i]
		var ruleName string

		if rule.IsConstraint() {
			ruleName = rule.Constraint.Name
		} else if rule.IsConstraintGroup() {
			ruleName = rule.ConstraintGroup.Name
		}

		if ruleName == targetName {
			rulePath := fmt.Sprintf("%s/%d", currentPath, i)

			// Check if this is the final part of the hierarchical name
			if depth == len(nameParts)-1 {
				return rulePath
			}

			// Not the final part - need to search within this group
			if rule.IsConstraintGroup() {
				nestedPath := findConstraintPathRecursive(
					rule.ConstraintGroup.Rules,
					nameParts,
					fmt.Sprintf("%s/rules", rulePath),
					depth+1,
				)
				if nestedPath != "" {
					return nestedPath
				}
			}
		}
	}

	return ""
}

// parseHierarchicalName splits a hierarchical name like "group/subgroup/constraint".
func parseHierarchicalName(name string) []string {
	return strings.Split(name, "/")
}
