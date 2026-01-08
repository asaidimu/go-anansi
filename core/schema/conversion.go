package schema

import (
	"maps"
	"encoding/json"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// ============================================================================
// SCHEMA DEFINITION CLONING
// ============================================================================

// DeepClone returns a deep copy of the schema definition
func (s *SchemaDefinition) DeepClone() (*SchemaDefinition, error) {
	if s == nil {
		return nil, nil
	}

	var clone SchemaDefinition
	if err := utils.Clone(*s, &clone); err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_CLONE_FAILED").
			WithMessage("failed to deep clone schema").
			WithCause(err).
			WithOperation("schema.SchemaDefinition.DeepClone")
	}

	return &clone, nil
}

// ResolveNestedSchema finds a definition in the registry and converts it to a
// temporary SchemaDefinition with all conditional fields flattened.
func (sd *SchemaDefinition) ResolveNestedSchema(id string) (*SchemaDefinition, error) {
	nsd, ok := sd.FindNestedSchemaById(id)
	if !ok {
		return nil, NewNestedSchemaNotFoundError(id).WithOperation("schema.SchemaDefinition.ResolveNestedSchema")
	}

	// Create a new temporary schema context
	tmp := &SchemaDefinition{
		Version: sd.Version,
		Name:          nsd.Name,
		Description:   nsd.Description,
		Metadata:      nsd.Metadata,
		Constraints:   nsd.Constraints,
		Indexes:       nsd.Indexes,
		Fields:        make(map[string]*FieldDefinition),
		NestedSchemas: sd.NestedSchemas, // Propagate the registry for recursive resolution
	}

	if nsd.Fields != nil {
		maps.Copy(tmp.Fields, nsd.Fields.FieldsMap)
		for _, set := range nsd.Fields.FieldsArray {
			maps.Copy(tmp.Fields, set.Fields)
		}
	} else if nsd.Type != nil {
		tmp.Fields[id] = &FieldDefinition{
			Description: nsd.Description,
			Name:        nsd.Name, Type: *nsd.Type, Schema: nsd.Schema,
			Default:   nsd.Default,
			ItemsType: nsd.ItemsType,
		}
	}

	return tmp, nil
}

// ToNestedSchema converts the SchemaDefinition back into a NestedSchemaDefinition.
// It detects if the schema was a "primitive wrapper" and handles structured fields.
func (sd *SchemaDefinition) ToNestedSchema() *NestedSchemaDefinition {
	nsd := &NestedSchemaDefinition{
		Name:        sd.Name,
		Description: sd.Description,
		Metadata:    sd.Metadata,
		Constraints: sd.Constraints,
		Indexes:     sd.Indexes,
	}

	// If the schema was created from a nsd.Type, ResolveNestedSchema put
	// the type definition into a field keyed by the ID.
	if len(sd.Fields) == 1 {
		// We look for a field that matches the schema's identity
		// (Assuming Name was used as the fallback ID in the resolver)
		if field, ok := sd.Fields[sd.Name]; ok {
			nsd.Type = &field.Type
			nsd.Schema = field.Schema
			nsd.Default = field.Default
			nsd.ItemsType = field.ItemsType
			return nsd
		}
	}

	// Since ResolveNestedSchema is "lossy" regarding conditions (it flattens them),
	// the most honest inverse is to put everything into the FieldsMap.
	nsd.Fields = &NestedSchemaFields{
		FieldsMap: make(map[string]*FieldDefinition),
	}
	maps.Copy(nsd.Fields.FieldsMap, sd.Fields)

	return nsd
}

// FieldSchemaResolution holds both the compiled structure and the original logic for a nested schema.
type FieldSchemaResolution struct {
	Schema *SchemaDefinition       // Flattened for unexpected field checks and base constraints
	Source *NestedSchemaDefinition // Original definition for conditional logic (When clauses)
}

