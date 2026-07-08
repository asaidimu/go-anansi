package definition_test

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

// Helper to generate a complex schema for stress testing
func generateComplexSchema(depth, fieldsPerLevel, arrayLength int) *definition.Schema {
	schema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name:    "ComplexRoot",
			Fields:  make(map[definition.FieldId]definition.Field),
			Indexes: make(map[definition.IndexID]definition.Index),
		},
		Schemas: make(map[definition.SchemaId]definition.NestedSchema),
		Version: common.MustNewVersion("1.0.0"),
	}

	// Create a nested schema for recursion or object fields
	createNestedSchema := func(currentDepth int) definition.NestedSchema {
		ns := definition.NestedSchema{
			BaseSchema: definition.BaseSchema{
				Fields: make(map[definition.FieldId]definition.Field),
			},
		}

		for i := 0; i < fieldsPerLevel; i++ {
			fieldName := definition.FieldName(fmt.Sprintf("field%d", i))
			fieldId := definition.FieldId(fmt.Sprintf("field%d_id", i))
			fieldType := definition.FieldTypeString

			if i%3 == 0 { // String
				fieldType = definition.FieldTypeString
			} else if i%3 == 1 { // Integer
				fieldType = definition.FieldTypeInteger
			} else { // Boolean
				fieldType = definition.FieldTypeBoolean
			}

			// Add a simple field
			ns.Fields[fieldId] = definition.Field{
				Name: fieldName,
				FieldProperties: definition.FieldProperties{
					Type: fieldType,
				},
			}
		}
		return ns
	}

	// Create root level fields
	for i := 0; i < fieldsPerLevel; i++ {
		fieldName := definition.FieldName(fmt.Sprintf("rootField%d", i))
		fieldId := definition.FieldId(fmt.Sprintf("rootField%d_id", i))

		if i == 0 && depth > 0 { // Nested object
			nestedSchemaID := definition.SchemaId(fmt.Sprintf("NestedSchemaDepth%d", depth))
			nestedSchema := createNestedSchema(depth - 1)
			schema.Schemas[nestedSchemaID] = nestedSchema
			schema.BaseSchema.Fields[fieldId] = definition.Field{
				Name: fieldName,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeObject,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: nestedSchemaID}),
				},
			}
		} else if i == 1 && arrayLength > 0 { // Array of strings
			arrayItemSchemaID := definition.SchemaId("ArrayItemSchema")
			schema.Schemas[arrayItemSchemaID] = definition.NestedSchema{
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			}
			schema.BaseSchema.Fields[fieldId] = definition.Field{
				Name: fieldName,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeArray,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: arrayItemSchemaID}),
				},
			}
		} else { // Simple fields
			fieldType := definition.FieldTypeString
			if i%2 == 0 {
				fieldType = definition.FieldTypeInteger
			}
			schema.BaseSchema.Fields[fieldId] = definition.Field{
				Name: fieldName,
				FieldProperties: definition.FieldProperties{
					Type: fieldType,
				},
			}
		}
	}

	return schema
}

// Helper to generate complex data for stress testing
func generateComplexData(depth, fieldsPerLevel, arrayLength int) map[string]any {
	data := make(map[string]any)

	for i := 0; i < fieldsPerLevel; i++ {
		fieldName := fmt.Sprintf("rootField%d", i)

		if i == 0 && depth > 0 { // Nested object
			nestedData := make(map[string]any)
			for j := 0; j < fieldsPerLevel; j++ {
				innerFieldName := fmt.Sprintf("field%d", j)
				if j%3 == 0 { // String
					nestedData[innerFieldName] = fmt.Sprintf("value%d-%d", i, j)
				} else if j%3 == 1 { // Integer
					nestedData[innerFieldName] = j
				} else { // Boolean
					nestedData[innerFieldName] = (j%2 == 0)
				}
			}
			data[fieldName] = nestedData
		} else if i == 1 && arrayLength > 0 { // Array of strings
			arr := make([]any, arrayLength)
			for j := 0; j < arrayLength; j++ {
				arr[j] = fmt.Sprintf("arrayItem%d-%d", i, j)
			}
			data[fieldName] = arr
		} else { // Simple fields
			if i%2 == 0 {
				data[fieldName] = i
			} else {
				data[fieldName] = fmt.Sprintf("rootValue%d", i)
			}
		}
	}
	return data
}

func BenchmarkValidator_ComplexSchema(b *testing.B) {
	// Parameters for the complex schema and data
	const (
		schemaDepth      = 10  // Increased from 5
		fieldsPerLevel   = 20  // Increased from 10
		arrayLength      = 500 // Increased from 100
		numSchemasInRepo = 500 // Increased from 100
	)

	// Generate a complex schema once for the benchmark setup
	complexSchema := generateComplexSchema(schemaDepth, fieldsPerLevel, arrayLength)

	// Add more nested schemas to simulate a larger schema repository
	for i := 0; i < numSchemasInRepo; i++ {
		schemaID := definition.SchemaId(fmt.Sprintf("AuxNestedSchema%d", i))
		complexSchema.Schemas[schemaID] = definition.NestedSchema{
			BaseSchema: definition.BaseSchema{
				Name: fmt.Sprintf("AuxNestedSchema%d", i),
				Fields: map[definition.FieldId]definition.Field{
					definition.FieldId("auxField1"): {Name: "auxField1", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				},
			},
		}
	}

	// Generate complex data once for the benchmark setup
	complexData := generateComplexData(schemaDepth, fieldsPerLevel, arrayLength)

	// No predicates needed for this benchmark, use an empty map
	predicates := make(definition.PredicateMap)
	validator, err := definition.NewDocumentValidator(complexSchema, predicates)
	if err != nil {
		b.Fatalf("Failed to create validator: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err != nil {
			b.Fatalf("Failed to create validator: %v", err)
		}
		_, _ = validator.Validate(complexData)
	}
}
