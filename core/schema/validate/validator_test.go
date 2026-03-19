package validate_test

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
	validator "github.com/asaidimu/go-anansi/v6/core/schema/validate"
)

// =============================================================================
// SCHEMA FIXTURES
// =============================================================================

// personSchema: flat object with required and optional scalar fields.
const personSchema = `{
  "name": "Person",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "id",       "type": "string",  "required": true  },
    "01900000-0000-7000-8000-000000000002": { "name": "name",     "type": "string",  "required": true  },
    "01900000-0000-7000-8000-000000000003": { "name": "age",      "type": "integer", "required": false },
    "01900000-0000-7000-8000-000000000004": { "name": "score",    "type": "number",  "required": false },
    "01900000-0000-7000-8000-000000000005": { "name": "verified", "type": "boolean", "required": false }
  }
}`

// productSchema: flat object with a required string enum field.
const productSchema = `{
  "name": "Product",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "sku",    "type": "string",  "required": true },
    "01900000-0000-7000-8000-000000000002": { "name": "price",  "type": "number",  "required": true },
    "01900000-0000-7000-8000-000000000003": {
      "name": "status",
      "type": "enum",
      "required": true,
      "schema": { "values": ["draft", "published", "archived"] }
    },
    "01900000-0000-7000-8000-000000000004": { "name": "stock", "type": "integer", "required": false }
  }
}`

// addressSchema: root schema that embeds a required child object.
const addressSchema = `{
  "name": "User",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "username", "type": "string", "required": true },
    "01900000-0000-7000-8000-000000000010": {
      "name": "address",
      "type": "object",
      "required": true,
      "schema": { "id": "01900000-0000-7000-8000-0000000000A1" }
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000A1": {
      "name": "Address",
      "fields": {
        "01900000-0000-7000-8000-000000000020": { "name": "street",  "type": "string", "required": true  },
        "01900000-0000-7000-8000-000000000021": { "name": "city",    "type": "string", "required": true  },
        "01900000-0000-7000-8000-000000000022": { "name": "country", "type": "string", "required": false }
      }
    }
  }
}`

// rangeSchema: two required integers coupled by a start < end constraint.
const rangeSchema = `{
  "name": "Range",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "start", "type": "integer", "required": true },
    "01900000-0000-7000-8000-000000000002": { "name": "end",   "type": "integer", "required": true }
  },
  "constraints": {
    "01900000-0000-7000-8000-000000000201": {
      "name": "start_before_end",
      "predicate": "lt",
      "fields": ["start", "end"]
    }
  }
}`

// eventSchema: optional start/end with the same coupling constraint.
// Used to exercise constraint presence-check semantics across all modes.
const eventSchema = `{
  "name": "Event",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "title", "type": "string",  "required": true  },
    "01900000-0000-7000-8000-000000000002": { "name": "start", "type": "integer", "required": false },
    "01900000-0000-7000-8000-000000000003": { "name": "end",   "type": "integer", "required": false }
  },
  "constraints": {
    "01900000-0000-7000-8000-000000000201": {
      "name": "start_before_end",
      "predicate": "lt",
      "fields": ["start", "end"]
    }
  }
}`

// nodeSchema: self-referencing linked list (recursive schema).
const nodeSchema = `{
  "name": "Node",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "value", "type": "string", "required": true },
    "01900000-0000-7000-8000-000000000002": {
      "name": "next",
      "type": "object",
      "required": false,
      "schema": { "id": "01900000-0000-7000-8000-0000000000B1" }
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000B1": {
      "name": "Node",
      "fields": {
        "01900000-0000-7000-8000-000000000011": { "name": "value", "type": "string", "required": true },
        "01900000-0000-7000-8000-000000000012": {
          "name": "next",
          "type": "object",
          "required": false,
          "schema": { "id": "01900000-0000-7000-8000-0000000000B1" }
        }
      }
    }
  }
}`

