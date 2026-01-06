package schema

/* import (
	"encoding/json"
	cjson "github.com/asaidimu/go-anansi/v6/core/json"

	"fmt"
)

// SchemaChangeToPatch converts a schema change to JSON Patch operations
func SchemaChangeToPatch(change SchemaChange, s *SchemaDefinition) ([]cjson.PatchOperation, error) {
	patches := make([]cjson.PatchOperation, 0)

	switch change.Type {
	case SchemaChangeTypeAddField:
		if change.SchemaChangeAddFieldPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing addField payload"}
		}
		patches = append(patches, cjson.PatchOperation{
			Op:    "add",
			Path:  fmt.Sprintf("/fields/%s", *change.ID),
			Value: change.Definition,
		})

	case SchemaChangeTypeRemoveField:
		patches = append(patches, cjson.PatchOperation{
			Op:   "remove",
			Path: fmt.Sprintf("/fields/%s", *change.ID),
		})

	case SchemaChangeTypeModifyField:
		if change.SchemaChangeModifyFieldPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing modifyField payload"}
		}
		fieldPath := fmt.Sprintf("/fields/%s", *change.ID)

		// Use reflection to iterate over changes
		changesMap := make(map[string]any)
		changesJSON, _ := json.Marshal(change.Changes)
		json.Unmarshal(changesJSON, &changesMap)

		for key, value := range changesMap {
			if value != nil {
				patches = append(patches, cjson.PatchOperation{
					Op:    "replace",
					Path:  fmt.Sprintf("%s/%s", fieldPath, key),
					Value: value,
				})
			}
		}

	case SchemaChangeTypeDeprecateField:
		patches = append(patches, cjson.PatchOperation{
			Op:    "add",
			Path:  fmt.Sprintf("/fields/%s/deprecated", *change.ID),
			Value: true,
		})

	case SchemaChangeTypeAddIndex:
		if change.SchemaChangeAddIndexPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing addIndex payload"}
		}
		if s.Indexes == nil {
			patches = append(patches, cjson.PatchOperation{
				Op:    "add",
				Path:  "/indexes",
				Value: []any{},
			})
		}
		patches = append(patches, cjson.PatchOperation{
			Op:    "add",
			Path:  "/indexes/-",
			Value: change.Definition,
		})

	case SchemaChangeTypeRemoveIndex:
		indexIdx := findIndexByName(s, *change.Name)
		if indexIdx >= 0 {
			patches = append(patches, cjson.PatchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("/indexes/%d", indexIdx),
			})
		}

	case SchemaChangeTypeModifyIndex:
		if change.SchemaChangeModifyIndexPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing modifyIndex payload"}
		}
		indexIdx := findIndexByName(s, *change.Name)
		if indexIdx >= 0 {
			changesMap := make(map[string]any)
			changesJSON, _ := json.Marshal(change.Changes)
			json.Unmarshal(changesJSON, &changesMap)

			for key, value := range changesMap {
				if value != nil {
					patches = append(patches, cjson.PatchOperation{
						Op:    "replace",
						Path:  fmt.Sprintf("/indexes/%d/%s", indexIdx, key),
						Value: value,
					})
				}
			}
		}

	case SchemaChangeTypeAddConstraint:
		if change.SchemaChangeAddConstraintPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing addConstraint payload"}
		}
		if s.Constraints == nil {
			patches = append(patches, cjson.PatchOperation{
				Op:    "add",
				Path:  "/constraints",
				Value: []any{},
			})
		}
		patches = append(patches, cjson.PatchOperation{
			Op:    "add",
			Path:  "/constraints/-",
			Value: change.Constraint,
		})

	case SchemaChangeTypeRemoveConstraint:
		constraintIdx := findConstraintIndexByName(s, *change.Name)
		if constraintIdx >= 0 {
			patches = append(patches, cjson.PatchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("/constraints/%d", constraintIdx),
			})
		}

	case SchemaChangeTypeModifyConstraint:
		if change.SchemaChangeModifyConstraintPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing modifyConstraint payload"}
		}
		constraintPath := findConstraintPath(s, *change.Name)
		if constraintPath != "" {
			changesMap := make(map[string]any)
			changesJSON, _ := json.Marshal(change.Changes)
			json.Unmarshal(changesJSON, &changesMap)

			for key, value := range changesMap {
				if value != nil {
					patches = append(patches, cjson.PatchOperation{
						Op:    "replace",
						Path:  fmt.Sprintf("%s/%s", constraintPath, key),
						Value: value,
					})
				}
			}
		}

	case SchemaChangeTypeAddSchema:
		if change.SchemaChangeAddSchemaPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing addSchema payload"}
		}
		if s.Registry == nil {
			patches = append(patches, cjson.PatchOperation{
				Op:    "add",
				Path:  "/registry",
				Value: map[string]any{"schemas": map[string]any{}},
			})
		}
		if s.Registry.Schemas == nil {
			patches = append(patches, cjson.PatchOperation{
				Op:    "add",
				Path:  "/registry/schemas",
				Value: map[string]any{},
			})
		}
		patches = append(patches, cjson.PatchOperation{
			Op:    "add",
			Path:  fmt.Sprintf("/registry/schemas/%s", *change.ID),
			Value: change.Definition,
		})

	case SchemaChangeTypeRemoveSchema:
		patches = append(patches, cjson.PatchOperation{
			Op:   "remove",
			Path: fmt.Sprintf("/registry/schemas/%s", *change.ID),
		})

	case SchemaChangeTypeModifySchema:
		if change.SchemaChangeModifySchemaPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing modifySchema payload"}
		}
		// Recursively apply nested schema changes
		for _, nestedChange := range change.Changes {
			nestedPatches, err := SchemaChangeToPatch(nestedChange, s)
			if err != nil {
				return nil, err
			}
			// Prefix paths with the schema ID
			for _, p := range nestedPatches {
				p.Path = fmt.Sprintf("/registry/schemas/%s%s", *change.ID, p.Path)
				patches = append(patches, p)
			}
		}

	case SchemaChangeTypeModifyProperty:
		if change.SchemaChangeModifyPropertyPayload == nil {
			return nil, &cjson.JsonPatchError{Message: "missing modifyProperty payload"}
		}
		patches = append(patches, cjson.PatchOperation{
			Op:    "replace",
			Path:  fmt.Sprintf("/%s", *change.ID),
			Value: change.Value,
		})

	default:
		return nil, &cjson.JsonPatchError{
			Message: fmt.Sprintf("unknown schema change type: %s", change.Type),
		}
	}

	return patches, nil
}

// findIndexByName finds the index of an index definition by name
func findIndexByName(s *SchemaDefinition, name string) int {
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

// findConstraintIndexByName finds the index of a constraint by name
func findConstraintIndexByName(s *SchemaDefinition, name string) int {
	if s.Constraints == nil {
		return -1
	}

	for i, rule := range s.Constraints {
		if rule.IsConstraint() && rule.Constraint.Name == name {
			return i
		}
		if rule.IsConstraintGroup() && rule.ConstraintGroup.Name == name {
			return i
		}
	}
	return -1
}

// findConstraintPath recursively finds the path to a constraint in the schema
func findConstraintPath(s *SchemaDefinition, name string) string {
	if s.Constraints == nil {
		return ""
	}

	for i, rule := range s.Constraints {
		if rule.IsConstraint() && rule.Constraint.Name == name {
			return fmt.Sprintf("/constraints/%d", i)
		}

		if rule.IsConstraintGroup() {
			if rule.ConstraintGroup.Name == name {
				return fmt.Sprintf("/constraints/%d", i)
			}
			path := searchRulesForConstraint(rule.ConstraintGroup.Rules, name)
			if path != "" {
				return fmt.Sprintf("/constraints/%d%s", i, path)
			}
		}
	}
	return ""
}

// searchRulesForConstraint recursively searches constraint rules for a named constraint
func searchRulesForConstraint(rules []ConstraintRule[FieldType], name string) string {
	for i, rule := range rules {
		if rule.IsConstraint() && rule.Constraint.Name == name {
			return fmt.Sprintf("/rules/%d", i)
		}

		if rule.IsConstraintGroup() {
			if rule.ConstraintGroup.Name == name {
				return fmt.Sprintf("/rules/%d", i)
			}
			path := searchRulesForConstraint(rule.ConstraintGroup.Rules, name)
			if path != "" {
				return fmt.Sprintf("/rules/%d%s", i, path)
			}
		}
	}
	return ""
} */

