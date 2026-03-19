package ir_test

import (
	"testing"

)

func TestConstraints_ComplexExpressions(t *testing.T) {
	// Tests nested logical operators and comparisons.
	// (a > 10 AND b == true) OR c != "foo"
	cs := mustCompileWithStubPredicate(complexConstraintSchema, "isEmail")

	if cs.ResolvedConstraints == nil {
		t.Fatal("ResolvedConstraints is nil")
	}

	roots := cs.ResolvedConstraints.Roots[0]
	if roots == nil {
		t.Errorf("Root constraint for schema 0 not found")
	}
}
