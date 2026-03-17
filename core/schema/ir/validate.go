package ir

import "encoding/hex"

// validate.go implements Pass 1.5: semantic validation of a parsed sourceSchema
// against the rules expressed in meta_schema.json. It runs after JSON
// unmarshalling (Pass 1) and before any IR construction (Pass 2+).
//
// validateSource returns all violations found. The compiler halts if any are
// returned — downstream passes assume a structurally valid source.

// inlinePrimitiveTypes is the set of types valid in an inline FieldSchema.type.
// Matches InlinePrimitiveTypeEnum in meta_schema.json.
var inlinePrimitiveTypes = map[string]bool{
	"unknown": true,
	"string":  true,
	"number":  true,
	"integer": true,
	"decimal": true,
	"boolean": true,
	"bytes":   true,
	"record":  true,
}

// validFieldTypes is the set of all valid values for Field.type and
// NestedSchemaType.type. Matches FieldTypeEnum in meta_schema.json.
var validFieldTypes = map[string]bool{
	"unknown":   true,
	"string":    true,
	"number":    true,
	"integer":   true,
	"decimal":   true,
	"boolean":   true,
	"bytes":     true,
	"array":     true,
	"set":       true,
	"enum":      true,
	"object":    true,
	"record":    true,
	"union":     true,
	"composite": true,
	"geometry":  true,
}

// scalarFieldTypes is the set of types that are scalar (non-schema-bearing)
// at the field level. Schema-bearing types require a schema reference.
var scalarFieldTypes = map[string]bool{
	"unknown":  true,
	"string":   true,
	"number":   true,
	"integer":  true,
	"decimal":  true,
	"boolean":  true,
	"bytes":    true,
	"geometry": true,
}

// uniqueInvalidTypes is the set of field types for which unique:true is an error.
// enum and record are excluded — unique on those is semantically valid.
var uniqueInvalidTypes = map[string]bool{
	"array":     true,
	"set":       true,
	"object":    true,
	"union":     true,
	"composite": true,
}

// singleSchemaTypes is the set of field/nested-schema types that require
// exactly one FieldSchema reference (not an array). record is excluded —
// a record without a schema is a valid untyped key-value map.
var singleSchemaTypes = map[string]bool{
	"array":  true,
	"set":    true,
	"object": true,
	"enum":   true,
}

// arraySchemaTypes is the set of field/nested-schema types that require
// an array of FieldSchema references.
var arraySchemaTypes = map[string]bool{
	"union":     true,
	"composite": true,
}

// validNestedTypeSchemaTypes is the set of types valid for a NestedSchemaType.
// Scalars are excluded — they cannot be named type schemas.
var validNestedTypeSchemaTypes = map[string]bool{
	"enum":      true,
	"array":     true,
	"set":       true,
	"record":    true,
	"union":     true,
	"composite": true,
}

// validIndexTypes matches IndexTypeEnum in meta_schema.json.
var validIndexTypes = map[string]bool{
	"normal":   true,
	"unique":   true,
	"primary":  true,
	"spatial":  true,
	"fulltext": true,
}

// validIndexOrders matches IndexOrderEnum in meta_schema.json.
var validIndexOrders = map[string]bool{
	"asc":  true,
	"desc": true,
}

// validLogicalOperators matches LogicalOperatorEnum in meta_schema.json.
var validLogicalOperators = map[string]bool{
	"and":  true,
	"or":   true,
	"not":  true,
	"nor":  true,
	"xor":  true,
	"nand": true,
	"xnor": true,
}

// validComparisonOperators matches ComparisonOperatorEnum in meta_schema.json.
var validComparisonOperators = map[string]bool{
	"eq":        true,
	"neq":       true,
	"lt":        true,
	"lte":       true,
	"gt":        true,
	"gte":       true,
	"in":        true,
	"nin":       true,
	"contains":  true,
	"ncontains": true,
	"exists":    true,
	"nexists":   true,
}

