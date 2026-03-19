package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

func TestAddress_ThroughUnionField(t *testing.T) {
	cs := mustCompile(unionSchema, nil)

	// "payload.typeA" resolves via variants of union field "payload".
	dp, err := cs.Address("payload.typeA")
	if err != nil {
		t.Fatalf("Address(payload.typeA): %v", err)
	}
	if dp.Type() != document.TypeString {
		t.Errorf("typeA DataType: got %v, want TypeString", dp.Type())
	}
}

func TestAddress_ThroughCompositeField(t *testing.T) {
	cs := mustCompile(compositeSchema, nil)

	// "comp.f1" resolves via variants of composite field "comp".
	dp1, err := cs.Address("comp.f1")
	if err != nil {
		t.Fatalf("Address(comp.f1): %v", err)
	}
	if dp1.Type() != document.TypeString {
		t.Errorf("f1 DataType: got %v, want TypeString", dp1.Type())
	}

	dp2, err := cs.Address("comp.f2")
	if err != nil {
		t.Fatalf("Address(comp.f2): %v", err)
	}
	if dp2.Type() != document.TypeInt {
		t.Errorf("f2 DataType: got %v, want TypeInt", dp2.Type())
	}
}
