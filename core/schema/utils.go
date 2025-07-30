package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func FieldTypePtr(fd FieldType) *FieldType {
	return &fd
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

// GetFieldValue retrieves a field value from a record, supporting nested field access.
func (d *Document) GetFieldValue(path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = *d

	for i, part := range parts {
		if current == nil {
			return nil, false
		}
		currentMap, ok := current.(map[string]any)
		if !ok {
			// If it's a schema.Document, convert it to map[string]any
			if doc, isDoc := current.(Document); isDoc {
				currentMap = doc
				ok = true
			} else {
				return nil, false
			}
		}
		value, exists := currentMap[part]
		if !exists {
			return nil, false
		}

		if i == len(parts)-1 {
			return value, true
		}
		current = value
	}
	return nil, false
}


func (f *FieldDefinition) CoerceValue(value any) (any, bool) {
	str, ok := value.(string)
	if !ok {
		return value, false
	}
	switch f.Type {
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

func (s *SchemaDefinition) GetValueByPath(data any, path string) (any, bool) {
	return getValueByPath(data, path)
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

func getValueByPath(data any, path string) (any, bool) {
	if path == "" {
		return data, true
	}
	keys := strings.Split(path, ".")
	current := data
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {

			return nil, false
		}
		current, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}


func EvaluateLogicalOperator(operator LogicalOperator, results []bool) bool {
	switch operator {
	case LogicalAnd:
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	case LogicalOr:
		for _, r := range results {
			if r {
				return true
			}
		}
		return len(results) == 0
	case LogicalNot:
		return len(results) == 1 && !results[0]
	case LogicalNor:
		for _, r := range results {
			if r {
				return false
			}
		}
		return true
	case LogicalXor:
		trueCount := 0
		for _, r := range results {
			if r {
				trueCount++
			}
		}
		return trueCount == 1
	}
	return false
}

