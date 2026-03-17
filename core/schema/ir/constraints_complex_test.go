package ir

import (
	"testing"
	"github.com/asaidimu/go-anansi/v6/core/document"
)

func TestConstraints_ComplexNesting(t *testing.T) {
	src := []byte(`{
	  "name": "NestedConstraints",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0200-7200-8000-070e151c232a": { "name": "a", "type": "integer" },
	    "019ca000-0201-7201-810d-141b22293037": { "name": "b", "type": "integer" },
	    "019ca000-0202-7202-821a-21282f363d44": { "name": "c", "type": "integer" }
	  },
	  "constraints": {
	    "019ca000-0210-7210-90d0-d7dee5ecf3fa": {
	      "name": "complex",
	      "operator": "or",
	      "rules": [
	        {
	          "name": "sub1",
	          "operator": "and",
	          "rules": [
	            { "name": "a_gt_0", "predicate": "gt", "fields": ["a"], "parameters": 0 },
	            { "name": "b_gt_0", "predicate": "gt", "fields": ["b"], "parameters": 0 }
	          ]
	        },
	        { "name": "c_eq_100", "predicate": "eq", "fields": ["c"], "parameters": 100 }
	      ]
	    }
	  }
	}`)

	pm := PredicateMap{
		"gt": func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
		"eq": func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
	}

	cs, err := Compile(mustParse(src), pm)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	rt := cs.ResolvedConstraints
	if rt == nil {
		t.Fatal("ResolvedConstraints missing")
	}

	if len(rt.Roots) != 1 {
		t.Fatalf("expected 1 root constraint, got %d", len(rt.Roots))
	}

	root, ok := rt.Roots[0].(ResolvedConstraintGroup)
	if !ok {
		t.Fatal("root is not a group")
	}
	if root.Operator != LogicalOr {
		t.Errorf("expected OR, got %v", root.Operator)
	}
	if len(root.Constraints) != 2 {
		t.Fatalf("expected 2 children in root, got %d", len(root.Constraints))
	}

	// First child should be an AND group
	sub1, ok := root.Constraints[0].(ResolvedConstraintGroup)
	if !ok {
		t.Fatal("first child is not a group")
	}
	if sub1.Operator != LogicalAnd {
		t.Errorf("expected AND, got %v", sub1.Operator)
	}
	if len(sub1.Constraints) != 2 {
		t.Errorf("expected 2 children in sub1, got %d", len(sub1.Constraints))
	}

	// Second child of root should be a leaf
	sub2, ok := root.Constraints[1].(ResolvedConstraint)
	if !ok {
		t.Fatal("second child of root is not a leaf")
	}
	if len(sub2.Fields) != 1 {
		t.Errorf("expected 1 field for c_eq_100, got %d", len(sub2.Fields))
	}
}

func TestConstraints_UnknownPredicateError(t *testing.T) {
	src := []byte(`{
	  "name": "BadPredicate",
	  "version": "1.0.0",
	  "fields": { "019ca000-0200-7200-8000-070e151c232a": { "name": "a", "type": "integer" } },
	  "constraints": {
	    "019ca000-0210-7210-90d0-d7dee5ecf3fa": { "name": "bad", "predicate": "nonexistent", "fields": ["a"] }
	  }
	}`)

	_, err := Compile(mustParse(src), nil)
	if err == nil {
		t.Fatal("expected error for unknown predicate, got nil")
	}
	errs := allErrors(err)
	found := false
	for _, e := range errs {
		if e.Pass == PassConstraints {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected PassConstraints error")
	}
}