// ResolveFieldSchema resolves a field into its validation-ready schemas, preserving source metadata.
func (sd *SchemaDefinition) ResolveFieldSchema(field *FieldDefinition) ([]FieldSchemaResolution, error) {
	var refs []NestedSchemaReference
	switch field.Type {
	case FieldTypeObject, FieldTypeArray, FieldTypeRecord:
		if ref, ok := field.Schema.(NestedSchemaReference); ok {
			refs = append(refs, ref)
		}
	case FieldTypeUnion:
		if unionRefs, ok := field.Schema.([]NestedSchemaReference); ok {
			refs = unionRefs
		}
	default:
		return nil, nil
	}

	results := make([]FieldSchemaResolution, 0, len(refs))
	for _, ref := range refs {
		nsd, exists := sd.FindNestedSchemaById(ref.ID)
		if !exists {
			return nil, NewNestedSchemaNotFoundError(ref.ID).WithOperation("schema.SchemaDefinition.ResolveFieldSchema")
		}

		tmp, err := sd.ResolveNestedSchema(ref.ID)
		if err != nil {
			return nil, err
		}

		if len(ref.Constraints) > 0 {
			tmp.Constraints = append(tmp.Constraints, ref.Constraints...)
		}
		if len(ref.Indexes) > 0 {
			tmp.Indexes = append(tmp.Indexes, ref.Indexes...)
		}

		results = append(results, FieldSchemaResolution{
			Schema: tmp,
			Source: nsd,
		})
	}
	return results, nil
}

// MustDeepClone returns a deep copy of the schema definition, panics on error
func (s *SchemaDefinition) MustDeepClone() *SchemaDefinition {
	clone, err := s.DeepClone()
	if err != nil {
		panic(err)
	}
	return clone
}

// ShallowClone returns a shallow copy of the schema without deeply cloning nested structures
// This is useful when you want to modify top-level properties without affecting nested schemas
func (s *SchemaDefinition) ShallowClone() *SchemaDefinition {
	if s == nil {
		return nil
	}

	clone := &SchemaDefinition{
		Name:          s.Name,
		Description:   s.Description,
		Version:       s.Version,
		Fields:        s.Fields,      // Shallow copy - same map reference
		Indexes:       s.Indexes,     // Shallow copy - same slice reference
		Migrations:    s.Migrations,  // Shallow copy - same slice reference
		Constraints:   s.Constraints, // Shallow copy - same slice reference
		Hint:          s.Hint,
		NestedSchemas: s.NestedSchemas, // Shallow copy - same map reference
		Metadata:      s.Metadata,      // Shallow copy - same map reference
		Mock:          s.Mock,
	}

	return clone
}

// ============================================================================
// MAP CONVERSION
// ============================================================================

// ToMap converts the schema to a map[string]any representation
// This is useful for serialization to formats other than JSON
func (s *SchemaDefinition) ToMap() map[string]any {
	if s == nil {
		return nil
	}

	result := make(map[string]any)

	// Required fields
	result["name"] = s.Name
	result["version"] = s.Version

	// Optional fields
	if s.Description != nil {
		result["description"] = *s.Description
	}

	if len(s.Metadata) > 0 {
		result["metadata"] = s.Metadata
	}

	// Fields
	if len(s.Fields) > 0 {
		fieldsMap := make(map[string]any)
		for id, field := range s.Fields {
			fieldsMap[string(id)] = fieldDefinitionToMap(field)
		}
		result["fields"] = fieldsMap
	}

	// Indexes
	if len(s.Indexes) > 0 {
		indexesList := make([]any, 0, len(s.Indexes))
		for _, ior := range s.Indexes {
			if ior.IsIndex() {
				indexesList = append(indexesList, indexDefinitionToMap(ior.Index))
			}
		}
		result["indexes"] = indexesList
	}

	// Constraints
	if len(s.Constraints) > 0 {
		constraintsList := make([]any, 0, len(s.Constraints))
		for i := range s.Constraints {
			constraintsList = append(constraintsList, constraintRuleToMap(&s.Constraints[i]))
		}
		result["constraints"] = constraintsList
	}

	// Nested schemas
	if len(s.NestedSchemas) > 0 {
		nestedSchemasMap := make(map[string]any)
		for id, nestedSchema := range s.NestedSchemas {
			nestedSchemasMap[string(id)] = nestedSchemaDefinitionToMap(nestedSchema)
		}
		result["nestedSchemas"] = nestedSchemasMap
	}

	// Migrations
	if len(s.Migrations) > 0 {
		migrationsList := make([]any, 0, len(s.Migrations))
		for i := range s.Migrations {
			migrationsList = append(migrationsList, migrationToMap(&s.Migrations[i]))
		}
		result["migrations"] = migrationsList
	}

	// Hints
	if s.Hint != nil && len(*s.Hint) > 0 {
		result["hints"] = *s.Hint
	}

	return result
}

