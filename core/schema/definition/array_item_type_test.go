package definition

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
)

func literal(t *testing.T, v any) LiteralValue {
	t.Helper()
	lv, err := newLiteralValue(v)
	if err != nil {
		t.Fatal(err)
	}
	return lv
}

// TestNamedArrayItemClassification verifies the systematic rule that a named
// array item schema with no fields is a value alias and compiles to a
// primitive-array container, while an object-like item compiles to an
// array-of-object (ItemSchema). New value-shape types need no new branches
// here — the rule is structural, not per-type.
func TestNamedArrayItemClassification(t *testing.T) {
	strAlias := SchemaId("str_alias")
	strEnum := SchemaId("str_enum")
	objItem := SchemaId("obj_item")

	sc := &Schema{
		BaseSchema: BaseSchema{
			Fields: map[FieldId]Field{
				"f1": {Name: "stringArray", FieldProperties: FieldProperties{Type: FieldTypeArray, Schema: NewSchemaReference(SchemaReference{ID: strAlias})}},
				"f2": {Name: "stringEnumArray", FieldProperties: FieldProperties{Type: FieldTypeArray, Schema: NewSchemaReference(SchemaReference{ID: strEnum})}},
				"f3": {Name: "objectArray", FieldProperties: FieldProperties{Type: FieldTypeArray, Schema: NewSchemaReference(SchemaReference{ID: objItem})}},
			},
		},
		Schemas: map[SchemaId]NestedSchema{
			strAlias: {FieldProperties: FieldProperties{Type: FieldTypeString}},
			strEnum:  {FieldProperties: FieldProperties{Type: FieldTypeEnum}, Values: []LiteralValue{literal(t, "a"), literal(t, "b")}},
			objItem:  {BaseSchema: BaseSchema{Fields: map[FieldId]Field{"x": {Name: "x", FieldProperties: FieldProperties{Type: FieldTypeString}}}}},
		},
	}

	rs, err := Compile(sc)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := Link(rs)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]FieldDescriptor, len(cs.FieldsMeta))
	for i, meta := range cs.FieldsMeta {
		byName[meta.Name] = cs.Descriptors[i]
	}

	cases := []struct {
		name string
		dt   container.DataType
		term bool
	}{
		{"stringArray", container.TypeArrayString, true},
		{"stringEnumArray", container.TypeArrayString, true},
		{"objectArray", container.TypeArrayObject, false},
	}
	for _, tc := range cases {
		fd, ok := byName[tc.name]
		if !ok {
			t.Errorf("field %q: not found in compiled links", tc.name)
			continue
		}
		if got := fd.DataType(); got != tc.dt {
			t.Errorf("field %q: data type = %v, want %v", tc.name, got, tc.dt)
		}
		if got := fd.Terminal(); got != tc.term {
			t.Errorf("field %q: terminal = %v, want %v", tc.name, got, tc.term)
		}
	}
}

// TestNamedEnumArrayItemRetainsEnum ensures value-alias compilation keeps the
// enum lookup so per-item membership validation is not lost.
func TestNamedEnumArrayItemRetainsEnum(t *testing.T) {
	enumID := SchemaId("enum_alias")

	sc := &Schema{
		BaseSchema: BaseSchema{
			Fields: map[FieldId]Field{
				"f1": {Name: "enumArray", FieldProperties: FieldProperties{Type: FieldTypeArray, Schema: NewSchemaReference(SchemaReference{ID: enumID})}},
			},
		},
		Schemas: map[SchemaId]NestedSchema{
			enumID: {FieldProperties: FieldProperties{Type: FieldTypeEnum}, Values: []LiteralValue{literal(t, "a"), literal(t, "b")}},
		},
	}

	rs, err := Compile(sc)
	if err != nil {
		t.Fatal(err)
	}

	arr := rs.Fields[0].Container
	if arr.ItemSchema != nil {
		t.Error("enum array must not compile to a named item schema")
	}
	if arr.ItemEnum == nil {
		t.Fatal("enum array must retain ItemEnum for per-item membership validation")
	}
	if arr.ItemType != FieldTypeString {
		t.Errorf("enum array item type = %v, want string-base", arr.ItemType)
	}
	if arr.Record {
		t.Error("array compiled as record")
	}
	if _, ok := arr.ItemEnum.Lookup["a"]; !ok {
		t.Error("enum lookup missing value 'a'")
	}
}

// TestNamedEnumArrayValidation verifies the validator still enforces enum
// membership on value-array items.
func TestNamedEnumArrayValidation(t *testing.T) {
	enumID := SchemaId("enum_alias")

	sc := &Schema{
		BaseSchema: BaseSchema{
			Fields: map[FieldId]Field{
				"f1": {Name: "colorTags", Required: true, FieldProperties: FieldProperties{Type: FieldTypeArray, Schema: NewSchemaReference(SchemaReference{ID: enumID})}},
			},
		},
		Schemas: map[SchemaId]NestedSchema{
			enumID: {FieldProperties: FieldProperties{Type: FieldTypeEnum}, Values: []LiteralValue{literal(t, "red"), literal(t, "green")}},
		},
	}

	v, err := NewDocumentValidator(sc, nil)
	if err != nil {
		t.Fatal(err)
	}

	valid := map[string]any{"colorTags": []any{"red", "green", "red"}}
	if issues, _ := v.Validate(valid); len(issues) != 0 {
		t.Fatalf("valid enum array rejected: %+v", issues)
	}

	invalid := map[string]any{"colorTags": []any{"red", "blue"}}
	if issues, _ := v.Validate(invalid); len(issues) == 0 {
		t.Fatal("invalid enum member accepted")
	}
}
