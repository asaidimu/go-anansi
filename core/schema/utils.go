package schema

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/utils"
)

func FieldTypePtr(fd FieldType) *FieldType {
	return &fd
}

var complexFieldTypes = map[FieldType]bool{
	FieldTypeObject: true,
	FieldTypeArray:  true,
	FieldTypeSet:    true,
	FieldTypeRecord: true,
	FieldTypeUnion:  true,
}

// FindNestedSchema finds a nested schema by it's name
func (s *FieldType) IsComplex() bool {
	return complexFieldTypes[*s]
}

// FindNestedSchema finds a nested schema by it's name
func (s *SchemaDefinition) FindNestedSchema(name string) (*NestedSchemaDefinition, bool) {
	if s.NestedSchemas == nil {
		return nil, false
	}

	for _, schema := range s.NestedSchemas {
		if schema.Name == name {
			return schema, true
		}
	}
	return nil, false
}

// FindField finds a field by its dot-separated path.
func (s *SchemaDefinition) FindField(path string) *FieldDefinition {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil
	}

	var rootField *FieldDefinition
	for _, field := range s.Fields {
		if field.Name == parts[0] {
			rootField = field
			break
		}
	}

	if rootField == nil {
		return nil
	}

	if len(parts) == 1 {
		return rootField
	}

	return rootField.FindNestedField(s, parts[1:])
}

func (s *SchemaDefinition) From(jsonSchema []byte) error {
	return utils.FromJSON(jsonSchema, s)
}

// FindNestedField finds a nested field by its path segments from the current field.
func (fd *FieldDefinition) FindNestedField(schema *SchemaDefinition, path []string) *FieldDefinition {
	currentField := fd
	for _, part := range path {
		if currentField == nil {
			return nil
		}

		var nextField *FieldDefinition
		switch currentField.Type {
		case FieldTypeObject:
			var fieldSchema *NestedSchemaReference
			fieldSchema, ok := currentField.Schema.(*NestedSchemaReference)
			if !ok {
				if schema, ok := currentField.Schema.(NestedSchemaReference); ok {
					fieldSchema = &schema
				} else {
					return nil
				}
			}
			nestedSchema, ok := schema.FindNestedSchema(fieldSchema.ID)
			if !ok {
				return nil
			}
			nextField = nestedSchema.FindField(part)
		case FieldTypeUnion:
			var fieldSchemas []NestedSchemaReference
			// Try to unmarshal as []NestedSchemaReference
			if schemas, ok := currentField.Schema.([]NestedSchemaReference); ok {
				fieldSchemas = schemas
			} else if schemasPtr, ok := currentField.Schema.([]*NestedSchemaReference); ok {
				// If it's []*NestedSchemaReference, convert to []NestedSchemaReference
				fieldSchemas = make([]NestedSchemaReference, len(schemasPtr))
				for i, s := range schemasPtr {
					if s != nil {
						fieldSchemas[i] = *s
					}
				}
			} else {
				return nil // Not a supported union schema type
			}

			for _, fs := range fieldSchemas {
				nestedSchema, ok := schema.FindNestedSchema(fs.ID)
				if !ok {
					continue
				}
				if f := nestedSchema.FindField(part); f != nil {
					nextField = f
					break
				}
			}
		default:
			return nil // Not a container type, so it can't have nested fields.
		}
		currentField = nextField
	}
	return currentField
}

// FindField finds a field by its name in a nested schema.
func (nsd *NestedSchemaDefinition) FindField(name string) *FieldDefinition {
	if *nsd.IsStructured {
		if nsd.StructuredFieldsMap != nil {
			for fieldName, field := range nsd.StructuredFieldsMap {
				if fieldName == name {
					return field
				}
			}
		}
		if nsd.StructuredFieldsArray != nil {
			for _, conditionalFields := range nsd.StructuredFieldsArray {
				for fieldName, field := range conditionalFields.Fields {
					if fieldName == name {
						return field
					}
				}
			}
		}
	}
	return nil
}

// Type checking and coercion utilities
func (expectedType FieldType) Coerce(value any) (any, bool) {
	str, ok := value.(string)
	if !ok {
		return value, false
	}
	switch expectedType {
	case FieldTypeBoolean:
		lower := strings.ToLower(str)
		if lower == "true" {
			return true, true
		}
		if lower == "false" {
			return false, true
		}
	case FieldTypeInteger:
		if intVal, err := strconv.ParseInt(str, 10, 64); err == nil {
			return int(intVal), true
		}
	case FieldTypeNumber, FieldTypeDecimal:
		if floatVal, err := strconv.ParseFloat(str, 64); err == nil {
			return floatVal, true
		}
	}

	return value, false
}