// paymentSchema: union field — method is either a Card or a BankTransfer.
const paymentSchema = `{
  "name": "Payment",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "amount", "type": "number", "required": true },
    "01900000-0000-7000-8000-000000000002": {
      "name": "method",
      "type": "union",
      "required": true,
      "schema": [
        { "id": "01900000-0000-7000-8000-0000000000C1" },
        { "id": "01900000-0000-7000-8000-0000000000C2" }
      ]
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000C1": {
      "name": "Card",
      "fields": {
        "01900000-0000-7000-8000-000000000031": { "name": "card_number", "type": "string", "required": true },
        "01900000-0000-7000-8000-000000000032": { "name": "expiry",      "type": "string", "required": true }
      }
    },
    "01900000-0000-7000-8000-0000000000C2": {
      "name": "BankTransfer",
      "fields": {
        "01900000-0000-7000-8000-000000000041": { "name": "account_number", "type": "string", "required": true },
        "01900000-0000-7000-8000-000000000042": { "name": "routing_number", "type": "string", "required": true }
      }
    }
  }
}`

// orderSchema: root with a typed array of line items.
const orderSchema = `{
  "name": "Order",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "order_id", "type": "string", "required": true },
    "01900000-0000-7000-8000-000000000002": {
      "name": "items",
      "type": "array",
      "required": true,
      "schema": { "id": "01900000-0000-7000-8000-0000000000D1" }
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000D1": {
      "name": "LineItem",
      "fields": {
        "01900000-0000-7000-8000-000000000051": { "name": "product_id", "type": "string",  "required": true  },
        "01900000-0000-7000-8000-000000000052": { "name": "quantity",   "type": "integer", "required": true  },
        "01900000-0000-7000-8000-000000000053": { "name": "price",      "type": "number",  "required": false }
      }
    }
  }
}`

// =============================================================================
// HELPERS
// =============================================================================

// compile parses and compiles a JSON schema, failing the test on any error.
func compile(t *testing.T, src string, predicates ir.PredicateMap) *ir.Schema {
	t.Helper()
	parsed, err := ir.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cs, err := ir.Compile(parsed, predicates)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return cs
}

// key resolves a dot-separated path to a DocumentKey, failing on error.
func key(t *testing.T, cs *ir.Schema, path string) document.DocumentKey {
	t.Helper()
	dk, err := cs.DocumentKey(path)
	if err != nil {
		t.Fatalf("DocumentKey(%q): %v", path, err)
	}
	return dk
}

// mustNew builds a DocumentValidator, failing on error.
func mustNew(t *testing.T, cs *ir.Schema) *validator.DocumentValidator {
	t.Helper()
	v, err := validator.New(cs)
	if err != nil {
		t.Fatalf("validator.New: %v", err)
	}
	return v
}

// hasCode returns true if any issue carries the given code.
func hasCode(issues []common.Issue, code string) bool {
	for _, iss := range issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}

// codes returns all issue codes, for use in failure messages.
func codes(issues []common.Issue) []string {
	out := make([]string, len(issues))
	for i, iss := range issues {
		out[i] = iss.Code
	}
	return out
}

// ltPredicate: true iff fields[0] (int) < fields[1] (int).
var ltPredicate ir.Predicate = func(doc *document.Document, fields []document.DocumentKey, _ any) bool {
	a, _, _ := doc.GetInt(fields[0])
	b, _, _ := doc.GetInt(fields[1])
	return a < b
}

var ltPredicates = ir.PredicateMap{"lt": ltPredicate}

