package schema

import (
	"fmt"
)

func (schema *SchemaDefinition) Validate() error {
	basePath := ""
	return schema.validateSchemaSemanticRecursive(basePath)
}

func (schema *SchemaDefinition) validateSchemaSemanticRecursive(basePath string) error {
	for fieldName, fieldDef := range schema.Fields {
		fieldPath := buildPath(basePath, fieldName)

		if err := validateFieldSemantic(fieldDef, schema, fieldPath); err != nil {
			return err
		}

		if err := validateNestedSchemaSemantics(fieldDef, schema, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if fieldDef.Schema == nil {
		return nil
	}

	switch fieldDef.Type {
	case FieldTypeObject:
		return validateObjectFieldSemantic(fieldDef, schema, fieldPath)
	case FieldTypeRecord:
		return validateRecordFieldSemantic(fieldDef, schema, fieldPath)
	case FieldTypeUnion:
		return validateUnionFieldSemantic(fieldDef, schema, fieldPath)
	case FieldTypeArray, FieldTypeSet:
		return validateArraySetFieldSemantic(fieldDef, schema, fieldPath)
	case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeBoolean, FieldTypeDecimal, FieldTypeEnum:
		return fmt.Errorf("primitive field type '%s' at '%s' cannot have schema references", fieldDef.Type, fieldPath)
	}
	return nil
}

func validateObjectFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
		if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
			if nestedSchemaDef.IsStructured == nil || !*nestedSchemaDef.IsStructured {
				return fmt.Errorf("object field '%s' cannot reference literal nested schema '%s' - only structured schemas are allowed", fieldPath, ref.ID)
			}
		} else {
			return fmt.Errorf("object field '%s' references unknown nested schema '%s', %s", fieldPath, ref.ID, schema.Name)
		}
	}
	return nil
}

func validateRecordFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
		if _, exists := schema.FindNestedSchema(ref.ID); !exists {
			return fmt.Errorf("record field '%s' references unknown nested schema '%s'", fieldPath, ref.ID)
		}
	}
	return nil
}

func validateUnionFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if refs, ok := fieldDef.Schema.([]NestedSchemaReference); ok {
		for _, ref := range refs {
			if _, exists := schema.FindNestedSchema(ref.ID); !exists {
				return fmt.Errorf("union field '%s' references unknown nested schema '%s'", fieldPath, ref.ID)
			}
		}
	}
	return nil
}

func validateArraySetFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
		if fieldDef.ItemsType == nil {
			return fmt.Errorf("array/set field '%s' has schema reference but no ItemsType specified", fieldPath)
		}

		if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
			switch *fieldDef.ItemsType {
			case FieldTypeObject:
				if nestedSchemaDef.IsStructured == nil || !*nestedSchemaDef.IsStructured {
					return fmt.Errorf("array/set field '%s' with object ItemsType cannot reference literal nested schema '%s' - only structured schemas are allowed", fieldPath, ref.ID)
				}
			case FieldTypeRecord, FieldTypeUnion:
				// Both structured and literal schemas are valid
			case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeBoolean, FieldTypeDecimal, FieldTypeEnum:
				return fmt.Errorf("array/set field '%s' with primitive ItemsType '%s' cannot have schema references", fieldPath, *fieldDef.ItemsType)
			}
		} else {
			return fmt.Errorf("array/set field '%s' references unknown nested schema '%s'", fieldPath, ref.ID)
		}
	}
	return nil
}

func validateNestedSchemaSemantics(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if fieldDef.Schema != nil && fieldDef.Type == FieldTypeObject {
		if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
			if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
				if nestedSchemaDef.IsStructured != nil && *nestedSchemaDef.IsStructured {
					var tempSchema *SchemaDefinition
					if nestedSchemaDef.StructuredFieldsMap != nil {
						tempSchema = &SchemaDefinition{
							Name:          nestedSchemaDef.Name,
							Fields:        nestedSchemaDef.StructuredFieldsMap,
							NestedSchemas: schema.NestedSchemas,
						}
					}
					if tempSchema != nil {
						if err := tempSchema.validateSchemaSemanticRecursive(fieldPath); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}


