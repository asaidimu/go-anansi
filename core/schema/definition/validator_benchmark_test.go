package definition_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// BenchmarkValidator_Validate_SimpleSchema benchmarks the validation of a simple schema and object
func BenchmarkValidator_Validate_SimpleSchema(b *testing.B) {
	// Setup: Define a simple schema
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"name_id": {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age_id":  {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
			Constraints: map[definition.ConstraintId]definition.Constraint{
				"age_positive": {
					Name: "age_positive",
					ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
						Fields:    []definition.FieldName{"age"},
						Predicate: definition.PredicateName("is_positive"), // Assuming 'is_positive' predicate exists
					}),
				},
			},
		},
	}

	// Mock predicate
	predicates := definition.PredicateMap{
		"is_positive": func(params definition.PredicateParams) []common.Issue {
			if len(params.Keys) > 0 {
				value, _ := utils.GetValueByPath(params.Data, string(params.Keys[0]))
				if i, ok := value.(int); ok {
					if i > 0 {
						return nil
					}
				}
			}
			return []common.Issue{{Code: "NOT_POSITIVE", Message: "Value is not positive"}}
		},
	}

	// Resolve the schema and create a validator outside the benchmark loop
	resolvedSchema := &schema
	validator, err := definition.NewDocumentValidator(resolvedSchema, predicates)
	if err != nil {
		b.Fatalf("failed to create validator: %v", err)
	}

	// Define a simple object to validate
	object := map[string]any{
		"name": "John Doe",
		"age":  30,
	}

	b.ResetTimer() // Reset timer to exclude setup time

	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate(object)
	}
}

// Configuration for fuzzy schema generation
type FuzzySchemaConfig struct {
	MaxDepth           int
	MaxFields          int
	MaxArrayElements   int
	FieldTypeWeights   map[definition.FieldType]int
	AddConstraintsRate float64 // Probability of adding a constraint (0.0 - 1.0)
}

// generateFuzzySchema generates a random schema for benchmarking
// It populates the provided `allSchemas` map with nested schemas and returns the SchemaId of the generated schema.
func generateFuzzySchema(cfg FuzzySchemaConfig, currentDepth int, allSchemas map[definition.SchemaId]definition.NestedSchema) definition.SchemaId {
	schemaID := definition.SchemaId(fmt.Sprintf("nested_schema_%d_%d", currentDepth, rand.Intn(1000000)))

	baseSchema := definition.BaseSchema{
		Fields:      make(map[definition.FieldId]definition.Field),
		Constraints: make(map[definition.ConstraintId]definition.Constraint),
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	numFields := r.Intn(cfg.MaxFields) + 1 // At least one field
	for i := 0; i < numFields; i++ {
		fieldID := definition.FieldId(fmt.Sprintf("field_%d_%d", currentDepth, i))
		fieldName := definition.FieldName(fmt.Sprintf("field%d", i))
		fieldType := getRandomFieldType(r, cfg, currentDepth)

		field := definition.Field{
			Name:     fieldName,
			Required: r.Float32() < 0.5, // 50% chance of being required
			FieldProperties: definition.FieldProperties{
				Type: fieldType,
			},
		}

		if fieldType == definition.FieldTypeObject && currentDepth < cfg.MaxDepth {
			nestedSchemaRefID := generateFuzzySchema(cfg, currentDepth+1, allSchemas)
			field.FieldProperties.Schema = definition.NewSchemaReference(definition.SchemaReference{
				ID: nestedSchemaRefID,
			})
		} else if fieldType == definition.FieldTypeArray && currentDepth < cfg.MaxDepth {
			itemType := getRandomFieldType(r, cfg, currentDepth) // Type for array elements
			if itemType == definition.FieldTypeObject {
				nestedSchemaRefID := generateFuzzySchema(cfg, currentDepth+1, allSchemas)
				field.FieldProperties.Schema = definition.NewSchemaReference(definition.SchemaReference{
					ID: nestedSchemaRefID,
				})
			} else {
				// For primitive array types, the schema field will refer to its own field type properties.
				// Not explicitly handled here, will be inferred by resolver from FieldProperties.Type
			}
			// ArrayProperties is not a direct field; the element type is inferred from FieldProperties.Schema (if object) or FieldProperties.Type (if primitive)
		}
		baseSchema.Fields[fieldID] = field

		if r.Float64() < cfg.AddConstraintsRate {
			// Add a simple 'not_empty' constraint for strings or 'is_positive' for integers
			constraintID := definition.ConstraintId(fmt.Sprintf("const_%s", fieldID))
			var predicate string
			if fieldType == definition.FieldTypeString {
				predicate = "not_empty"
			} else if fieldType == definition.FieldTypeInteger {
				predicate = "is_positive"
			} else {
				continue // Don't add constraints for other types for simplicity in fuzzy generation
			}
			baseSchema.Constraints[constraintID] = definition.Constraint{
				Name: string(constraintID),
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
					Fields:    []definition.FieldName{fieldName},
					Predicate: definition.PredicateName(predicate),
				}),
			}
		}
	}

	allSchemas[schemaID] = definition.NestedSchema{
		BaseSchema: baseSchema,
	}
	return schemaID
}