// set populates a Document field from a Go value, dispatching on the
// DocumentKey's DataType. Fails the test for unsupported types.
func set(t *testing.T, doc *document.Document, dk document.DocumentKey, val any) {
	t.Helper()
	switch dk.Type() {
	case document.TypeString:
		s, ok := val.(string)
		if !ok {
			t.Fatalf("set: TypeString key requires string, got %T", val)
		}
		if err := doc.SetString(dk, s); err != nil {
			t.Fatalf("SetString: %v", err)
		}
	case document.TypeInt:
		var n int64
		switch v := val.(type) {
		case int:
			n = int64(v)
		case int64:
			n = v
		default:
			t.Fatalf("set: TypeInt key requires int/int64, got %T", val)
		}
		if err := doc.SetInt(dk, n); err != nil {
			t.Fatalf("SetInt: %v", err)
		}
	case document.TypeFloat:
		f, ok := val.(float64)
		if !ok {
			t.Fatalf("set: TypeFloat key requires float64, got %T", val)
		}
		if err := doc.SetFloat(dk, f); err != nil {
			t.Fatalf("SetFloat: %v", err)
		}
	case document.TypeBool:
		b, ok := val.(bool)
		if !ok {
			t.Fatalf("set: TypeBool key requires bool, got %T", val)
		}
		if err := doc.SetBool(dk, b); err != nil {
			t.Fatalf("SetBool: %v", err)
		}
	case document.TypeRecord:
		nested, ok := val.(*document.Document)
		if !ok {
			t.Fatalf("set: TypeRecord key requires *document.Document, got %T", val)
		}
		if err := doc.SetRecord(dk, map[string]*document.Document{"": nested}); err != nil {
			t.Fatalf("SetRecord: %v", err)
		}
	case document.TypeArrayObject:
		items, ok := val.([]*document.Document)
		if !ok {
			t.Fatalf("set: TypeArrayObject key requires []*document.Document, got %T", val)
		}
		if err := doc.SetArrayObject(dk, items); err != nil {
			t.Fatalf("SetArrayObject: %v", err)
		}
	default:
		t.Fatalf("set: unhandled DataType %v", dk.Type())
	}
}

// build constructs a Document from a map of path → value pairs.
func build(t *testing.T, cs *ir.Schema, fields map[string]any) *document.Document {
	t.Helper()
	doc := document.NewDocument()
	for path, val := range fields {
		set(t, doc, key(t, cs, path), val)
	}
	return doc
}

// =============================================================================
// COMPILATION GUARD
// All schema fixtures must compile cleanly and produce a valid graph.
// This is the first thing that runs and catches malformed fixture JSON.
// =============================================================================

func TestCompile_AllFixtures(t *testing.T) {
	fixtures := []struct {
		name       string
		src        string
		predicates ir.PredicateMap
	}{
		{"person", personSchema, nil},
		{"product", productSchema, nil},
		{"address", addressSchema, nil},
		{"range", rangeSchema, ltPredicates},
		{"event", eventSchema, ltPredicates},
		{"node", nodeSchema, nil},
		{"payment", paymentSchema, nil},
		{"order", orderSchema, nil},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			cs := compile(t, f.src, f.predicates)
			if _, err := validator.New(cs); err != nil {
				t.Fatalf("validator.New: %v", err)
			}
		})
	}
}

// =============================================================================
// REQUIRED FIELD TESTS  (personSchema)
// =============================================================================

func TestRequired_AllPresent_Passes(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"id": "u1", "name": "Alice"})
	if _, ok := v.Validate(doc); !ok {
		t.Fatal("all required fields present: expected pass")
	}
}

func TestRequired_Strict_IdMissing(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"name": "Alice"})
	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("missing required id: expected failure")
	}
	if !hasCode(issues, "REQUIRED_FIELD_MISSING") {
		t.Errorf("expected REQUIRED_FIELD_MISSING, got %v", codes(issues))
	}
}

func TestRequired_Strict_BothMissing_ReportsBoth(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	issues, ok := v.Validate(document.NewDocument())
	if ok {
		t.Fatal("both required fields missing: expected failure")
	}
	count := 0
	for _, iss := range issues {
		if iss.Code == "REQUIRED_FIELD_MISSING" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2×REQUIRED_FIELD_MISSING, got %d: %v", count, codes(issues))
	}
}

