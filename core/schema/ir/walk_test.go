package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestTerminalWalk_FlatSchema(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	var descriptors []uint32
	ir.TerminalWalk(cs, 0, func(fd uint32) {
		descriptors = append(descriptors, fd)
	})

	// flatSchema has 3 fields.
	if len(descriptors) != 3 {
		t.Errorf("Walk count: got %d, want 3", len(descriptors))
	}
}

func TestTerminalWalk_NestedObjectSchema(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	var descriptors []uint32
	ir.TerminalWalk(cs, 0, func(fd uint32) {
		descriptors = append(descriptors, fd)
	})

	// Person.address(0) in Person(0). Address is index 1.
	// address.street, address.city. Total terminal fields = 2.
	if len(descriptors) != 2 {
		t.Errorf("Walk count: got %d, want 2 (terminals only)", len(descriptors))
	}
}
