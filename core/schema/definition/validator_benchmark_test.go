package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

const (
	PoolSize = 50
)

func runBenchDocumentValidator(b *testing.B, name string, schemaDepth int, schemaWidth int) {
	// 1. Generate a complex, valid Schema using GenerateFuzzyData on MetaSchema
	// The depth and width parameters for schema generation are passed to FuzzyConfig
	schemaFuzzyConfig := definition.FuzzyConfig{
		MaxDepth:            3,   // Very small depth for debugging
		ContinueProbability: 0.1, // Very low probability for debugging
		ErrorRate:           0.0, // CRUCIAL: Schema itself must be valid
	}

	// Generate schema definition as map[string]any
	generatedSchemaMap := definition.GenerateFuzzyData(&definition.MetaSchema, schemaFuzzyConfig)
	// Manually set a valid version, as GenerateFuzzyData might produce an invalid string for FieldTypeString
	generatedSchemaMap["version"] = "1.0.0"

	// Marshal and Unmarshal to get a proper definition.Schema object
	generatedSchemaJSON, err := json.Marshal(generatedSchemaMap)
	if err != nil {
		return
	}

	var fullSchema definition.Schema
	err = json.Unmarshal(generatedSchemaJSON, &fullSchema)
	if err != nil {
		return
	}

	// Ensure the generated schema is actually valid according to MetaSchema
	metaValidator, err := definition.NewDocumentValidator(&definition.MetaSchema, mockPredicates())
	if err != nil {
		b.Fatalf("Failed to create MetaSchema validator: %v", err) // Indicates a problem with MetaSchema itself
	}
	metaIssues, isMetaValid := metaValidator.Validate(generatedSchemaMap)
	if !isMetaValid {
		b.Skipf("Skipping %s: Generated schema is NOT valid according to MetaSchema. Issues: %v\nJSON: %s", name, metaIssues, string(generatedSchemaJSON))
		return
	}

	// 2. Setup Validator for the generated schema
	validator, err := definition.NewDocumentValidator(&fullSchema, mockPredicates())
	if err != nil {
		b.Skipf("Skipping %s: Failed to create validator for generated schema: %v\nSchema JSON: %s", name, err, string(generatedSchemaJSON))
		return
	}

	// 3. Pre-bake Data using the generated schema
	validData := make([]map[string]any, PoolSize)
	dataFuzzyConfig := definition.FuzzyConfig{
		MaxDepth:            schemaDepth,
		ContinueProbability: float64(schemaWidth) / 10.0, // Data might be wider/deeper than schema
		ErrorRate:           0.0, // Data must be valid for the generated schema
	}
	for i := 0; i < PoolSize; i++ {
		validData[i] = definition.GenerateFuzzyData(&fullSchema, dataFuzzyConfig)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate(validData[i%PoolSize])
	}
}

func BenchmarkDocumentValidator_Robust_Small_Flat(b *testing.B) {
	runBenchDocumentValidator(b, "Small_Flat", 1, 10)
}

func BenchmarkDocumentValidator_Robust_Deep_Stress(b *testing.B) {
	runBenchDocumentValidator(b, "Deep_Stress", 20, 2)
}

func BenchmarkDocumentValidator_Robust_Wide_Stress(b *testing.B) {
	runBenchDocumentValidator(b, "Wide_Stress", 2, 50)
}

func BenchmarkDocumentValidator_Robust_Industrial_Complex(b *testing.B) {
	runBenchDocumentValidator(b, "Industrial_Complex", 8, 5)
}

func mockPredicates() definition.PredicateMap {
	return definition.PredicateMap{
		"is_positive": func(p definition.PredicateParams) []common.Issue { return nil },
	}
}
