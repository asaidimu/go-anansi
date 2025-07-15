package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FindField finds a field by its dot-separated path.
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
	if err := json.Unmarshal(jsonSchema, s); err != nil {
		return fmt.Errorf("Error unmarshaling schema: %w", err)
	}
	return nil
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
			var fieldSchema *FieldSchema
			fieldSchema, ok := currentField.Schema.(*FieldSchema)
			if !ok {
				if schema, ok := currentField.Schema.(FieldSchema); ok {
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
			var fieldSchemas []*FieldSchema
			fieldSchemas, ok := currentField.Schema.([]*FieldSchema)
			if !ok {
				if schemas, ok := currentField.Schema.([]FieldSchema); ok {
					fieldSchemas = make([]*FieldSchema, len(schemas))
					for i := range schemas {
						fieldSchemas[i] = &schemas[i] // Take the address of each element
					}
					return nil
				} else {
					return nil
				}
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
