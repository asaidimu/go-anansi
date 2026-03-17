package ir

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

func TestConstraints_UnknownFieldPath(t *testing.T) {
	// Global constraint referring to a non-existent field.
	src := []byte(`{
	  "name": "UnknownFieldConstraint",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0300-7300-8000-070e151c232a": { "name": "valid", "type": "string" }
	  },
	  "constraints": {
	    "019ca000-0310-7310-90d0-d7dee5ecf3fa": {
	      "name": "bad_field",
	      "predicate": "present",
	      "fields": ["nonexistent"]
	    }
	  }
	}`)

	pm := PredicateMap{
		"present": func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
	}

	_, err := Compile(mustParse(src), pm)
	if err == nil {
		t.Fatal("expected error for unknown field path in constraint")
	}

	errs := allErrors(err)
	found := false
	for _, e := range errs {
		if e.Pass == PassConstraints && e.Message != "" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected PassConstraints error for unknown field, got: %v", errs)
	}
}

func TestConstraints_AbsolutePathResolution(t *testing.T) {
	// Tests that constraints defined at root resolve absolute paths.
	src := []byte(`{
	  "name": "AbsolutePathTest",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0400-7400-8000-070e151c232a": {
	      "name": "nested",
	      "type": "object",
	      "schema": { "id": "019ca000-0410-7410-90d0-d7dee5ecf3fa" }
	    }
	  },
	  "schemas": {
	    "019ca000-0410-7410-90d0-d7dee5ecf3fa": {
	      "name": "Child",
	      "fields": {
	        "019ca000-0411-7411-91dd-e4ebf2f90007": { "name": "val", "type": "integer" }
	      }
	    }
	  },
	  "constraints": {
	    "019ca000-0420-7420-92ea-f1f8ff060d14": {
	      "name": "root_check",
	      "predicate": "positive",
	      "fields": ["nested.val"]
	    }
	  }
	}`)

	pm := PredicateMap{
		"positive": func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
	}

	cs, err := Compile(mustParse(src), pm)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	rt := cs.ResolvedConstraints
	if rt == nil {
		t.Fatal("ResolvedConstraints is nil")
	}

	if len(rt.Roots) != 1 {
		t.Fatalf("expected 1 root constraint, got %d", len(rt.Roots))
	}

	rc, ok := rt.Roots[0].(ResolvedConstraint)
	if !ok {
		t.Fatal("root constraint is not a leaf")
	}

	// The path "nested.val" should be resolved to an absolute DataPoint.
	if len(rc.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(rc.Fields))
	}
	
	expectedDP, err := cs.Address("nested.val")
	if err != nil {
		t.Fatalf("Address(nested.val) failed: %v", err)
	}

	if rc.Fields[0] != expectedDP {
		t.Errorf("expected field DataPoint %v, got %v", expectedDP, rc.Fields[0])
	}
}
