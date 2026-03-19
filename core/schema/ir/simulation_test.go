package ir_test

import (
	"testing"

)

func TestCompile_Simulation(t *testing.T) {
	// Tests compile on larger schema inputs (via testdata_test.go).
	schemas := [][]byte{flatSchema, nestedObjectSchema, enumSchema, unionSchema, cycleSchema}
	for _, src := range schemas {
		cs := mustCompile(src, nil)
		if cs == nil {
			t.Errorf("Compile(src) returned nil")
		}
	}
}
