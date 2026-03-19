package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestValidate_MetaSchema(t *testing.T) {
	// Tests that all fixtures in testdata_test.go are valid against meta_schema.json.
	cases := []struct {
		name string
		src  []byte
	}{
		{"flatSchema", flatSchema},
		{"nestedObjectSchema", nestedObjectSchema},
		{"enumSchema", enumSchema},
		{"unionSchema", unionSchema},
		{"cycleSchema", cycleSchema},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if errs := ir.Validate(c.src); len(errs) > 0 {
				t.Errorf("Validate(%s) failed: %v", c.name, errs)
			}
		})
	}
}

func TestValidate_InvalidSchemas(t *testing.T) {
	// Use known-invalid schemas to ensure Validate() detects errors.
	cases := []struct {
		name string
		src  []byte
	}{
		{"invalidJSON", []byte(`{ not valid json`)},
		{"invalidType", []byte(`{
		  "name": "InvalidType",
		  "version": "1.0.0",
		  "fields": {
		    "019ca000-0001-7001-810d-141b22293037": { "name": "x", "type": "bogustype" }
		  }
		}`)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if errs := ir.Validate(c.src); len(errs) == 0 {
				t.Errorf("Validate(%s) should have failed", c.name)
			}
		})
	}
}