// getRandomFieldType selects a random field type based on weights and current depth
func getRandomFieldType(r *rand.Rand, cfg FuzzySchemaConfig, currentDepth int) definition.FieldType {
	if currentDepth >= cfg.MaxDepth {
		// If at max depth, only allow primitive types
		primitiveTypes := []definition.FieldType{
			definition.FieldTypeString,
			definition.FieldTypeInteger,
			definition.FieldTypeBoolean,
			definition.FieldTypeNumber,
		}
		return primitiveTypes[r.Intn(len(primitiveTypes))]
	}

	// Calculate total weight
	totalWeight := 0
	for _, weight := range cfg.FieldTypeWeights {
		totalWeight += weight
	}

	// Adjust weights if object/array types are too heavily weighted at lower depths
	// (This is a simple heuristic; more sophisticated logic might be needed for complex scenarios)
	adjustedWeights := make(map[definition.FieldType]int)
	for k, v := range cfg.FieldTypeWeights {
		adjustedWeights[k] = v
	}

	if currentDepth >= cfg.MaxDepth-1 { // Reduce chance of complex types at second to last depth
		adjustedWeights[definition.FieldTypeObject] = adjustedWeights[definition.FieldTypeObject] / 2
		adjustedWeights[definition.FieldTypeArray] = adjustedWeights[definition.FieldTypeArray] / 2
		totalWeight = 0
		for _, weight := range adjustedWeights {
			totalWeight += weight
		}
	}

	// Pick a random number within the total weight
	randomWeight := r.Intn(totalWeight)

	// Determine field type based on weights
	cumulativeWeight := 0
	for fieldType, weight := range adjustedWeights {
		cumulativeWeight += weight
		if randomWeight < cumulativeWeight {
			return fieldType
		}
	}

	// Fallback (should not happen if weights are defined correctly)
	return definition.FieldTypeString
}

// generateFuzzyObject generates a random object conforming to the given schema
// It needs the full `definition.Schema` to resolve nested schema references.
func generateFuzzyObject(s definition.Schema, nestedSchemaId definition.SchemaId, cfg FuzzySchemaConfig) map[string]any {
	obj := make(map[string]any)

	nestedSchema, ok := s.Schemas[nestedSchemaId]
	if !ok {
		// If the schema ID is not found, it implies a top-level schema without an explicit ID, or an error.
		// For the top-level benchmark, we'll use s.BaseSchema.Fields
		if nestedSchemaId == "" {
			return generateFuzzyObjectFromBaseSchema(s.BaseSchema, s.Schemas, cfg)
		}
		return obj // Should ideally error out or handle this more robustly
	}

	return generateFuzzyObjectFromBaseSchema(nestedSchema.BaseSchema, s.Schemas, cfg)
}

