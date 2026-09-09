package document

import (
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func TestDebugEnrichIdempotent(t *testing.T) {
	s1, _ := definition.FromJSON([]byte(testSchemaJSON))
	if _, err := enrichSchema(s1); err != nil {
		t.Fatal(err)
	}
	s2, _ := definition.FromJSON([]byte(testSchemaJSON))
	if _, err := enrichSchema(s2); err != nil {
		t.Fatal(err)
	}
	if _, err := enrichSchema(s2); err != nil {
		t.Fatal(err)
	}

	// reparse both to normalize any map-iteration ordering
	r1, _ := definition.FromJSON(s1.ToJSON())
	r2, _ := definition.FromJSON(s2.ToJSON())
	if !reflect.DeepEqual(r1, r2) {
		t.Errorf("content differs:\n1=%s\n2=%s", r1.ToJSON(), r2.ToJSON())
	} else {
		t.Logf("content identical: %d fields, %d schemas", len(r1.Fields), len(r1.Schemas))
	}

	// enrich a fresh copy, then enrich again and confirm byte-stable after reparse
	s3, _ := definition.FromJSON([]byte(testSchemaJSON))
	if _, err := enrichSchema(s3); err != nil {
		t.Fatal(err)
	}
	r3, _ := definition.FromJSON(s3.ToJSON())
	if !reflect.DeepEqual(r1, r3) {
		t.Errorf("3rd enrichment differs")
	}
}
