package codegen

import (
	"encoding/json"
	"fmt"
	"go/format"
	"regexp"
	"strings"
	"unicode"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// StructGenerator handles the conversion from schema definitions to Go struct code
type StructGenerator struct {
	packageName string
	imports     map[string]bool
	// Track generated types to avoid duplicates and handle circular references
	generatedTypes map[string]bool
	// Store schema context for validation and type resolution
	schema *schema.SchemaDefinition
}

// NewStructGenerator creates a new struct generator
func NewStructGenerator(packageName string) *StructGenerator {
	return &StructGenerator{
		packageName:    packageName,
		imports:        make(map[string]bool),
		generatedTypes: make(map[string]bool),
	}
}

// GeneratedStruct represents a generated Go struct
type GeneratedStruct struct {
	Name   string
	Code   string
	Fields []GeneratedField
}

// GeneratedField represents a field in the generated struct
type GeneratedField struct {
	Name     string
	GoType   string
	JSONTag  string
	Required bool
	Comment  string
}

// SemanticFieldTypeInfo contains semantic information about field types
type SemanticFieldTypeInfo struct {
	BaseType          string
	IsReferenceType   bool
	AllowsSchemaRef   bool
	RequiresItemsType bool
	StructuredOnly    bool // Only allows structured nested schemas
}

// SemanticFieldTypeMapping provides semantic-aware type mapping
var SemanticFieldTypeMapping = map[schema.FieldType]SemanticFieldTypeInfo{
	schema.FieldTypeString: {
		BaseType:        "string",
		IsReferenceType: false,
		AllowsSchemaRef: false,
	},
	schema.FieldTypeNumber: {
		BaseType:        "float64",
		IsReferenceType: false,
		AllowsSchemaRef: false,
	},
	schema.FieldTypeInteger: {
		BaseType:        "int64",
		IsReferenceType: false,
		AllowsSchemaRef: false,
	},
	schema.FieldTypeDecimal: {
		BaseType:        "float64",
		IsReferenceType: false,
		AllowsSchemaRef: false,
	},
	schema.FieldTypeBoolean: {
		BaseType:        "bool",
		IsReferenceType: false,
		AllowsSchemaRef: false,
	},
	schema.FieldTypeEnum: {
		BaseType:        "string",
		IsReferenceType: false,
		AllowsSchemaRef: false,
	},
	schema.FieldTypeArray: {
		BaseType:          "[]interface{}",
		IsReferenceType:   true,
		AllowsSchemaRef:   true,
		RequiresItemsType: true,
	},
	schema.FieldTypeSet: {
		BaseType:          "[]interface{}",
		IsReferenceType:   true,
		AllowsSchemaRef:   true,
		RequiresItemsType: true,
	},
	schema.FieldTypeObject: {
		BaseType:        "map[string]interface{}",
		IsReferenceType: true,
		AllowsSchemaRef: true,
		StructuredOnly:  true, // Only structured schemas allowed
	},
	schema.FieldTypeRecord: {
		BaseType:        "map[string]interface{}",
		IsReferenceType: true,
		AllowsSchemaRef: true,
		StructuredOnly:  false, // Both structured and literal allowed
	},
	schema.FieldTypeUnion: {
		BaseType:        "interface{}",
		IsReferenceType: true,
		AllowsSchemaRef: true,
	},
	schema.FieldTypeUnknown: {
		BaseType:        "interface{}",
		IsReferenceType: true,
		AllowsSchemaRef: false,
	},
}

// GenerateStruct generates a Go struct from a schema definition with semantic validation
func (sg *StructGenerator) GenerateStruct(sc *schema.SchemaDefinition) (*GeneratedStruct, error) {
	sg.schema = sc

	// First validate the schema semantics
	if err := sc.Validate(); err != nil {
		return nil, common.SystemErrorFrom(err, ErrSchemaValidationFailed.Code, ErrSchemaValidationFailed.Message).
			WithOperation("codegen.StructGenerator.GenerateStruct")
	}

	structName := sg.toStructName(sc.Name)

	var fields []GeneratedField
	var fieldCode strings.Builder

	// Process main schema fields
	for fieldID, fieldDef := range sc.Fields {
		if fieldDef.Name == schema.VersionFieldName {
			continue // Skip internal version field
		}

		field, err := sg.generateFieldSemantic(fieldDef)
		if err != nil {
			return nil, common.SystemErrorFrom(err, ErrSchemaValidationFailed.Code,
				fmt.Sprintf("error generating field %s (ID: %s)", fieldDef.Name, fieldID)).
				WithOperation("codegen.StructGenerator.GenerateStruct")
		}

		fields = append(fields, *field)
		fieldCode.WriteString(sg.formatField(field))
		fieldCode.WriteString("\n")
	}

	// Generate the complete struct
	structCode := fmt.Sprintf(`// %s represents %s
type %s struct {
%s}`,
		structName,
		sc.Description,
		structName,
		fieldCode.String())

	return &GeneratedStruct{
		Name:   structName,
		Code:   structCode,
		Fields: fields,
	}, nil
}

// generateFieldSemantic creates a GeneratedField with semantic validation
func (sg *StructGenerator) generateFieldSemantic(fieldDef *schema.FieldDefinition) (*GeneratedField, error) {
	fieldName := sg.toFieldName(fieldDef.Name)

	// Get semantic info for this field type
	semanticInfo, exists := SemanticFieldTypeMapping[fieldDef.Type]
	if !exists {
		return nil, schema.ErrUnsupportedFieldType.WithOperation("codegen.StructGenerator.generateFieldSemantic").
			WithMessage(fmt.Sprintf("unsupported field type: %s for field '%s'", fieldDef.Type, fieldDef.Name))
	}

	// Validate semantic rules
	if err := sg.validateFieldSemantics(fieldDef, semanticInfo); err != nil {
		return nil, err
	}

	// Generate the appropriate Go type
	goType, err := sg.generateSemanticGoType(fieldDef, semanticInfo)
	if err != nil {
		return nil, err
	}

	// Handle optional fields with pointers (only for non-reference types)
	isRequired := fieldDef.Required != nil && *fieldDef.Required
	if !isRequired && !semanticInfo.IsReferenceType {
		goType = "*" + goType
	}

	jsonTag := fieldDef.Name
	if !isRequired {
		jsonTag += ",omitempty"
	}

	comment := ""
	if fieldDef.Description != nil {
		comment = *fieldDef.Description
	}

	return &GeneratedField{
		Name:     fieldName,
		GoType:   goType,
		JSONTag:  jsonTag,
		Required: isRequired,
		Comment:  comment,
	}, nil
}

// validateFieldSemantics validates semantic rules for a field
func (sg *StructGenerator) validateFieldSemantics(fieldDef *schema.FieldDefinition, semanticInfo SemanticFieldTypeInfo) error {
		// Rule 1: Primitive types cannot have schema references
		if !semanticInfo.AllowsSchemaRef && fieldDef.Schema != nil {
			return common.SystemErrorFrom(ErrPrimitiveFieldHasSchemaRef, ErrPrimitiveFieldHasSchemaRef.Code,
				fmt.Sprintf("primitive field type '%s' cannot have schema references", fieldDef.Type)).
				WithOperation("codegen.StructGenerator.validateFieldSemantics")
		}
	
		// Rule 2: Array/Set with schema reference must have ItemsType
		if semanticInfo.RequiresItemsType && fieldDef.Schema != nil && fieldDef.ItemsType == nil {
	                      return common.SystemErrorFrom(ErrArraySetNoItemsType, ErrArraySetNoItemsType.Code,
	                          fmt.Sprintf("array/set field '%s' has schema reference but no ItemsType specified", fieldDef.Name)).
	                          WithOperation("codegen.StructGenerator.validateFieldSemantics")
		}
	
		// Rule 3: Object fields can only reference structured schemas
		if semanticInfo.StructuredOnly && fieldDef.Schema != nil {
			if ref, ok := fieldDef.Schema.(schema.NestedSchemaReference); ok {
				if nestedSchema, exists := sg.schema.FindNestedSchema(ref.ID); exists {
					if nestedSchema.IsStructured == nil || !*nestedSchema.IsStructured {
						return common.SystemErrorFrom(ErrObjectReferencesLiteralSchema, ErrObjectReferencesLiteralSchema.Code,
							fmt.Sprintf("object field '%s' cannot reference literal nested schema '%s' - only structured schemas allowed", fieldDef.Name, ref.ID)).
							WithOperation("codegen.StructGenerator.validateFieldSemantics")
					}
				} else {
					return common.SystemErrorFrom(ErrUnknownNestedSchema, ErrUnknownNestedSchema.Code,
						fmt.Sprintf("field '%s' references unknown nested schema '%s'", fieldDef.Name, ref.ID)).
						WithOperation("codegen.StructGenerator.validateFieldSemantics")
				}
			}
		}

	return nil
}

// generateSemanticGoType generates Go type based on semantic rules
func (sg *StructGenerator) generateSemanticGoType(fieldDef *schema.FieldDefinition, semanticInfo SemanticFieldTypeInfo) (string, error) {
	switch fieldDef.Type {
	case schema.FieldTypeArray, schema.FieldTypeSet:
		return sg.generateArrayType(fieldDef, semanticInfo)
	case schema.FieldTypeObject:
		return sg.generateObjectType(fieldDef, semanticInfo)
	case schema.FieldTypeRecord:
		return sg.generateRecordType(fieldDef, semanticInfo)
	case schema.FieldTypeUnion:
		return sg.generateUnionType(fieldDef, semanticInfo)
	case schema.FieldTypeEnum:
		return sg.generateEnumType(fieldDef, semanticInfo)
	default:
		return semanticInfo.BaseType, nil
	}
}

// generateArrayType generates Go type for array/set fields
func (sg *StructGenerator) generateArrayType(fieldDef *schema.FieldDefinition, semanticInfo SemanticFieldTypeInfo) (string, error) {
	if fieldDef.ItemsType == nil {
		return semanticInfo.BaseType, nil
	}

	itemSemanticInfo, exists := SemanticFieldTypeMapping[*fieldDef.ItemsType]
	if !exists {
		return "", common.SystemErrorFrom(ErrUnsupportedItemsType, ErrUnsupportedItemsType.Code,
			fmt.Sprintf("unsupported ItemsType: %s", *fieldDef.ItemsType)).
			WithOperation("codegen.StructGenerator.generateArrayType")
	}

	// Handle schema references for item types
	if fieldDef.Schema != nil {
		if ref, ok := fieldDef.Schema.(schema.NestedSchemaReference); ok {
			switch *fieldDef.ItemsType {
			case schema.FieldTypeObject, schema.FieldTypeRecord:
				if nestedSchema, exists := sg.schema.FindNestedSchema(ref.ID); exists {
					structName := sg.toStructName(nestedSchema.Name)
					return "[]" + structName, nil
				}
			}
		}
	}

	return "[]" + itemSemanticInfo.BaseType, nil
}

// generateObjectType generates Go type for object fields
func (sg *StructGenerator) generateObjectType(fieldDef *schema.FieldDefinition, semanticInfo SemanticFieldTypeInfo) (string, error) {
	if fieldDef.Schema != nil {
		if ref, ok := fieldDef.Schema.(schema.NestedSchemaReference); ok {
			if nestedSchema, exists := sg.schema.FindNestedSchema(ref.ID); exists {
				return sg.toStructName(nestedSchema.Name), nil
			}
		}
	}
	return semanticInfo.BaseType, nil
}

// generateRecordType generates Go type for record fields
func (sg *StructGenerator) generateRecordType(fieldDef *schema.FieldDefinition, semanticInfo SemanticFieldTypeInfo) (string, error) {
	if fieldDef.Schema != nil {
		if ref, ok := fieldDef.Schema.(schema.NestedSchemaReference); ok {
			if nestedSchema, exists := sg.schema.FindNestedSchema(ref.ID); exists {
				// For literal schemas, we might want to use map[string]interface{}
				// For structured schemas, we use the generated struct type
				if nestedSchema.IsStructured != nil && *nestedSchema.IsStructured {
					return sg.toStructName(nestedSchema.Name), nil
				}
				// For literal schemas, return a specialized type if possible
				return sg.generateLiteralSchemaType(nestedSchema)
			}
		}
	}
	return semanticInfo.BaseType, nil
}

// generateUnionType generates Go type for union fields
func (sg *StructGenerator) generateUnionType(fieldDef *schema.FieldDefinition, semanticInfo SemanticFieldTypeInfo) (string, error) {
	if fieldDef.Schema != nil {
		if refs, ok := fieldDef.Schema.([]schema.NestedSchemaReference); ok {
			// Generate a union interface type
			unionTypeName := sg.toStructName(fieldDef.Name + "Union")
			sg.generateUnionInterface(unionTypeName, refs)
			return unionTypeName, nil
		}
	}
	return semanticInfo.BaseType, nil
}

// generateEnumType generates Go type for enum fields
func (sg *StructGenerator) generateEnumType(fieldDef *schema.FieldDefinition, semanticInfo SemanticFieldTypeInfo) (string, error) {
	if fieldDef.Values != nil && len(fieldDef.Values) > 0 {
		// Generate a custom enum type
		enumTypeName := sg.toStructName(fieldDef.Name + "Enum")
		sg.generateEnumConstants(enumTypeName, fieldDef.Values)
		return enumTypeName, nil
	}
	return semanticInfo.BaseType, nil
}

// generateLiteralSchemaType generates type for literal nested schemas
func (sg *StructGenerator) generateLiteralSchemaType(nestedSchema *schema.NestedSchemaDefinition) (string, error) {
	if nestedSchema.Type != nil {
		semanticInfo, exists := SemanticFieldTypeMapping[*nestedSchema.Type]
		if exists {
			return semanticInfo.BaseType, nil
		}
	}
	return "map[string]interface{}", nil
}

// generateUnionInterface generates an interface type for union fields
func (sg *StructGenerator) generateUnionInterface(typeName string, refs []schema.NestedSchemaReference) {
	// This would generate an interface that all union member types implement
	// Implementation depends on how you want to handle union types
	sg.generatedTypes[typeName] = true
}

// generateEnumConstants generates constants for enum types
func (sg *StructGenerator) generateEnumConstants(typeName string, values []any) {
	// This would generate a custom type with constants
	// Implementation depends on how you want to handle enums
	sg.generatedTypes[typeName] = true
}

// GenerateNestedStructs generates structs for all nested schemas with semantic validation
func (sg *StructGenerator) GenerateNestedStructs(schema *schema.SchemaDefinition) ([]*GeneratedStruct, error) {
	sg.schema = schema
	var structs []*GeneratedStruct

	for nestedSchemaID, nestedSchema := range schema.NestedSchemas {
		if nestedSchema.IsStructured != nil && *nestedSchema.IsStructured {
			nestedStruct, err := sg.generateNestedStructSemantic(nestedSchemaID, nestedSchema)
			if err != nil {
				return nil, common.SystemErrorFrom(err, ErrSchemaValidationFailed.Code,
					fmt.Sprintf("error generating nested struct %s (ID: %s)", nestedSchema.Name, nestedSchemaID)).
					WithOperation("codegen.StructGenerator.GenerateNestedStructs")
			}
			structs = append(structs, nestedStruct)
		}
	}

	return structs, nil
}

// generateNestedStructSemantic creates a struct from a nested schema definition with validation
func (sg *StructGenerator) generateNestedStructSemantic(nestedSchemaID string, nestedSchema *schema.NestedSchemaDefinition) (*GeneratedStruct, error) {
	structName := sg.toStructName(nestedSchema.Name)

	var fields []GeneratedField
	var fieldCode strings.Builder

	// Handle structured nested schemas
	if nestedSchema.StructuredFieldsMap != nil {
		for fieldID, fieldDef := range nestedSchema.StructuredFieldsMap {
			tempSchema := &schema.SchemaDefinition{
				Name:          nestedSchema.Name,
				Fields:        nestedSchema.StructuredFieldsMap,
				NestedSchemas: sg.schema.NestedSchemas,
			}

			// Temporarily set schema context
			originalSchema := sg.schema
			sg.schema = tempSchema

			field, err := sg.generateFieldSemantic(fieldDef)
			if err != nil {
				sg.schema = originalSchema
				return nil, common.SystemErrorFrom(err, ErrSchemaValidationFailed.Code,
					fmt.Sprintf("error generating field %s (ID: %s) in nested schema %s", fieldDef.Name, fieldID, nestedSchema.Name)).
					WithOperation("codegen.StructGenerator.generateNestedStructSemantic")
			}

			sg.schema = originalSchema

			fields = append(fields, *field)
			fieldCode.WriteString(sg.formatField(field))
			fieldCode.WriteString("\n")
		}
	}

	description := "Generated nested struct"
	if nestedSchema.Description != nil {
		description = *nestedSchema.Description
	}

	structCode := fmt.Sprintf(`// %s represents %s
type %s struct {
%s}`,
		structName,
		description,
		structName,
		fieldCode.String())

	return &GeneratedStruct{
		Name:   structName,
		Code:   structCode,
		Fields: fields,
	}, nil
}

// formatField formats a GeneratedField as Go struct field code
func (sg *StructGenerator) formatField(field *GeneratedField) string {
	comment := ""
	if field.Comment != "" {
		comment = fmt.Sprintf(" // %s", field.Comment)
	}

	return fmt.Sprintf("\t%s %s `json:\"%s\"`%s",
		field.Name,
		field.GoType,
		field.JSONTag,
		comment)
}

// GenerateComplete generates a complete Go file with all structs and semantic validation
func (sg *StructGenerator) GenerateComplete(sc *schema.SchemaDefinition) (string, error) {
	sg.schema = sc

	// Validate schema semantics first
	if err := sc.Validate(); err != nil {
		return "", common.SystemErrorFrom(err, ErrSchemaValidationFailed.Code, ErrSchemaValidationFailed.Message).
			WithOperation("codegen.StructGenerator.GenerateComplete")
	}

	var code strings.Builder

	// Package declaration
	code.WriteString(fmt.Sprintf("package %s\n\n", sg.packageName))

	// Imports (add as needed)
	if len(sg.imports) > 0 {
		code.WriteString("import (\n")
		for imp := range sg.imports {
			code.WriteString(fmt.Sprintf("\t\"%s\"\n", imp))
		}
		code.WriteString(")\n\n")
	}

	// Generate nested structs first
	nestedStructs, err := sg.GenerateNestedStructs(sc)
	if err != nil {
		return "", err
	}

	for _, nestedStruct := range nestedStructs {
		code.WriteString(nestedStruct.Code)
		code.WriteString("\n\n")
	}

	// Generate main struct
	mainStruct, err := sg.GenerateStruct(sc)
	if err != nil {
		return "", err
	}

	code.WriteString(mainStruct.Code)
	code.WriteString("\n")

	// Format the generated code
	formatted, err := format.Source([]byte(code.String()))
	if err != nil {
		// Return unformatted code if formatting fails
		return code.String(), nil
	}

	return string(formatted), nil
}

// Helper methods

// toStructName converts a schema name to a valid Go struct name
func (sg *StructGenerator) toStructName(name string) string {
	return sg.toPascalCase(name)
}

// toFieldName converts a field name to a valid Go field name
func (sg *StructGenerator) toFieldName(name string) string {
	return sg.toPascalCase(name)
}

// toPascalCase converts a string to PascalCase
func (sg *StructGenerator) toPascalCase(s string) string {
	// Handle common separators
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	words := re.Split(s, -1)

	var result strings.Builder
	for _, word := range words {
		if word == "" {
			continue
		}

		// Capitalize first letter, lowercase the rest
		runes := []rune(word)
		if len(runes) > 0 {
			result.WriteRune(unicode.ToUpper(runes[0]))
			for i := 1; i < len(runes); i++ {
				result.WriteRune(unicode.ToLower(runes[i]))
			}
		}
	}

	return result.String()
}

// Example usage function with semantic validation
func GenerateStructFromJSONSemantic(schemaJSON string) (string, error) {
	var sc schema.SchemaDefinition
	if err := json.Unmarshal([]byte(schemaJSON), &sc); err != nil {
		return "", common.SystemErrorFrom(err, ErrFailedToParseSchemaJSON.Code, ErrFailedToParseSchemaJSON.Message).
			WithOperation("GenerateStructFromJSONSemantic")
	}

	generator := NewStructGenerator("models")
	return generator.GenerateComplete(&sc)
}