func TestRequired_PartialStrict_RequiredMissing_Passes(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	// Only optional field set — PartialStrict skips required checks.
	doc := build(t, cs, map[string]any{"age": 30})
	if issues, ok := v.ValidatePartial(doc); !ok {
		t.Fatalf("PartialStrict missing required fields: expected pass, got %v", codes(issues))
	}
}

func TestRequired_Loose_EmptyDocument_Passes(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	if _, ok := v.ValidateLoose(document.NewDocument()); !ok {
		t.Fatal("Loose empty document: expected pass")
	}
}

func TestRequired_OptionalFieldsAbsent_Passes(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"id": "u1", "name": "Bob"})
	if _, ok := v.Validate(doc); !ok {
		t.Fatal("absent optional fields: expected pass")
	}
}

func TestRequired_NullValue_FailsRequiredCheck(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	doc := document.NewDocument()
	doc.SetNull(key(t, cs, "id")) // explicitly null ≠ HasValue

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("null required field: expected failure")
	}
	if !hasCode(issues, "REQUIRED_FIELD_MISSING") {
		t.Errorf("expected REQUIRED_FIELD_MISSING for null required field, got %v", codes(issues))
	}
}

// =============================================================================
// ENUM VALIDATION TESTS  (productSchema)
// =============================================================================

func TestEnum_AllValidValues_Pass(t *testing.T) {
	cs := compile(t, productSchema, nil)
	v := mustNew(t, cs)

	for _, status := range []string{"draft", "published", "archived"} {
		doc := build(t, cs, map[string]any{
			"sku":    "SKU-001",
			"price":  9.99,
			"status": status,
		})
		if issues, ok := v.Validate(doc); !ok {
			t.Errorf("status=%q: expected pass, got %v", status, codes(issues))
		}
	}
}

func TestEnum_InvalidValue_Fails(t *testing.T) {
	cs := compile(t, productSchema, nil)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{
		"sku":    "SKU-001",
		"price":  9.99,
		"status": "discontinued",
	})
	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("invalid enum value: expected failure")
	}
	if !hasCode(issues, "ENUM_VIOLATION") {
		t.Errorf("expected ENUM_VIOLATION, got %v", codes(issues))
	}
}

func TestEnum_RequiredMissing_Fails(t *testing.T) {
	cs := compile(t, productSchema, nil)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"sku": "SKU-001", "price": 9.99})
	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("missing required enum field: expected failure")
	}
	if !hasCode(issues, "REQUIRED_FIELD_MISSING") {
		t.Errorf("expected REQUIRED_FIELD_MISSING, got %v", codes(issues))
	}
}

// =============================================================================
// NESTED OBJECT TESTS  (addressSchema)
// =============================================================================

func TestNested_FullyPopulated_Passes(t *testing.T) {
	cs := compile(t, addressSchema, nil)
	v := mustNew(t, cs)

	// The sub-document for 'address' uses full paths — the address space
	// resolves them to keys in the Address sub-schema.
	addrDoc := build(t, cs, map[string]any{
		"address.street":  "123 Main St",
		"address.city":    "Nairobi",
		"address.country": "Kenya",
	})

	doc := document.NewDocument()
	set(t, doc, key(t, cs, "username"), "alice")
	set(t, doc, key(t, cs, "address"), addrDoc)

	if _, ok := v.Validate(doc); !ok {
		t.Fatal("fully populated nested object: expected pass")
	}
}

func TestNested_SubField_RequiredMissing_Fails(t *testing.T) {
	cs := compile(t, addressSchema, nil)
	v := mustNew(t, cs)

	// city is required in Address but not set.
	addrDoc := build(t, cs, map[string]any{
		"address.street": "123 Main St",
	})

	doc := document.NewDocument()
	set(t, doc, key(t, cs, "username"), "alice")
	set(t, doc, key(t, cs, "address"), addrDoc)

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("nested sub-field city missing: expected failure")
	}
	if !hasCode(issues, "REQUIRED_FIELD_MISSING") {
		t.Errorf("expected REQUIRED_FIELD_MISSING for address.city, got %v", codes(issues))
	}
}

