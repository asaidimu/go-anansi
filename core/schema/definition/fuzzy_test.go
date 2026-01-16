package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

func TestGenerateFuzzyData_ValidData(t *testing.T) {
	// 1. Define a sample schema
	// This schema will include various field types: string, integer, boolean, enum, object, array.
	sampleSchema := &definition.Schema{
		Version: *common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name:        "TestSchema",
			Description: "A sample schema for fuzzy data generation testing",
			Fields: map[definition.FieldId]definition.Field{
				"id": {
					Name:        "id",
					Description: "Unique identifier",
					Required:    true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"age": {
					Name:        "age",
					Description: "User's age",
					Required:    false, // Optional field
					FieldProperties: definition.FieldProperties{
						Type:    definition.FieldTypeInteger,
						Default: definition.MustNewLiteralValue(int64(30)),
					},
				},
				"isActive": {
					Name:        "isActive",
					Description: "Is user active",
					Required:    true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeBoolean,
					},
				},
				"status": {
					Name:        "status",
					Description: "User's status",
					Required:    true,
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeEnum,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "UserStatusEnum"}),
					},
				},
				"address": {
					Name:        "address",
					Description: "User's address",
					Required:    false,
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeObject,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "AddressSchema"}),
					},
				},
				"tags": {
					Name:        "tags",
					Description: "List of tags",
					Required:    false,
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeArray,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "TagType"}),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"UserStatusEnum": {
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
				Values: []definition.LiteralValue{
					definition.MustNewLiteralValue("active"),
					definition.MustNewLiteralValue("inactive"),
					definition.MustNewLiteralValue("pending"),
				},
			},
			"AddressSchema": {
				BaseSchema: definition.BaseSchema{
					Name:        "Address",
					Description: "User address details",
					Fields: map[definition.FieldId]definition.Field{
						"street": {
							Name:            "street",
							Required:        true,
							FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString},
						},
						"city": {
							Name:            "city",
							Required:        true,
							FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString},
						},
						"zipCode": {
							Name:            "zipCode",
							Required:        false,
							FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString},
						},
					},
				},
			},
			"TagType": {
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
		},
	}

	// 2. Generate data with ErrorRate = 0.0 (to ensure valid data)
	config := definition.FuzzyConfig{
		MaxDepth:            5,   // Set a reasonable depth
		ContinueProbability: 0.8, // High probability to explore nested structures
		ErrorRate:           0.0, // Crucial: No errors should be injected
	}

	generatedData := definition.GenerateFuzzyData(sampleSchema, config)

	// Print generated data for inspection (optional, but helpful for debugging)
	jsonData, err := json.MarshalIndent(generatedData, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal generated data: %v", err)
	}
	t.Logf("Generated Data:\n%s", string(jsonData))

	// 3. Validate the generated data against the schema
	validator, err := definition.NewDocumentValidator(sampleSchema, nil)
	if err != nil {
		t.Fatalf("Failed to create document validator: %v", err)
	}

	issues, isValid := validator.Validate(generatedData)

	// 4. Assert no validation issues
	if !isValid {
		t.Errorf("Generated data is NOT valid. Issues found:\n")
		for _, issue := range issues {
			t.Errorf("- %s: %s at path '%s'", issue.Code, issue.Message, issue.Path)
		}
	}
}

func TestGenerateFuzzyData_DefaultValues(t *testing.T) {
	sampleSchema := &definition.Schema{
		Version: *common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "DefaultTestSchema",
			Fields: map[definition.FieldId]definition.Field{
				"name": {
					Name:     "name",
					Required: false,
					FieldProperties: definition.FieldProperties{
						Type:    definition.FieldTypeString,
						Default: definition.MustNewLiteralValue("anonymous"),
					},
				},
				"count": {
					Name:     "count",
					Required: false,
					FieldProperties: definition.FieldProperties{
						Type:    definition.FieldTypeInteger,
						Default: definition.MustNewLiteralValue(int64(100)),
					},
				},
				"isActive": {
					Name:     "isActive",
					Required: false,
					FieldProperties: definition.FieldProperties{
						Type:    definition.FieldTypeBoolean,
						Default: definition.MustNewLiteralValue(true),
					},
				},
			},
		},
	}

	config := definition.FuzzyConfig{
		MaxDepth:            1,
		ContinueProbability: 0.0, // Don't recurse
		ErrorRate:           0.0, // No errors
	}

	generatedData := definition.GenerateFuzzyData(sampleSchema, config)

	validator, err := definition.NewDocumentValidator(sampleSchema, nil)
	if err != nil {
		t.Fatalf("Failed to create document validator: %v", err)
	}

	issues, isValid := validator.Validate(generatedData)

	if !isValid {
		t.Errorf("Generated data with defaults is NOT valid. Issues found:\n")
		for _, issue := range issues {
			t.Errorf("- %s: %s at path '%s'", issue.Code, issue.Message, issue.Path)
		}
	}

	// Verify default values are present and correct if fields are not explicitly generated otherwise
	// Note: GenerateFuzzyData will always populate fields if they have a default and ErrorRate is 0.
	if name, ok := generatedData["name"].(string); !ok || name != "anonymous" {
		t.Errorf("Expected 'name' to be 'anonymous', got '%v'", generatedData["name"])
	}
	if count, ok := generatedData["count"].(int64); !ok || count != 100 {
		t.Errorf("Expected 'count' to be 100, got '%v'", generatedData["count"])
	}
	if isActive, ok := generatedData["isActive"].(bool); !ok || isActive != true {
		t.Errorf("Expected 'isActive' to be true, got '%v'", generatedData["isActive"])
	}
}