// validateSource runs all Pass 1.5 checks on a parsed sourceSchema.
func validateSource(s *sourceSchema) []CompileError {
	var errs []CompileError

	// ── Root schema maps: UUID key validation ─────────────────────────────────
	errs = append(errs, validateUUIDKeys("fields", s.Fields)...)
	errs = append(errs, validateUUIDKeys("schemas", s.Schemas)...)
	errs = append(errs, validateUUIDKeys("indexes", s.Indexes)...)
	errs = append(errs, validateUUIDKeys("constraints", s.Constraints)...)

	// ── Root fields ───────────────────────────────────────────────────────────
	for uuid, f := range s.Fields {
		errs = append(errs, validateField(uuid, f)...)
	}

	// ── Nested schemas ────────────────────────────────────────────────────────
	for uuid, ns := range s.Schemas {
		errs = append(errs, validateNestedSchema(uuid, ns)...)
	}

	// ── Root indexes ──────────────────────────────────────────────────────────
	for uuid, idx := range s.Indexes {
		errs = append(errs, validateIndex(uuid, idx)...)
	}

	// ── Root constraints ──────────────────────────────────────────────────────
	for uuid, c := range s.Constraints {
		errs = append(errs, validateConstraint(uuid, c)...)
	}

	return errs
}

// ── Field validation ──────────────────────────────────────────────────────────

func validateField(uuid string, f sourceField) []CompileError {
	var errs []CompileError

	errorf := func(msg string) {
		errs = append(errs, CompileError{
			Pass:      PassValidate,
			FieldUUID: uuid,
			Message:   msg,
		})
	}

	// name is required.
	if f.Name == "" {
		errorf("field missing required property: name")
	}

	// type is required and must be valid.
	if f.Type == "" {
		errorf("field missing required property: type")
		// Cannot validate schema/unique without a known type.
		return errs
	}
	if !validFieldTypes[f.Type] {
		errorf("field has invalid type: " + f.Type)
		return errs
	}

	// unique is invalid on structurally complex types.
	if f.Unique && uniqueInvalidTypes[f.Type] {
		errorf("unique:true is invalid for field of type " + f.Type)
	}

	// schema presence rules.
	if scalarFieldTypes[f.Type] {
		if f.Schema != nil {
			errorf("scalar field of type " + f.Type + " must not have a schema reference")
		}
	} else if f.Type == "record" {
		// record is an untyped key-value map when schema is absent; schema is
		// optional and, when present, types the values.
		if f.Schema != nil {
			errs = append(errs, validateSingleFieldSchema(uuid, f.Schema)...)
		}
	} else if singleSchemaTypes[f.Type] {
		if f.Schema == nil {
			errorf("field of type " + f.Type + " requires a schema reference")
		} else {
			errs = append(errs, validateSingleFieldSchema(uuid, f.Schema)...)
		}
	} else if arraySchemaTypes[f.Type] {
		if f.Schema == nil {
			errorf("field of type " + f.Type + " requires a schema reference array")
		} else {
			errs = append(errs, validateArrayFieldSchema(uuid, f.Schema)...)
		}
	}

	return errs
}

// validateSingleFieldSchema validates a single FieldSchema reference value.
func validateSingleFieldSchema(fieldUUID string, schema any) []CompileError {
	var errs []CompileError

	errorf := func(msg string) {
		errs = append(errs, CompileError{
			Pass:      PassValidate,
			FieldUUID: fieldUUID,
			Message:   msg,
		})
	}

	ref, errStr := parseFieldSchemaSingle(schema)
	if errStr != "" {
		errorf("invalid schema reference: " + errStr)
		return errs
	}

	// id and type are mutually exclusive.
	if ref.ID != "" && ref.Type != "" {
		errorf("schema reference must not have both id and type")
		return errs
	}

	// Named ref: id must look like a UUID (full v7 check happens at key level;
	// here we just ensure it's non-empty if present).
	if ref.ID != "" {
		if !isValidUUIDv7(ref.ID) {
			errorf("schema reference id is not a valid UUID v7: " + ref.ID)
		}
		return errs
	}

	// Inline ref: type must be a valid InlinePrimitiveTypeEnum value.
	if ref.Type != "" {
		if !inlinePrimitiveTypes[ref.Type] {
			errorf("inline schema reference has invalid type: " + ref.Type +
				" (structural types require a named schema)")
		}
		// values only valid on scalar primitives (not record).
		if len(ref.Values) > 0 && ref.Type == "record" {
			errorf("inline schema reference of type record must not have values")
		}
	}

	return errs
}

// validateArrayFieldSchema validates an array-of-FieldSchema reference value.
// Only named refs (id form) are valid in arrays — no inline schemas.
func validateArrayFieldSchema(fieldUUID string, schema any) []CompileError {
	var errs []CompileError

	errorf := func(msg string) {
		errs = append(errs, CompileError{
			Pass:      PassValidate,
			FieldUUID: fieldUUID,
			Message:   msg,
		})
	}

	refs, errStr := parseFieldSchemaArray(schema)
	if errStr != "" {
		errorf("invalid schema reference array: " + errStr)
		return errs
	}

	if len(refs) == 0 {
		errorf("schema reference array must not be empty")
		return errs
	}

	for i, ref := range refs {
		prefix := "schema reference [" + itoa(i) + "]: "
		if ref.ID == "" {
			errorf(prefix + "only named refs (id) are valid in union/composite schema arrays")
			continue
		}
		if ref.Type != "" {
			errorf(prefix + "inline schemas are not permitted in union/composite schema arrays")
		}
		if !isValidUUIDv7(ref.ID) {
			errorf(prefix + "id is not a valid UUID v7: " + ref.ID)
		}
	}

	return errs
}

