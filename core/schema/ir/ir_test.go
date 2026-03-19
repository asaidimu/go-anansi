package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestDescriptorToDataPoint(t *testing.T) {
	tests := []struct {
		name     string
		fd       uint32
		wantType document.DataType
		wantID   int32
	}{
		{
			name:     "String field",
			fd:       ir.PackDescriptor(ir.TypeString, 1, 10, 0, false, false, false),
			wantType: document.TypeString,
			wantID:   (1 << 7) | 10, // owner=1, index=10
		},
		{
			name:     "Integer field",
			fd:       ir.PackDescriptor(ir.TypeInteger, 2, 20, 0, false, false, false),
			wantType: document.TypeInt,
			wantID:   (2 << 7) | 20,
		},
		{
			name:     "Number field (float)",
			fd:       ir.PackDescriptor(ir.TypeNumber, 3, 30, 0, false, false, false),
			wantType: document.TypeFloat,
			wantID:   (3 << 7) | 30,
		},
		{
			name:     "Boolean field",
			fd:       ir.PackDescriptor(ir.TypeBoolean, 4, 40, 0, false, false, false),
			wantType: document.TypeBool,
			wantID:   (4 << 7) | 40,
		},
		{
			name:     "Bytes field",
			fd:       ir.PackDescriptor(ir.TypeBytes, 5, 50, 0, false, false, false),
			wantType: document.TypeBytes,
			wantID:   (5 << 7) | 50,
		},
		{
			name:     "Decimal field (stored as int)",
			fd:       ir.PackDescriptor(ir.TypeDecimal, 6, 60, 0, false, false, false),
			wantType: document.TypeInt,
			wantID:   (6 << 7) | 60,
		},
		{
			name:     "Geometry field",
			fd:       ir.PackDescriptor(ir.TypeGeometry, 7, 70, 0, false, false, false),
			wantType: document.TypeGeometry,
			wantID:   (7 << 7) | 70,
		},
		{
			name:     "Record field",
			fd:       ir.PackDescriptor(ir.TypeRecord, 8, 80, 0, false, false, false),
			wantType: document.TypeRecord,
			wantID:   (8 << 7) | 80,
		},
		{
			name:     "Array field (Object array)",
			fd:       ir.PackDescriptor(ir.TypeArray, 9, 90, 0, false, false, false),
			wantType: document.TypeArrayObject,
			wantID:   (9 << 7) | 90,
		},
		{
			name:     "Set field (Object array)",
			fd:       ir.PackDescriptor(ir.TypeSet, 10, 100, 0, false, false, false),
			wantType: document.TypeArrayObject,
			wantID:   (10 << 7) | 100,
		},
		{
			name:     "Object field",
			fd:       ir.PackDescriptor(ir.TypeObject, 11, 110, 0, false, false, false),
			wantType: document.TypeRecord,
			wantID:   (11 << 7) | 110,
		},
		{
			name:     "Unknown field",
			fd:       ir.PackDescriptor(ir.TypeUnknown, 0, 0, 0, false, false, false),
			wantType: document.TypeUnknown,
			wantID:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ir.DescriptorToDataPoint(tt.fd)
			if got.Type() != tt.wantType {
				t.Errorf("Type() = %v, want %v", got.Type(), tt.wantType)
			}
			if got.ID() != tt.wantID {
				t.Errorf("ID() = %v, want %v", got.ID(), tt.wantID)
			}
		})
	}
}

func TestIsSchemaBearing_AllTypes(t *testing.T) {
	tests := []struct {
		typ  ir.FieldTypeEnum
		want bool
	}{
		{ir.TypeUnknown, false},
		{ir.TypeString, false},
		{ir.TypeNumber, false},
		{ir.TypeInteger, false},
		{ir.TypeBoolean, false},
		{ir.TypeBytes, false},
		{ir.TypeArray, true},
		{ir.TypeSet, true},
		{ir.TypeEnum, true},
		{ir.TypeObject, true},
		{ir.TypeRecord, true},
		{ir.TypeUnion, true},
		{ir.TypeComposite, true},
		{ir.TypeGeometry, false},
		{ir.TypeDecimal, false},
	}

	for _, tt := range tests {
		// Test via FieldTypeEnum method
		if got := tt.typ.IsSchemaBearing(); got != tt.want {
			t.Errorf("FieldTypeEnum(%v).IsSchemaBearing() = %v, want %v", tt.typ, got, tt.want)
		}

		// Test via descriptor helper
		fd := ir.PackDescriptor(tt.typ, 0, 0, 0, false, false, false)
		if got := ir.IsSchemaBearing(fd); got != tt.want {
			t.Errorf("IsSchemaBearing(fd) [type=%v] = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestEnumElemTypeToArrayDataType(t *testing.T) {
	tests := []struct {
		typ  ir.FieldTypeEnum
		want document.DataType
	}{
		{ir.TypeInteger, document.TypeArrayInt},
		{ir.TypeDecimal, document.TypeArrayInt},
		{ir.TypeNumber, document.TypeArrayFloat},
		{ir.TypeString, document.TypeArrayString},
		{ir.TypeBoolean, document.TypeArrayBool},
		{ir.TypeUnknown, document.TypeArrayUnknown},
		{ir.TypeBytes, document.TypeArrayUnknown}, // Not supported for enums
	}

	for _, tt := range tests {
		if got := ir.EnumElemTypeToArrayDataType(tt.typ); got != tt.want {
			t.Errorf("EnumElemTypeToArrayDataType(%v) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestDescriptorToEnumDocumentKey(t *testing.T) {
	// 5 << 7 | 10 = 640 + 10 = 650
	fd := ir.PackDescriptor(ir.TypeEnum, 5, 10, 0, false, false, false)
	
	// String array enum
	dk := ir.DescriptorToEnumDocumentKey(fd, document.TypeArrayString)
	if dk.DataPoint().Type() != document.TypeArrayString {
		t.Errorf("Type() = %v, want %v", dk.DataPoint().Type(), document.TypeArrayString)
	}
	if dk.DataPoint().ID() != 650 {
		t.Errorf("ID() = %v, want 650", dk.DataPoint().ID())
	}

	// Int array enum
	dk = ir.DescriptorToEnumDocumentKey(fd, document.TypeArrayInt)
	if dk.DataPoint().Type() != document.TypeArrayInt {
		t.Errorf("Type() = %v, want %v", dk.DataPoint().Type(), document.TypeArrayInt)
	}
	if dk.DataPoint().ID() != 650 {
		t.Errorf("ID() = %v, want 650", dk.DataPoint().ID())
	}
}

func TestInferEnumElemType(t *testing.T) {
	tests := []struct {
		val  any
		want ir.FieldTypeEnum
	}{
		{"hello", ir.TypeString},
		{true, ir.TypeBoolean},
		{int64(123), ir.TypeInteger},
		{float64(123.0), ir.TypeInteger}, // Whole float -> Integer
		{float64(123.45), ir.TypeNumber}, // Fractional float -> Number
		{[]byte("bytes"), ir.TypeUnknown}, // Not supported
		{nil, ir.TypeUnknown},
	}

	for _, tt := range tests {
		if got := ir.InferEnumElemType(tt.val); got != tt.want {
			t.Errorf("InferEnumElemType(%v) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