func FuzzGeneratedSchemas(f *testing.F) {
	// Seed corpus with sensible values for FuzzyConfig parameters
	f.Add(3, 0.6, 0.05) // maxDepth, continueProbability, errorRate

	f.Fuzz(func(t *testing.T, maxDepth int, continueProbability float64, errorRate float64) {
		config := definition.FuzzyConfig{
			MaxDepth:            maxDepth,
			ContinueProbability: continueProbability,
			ErrorRate:           errorRate,
		}
		// 1. Generate a fuzzy schema definition (map[string]any) using MetaSchema
		// MaxDepth for schema generation is capped to prevent excessively large schemas
		// Set a reasonable max depth for generating the meta-schema itself
		config.MaxDepth = 4
		if config.ContinueProbability > 0.8 { // Cap probability for meta-schema generation
			config.ContinueProbability = 0.8
		}

		generatedSchemaMap := definition.GenerateFuzzyData(&definition.MetaSchema, config)

		// 2. Marshal the generated map to JSON
		generatedSchemaJSON, err := json.Marshal(generatedSchemaMap)
		if err != nil {
			// t.Logf("Skipping: Could not marshal generated schema map to JSON: %v", err)
			return // Skip this iteration, as marshal failure means it's not a valid JSON representation
		}

		// 3. Unmarshal the JSON back into a definition.Schema object
		var s definition.Schema
		err = json.Unmarshal(generatedSchemaJSON, &s)
		if err != nil {
			t.Logf("Skipping: Generated JSON does not unmarshal into a valid Schema object: %v\nJSON: %s", err, string(generatedSchemaJSON))
			return // Skip if it's not even a syntactically valid schema definition
		}

		// 4. Validate the unmarshaled schema object against MetaSchema itself
		metaValidator, err := definition.NewDocumentValidator(&definition.MetaSchema, nil)
		if err != nil {
			t.Fatalf("Failed to create MetaSchema validator: %v", err) // This indicates an issue with MetaSchema or validator creation
		}

		metaIssues, isMetaValid := metaValidator.Validate(generatedSchemaMap) // Validate the map directly, as validator expects map[string]any

		if !isMetaValid {
			t.Logf("Generated schema is NOT valid according to MetaSchema. Issues:\nJSON: %s", string(generatedSchemaJSON))
			for _, issue := range metaIssues {
				t.Logf("- %s: %s at path '%s'", issue.Code, issue.Message, issue.Path)
			}
			// It's okay for fuzzing to generate invalid inputs, so we don't fail here.
			// The goal is to ensure the system handles invalid schemas gracefully.
			return
		}

		// If we reach here, 's' is a valid schema definition.
		t.Logf("Successfully generated and meta-validated a schema.\nGenerated Schema JSON:\n%s", string(generatedSchemaJSON))

		// 5. Create a validator for the *generated* valid schema
		generatedSchemaValidator, err := definition.NewDocumentValidator(&s, nil)
		if err != nil {
			t.Errorf("Failed to create validator for generated schema: %v\nGenerated Schema JSON:\n%s", err, string(generatedSchemaJSON))
			return // This is a failure, as a meta-valid schema should produce a valid validator
		}

		// 6. Generate fuzzy data using *this generated schema*
		dataConfig := definition.FuzzyConfig{
			MaxDepth:            5,   // Set a reasonable depth for actual data
			ContinueProbability: 0.8, // High probability to explore nested structures
			ErrorRate:           0.0, // CRUCIAL: Generate VALID data for the generated schema
		}
		generatedData := definition.GenerateFuzzyData(&s, dataConfig)

		generatedDataJSON, _ := json.MarshalIndent(generatedData, "", "  ")
		// 7. Validate the generated data against *this generated schema's validator*
		dataIssues, isDataValid := generatedSchemaValidator.Validate(generatedData)

		if !isDataValid {
			t.Errorf("Generated data for a meta-valid schema is NOT valid. Issues found:\nGenerated Schema JSON:\n%s\nGenerated Data JSON:\n%s", string(generatedSchemaJSON), string(generatedDataJSON))
			for _, issue := range dataIssues {
				t.Errorf("- %s: %s at path '%s'", issue.Code, issue.Message, issue.Path)
			}
		} else {
			t.Logf("Generated data for meta-valid schema is VALID.\nGenerated Data:\n%s", string(generatedDataJSON))
		}
	})
}

// Helper to ensure all types are available for testing
func init() {
	// Ensure that all FieldType values used in the test schema are properly defined
	// (This is usually done by the package itself, but explicit check adds robustness)
	_ = definition.FieldTypeString
	_ = definition.FieldTypeInteger
	_ = definition.FieldTypeBoolean
	_ = definition.FieldTypeEnum
	_ = definition.FieldTypeObject
	_ = definition.FieldTypeArray
	_ = definition.FieldTypeRecord // Ensure Record is also available
	_ = definition.FieldTypeNumber
	_ = definition.FieldTypeDecimal
	_ = definition.FieldTypeUnion
	_ = definition.FieldTypeComposite
	_ = definition.FieldTypeGeometry
	_ = definition.FieldTypeUnknown
}