func TestNested_ParentObjectMissing_Fails(t *testing.T) {
	cs := compile(t, addressSchema, nil)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"username": "alice"})
	// address entirely absent

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("required parent object missing: expected failure")
	}
	if !hasCode(issues, "REQUIRED_FIELD_MISSING") {
		t.Errorf("expected REQUIRED_FIELD_MISSING for address, got %v", codes(issues))
	}
}

func TestNested_OptionalSubField_Absent_Passes(t *testing.T) {
	cs := compile(t, addressSchema, nil)
	v := mustNew(t, cs)

	// country is optional — omitting it is fine.
	addrDoc := build(t, cs, map[string]any{
		"address.street": "456 Other St",
		"address.city":   "Mombasa",
	})

	doc := document.NewDocument()
	set(t, doc, key(t, cs, "username"), "bob")
	set(t, doc, key(t, cs, "address"), addrDoc)

	if _, ok := v.Validate(doc); !ok {
		t.Fatal("optional sub-field absent: expected pass")
	}
}

// =============================================================================
// CONSTRAINT TESTS  (rangeSchema — required fields)
// =============================================================================

func TestConstraint_StartLtEnd_Passes(t *testing.T) {
	cs := compile(t, rangeSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"start": 10, "end": 20})
	if _, ok := v.Validate(doc); !ok {
		t.Fatal("start < end: expected pass")
	}
}

func TestConstraint_StartGtEnd_Fails(t *testing.T) {
	cs := compile(t, rangeSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"start": 20, "end": 10})
	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("start > end: expected failure")
	}
	if !hasCode(issues, "CONSTRAINT_VIOLATION") {
		t.Errorf("expected CONSTRAINT_VIOLATION, got %v", codes(issues))
	}
}

func TestConstraint_Equal_Fails(t *testing.T) {
	cs := compile(t, rangeSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"start": 10, "end": 10})
	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("start == end (strict lt): expected failure")
	}
	if !hasCode(issues, "CONSTRAINT_VIOLATION") {
		t.Errorf("expected CONSTRAINT_VIOLATION, got %v", codes(issues))
	}
}

// =============================================================================
// CONSTRAINT PRESENCE SEMANTICS  (eventSchema — optional coupled fields)
// =============================================================================

func TestConstraint_NonePresent_Strict_Incomplete(t *testing.T) {
	cs := compile(t, eventSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"title": "Conf"})
	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("Strict: no constraint fields present — expected CONSTRAINT_INCOMPLETE")
	}
	if !hasCode(issues, "CONSTRAINT_INCOMPLETE") {
		t.Errorf("expected CONSTRAINT_INCOMPLETE, got %v", codes(issues))
	}
}

func TestConstraint_NonePresent_PartialStrict_Skipped(t *testing.T) {
	cs := compile(t, eventSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"title": "Conf"})
	if issues, ok := v.ValidatePartial(doc); !ok {
		t.Fatalf("PartialStrict: no constraint fields — expected skip, got %v", codes(issues))
	}
}

func TestConstraint_NonePresent_Loose_Skipped(t *testing.T) {
	cs := compile(t, eventSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"title": "Conf"})
	if _, ok := v.ValidateLoose(doc); !ok {
		t.Fatal("Loose: no constraint fields — expected skip")
	}
}

func TestConstraint_PartialPresent_Strict_Incomplete(t *testing.T) {
	cs := compile(t, eventSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"title": "Conf", "start": 100})
	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("Strict: only one coupled field — expected CONSTRAINT_INCOMPLETE")
	}
	if !hasCode(issues, "CONSTRAINT_INCOMPLETE") {
		t.Errorf("expected CONSTRAINT_INCOMPLETE, got %v", codes(issues))
	}
}

