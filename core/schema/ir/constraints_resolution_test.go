package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestConstraintResolution_PathLookup(t *testing.T) {
	// Tests that field names in constraint expressions are correctly resolved
	// to DataPoints via Address space.
	cs := mustCompileWithStubPredicate(constrainedSchema, "isEmail")

	rt := cs.ResolvedConstraints
	if rt == nil {
		t.Fatal("ResolvedConstraints is nil")
	}

	rc, ok := rt.Roots[0].(ir.ResolvedConstraint)
	if !ok {
		t.Fatal("root constraint is not a ResolvedConstraint")
	}

	if len(rc.Fields) != 1 {
		t.Fatalf("Fields: got %d, want 1", len(rc.Fields))
	}

	if rc.Fields[0] == 0 {
		t.Error("Resolved field DataPoint is zero")
	}
}
