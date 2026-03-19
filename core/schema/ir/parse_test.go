package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestParse_FlatSchema(t *testing.T) {
	_, err := ir.Parse(flatSchema)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
}
