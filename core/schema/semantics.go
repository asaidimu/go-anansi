package schema

import (
	"fmt"
)

// TODO: Re-think these functionality

// Deprecated
func (schema *SchemaDefinition) Validate() error {
	basePath := ""
	return schema.validateSchemaSemanticRecursive(basePath)
}

func buildPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

func (schema *SchemaDefinition) validateSchemaSemanticRecursive(basePath string) error {
	for fieldId, fieldDef := range schema.Fields {
		fieldPath := buildPath(basePath, fieldId)

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
		return ErrPrimitiveFieldSchemaReference.
			WithOperation("schema.validateFieldSemantic").
			WithMessage(fmt.Sprintf("primitive field type '%s' at '%s' cannot have schema references", fieldDef.Type, fieldPath))

	}
	return nil
}

func validateObjectFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
		if nestedSchemaDef, exists := schema.FindNestedSchemaById(ref.ID); exists {
			if !nestedSchemaDef.IsStructured() {
				return ErrObjectFieldLiteralSchemaReference.
					WithOperation("schema.validateObjectFieldSemantic").
					WithMessage(fmt.Sprintf("object field '%s' cannot reference literal nested schema '%s' - only structured schemas are allowed", fieldPath, ref.ID))
			}
		} else {
			return ErrUnknownNestedSchemaReference.
				WithOperation("schema.validateObjectFieldSemantic").
				WithMessage(fmt.Sprintf("object field '%s' references unknown nested schema '%s'", fieldPath, ref.ID))
		}
	}
	return nil
}

func validateRecordFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
		if _, exists := schema.FindNestedSchemaById(ref.ID); !exists {
			return ErrUnknownNestedSchemaReference.
				WithOperation("schema.validateRecordFieldSemantic").
				WithMessage(fmt.Sprintf("record field '%s' references unknown nested schema '%s'", fieldPath, ref.ID))
		}
	}
	return nil
}

func validateUnionFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if refs, ok := fieldDef.Schema.([]NestedSchemaReference); ok {
		for _, ref := range refs {
			if _, exists := schema.FindNestedSchema(ref.ID); !exists {
				return ErrUnknownNestedSchemaReference.
					WithOperation("schema.validateUnionFieldSemantic").
					WithMessage(fmt.Sprintf("union field '%s' references unknown nested schema '%s'", fieldPath, ref.ID))
			}
		}
	}
	return nil
}

func validateArraySetFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
		if fieldDef.ItemsType == nil {
			return ErrArraySetMissingItemsType.
				WithOperation("schema.validateArraySetFieldSemantic").
				WithMessage(fmt.Sprintf("array/set field '%s' has schema reference but no ItemsType specified", fieldPath))
		}

		if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
			switch *fieldDef.ItemsType {
			case FieldTypeObject:
				if !nestedSchemaDef.IsStructured() {
					return ErrObjectFieldLiteralSchemaReference.
						WithOperation("schema.validateArraySetFieldSemantic").
						WithMessage(fmt.Sprintf("array/set field '%s' with object ItemsType cannot reference literal nested schema '%s' - only structured schemas are allowed", fieldPath, ref.ID))
				}
			case FieldTypeRecord, FieldTypeUnion:
				// Both structured and literal schemas are valid
			case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeBoolean, FieldTypeDecimal, FieldTypeEnum:
				return ErrPrimitiveFieldSchemaReference.
					WithOperation("schema.validateArraySetFieldSemantic").
					WithMessage(fmt.Sprintf("array/set field '%s' with primitive ItemsType '%s' cannot have schema references", fieldPath, *fieldDef.ItemsType))
			}
		} else {
			return ErrUnknownNestedSchemaReference.
				WithOperation("schema.validateArraySetFieldSemantic").
				WithMessage(fmt.Sprintf("array/set field '%s' references unknown nested schema '%s'", fieldPath, ref.ID))
		}
	}
	return nil
}

func validateNestedSchemaSemantics(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	if fieldDef.Schema != nil && fieldDef.Type == FieldTypeObject {
		if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
			if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
				if nestedSchemaDef.IsStructured() {
					var tempSchema *SchemaDefinition
					if nestedSchemaDef.Fields.FieldsMap != nil {
						tempSchema = &SchemaDefinition{
							Name:          nestedSchemaDef.Name,
							Fields:        nestedSchemaDef.Fields.FieldsMap,
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