func (fieldDef *FieldDefinition) ValidateType(value any) bool {
	if value == nil {
		return true
	}
	var ok bool
	switch fieldDef.Type {
	case FieldTypeString:
		_, ok = value.(string)
	case FieldTypeNumber, FieldTypeDecimal:
		switch value.(type) {
		case float64, float32, int, int64, int32:
			ok = true
		default:
			ok = false
		}
	case FieldTypeInteger:
		switch value.(type) {
		case int, int64, int32, int16, int8:
			ok = true
		default:
			ok = false
		}
	case FieldTypeBoolean:
		_, ok = value.(bool)
	case FieldTypeArray, FieldTypeSet:
		_, ok = value.([]any)
	case FieldTypeObject, FieldTypeRecord:
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Map {
			if rv.Type().Key().Kind() == reflect.String {
				ok = true
			}
		}
	case FieldTypeUnion, FieldTypeEnum, FieldTypeUnknown:
		return true
	}

	if !ok {
		return false
	}
	return true
}

func (condition *FieldInclusionCondition) Evaluate(data map[string]any) bool {
	if condition == nil {
		return true // No condition means always included
	}
	fieldValue, exists := data[condition.Field]
	if !exists {
		return false // Condition field doesn't exist
	}
	// Use reflect.DeepEqual for robust value comparison
	return reflect.DeepEqual(fieldValue, condition.Value)
}

func (s *SchemaDefinition) FieldNames() []string {
	names := make([]string, 0, len(s.Fields))
	for _, field := range s.Fields {
		names = append(names, field.Name)
	}
	sort.Strings(names)
	return names
}

func (s *SchemaDefinition) GetFields() []*FieldDefinition {
	fields := make([]*FieldDefinition, 0, len(s.Fields))
	for _, field := range s.Fields {
		fields = append(fields, field)
	}

	// Sort fields by Name
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})

	return fields
}

func (s *SchemaDefinition) ToJSON() (string, error) {
	return utils.ToJSON(s)
}

func (s *SchemaDefinition) ToJSONBytes() ([]byte, error) {
	return utils.ToJSONBytes(s)
}

func (s *SchemaDefinition) MustToJSON() string {
	jsonStr, err := s.ToJSON()
	if err != nil {
		panic(fmt.Sprintf("MustToJSON failed: %v", err))
	}
	return jsonStr
}

func (s *SchemaDefinition) Clone() (*SchemaDefinition, error) {
	var newSchema SchemaDefinition
	if err := utils.Clone(*s, &newSchema); err != nil {
		return nil, fmt.Errorf("failed to clone schema: %w", err)
	}
	return &newSchema, nil
}

func (s *SchemaDefinition) MustClone() *SchemaDefinition {
	clone, err := s.Clone()
	if err != nil {
		panic(fmt.Sprintf("MustClone failed: %v", err))
	}
	return clone
}

// AddField adds a field to the schema along with any required nested schemas.
// Returns an error if a field with the same name already exists.
// The provider function receives the current schema state and supplies the primary nested schema and its dependencies.
// This allows the provider to be intelligent about reusing existing nested schemas.
func (s *SchemaDefinition) AddField(field *FieldDefinition, provider func(*SchemaDefinition) (*NestedSchemaDefinition, []*NestedSchemaDefinition)) (*SchemaDefinition, error) {
	if s == nil || field == nil {
		return s, nil
	}

	clone := s.MustClone()

	// Ensure Fields map is initialized
	if clone.Fields == nil {
		clone.Fields = make(map[string]*FieldDefinition)
	}

	// Check if field already exists
	if _, exists := clone.Fields[field.Name]; exists {
		return nil, fmt.Errorf("field '%s' already exists in schema", field.Name)
	}

	// Add the field
	clone.Fields[field.Name] = field

	// If a provider is given, add the nested schemas
	if provider != nil {
		primary, dependencies := provider(clone)

		// Ensure NestedSchemas map is initialized
		if clone.NestedSchemas == nil {
			clone.NestedSchemas = make(map[string]*NestedSchemaDefinition)
		}

		// Add primary nested schema if provided and not already present
		if primary != nil {
			clone.NestedSchemas[primary.Name] = primary
		}

		// Add dependencies if not already present
		for _, dep := range dependencies {
			if dep != nil {
				clone.NestedSchemas[dep.Name] = dep
			}
		}
	}

	return clone, nil
}

// MustAddField adds a field to the schema along with any required nested schemas.
// Panics if a field with the same name already exists.
// The provider function receives the current schema state and supplies the primary nested schema and its dependencies.
// This allows the provider to be intelligent about reusing existing nested schemas.
func (s *SchemaDefinition) MustAddField(field *FieldDefinition, provider func(*SchemaDefinition) (*NestedSchemaDefinition, []*NestedSchemaDefinition)) *SchemaDefinition {
	result, err := s.AddField(field, provider)
	if err != nil {
		panic(err)
	}
	return result
}