func generateFuzzyObjectFromBaseSchema(baseSchema definition.BaseSchema, allSchemas map[definition.SchemaId]definition.NestedSchema, cfg FuzzySchemaConfig) map[string]any {
	obj := make(map[string]any)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for _, field := range baseSchema.Fields {
		var value any
		switch field.FieldProperties.Type {
		case definition.FieldTypeString:
			value = fmt.Sprintf("random_string_%d", r.Intn(1000))
		case definition.FieldTypeInteger:
			value = r.Intn(1000)
		case definition.FieldTypeBoolean:
			value = r.Intn(2) == 1
		case definition.FieldTypeNumber:
			value = r.Float64() * 1000
		case definition.FieldTypeObject:
			if !field.FieldProperties.Schema.IsZero() {
				schemaRef, err := definition.FieldSchemaAs[definition.SchemaReference](field.FieldProperties.Schema)
				if err == nil {
					value = generateFuzzyObject(definition.Schema{Schemas: allSchemas}, schemaRef.ID, cfg)
				}
			}
		case definition.FieldTypeArray:
			if !field.FieldProperties.Schema.IsZero() {
				schemaRef, err := definition.FieldSchemaAs[definition.SchemaReference](field.FieldProperties.Schema)
				if err == nil {
					arrayLen := r.Intn(cfg.MaxArrayElements) + 1
					arr := make([]any, arrayLen)
					for i := 0; i < arrayLen; i++ {
						arr[i] = generateFuzzyObject(definition.Schema{Schemas: allSchemas}, schemaRef.ID, cfg)
					}
					value = arr
				}
			} else {
				// Array of primitives, infer type from FieldProperties.Type
				// This is a simplification; a real schema would likely have a more explicit way to define array item type
				arrayLen := r.Intn(cfg.MaxArrayElements) + 1
				arr := make([]any, arrayLen)
				for i := 0; i < arrayLen; i++ {
					arr[i] = generateFuzzyPrimitiveValue(field.FieldProperties.Type, r)
				}
				value = arr
			}
		}
		obj[string(field.Name)] = value
	}
	return obj
}

// generateFuzzyPrimitiveValue generates a random primitive value for array elements
func generateFuzzyPrimitiveValue(fieldType definition.FieldType, r *rand.Rand) any {
	switch fieldType {
	case definition.FieldTypeString:
		return fmt.Sprintf("array_string_%d", r.Intn(100))
	case definition.FieldTypeInteger:
		return r.Intn(100)
	case definition.FieldTypeBoolean:
		return r.Intn(2) == 1
	case definition.FieldTypeNumber:
		return r.Float64() * 100
	}
	return nil
}

// BenchmarkValidator_Validate_FuzzySchema benchmarks the validation of a randomly generated complex schema and object
func BenchmarkValidator_Validate_FuzzySchema(b *testing.B) {
	cfg := FuzzySchemaConfig{
		MaxDepth:         3, // Reduced depth for initial testing to manage complexity
		MaxFields:        5,
		MaxArrayElements: 3,
		FieldTypeWeights: map[definition.FieldType]int{
			definition.FieldTypeString:  40,
			definition.FieldTypeInteger: 20,
			definition.FieldTypeBoolean: 10,
			definition.FieldTypeNumber:  10,
			definition.FieldTypeObject:  10, // Reduced weight for objects/arrays to control depth
			definition.FieldTypeArray:   10,
		},
		AddConstraintsRate: 0.2, // 20% chance of adding a constraint to a field
	}

	// Generate schema and object once for all benchmark iterations
	rootSchemas := make(map[definition.SchemaId]definition.NestedSchema)
	rootSchemaID := generateFuzzySchema(cfg, 0, rootSchemas)

	fullSchema := definition.Schema{
		BaseSchema: rootSchemas[rootSchemaID].BaseSchema,
		Schemas:    rootSchemas,
	}

	fuzzyObject := generateFuzzyObject(fullSchema, "", cfg) // Pass "" as nestedSchemaId for the top-level object

	// Mock predicates that might be generated (not_empty and is_positive)
	predicates := definition.PredicateMap{
		"is_positive": func(params definition.PredicateParams) []common.Issue {
			if len(params.Keys) > 0 {
				value, _ := utils.GetValueByPath(params.Data, string(params.Keys[0]))
				if i, ok := value.(int); ok {
					if i > 0 {
						return nil
					}
				}
			}
			return []common.Issue{{Code: "NOT_POSITIVE", Message: "Value is not positive"}}
		},
	}

	// Resolve the fuzzy schema and create a validator
	resolvedFuzzySchema := &fullSchema
	validator, err := definition.NewDocumentValidator(resolvedFuzzySchema, predicates)
	if err != nil {
		b.Fatalf("failed to create validator: %v", err)
	}

	b.ResetTimer() // Reset timer to exclude setup time

	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate(fuzzyObject)
	}
}