func TestConstraint_PartialPresent_PartialStrict_PartialUpdate(t *testing.T) {
	cs := compile(t, eventSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"title": "Conf", "start": 100})
	issues, ok := v.ValidatePartial(doc)
	if ok {
		t.Fatal("PartialStrict: partial field presence — expected CONSTRAINT_PARTIAL_UPDATE")
	}
	if !hasCode(issues, "CONSTRAINT_PARTIAL_UPDATE") {
		t.Errorf("expected CONSTRAINT_PARTIAL_UPDATE, got %v", codes(issues))
	}
}

func TestConstraint_PartialPresent_Loose_Skipped(t *testing.T) {
	cs := compile(t, eventSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"title": "Conf", "start": 100})
	if _, ok := v.ValidateLoose(doc); !ok {
		t.Fatal("Loose: partial field presence — expected skip")
	}
}

func TestConstraint_BothPresent_Valid_Passes(t *testing.T) {
	cs := compile(t, eventSchema, ltPredicates)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"title": "Conf", "start": 100, "end": 200})
	if _, ok := v.Validate(doc); !ok {
		t.Fatal("both fields present and valid: expected pass")
	}
}

// =============================================================================
// DEPENDENCY SKIP
// A failing required node must prevent the constraint predicate from running.
// =============================================================================

func TestDependency_RequiredFails_ConstraintNotCalled(t *testing.T) {
	predicateCalled := false
	preds := ir.PredicateMap{
		"lt": func(_ *document.Document, _ []document.DocumentKey, _ any) bool {
			predicateCalled = true
			return true
		},
	}
	cs := compile(t, rangeSchema, preds)
	v := mustNew(t, cs)

	// Both required fields absent — required nodes fail,
	// constraint node must be skipped entirely.
	v.Validate(document.NewDocument())

	if predicateCalled {
		t.Fatal("constraint predicate must not be called when required fields are missing")
	}
}

// =============================================================================
// UNION VALIDATION TESTS  (paymentSchema)
// =============================================================================

func TestUnion_CardVariant_Passes(t *testing.T) {
	cs := compile(t, paymentSchema, nil)
	v := mustNew(t, cs)

	cardDoc := build(t, cs, map[string]any{
		"method.card_number": "4111111111111111",
		"method.expiry":      "12/26",
	})
	doc := document.NewDocument()
	set(t, doc, key(t, cs, "amount"), 99.0)
	set(t, doc, key(t, cs, "method"), cardDoc)

	if _, ok := v.Validate(doc); !ok {
		t.Fatal("card union variant: expected pass")
	}
}

func TestUnion_BankVariant_Passes(t *testing.T) {
	cs := compile(t, paymentSchema, nil)
	v := mustNew(t, cs)

	bankDoc := build(t, cs, map[string]any{
		"method.account_number": "1234567890",
		"method.routing_number": "021000021",
	})
	doc := document.NewDocument()
	set(t, doc, key(t, cs, "amount"), 500.0)
	set(t, doc, key(t, cs, "method"), bankDoc)

	if _, ok := v.Validate(doc); !ok {
		t.Fatal("bank transfer union variant: expected pass")
	}
}

func TestUnion_NoVariantMatches_Fails(t *testing.T) {
	cs := compile(t, paymentSchema, nil)
	v := mustNew(t, cs)

	emptyDoc := document.NewDocument()
	doc := document.NewDocument()
	set(t, doc, key(t, cs, "amount"), 10.0)
	set(t, doc, key(t, cs, "method"), emptyDoc)

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("no union variant matches: expected failure")
	}
	if !hasCode(issues, "UNION_MISMATCH") {
		t.Errorf("expected UNION_MISMATCH, got %v", codes(issues))
	}
}

