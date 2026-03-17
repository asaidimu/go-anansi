package ir

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// validate_test.go tests Pass 1.5 (validate.go) and the TypeDecimal addition.
//
// Structure:
//   - Valid schemas pass Parse without error.
//   - Invalid schemas are rejected by Parse with PassValidate errors.
//   - TypeDecimal tests confirm descriptor encoding, storage mapping, enum
//     storage type, and round-trip serialization.

// ── Helpers ───────────────────────────────────────────────────────────────

// parseExpectError calls Parse on src and asserts that it fails with at least
// one PassValidate error. Returns all errors for further inspection.
func parseExpectError(t *testing.T, src []byte) []CompileError {
	t.Helper()
	_, err := Parse(src)
	if err == nil {
		t.Fatal("expected Parse to return an error, got nil")
	}
	errs := allErrors(err)
	for _, e := range errs {
		if e.Pass == PassValidate {
			return errs
		}
	}
	t.Fatalf("expected at least one PassValidate error, got: %v", errs)
	return nil
}

// hasValidateError returns true if errs contains a PassValidate error whose
// Message contains substr.
func hasValidateError(errs []CompileError, substr string) bool {
	for _, e := range errs {
		if e.Pass == PassValidate && contains(e.Message, substr) {
			return true
		}
	}
	return false
}

// contains reports whether s contains substr (stdlib strings not imported here
// to avoid a dependency; use a simple loop).
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ── Valid schemas pass Parse ───────────────────────────────────────────────

