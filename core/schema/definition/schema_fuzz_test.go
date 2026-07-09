package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// FuzzSchema tests the unmarshaling and resolution of the Schema object
func FuzzSchema(f *testing.F) {
	// Seed corpus with known valid and invalid JSON inputs
	f.Add([]byte(`{"name": "test", "version": "1.0.0"}`))
	f.Add([]byte(`{"name": "test", "version": "1.0.0", "fields": {"id1": {"name": "field1", "type": "string"}}}`))
	f.Add([]byte(`{}`)) // Empty JSON
	f.Add([]byte(`{"version": "1.0.0", "schemas": {"nested1": {"type": "string"}}}`)) // Schema with nested but without name in root

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("fuzzing caused a panic: %v", r)
			}
		}()

		var schema definition.Schema
		err := json.Unmarshal(data, &schema)
		if err != nil {
			// If unmarshaling fails, we don't proceed with resolution and validation,
			// as the input is considered malformed JSON, which is expected for fuzzing.
			// We only care if it panics or causes other unexpected errors.
			return
		}

		// Try to resolve the schema (now, the unmarshaled schema is directly used)
		resolvedSchema := &schema // Assume unmarshaled schema is the resolved schema

		// Try to create a validator and validate a dummy object
		validator, err := definition.NewDocumentValidator(resolvedSchema, nil)
		if err != nil {
			t.Errorf("failed to create validator: %v", err)
			return
		}
		// Provide a minimal object that might trigger some paths but not necessarily require complex structure
		_, _ = validator.Validate(map[string]any{})
	})
}