// ── Nested schema validation ──────────────────────────────────────────────────

func validateNestedSchema(uuid string, ns sourceNestedSchema) []CompileError {
	var errs []CompileError

	errorf := func(msg string) {
		errs = append(errs, CompileError{
			Pass:       PassValidate,
			SchemaUUID: uuid,
			Message:    msg,
		})
	}

	if ns.Name == "" {
		errorf("nested schema missing required property: name")
	}

	hasFields := len(ns.Fields) > 0
	hasType := ns.Type != ""

	// Must be exactly one form — object schema or type schema.
	if hasFields && hasType {
		errorf("nested schema must not have both fields and type (object vs type schema ambiguity)")
		return errs
	}
	if !hasFields && !hasType {
		errorf("nested schema must have either fields (object schema) or type (type schema)")
		return errs
	}

	if hasFields {
		// Object schema.
		if len(ns.Values) > 0 {
			errorf("object schema must not have values")
		}
		if ns.Schema != nil {
			errorf("object schema must not have a schema reference")
		}
		errs = append(errs, validateUUIDKeys("fields", ns.Fields)...)
		errs = append(errs, validateUUIDKeys("indexes", ns.Indexes)...)
		for fieldUUID, f := range ns.Fields {
			errs = append(errs, validateField(fieldUUID, f)...)
		}
		for idxUUID, idx := range ns.Indexes {
			errs = append(errs, validateIndex(idxUUID, idx)...)
		}
		return errs
	}

	// Type schema.
	if !validNestedTypeSchemaTypes[ns.Type] {
		errorf("nested type schema has invalid type: " + ns.Type +
			" (scalar types are not valid named type schemas)")
		return errs
	}

	switch ns.Type {
	case "enum":
		if len(ns.Values) == 0 {
			errorf("enum type schema must have a non-empty values array")
		}
		if ns.Schema != nil {
			errorf("enum type schema must not have a schema reference")
		}

	case "array", "set":
		if ns.Schema == nil {
			errorf("type schema of type " + ns.Type + " requires a schema reference")
		} else {
			errs = append(errs, validateSingleFieldSchema(uuid, ns.Schema)...)
		}
		if len(ns.Values) > 0 {
			errorf("type schema of type " + ns.Type + " must not have values")
		}

	case "record":
		// schema is optional — absent means untyped values; present must be valid.
		if ns.Schema != nil {
			errs = append(errs, validateSingleFieldSchema(uuid, ns.Schema)...)
		}
		if len(ns.Values) > 0 {
			errorf("type schema of type record must not have values")
		}

	case "union", "composite":
		if ns.Schema == nil {
			errorf("type schema of type " + ns.Type + " requires a schema reference array")
		} else {
			errs = append(errs, validateArrayFieldSchema(uuid, ns.Schema)...)
		}
		if len(ns.Values) > 0 {
			errorf("type schema of type " + ns.Type + " must not have values")
		}
	}

	return errs
}

// ── Index validation ──────────────────────────────────────────────────────────

func validateIndex(uuid string, idx sourceIndex) []CompileError {
	var errs []CompileError

	errorf := func(msg string) {
		errs = append(errs, CompileError{
			Pass:    PassValidate,
			Message: "index " + uuid + ": " + msg,
		})
	}

	if idx.Name == "" {
		errorf("missing required property: name")
	}

	if idx.Type == "" {
		errorf("missing required property: type")
	} else if !validIndexTypes[idx.Type] {
		errorf("invalid index type: " + idx.Type)
	}

	if len(idx.Fields) == 0 {
		errorf("missing required property: fields (must be non-empty)")
	}

	if idx.Order != "" && !validIndexOrders[idx.Order] {
		errorf("invalid index order: " + idx.Order)
	}

	if idx.Condition != nil {
		errs = append(errs, validateIndexCondition(uuid, idx.Condition)...)
	}

	return errs
}