// fieldDefinitionToMap converts a field definition to a map
func fieldDefinitionToMap(field *FieldDefinition) map[string]any {
	result := make(map[string]any)

	result["name"] = field.Name
	result["type"] = string(field.Type)

	if field.Required != nil {
		result["required"] = *field.Required
	}
	if field.Unique != nil {
		result["unique"] = *field.Unique
	}
	if field.Deprecated != nil {
		result["deprecated"] = *field.Deprecated
	}
	if field.Description != nil {
		result["description"] = *field.Description
	}
	if field.Default != nil {
		result["default"] = field.Default
	}
	if field.Values != nil {
		result["values"] = field.Values
	}
	if field.Schema != nil {
		result["schema"] = field.Schema
	}
	if field.ItemsType != nil {
		result["itemsType"] = string(*field.ItemsType)
	}
	if len(field.Constraints) > 0 {
		constraintsList := make([]any, 0, len(field.Constraints))
		for i := range field.Constraints {
			constraintsList = append(constraintsList, constraintRuleToMap(&field.Constraints[i]))
		}
		result["constraints"] = constraintsList
	}
	if field.Hint != nil {
		result["hint"] = field.Hint
	}

	return result
}

// indexDefinitionToMap converts an index definition to a map
func indexDefinitionToMap(index *IndexDefinition) map[string]any {
	result := make(map[string]any)

	result["name"] = index.Name
	result["fields"] = index.Fields
	result["type"] = string(index.Type)

	if index.Unique != nil {
		result["unique"] = *index.Unique
	}
	if index.Description != nil {
		result["description"] = *index.Description
	}
	if index.Order != nil {
		result["order"] = *index.Order
	}
	if index.Partial != nil {
		result["partial"] = index.Partial
	}

	return result
}

// constraintRuleToMap converts a constraint rule to a map
func constraintRuleToMap(rule *ConstraintRule) map[string]any {
	if rule.IsConstraint() {
		result := make(map[string]any)
		result["name"] = rule.Constraint.Name
		result["predicate"] = rule.Constraint.Predicate

		if rule.Constraint.Type != nil {
			result["type"] = string(*rule.Constraint.Type)
		}
		if rule.Constraint.Field != nil {
			result["field"] = *rule.Constraint.Field
		}
		if len(rule.Constraint.Fields) > 0 {
			result["fields"] = rule.Constraint.Fields
		}
		if rule.Constraint.Parameters != nil {
			result["parameters"] = rule.Constraint.Parameters
		}
		if rule.Constraint.Description != nil {
			result["description"] = *rule.Constraint.Description
		}
		if rule.Constraint.ErrorMessage != nil {
			result["errorMessage"] = *rule.Constraint.ErrorMessage
		}

		return result
	}

	if rule.IsConstraintGroup() {
		result := make(map[string]any)
		result["name"] = rule.ConstraintGroup.Name
		result["operator"] = string(rule.ConstraintGroup.Operator)

		rulesList := make([]any, 0, len(rule.ConstraintGroup.Rules))
		for i := range rule.ConstraintGroup.Rules {
			rulesList = append(rulesList, constraintRuleToMap(&rule.ConstraintGroup.Rules[i]))
		}
		result["rules"] = rulesList

		return result
	}

	return nil
}

// nestedSchemaDefinitionToMap converts a nested schema definition to a map
func nestedSchemaDefinitionToMap(nsd *NestedSchemaDefinition) map[string]any {
	result := make(map[string]any)

	result["name"] = nsd.Name

	if nsd.ID != nil {
		result["id"] = *nsd.ID
	}
	if nsd.Description != nil {
		result["description"] = *nsd.Description
	}
	if len(nsd.Metadata) > 0 {
		result["metadata"] = nsd.Metadata
	}
	if nsd.Concrete != nil {
		result["concrete"] = *nsd.Concrete
	}

	// Indexes
	if len(nsd.Indexes) > 0 {
		indexesList := make([]any, 0, len(nsd.Indexes))
		for _, ior := range nsd.Indexes {
			if ior.IsIndex() {
				indexesList = append(indexesList, indexDefinitionToMap(ior.Index))
			}
		}
		result["indexes"] = indexesList
	}

	// Constraints
	if len(nsd.Constraints) > 0 {
		constraintsList := make([]any, 0, len(nsd.Constraints))
		for i := range nsd.Constraints {
			constraintsList = append(constraintsList, constraintRuleToMap(&nsd.Constraints[i]))
		}
		result["constraints"] = constraintsList
	}

	// Fields or Type
	if nsd.IsStructured() {
		if nsd.Fields.IsMap() {
			fieldsMap := make(map[string]any)
			for name, field := range nsd.Fields.FieldsMap {
				fieldsMap[name] = fieldDefinitionToMap(field)
			}
			result["fields"] = fieldsMap
		} else if nsd.Fields.IsArray() {
			result["fields"] = nsd.Fields.FieldsArray
		}
	} else if nsd.IsTyped() {
		result["type"] = string(*nsd.Type)
		if nsd.Default != nil {
			result["default"] = nsd.Default
		}
		if nsd.Schema != nil {
			result["schema"] = nsd.Schema
		}
		if nsd.ItemsType != nil {
			result["itemsType"] = string(*nsd.ItemsType)
		}
	}

	return result
}