// TestValidate_ValidSchemasPass ensures all existing valid fixtures still pass
// Parse after the validation pass is inserted.
func TestValidate_ValidSchemasPass(t *testing.T) {
	fixtures := map[string][]byte{
		"flatSchema":         flatSchema,
		"nestedObjectSchema": nestedObjectSchema,
		"enumSchema":         enumSchema,
		"inlineEnumSchema":   inlineEnumSchema,
		"cycleSchema":        cycleSchema,
		"unionSchema":        unionSchema,
		"indexedSchema":      indexedSchema,
		"constrainedSchema":  constrainedSchema,
		"defaultSchema":      defaultSchema,
		"decimalSchema":      decimalSchema,
		"decimalEnumSchema":  decimalEnumSchema,
	}
	for name, src := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(src); err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// ── Field validation ───────────────────────────────────────────────────────

func TestValidate_FieldMissingName(t *testing.T) {
	errs := parseExpectError(t, invalidFieldMissingName)
	if !hasValidateError(errs, "name") {
		t.Errorf("expected error mentioning 'name', got: %v", errs)
	}
}

func TestValidate_FieldMissingType(t *testing.T) {
	errs := parseExpectError(t, invalidFieldMissingType)
	if !hasValidateError(errs, "type") {
		t.Errorf("expected error mentioning 'type', got: %v", errs)
	}
}

func TestValidate_FieldUnknownType(t *testing.T) {
	errs := parseExpectError(t, invalidUnknownFieldType)
	if !hasValidateError(errs, "bogustype") {
		t.Errorf("expected error mentioning 'bogustype', got: %v", errs)
	}
}

func TestValidate_ScalarFieldWithSchema(t *testing.T) {
	errs := parseExpectError(t, invalidScalarWithSchema)
	if !hasValidateError(errs, "scalar") {
		t.Errorf("expected error mentioning 'scalar', got: %v", errs)
	}
}

func TestValidate_ArrayFieldMissingSchema(t *testing.T) {
	errs := parseExpectError(t, invalidArrayMissingSchema)
	if !hasValidateError(errs, "schema reference") {
		t.Errorf("expected error mentioning 'schema reference', got: %v", errs)
	}
}

func TestValidate_UnionFieldRequiresArraySchema(t *testing.T) {
	errs := parseExpectError(t, invalidUnionWithSingleSchema)
	if !hasValidateError(errs, "array") {
		t.Errorf("expected error mentioning 'array', got: %v", errs)
	}
}

func TestValidate_ObjectFieldRejectsArraySchema(t *testing.T) {
	errs := parseExpectError(t, invalidSingleTypeWithArraySchema)
	if !hasValidateError(errs, "single schema ref") {
		t.Errorf("expected error mentioning 'single schema ref', got: %v", errs)
	}
}

func TestValidate_InlineSchemaRejectsStructuralType(t *testing.T) {
	errs := parseExpectError(t, invalidInlineStructuralType)
	if !hasValidateError(errs, "object") {
		t.Errorf("expected error mentioning 'object', got: %v", errs)
	}
}

func TestValidate_SchemaRefRejectsBothIdAndType(t *testing.T) {
	errs := parseExpectError(t, invalidSchemaRefBothIdAndType)
	if !hasValidateError(errs, "must not have both id and type") {
		t.Errorf("expected error mentioning 'must not have both id and type', got: %v", errs)
	}
}

func TestValidate_InlineRecordRejectsValues(t *testing.T) {
	errs := parseExpectError(t, invalidInlineRecordWithValues)
	if !hasValidateError(errs, "record") {
		t.Errorf("expected error mentioning 'record', got: %v", errs)
	}
}

func TestValidate_UniqueOnStructuralTypeIsError(t *testing.T) {
	errs := parseExpectError(t, invalidUniqueOnStructural)
	if !hasValidateError(errs, "unique") {
		t.Errorf("expected error mentioning 'unique', got: %v", errs)
	}
}

// unique:true on enum and record must NOT be an error.
func TestValidate_UniqueOnEnumIsAllowed(t *testing.T) {
	src := []byte(`{
  "name": "OK",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7030-b070-777e858c939a": {
      "name": "status",
      "type": "enum",
      "unique": true,
      "schema": { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" }
    }
  },
  "schemas": {
    "019ca000-0020-7020-a0a0-a7aeb5bcc3ca": {
      "name": "StatusEnum",
      "type": "enum",
      "values": ["a", "b"]
    }
  }
}`)
	if _, err := Parse(src); err != nil {
		t.Errorf("unique:true on enum should be allowed, got: %v", err)
	}
}

func TestValidate_UniqueOnRecordIsAllowed(t *testing.T) {
	src := []byte(`{
  "name": "OK",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "meta",
      "type": "record",
      "unique": true,
      "schema": { "type": "string" }
    }
  }
}`)
	if _, err := Parse(src); err != nil {
		t.Errorf("unique:true on record should be allowed, got: %v", err)
	}
}

// TestValidate_RecordFieldWithNoSchemaIsValid confirms that a record field
// without a schema reference is valid — record means untyped key-value map
// when schema is absent.
func TestValidate_RecordFieldWithNoSchemaIsValid(t *testing.T) {
	src := []byte(`{
  "name": "OK",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "meta",
      "type": "record"
    }
  }
}`)
	if _, err := Parse(src); err != nil {
		t.Errorf("record field with no schema should be valid, got: %v", err)
	}
}

// ── Nested schema validation ───────────────────────────────────────────────

func TestValidate_NestedSchemaMissingName(t *testing.T) {
	errs := parseExpectError(t, invalidNestedMissingName)
	if !hasValidateError(errs, "name") {
		t.Errorf("expected error mentioning 'name', got: %v", errs)
	}
}

func TestValidate_NestedSchemaBothForms(t *testing.T) {
	errs := parseExpectError(t, invalidNestedBothFormsAmbiguous)
	if !hasValidateError(errs, "both") {
		t.Errorf("expected error mentioning 'both', got: %v", errs)
	}
}

func TestValidate_NestedSchemaNeitherForm(t *testing.T) {
	errs := parseExpectError(t, invalidNestedNeitherForm)
	if !hasValidateError(errs, "either") {
		t.Errorf("expected error mentioning 'either', got: %v", errs)
	}
}

func TestValidate_NestedSchemaScalarTypeRejected(t *testing.T) {
	errs := parseExpectError(t, invalidNestedScalarType)
	if !hasValidateError(errs, "scalar") {
		t.Errorf("expected error mentioning 'scalar', got: %v", errs)
	}
}

func TestValidate_EnumSchemaMissingValues(t *testing.T) {
	errs := parseExpectError(t, invalidEnumSchemaMissingValues)
	if !hasValidateError(errs, "values") {
		t.Errorf("expected error mentioning 'values', got: %v", errs)
	}
}

func TestValidate_EnumSchemaWithSchemaRef(t *testing.T) {
	errs := parseExpectError(t, invalidEnumSchemaWithSchemaRef)
	if !hasValidateError(errs, "schema reference") {
		t.Errorf("expected error mentioning 'schema reference', got: %v", errs)
	}
}

func TestValidate_ArraySchemaMissingRef(t *testing.T) {
	errs := parseExpectError(t, invalidArraySchemaMissingRef)
	if !hasValidateError(errs, "schema reference") {
		t.Errorf("expected error mentioning 'schema reference', got: %v", errs)
	}
}

func TestValidate_UnionSchemaWithValues(t *testing.T) {
	errs := parseExpectError(t, invalidUnionSchemaWithValues)
	if !hasValidateError(errs, "values") {
		t.Errorf("expected error mentioning 'values', got: %v", errs)
	}
}

func TestValidate_UnionSchemaNotArray(t *testing.T) {
	errs := parseExpectError(t, invalidUnionSchemaNotArray)
	if !hasValidateError(errs, "array") {
		t.Errorf("expected error mentioning 'array', got: %v", errs)
	}
}

func TestValidate_ObjectSchemaWithValues(t *testing.T) {
	errs := parseExpectError(t, invalidObjectSchemaWithValues)
	if !hasValidateError(errs, "values") {
		t.Errorf("expected error mentioning 'values', got: %v", errs)
	}
}

func TestValidate_ObjectSchemaWithSchemaRef(t *testing.T) {
	errs := parseExpectError(t, invalidObjectSchemaWithSchemaRef)
	if !hasValidateError(errs, "schema reference") {
		t.Errorf("expected error mentioning 'schema reference', got: %v", errs)
	}
}

// ── Index validation ───────────────────────────────────────────────────────

func TestValidate_IndexMissingName(t *testing.T) {
	errs := parseExpectError(t, invalidIndexMissingName)
	if !hasValidateError(errs, "name") {
		t.Errorf("expected error mentioning 'name', got: %v", errs)
	}
}

func TestValidate_IndexMissingType(t *testing.T) {
	errs := parseExpectError(t, invalidIndexMissingType)
	if !hasValidateError(errs, "type") {
		t.Errorf("expected error mentioning 'type', got: %v", errs)
	}
}

func TestValidate_IndexBadType(t *testing.T) {
	errs := parseExpectError(t, invalidIndexBadType)
	if !hasValidateError(errs, "badtype") {
		t.Errorf("expected error mentioning 'badtype', got: %v", errs)
	}
}

func TestValidate_IndexMissingFields(t *testing.T) {
	errs := parseExpectError(t, invalidIndexMissingFields)
	if !hasValidateError(errs, "fields") {
		t.Errorf("expected error mentioning 'fields', got: %v", errs)
	}
}

func TestValidate_IndexBadOrder(t *testing.T) {
	errs := parseExpectError(t, invalidIndexBadOrder)
	if !hasValidateError(errs, "sideways") {
		t.Errorf("expected error mentioning 'sideways', got: %v", errs)
	}
}

func TestValidate_IndexConditionLeafMissingField(t *testing.T) {
	errs := parseExpectError(t, invalidConditionLeafMissingField)
	if !hasValidateError(errs, "field") {
		t.Errorf("expected error mentioning 'field', got: %v", errs)
	}
}

func TestValidate_IndexConditionLeafMissingOperator(t *testing.T) {
	errs := parseExpectError(t, invalidConditionLeafMissingOp)
	if !hasValidateError(errs, "operator") {
		t.Errorf("expected error mentioning 'operator', got: %v", errs)
	}
}

func TestValidate_IndexConditionLeafBadOperator(t *testing.T) {
	errs := parseExpectError(t, invalidConditionLeafBadOp)
	if !hasValidateError(errs, "like") {
		t.Errorf("expected error mentioning 'like', got: %v", errs)
	}
}

func TestValidate_IndexConditionLeafMissingValue(t *testing.T) {
	errs := parseExpectError(t, invalidConditionLeafMissingValue)
	if !hasValidateError(errs, "value") {
		t.Errorf("expected error mentioning 'value', got: %v", errs)
	}
}

func TestValidate_IndexConditionGroupMissingOperator(t *testing.T) {
	errs := parseExpectError(t, invalidConditionGroupMissingOp)
	if !hasValidateError(errs, "operator") {
		t.Errorf("expected error mentioning 'operator', got: %v", errs)
	}
}

func TestValidate_IndexConditionGroupBadOperator(t *testing.T) {
	errs := parseExpectError(t, invalidConditionGroupBadOp)
	if !hasValidateError(errs, "maybe") {
		t.Errorf("expected error mentioning 'maybe', got: %v", errs)
	}
}

func TestValidate_IndexConditionMixedLeafAndGroup(t *testing.T) {
	errs := parseExpectError(t, invalidConditionMixed)
	if !hasValidateError(errs, "mix") {
		t.Errorf("expected error mentioning 'mix', got: %v", errs)
	}
}

// ── Constraint validation ──────────────────────────────────────────────────

func TestValidate_ConstraintMissingName(t *testing.T) {
	errs := parseExpectError(t, invalidConstraintMissingName)
	if !hasValidateError(errs, "name") {
		t.Errorf("expected error mentioning 'name', got: %v", errs)
	}
}

func TestValidate_ConstraintNeitherForm(t *testing.T) {
	errs := parseExpectError(t, invalidConstraintNeitherForm)
	if !hasValidateError(errs, "either") {
		t.Errorf("expected error mentioning 'either', got: %v", errs)
	}
}

func TestValidate_ConstraintBothForms(t *testing.T) {
	errs := parseExpectError(t, invalidConstraintBothForms)
	if !hasValidateError(errs, "mix") {
		t.Errorf("expected error mentioning 'mix', got: %v", errs)
	}
}

func TestValidate_ConstraintGroupMissingOperator(t *testing.T) {
	errs := parseExpectError(t, invalidConstraintGroupMissingOp)
	if !hasValidateError(errs, "operator") {
		t.Errorf("expected error mentioning 'operator', got: %v", errs)
	}
}

func TestValidate_ConstraintGroupBadOperator(t *testing.T) {
	errs := parseExpectError(t, invalidConstraintGroupBadOp)
	if !hasValidateError(errs, "maybe") {
		t.Errorf("expected error mentioning 'maybe', got: %v", errs)
	}
}

func TestValidate_ConstraintGroupEmptyRules(t *testing.T) {
	errs := parseExpectError(t, invalidConstraintGroupEmptyRules)
	if !hasValidateError(errs, "rule") {
		t.Errorf("expected error mentioning 'rule', got: %v", errs)
	}
}

// ── UUID key validation ────────────────────────────────────────────────────

func TestValidate_NonUUIDFieldKey(t *testing.T) {
	errs := parseExpectError(t, invalidNonUUIDFieldKey)
	if !hasValidateError(errs, "not-a-uuid") {
		t.Errorf("expected error mentioning the bad key, got: %v", errs)
	}
}

func TestValidate_NonUUIDSchemaKey(t *testing.T) {
	errs := parseExpectError(t, invalidNonUUIDSchemaKey)
	if !hasValidateError(errs, "not-a-uuid") {
		t.Errorf("expected error mentioning the bad key, got: %v", errs)
	}
}

func TestValidate_NonUUIDIndexKey(t *testing.T) {
	errs := parseExpectError(t, invalidNonUUIDIndexKey)
	if !hasValidateError(errs, "not-a-uuid") {
		t.Errorf("expected error mentioning the bad key, got: %v", errs)
	}
}

func TestValidate_NonUUIDConstraintKey(t *testing.T) {
	errs := parseExpectError(t, invalidNonUUIDConstraintKey)
	if !hasValidateError(errs, "not-a-uuid") {
		t.Errorf("expected error mentioning the bad key, got: %v", errs)
	}
}

func TestValidate_UUIDWrongVersion(t *testing.T) {
	errs := parseExpectError(t, invalidUUIDWrongVersion)
	if !hasValidateError(errs, "UUID v7") {
		t.Errorf("expected error mentioning 'UUID v7', got: %v", errs)
	}
}

func TestValidate_UUIDWrongVariant(t *testing.T) {
	errs := parseExpectError(t, invalidUUIDWrongVariant)
	if !hasValidateError(errs, "UUID v7") {
		t.Errorf("expected error mentioning 'UUID v7', got: %v", errs)
	}
}

// PassValidate errors must come before PassSchemaIndex and later.
func TestValidate_ErrorPassIsValidate(t *testing.T) {
	_, err := Parse(invalidNonUUIDFieldKey)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, e := range allErrors(err) {
		if e.Pass != PassValidate {
			t.Errorf("expected PassValidate, got %v", e.Pass)
		}
	}
}

// ── TypeDecimal ────────────────────────────────────────────────────────────

func TestDecimal_FieldTypeIsTypeDecimal(t *testing.T) {
	cs := mustCompile(decimalSchema, nil)
	fd := findDescriptor(cs, 0, "amount")
	if fd == 0 {
		t.Fatal("amount descriptor not found")
	}
	if got := ExtractType(fd); got != TypeDecimal {
		t.Errorf("type: got %v, want TypeDecimal", got)
	}
}

func TestDecimal_MapsToDocumentTypeInt(t *testing.T) {
	// fieldTypeToDataType(TypeDecimal) must return document.TypeInt.
	if got := fieldTypeToDataType(TypeDecimal); got != document.TypeInt {
		t.Errorf("fieldTypeToDataType(TypeDecimal): got %v, want TypeInt", got)
	}
}

func TestDecimal_IsNotSchemaBearing(t *testing.T) {
	if TypeDecimal.IsSchemaBearing() {
		t.Error("TypeDecimal must not be schema-bearing")
	}
}

func TestDecimal_SerializesAsDecimal(t *testing.T) {
	cs := mustCompile(decimalSchema, nil)
	out, err := Serialize(cs)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if !contains(string(out), `"decimal"`) {
		t.Errorf("serialized output does not contain \"decimal\": %s", out)
	}
}

func TestDecimal_RoundTrip(t *testing.T) {
	cs := mustCompile(decimalSchema, nil)
	out, err := Serialize(cs)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	cs2 := mustCompile(out, nil)
	fd := findDescriptor(cs2, 0, "amount")
	if fd == 0 {
		t.Fatal("amount descriptor not found after round-trip")
	}
	if got := ExtractType(fd); got != TypeDecimal {
		t.Errorf("round-trip type: got %v, want TypeDecimal", got)
	}
}

func TestDecimal_EnumValuesStoredAsArrayInt(t *testing.T) {
	cs := mustCompile(decimalEnumSchema, nil)
	if cs.Store == nil {
		t.Fatal("Store should not be nil for schema with enum field")
	}

	tierFd := findDescriptor(cs, 0, "tier")
	if tierFd == 0 {
		t.Fatal("tier descriptor not found")
	}

	// Decimal enum values are stored as TypeArrayInt.
	id := int32((tierFd >> 8) & 0x7FFF)
	dp, err := document.NewDataPoint(document.TypeArrayInt, id)
	if err != nil {
		t.Fatalf("NewDataPoint: %v", err)
	}
	vals, ok, err := cs.Store.GetArrayInt(dp)
	if err != nil {
		t.Fatalf("GetArrayInt: %v", err)
	}
	if !ok {
		t.Fatal("decimal enum values not found in Store as TypeArrayInt")
	}
	if len(vals) != 3 {
		t.Errorf("enum values count: got %d, want 3", len(vals))
	}
	want := map[int64]bool{100: true, 200: true, 300: true}
	for _, v := range vals {
		if !want[v] {
			t.Errorf("unexpected enum value: %d", v)
		}
	}
}

func TestDecimal_OrdinalValue(t *testing.T) {
	// TypeDecimal must be 14 — one past TypeGeometry (13).
	// This test pins the bit layout so any future iota reorder is caught.
	if TypeDecimal != 14 {
		t.Errorf("TypeDecimal ordinal: got %d, want 14", TypeDecimal)
	}
}
