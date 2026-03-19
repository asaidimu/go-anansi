package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestExtractType(t *testing.T) {
	cases := []struct {
		fd   uint32
		want ir.FieldTypeEnum
	}{
		{ir.PackDescriptor(ir.TypeString, 0, 0, 0, false, false, false), ir.TypeString},
		{ir.PackDescriptor(ir.TypeEnum, 1, 2, 3, true, true, true), ir.TypeEnum},
		{ir.PackDescriptor(ir.TypeObject, 127, 255, 127, false, false, false), ir.TypeObject},
	}
	for _, c := range cases {
		if got := ir.ExtractType(c.fd); got != c.want {
			t.Errorf("ExtractType(0x%08X): got %v, want %v", c.fd, got, c.want)
		}
	}
}

func TestExtractFieldIndex(t *testing.T) {
	cases := []struct {
		fd   uint32
		want uint8
	}{
		{ir.PackDescriptor(ir.TypeString, 0, 0, 0, false, false, false), 0},
		{ir.PackDescriptor(ir.TypeEnum, 1, 42, 3, true, true, true), 42},
		{ir.PackDescriptor(ir.TypeObject, 127, 127, 127, false, false, false), 127},
	}
	for _, c := range cases {
		if got := ir.ExtractFieldIndex(c.fd); got != c.want {
			t.Errorf("ExtractFieldIndex(0x%08X): got %d, want %d", c.fd, got, c.want)
		}
	}
}

func TestExtractOwnerSchema(t *testing.T) {
	cases := []struct {
		fd   uint32
		want uint8
	}{
		{ir.PackDescriptor(ir.TypeString, 0, 0, 0, false, false, false), 0},
		{ir.PackDescriptor(ir.TypeEnum, 42, 2, 3, true, true, true), 42},
		{ir.PackDescriptor(ir.TypeObject, 127, 127, 127, false, false, false), 127},
	}
	for _, c := range cases {
		if got := ir.ExtractOwnerSchema(c.fd); got != c.want {
			t.Errorf("ExtractOwnerSchema(0x%08X): got %d, want %d", c.fd, got, c.want)
		}
	}
}

func TestExtractTargetSchema(t *testing.T) {
	cases := []struct {
		fd   uint32
		want uint8
	}{
		{ir.PackDescriptor(ir.TypeString, 0, 0, 0, false, false, false), 0},
		{ir.PackDescriptor(ir.TypeEnum, 1, 2, 42, true, true, true), 42},
		{ir.PackDescriptor(ir.TypeObject, 127, 127, 127, false, false, false), 127},
	}
	for _, c := range cases {
		if got := ir.ExtractTargetSchema(c.fd); got != c.want {
			t.Errorf("ExtractTargetSchema(0x%08X): got %d, want %d", c.fd, got, c.want)
		}
	}
}

func TestPackDescriptor_Flags(t *testing.T) {
	withFlags := ir.PackDescriptor(ir.TypeString, 1, 2, 0, true, true, true) | ir.FDMaskTerminal

	if (withFlags & ir.FDMaskRequired) == 0 {
		t.Error("Required bit not set")
	}
	if (withFlags & ir.FDMaskUnique) == 0 {
		t.Error("Unique bit not set")
	}
	if (withFlags & ir.FDMaskDeprecated) == 0 {
		t.Error("Deprecated bit not set")
	}
	if (withFlags & ir.FDMaskTerminal) == 0 {
		t.Error("Terminal bit not set")
	}
}
