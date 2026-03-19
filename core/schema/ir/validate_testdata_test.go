package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestValidate_Fixtures(t *testing.T) {
	// Tests that all fixtures in testdata_test.go are valid.
	fixtures := [][]byte{
		flatSchema,
		nestedObjectSchema,
		enumSchema,
		unionSchema,
		cycleSchema,
		compositeSchema,
		indexedSchema,
		constrainedSchema,
		complexCycleSchema,
		complexConstraintSchema,
	}
	for i, src := range fixtures {
		if errs := ir.Validate(src); len(errs) > 0 {
			t.Errorf("fixture %d failed validation: %v", i, errs)
		}
	}
}