func TestUnion_CardVariant_MissingRequiredField_Fails(t *testing.T) {
	cs := compile(t, paymentSchema, nil)
	v := mustNew(t, cs)

	// card_number present but expiry missing — neither variant fully matches.
	incompleteCard := build(t, cs, map[string]any{
		"method.card_number": "4111111111111111",
	})
	doc := document.NewDocument()
	set(t, doc, key(t, cs, "amount"), 50.0)
	set(t, doc, key(t, cs, "method"), incompleteCard)

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("incomplete card variant: expected failure")
	}
	if !hasCode(issues, "UNION_MISMATCH") {
		t.Errorf("expected UNION_MISMATCH, got %v", codes(issues))
	}
}

// =============================================================================
// ARRAY VALIDATION TESTS  (orderSchema)
// =============================================================================

func TestArray_ValidItems_Passes(t *testing.T) {
	cs := compile(t, orderSchema, nil)
	v := mustNew(t, cs)

	item1 := build(t, cs, map[string]any{
		"items.product_id": "P1",
		"items.quantity":   int64(2),
		"items.price":      9.99,
	})
	item2 := build(t, cs, map[string]any{
		"items.product_id": "P2",
		"items.quantity":   int64(1),
	})

	doc := document.NewDocument()
	set(t, doc, key(t, cs, "order_id"), "ORD-001")
	set(t, doc, key(t, cs, "items"), []*document.Document{item1, item2})

	if _, ok := v.Validate(doc); !ok {
		t.Fatal("valid array items: expected pass")
	}
}

func TestArray_ItemMissingRequired_Fails(t *testing.T) {
	cs := compile(t, orderSchema, nil)
	v := mustNew(t, cs)

	badItem := build(t, cs, map[string]any{
		"items.quantity": int64(1),
		// product_id missing
	})

	doc := document.NewDocument()
	set(t, doc, key(t, cs, "order_id"), "ORD-002")
	set(t, doc, key(t, cs, "items"), []*document.Document{badItem})

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("array item missing required field: expected failure")
	}
	if !hasCode(issues, "REQUIRED_FIELD_MISSING") {
		t.Errorf("expected REQUIRED_FIELD_MISSING in array item, got %v", codes(issues))
	}
}

func TestArray_EmptySlice_Passes(t *testing.T) {
	cs := compile(t, orderSchema, nil)
	v := mustNew(t, cs)

	doc := document.NewDocument()
	set(t, doc, key(t, cs, "order_id"), "ORD-003")
	set(t, doc, key(t, cs, "items"), []*document.Document{})

	if _, ok := v.Validate(doc); !ok {
		t.Fatal("empty array: expected pass")
	}
}

func TestArray_MultipleItemErrors_AllReported(t *testing.T) {
	cs := compile(t, orderSchema, nil)
	v := mustNew(t, cs)

	// Two bad items — both missing required fields.
	bad1 := build(t, cs, map[string]any{"items.quantity": int64(1)})
	bad2 := build(t, cs, map[string]any{"items.quantity": int64(2)})

	doc := document.NewDocument()
	set(t, doc, key(t, cs, "order_id"), "ORD-004")
	set(t, doc, key(t, cs, "items"), []*document.Document{bad1, bad2})

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("two bad array items: expected failure")
	}
	count := 0
	for _, iss := range issues {
		if iss.Code == "REQUIRED_FIELD_MISSING" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2×REQUIRED_FIELD_MISSING across two items, got %d: %v", count, codes(issues))
	}
}

// =============================================================================
// RECURSIVE SCHEMA TESTS  (nodeSchema)
// =============================================================================

func TestRecursive_SingleNode_Passes(t *testing.T) {
	cs := compile(t, nodeSchema, nil)
	v := mustNew(t, cs)

	doc := build(t, cs, map[string]any{"value": "head"})
	if _, ok := v.Validate(doc); !ok {
		t.Fatal("single recursive node: expected pass")
	}
}

func TestRecursive_TwoNodes_Passes(t *testing.T) {
	cs := compile(t, nodeSchema, nil)
	v := mustNew(t, cs)

	tail := build(t, cs, map[string]any{"next.value": "tail"})

	head := document.NewDocument()
	set(t, head, key(t, cs, "value"), "head")
	set(t, head, key(t, cs, "next"), tail)

	if _, ok := v.Validate(head); !ok {
		t.Fatal("two-node linked list: expected pass")
	}
}