// migrationToMap converts a migration to a map
func migrationToMap(migration *Migration) map[string]any {
	result := make(map[string]any)

	result["id"] = migration.ID
	result["version"] = map[string]any{
		"source": migration.Version.Source,
		"target": migration.Version.Target,
	}
	result["description"] = migration.Description
	result["transform"] = migration.Transform
	result["createdAt"] = migration.CreatedAt
	result["checksum"] = migration.Checksum

	if len(migration.Changes) > 0 {
		result["changes"] = migration.Changes
	}
	if len(migration.Rollback) > 0 {
		result["rollback"] = migration.Rollback
	}
	if migration.Status != "" {
		result["status"] = migration.Status
	}

	return result
}

// FromMap creates a schema definition from a map[string]any representation
func FromMap(data map[string]any) (*SchemaDefinition, error) {
	if data == nil {
		return nil, common.NewSystemError("ERR_SCHEMA_FROM_MAP").
			WithMessage("data cannot be nil").
			WithOperation("schema.FromMap")
	}

	// Convert map to JSON then unmarshal to SchemaDefinition
	// This ensures proper type conversion and validation
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_FROM_MAP").
			WithMessage("failed to marshal map to JSON").
			WithCause(err).
			WithOperation("schema.FromMap")
	}

	var schema SchemaDefinition
	if err := json.Unmarshal(jsonBytes, &schema); err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_FROM_MAP").
			WithMessage("failed to unmarshal JSON to schema").
			WithCause(err).
			WithOperation("schema.FromMap")
	}

	return &schema, nil
}

// MustFromMap creates a schema definition from a map, panics on error
func MustFromMap(data map[string]any) *SchemaDefinition {
	schema, err := FromMap(data)
	if err != nil {
		panic(err)
	}
	return schema
}

// ============================================================================
// JSON SERIALIZATION (ensure these exist)
// ============================================================================

// ToJSON converts the schema to a JSON string
func (s *SchemaDefinition) ToJSON() (string, error) {
	return utils.ToJSON(s)
}

// ToJSONBytes converts the schema to JSON bytes
func (s *SchemaDefinition) ToJSONBytes() ([]byte, error) {
	return utils.ToJSONBytes(s)
}

// MustToJSON converts the schema to a JSON string, panics on error
func (s *SchemaDefinition) MustToJSON() string {
	jsonStr, err := s.ToJSON()
	if err != nil {
		panic(err)
	}
	return jsonStr
}

// FromJSON creates a schema from a JSON string without validation
func FromJSON(jsonStr string) (*SchemaDefinition, error) {
	var schema SchemaDefinition
	if err := utils.FromJSON([]byte(jsonStr), &schema); err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_FROM_JSON").
			WithMessage("failed to parse JSON").
			WithCause(err).
			WithOperation("schema.FromJSON")
	}
	return &schema, nil
}

// FromJSONBytes creates a schema from JSON bytes
func FromJSONBytes(data []byte) (*SchemaDefinition, error) {
	var schema SchemaDefinition
	if err := utils.FromJSON(data, &schema); err != nil {
		return nil, common.NewSystemError("ERR_SCHEMA_FROM_JSON").
			WithMessage("failed to parse JSON bytes").
			WithCause(err).
			WithOperation("schema.FromJSONBytes")
	}
	return &schema, nil
}

// MustFromJSON creates a schema from JSON, panics on error
func MustFromJSON(jsonStr string) *SchemaDefinition {
	schema, err := FromJSON(jsonStr)
	if err != nil {
		panic(err)
	}
	return schema
}