func validateIndexCondition(indexUUID string, cond *sourceIndexCondition) []CompileError {
	var errs []CompileError

	errorf := func(msg string) {
		errs = append(errs, CompileError{
			Pass:    PassValidate,
			Message: "index " + indexUUID + " condition: " + msg,
		})
	}

	isLeaf := cond.Field != "" || cond.Value != nil
	isGroup := len(cond.Conditions) > 0

	if isLeaf && isGroup {
		errorf("condition must not mix leaf fields (field/value) with group fields (conditions)")
		return errs
	}

	if isGroup {
		if cond.Operator == "" {
			errorf("condition group missing required property: operator")
		} else if !validLogicalOperators[cond.Operator] {
			errorf("condition group has invalid logical operator: " + cond.Operator)
		}
		for _, child := range cond.Conditions {
			errs = append(errs, validateIndexCondition(indexUUID, child)...)
		}
		return errs
	}

	// Leaf.
	if cond.Field == "" {
		errorf("condition leaf missing required property: field")
	}
	if cond.Operator == "" {
		errorf("condition leaf missing required property: operator")
	} else if !validComparisonOperators[cond.Operator] {
		errorf("condition leaf has invalid comparison operator: " + cond.Operator)
	}
	if cond.Value == nil {
		errorf("condition leaf missing required property: value")
	}

	return errs
}

// ── Constraint validation ─────────────────────────────────────────────────────

func validateConstraint(uuid string, c sourceConstraint) []CompileError {
	var errs []CompileError

	errorf := func(msg string) {
		errs = append(errs, CompileError{
			Pass:    PassValidate,
			Message: "constraint " + uuid + ": " + msg,
		})
	}

	if c.Name == "" {
		errorf("missing required property: name")
	}

	isLeaf := c.Predicate != ""
	isGroup := c.Operator != "" || len(c.Rules) > 0

	if isLeaf && isGroup {
		errorf("constraint must not mix leaf properties (predicate) with group properties (operator/rules)")
		return errs
	}

	if !isLeaf && !isGroup {
		errorf("constraint must have either predicate (leaf) or operator+rules (group)")
		return errs
	}

	if isGroup {
		if c.Operator == "" {
			errorf("constraint group missing required property: operator")
		} else if !validLogicalOperators[c.Operator] {
			errorf("constraint group has invalid logical operator: " + c.Operator)
		}
		if len(c.Rules) == 0 {
			errorf("constraint group must have at least one rule")
		}
		for i, rule := range c.Rules {
			if rule == nil {
				errorf("constraint group rule [" + itoa(i) + "] is nil")
				continue
			}
			errs = append(errs, validateConstraint(uuid+"/rule/"+itoa(i), *rule)...)
		}
	}

	return errs
}

// ── UUID key validation ───────────────────────────────────────────────────────

// validateUUIDKeys checks that all keys in a map are valid UUID v7 strings.
// T is unconstrained — only the keys are inspected.
func validateUUIDKeys[T any](mapName string, m map[string]T) []CompileError {
	var errs []CompileError
	for key := range m {
		if !isValidUUIDv7(key) {
			errs = append(errs, CompileError{
				Pass:    PassValidate,
				Message: mapName + " key is not a valid UUID v7: " + key,
			})
		}
	}
	return errs
}

// ── UUID v7 validation ────────────────────────────────────────────────────────

// isValidUUIDv7 returns true iff s is a well-formed UUID v7 string.
//
// UUID v7 format: xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx
//   - 8-4-4-4-12 hex groups separated by hyphens
//   - version nibble (first nibble of group 3) must be 7
//   - variant nibble (first nibble of group 4) must be 8, 9, a, or b
func isValidUUIDv7(s string) bool {
	// Length: 32 hex + 4 hyphens = 36.
	if len(s) != 36 {
		return false
	}

	// Hyphen positions: 8, 13, 18, 23.
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}

	// Decode all hex segments without allocating.
	// Segments: [0:8], [9:13], [14:18], [19:23], [24:36]
	var buf [16]byte
	segments := [5]string{s[0:8], s[9:13], s[14:18], s[19:23], s[24:36]}
	pos := 0
	for _, seg := range segments {
		decoded, err := hex.DecodeString(seg)
		if err != nil {
			return false
		}
		copy(buf[pos:], decoded)
		pos += len(decoded)
	}

	// Version nibble: high nibble of byte 6 must be 0x7.
	if (buf[6]>>4) != 0x7 {
		return false
	}

	// Variant nibble: high 2 bits of byte 8 must be 10xx (0x8–0xb).
	if (buf[8]>>6) != 0x2 {
		return false
	}

	return true
}