func TestRecursive_InnerNodeMissingRequired_Fails(t *testing.T) {
	cs := compile(t, nodeSchema, nil)
	v := mustNew(t, cs)

	// inner node has no 'value' field set
	inner := document.NewDocument()

	head := document.NewDocument()
	set(t, head, key(t, cs, "value"), "head")
	set(t, head, key(t, cs, "next"), inner)

	issues, ok := v.Validate(head)
	if ok {
		t.Fatal("inner node missing required field: expected failure")
	}
	if !hasCode(issues, "REQUIRED_FIELD_MISSING") {
		t.Errorf("expected REQUIRED_FIELD_MISSING in inner node, got %v", codes(issues))
	}
}

// =============================================================================
// UNEXPECTED FIELD TESTS  (personSchema)
// =============================================================================

func TestUnexpected_Strict_ExtraField_Fails(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	// Manufacture a key whose fieldIndex (99) is out of range for personSchema.
	ghostFD := uint32(ir.TypeString)<<1 | ir.FDMaskTerminal | (uint32(99) << 8)
	dp, _ := document.NewDataPoint(document.TypeString, 999)
	ghostKey := document.NewDocumentKey(dp, ghostFD)

	doc := build(t, cs, map[string]any{"id": "u1", "name": "Alice"})
	doc.SetString(ghostKey, "ghost")

	issues, ok := v.Validate(doc)
	if ok {
		t.Fatal("extra field in Strict mode: expected failure")
	}
	if !hasCode(issues, "UNEXPECTED_FIELD") {
		t.Errorf("expected UNEXPECTED_FIELD, got %v", codes(issues))
	}
}

func TestUnexpected_Loose_ExtraField_Passes(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	ghostFD := uint32(ir.TypeString)<<1 | ir.FDMaskTerminal | (uint32(99) << 8)
	dp, _ := document.NewDataPoint(document.TypeString, 999)
	ghostKey := document.NewDocumentKey(dp, ghostFD)

	doc := build(t, cs, map[string]any{"id": "u1", "name": "Alice"})
	doc.SetString(ghostKey, "ghost")

	if _, ok := v.ValidateLoose(doc); !ok {
		t.Fatal("extra field in Loose mode: expected pass")
	}
}

func TestUnexpected_PartialStrict_ExtraField_Fails(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	ghostFD := uint32(ir.TypeString)<<1 | ir.FDMaskTerminal | (uint32(99) << 8)
	dp, _ := document.NewDataPoint(document.TypeString, 999)
	ghostKey := document.NewDocumentKey(dp, ghostFD)

	doc := build(t, cs, map[string]any{"id": "u1", "name": "Alice"})
	doc.SetString(ghostKey, "ghost")

	issues, ok := v.ValidatePartial(doc)
	if ok {
		t.Fatal("extra field in PartialStrict mode: expected failure")
	}
	if !hasCode(issues, "UNEXPECTED_FIELD") {
		t.Errorf("expected UNEXPECTED_FIELD, got %v", codes(issues))
	}
}

// =============================================================================
// CONCURRENT VALIDATION
// =============================================================================

func TestConcurrent_SharedValidator(t *testing.T) {
	cs := compile(t, personSchema, nil)
	v := mustNew(t, cs)

	const workers = 64
	errc := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func(i int) {
			doc := build(t, cs, map[string]any{
				"id":   fmt.Sprintf("u%d", i),
				"name": fmt.Sprintf("User%d", i),
				"age":  i,
			})
			if _, ok := v.Validate(doc); !ok {
				errc <- fmt.Errorf("worker %d: unexpected failure", i)
				return
			}
			errc <- nil
		}(i)
	}

	for i := 0; i < workers; i++ {
		if err := <-errc; err != nil {
			t.Error(err)
		}
	}
}
