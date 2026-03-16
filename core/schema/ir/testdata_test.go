package ir

// testdata_test.go provides shared JSON fixtures and helper functions used
// across all test files. All fixtures are valid according to meta_schema.json
// unless the name indicates otherwise (prefix "invalid").

// ── Field UUID constants ───────────────────────────────────────────────────
// UUIDs are real UUID v7 values. Lexicographic sort determines field_index.
// Within each fixture the UUIDs are chosen so the sort order is explicit.

const (
	// flat schema field UUIDs — lex order: nameUUID < descUUID < versionUUID
	flatNameUUID    = "019ca000-0001-7000-0000-000000000001"
	flatDescUUID    = "019ca000-0002-7000-0000-000000000002"
	flatVersionUUID = "019ca000-0003-7000-0000-000000000003"

	// nested schema UUID
	nestedAddressSchemaUUID = "019ca000-0010-7000-0000-000000000010"

	// object schema field UUIDs
	objStreetUUID = "019ca000-0011-7000-0000-000000000011"
	objCityUUID   = "019ca000-0012-7000-0000-000000000012"

	// enum schema UUID
	enumStatusSchemaUUID = "019ca000-0020-7000-0000-000000000020"

	// root field pointing at enum schema
	rootStatusFieldUUID = "019ca000-0030-7000-0000-000000000030"

	// union/composite schema UUIDs
	unionVariantAUUID = "019ca000-0040-7000-0000-000000000040"
	unionVariantBUUID = "019ca000-0041-7000-0000-000000000041"
	unionSchemaUUID   = "019ca000-0042-7000-0000-000000000042"
)

// ── Fixtures ───────────────────────────────────────────────────────────────

// flatSchema is the minimal valid schema: one root object with three scalar
// fields and no nested schemas, indexes, or constraints.
var flatSchema = []byte(`{
  "name": "Flat",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "name",    "type": "string",  "required": true },
    "019ca000-0002-7000-0000-000000000002": { "name": "desc",    "type": "string" },
    "019ca000-0003-7000-0000-000000000003": { "name": "version", "type": "string",  "required": true }
  }
}`)

// nestedObjectSchema has a root schema with a field pointing at a nested
// object schema (Address). Tests descriptor target_schema resolution and
// SchemaOffsets for two schema indices.
var nestedObjectSchema = []byte(`{
  "name": "Person",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7000-0000-000000000030": {
      "name": "address",
      "type": "object",
      "required": true,
      "schema": { "id": "019ca000-0010-7000-0000-000000000010" }
    }
  },
  "schemas": {
    "019ca000-0010-7000-0000-000000000010": {
      "name": "Address",
      "fields": {
        "019ca000-0011-7000-0000-000000000011": { "name": "street", "type": "string", "required": true },
        "019ca000-0012-7000-0000-000000000012": { "name": "city",   "type": "string", "required": true }
      }
    }
  }
}`)

// enumSchema has a root field of type enum pointing at a named enum schema
// with string values. Tests Store population for named enum refs.
var enumSchema = []byte(`{
  "name": "Order",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7000-0000-000000000030": {
      "name": "status",
      "type": "enum",
      "schema": { "id": "019ca000-0020-7000-0000-000000000020" }
    }
  },
  "schemas": {
    "019ca000-0020-7000-0000-000000000020": {
      "name": "StatusEnum",
      "type": "enum",
      "values": ["pending", "active", "closed"]
    }
  }
}`)

// inlineEnumSchema has a root field of type enum with values inline in the
// field's schema ref rather than in a named schema.
var inlineEnumSchema = []byte(`{
  "name": "Task",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7000-0000-000000000030": {
      "name": "priority",
      "type": "enum",
      "schema": { "type": "string", "values": ["low", "medium", "high"] }
    }
  }
}`)

// cycleSchema has a root with a field pointing to NodeSchema, which has a
// field pointing back to itself (self-reference). This is the simplest
// cycle expressible with named schemas. Tests that cycle detection sets
// terminal=0 on the back-edge field and terminal=1 on acyclic fields.
//
// Graph: root.node → Node, Node.next → Node (self-reference = cycle).
// root.label — scalar, terminal=1.
// root.node  — object → Node, no cycle from root's frame, terminal=1.
// Node.value — scalar, terminal=1.
// Node.next  — object → Node, Node is on the path, terminal=0.
var cycleSchema = []byte(`{
  "name": "Tree",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "label", "type": "string" },
    "019ca000-0030-7000-0000-000000000030": {
      "name": "node",
      "type": "object",
      "schema": { "id": "019ca000-0010-7000-0000-000000000010" }
    }
  },
  "schemas": {
    "019ca000-0010-7000-0000-000000000010": {
      "name": "Node",
      "fields": {
        "019ca000-0011-7000-0000-000000000011": { "name": "value", "type": "string" },
        "019ca000-0012-7000-0000-000000000012": {
          "name": "next",
          "type": "object",
          "schema": { "id": "019ca000-0010-7000-0000-000000000010" }
        }
      }
    }
  }
}`)

// unionSchema has a root field of type union pointing at two named object
// schemas. Tests Variants map population and terminal bit on union fields.
var unionSchema = []byte(`{
  "name": "Event",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7000-0000-000000000030": {
      "name": "payload",
      "type": "union",
      "schema": [
        { "id": "019ca000-0040-7000-0000-000000000040" },
        { "id": "019ca000-0041-7000-0000-000000000041" }
      ]
    }
  },
  "schemas": {
    "019ca000-0040-7000-0000-000000000040": {
      "name": "VariantA",
      "fields": {
        "019ca000-0001-7000-0000-000000000001": { "name": "typeA", "type": "string" }
      }
    },
    "019ca000-0041-7000-0000-000000000041": {
      "name": "VariantB",
      "fields": {
        "019ca000-0002-7000-0000-000000000002": { "name": "typeB", "type": "integer" }
      }
    }
  }
}`)

// indexedSchema has a root schema with an index definition (no condition).
// Tests cold IndexDescriptor construction and ResolvedIndexes key assignment.
var indexedSchema = []byte(`{
  "name": "Product",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "sku",  "type": "string", "required": true },
    "019ca000-0002-7000-0000-000000000002": { "name": "price","type": "number" }
  },
  "indexes": {
    "019ca000-0050-7000-0000-000000000050": {
      "name": "idx_sku",
      "type": "unique",
      "fields": ["sku"]
    }
  }
}`)

// constrainedSchema has a root schema with one constraint referencing a
// predicate that must exist in the PredicateMap.
var constrainedSchema = []byte(`{
  "name": "Account",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "email", "type": "string", "required": true }
  },
  "constraints": {
    "019ca000-0060-7000-0000-000000000060": {
      "name": "validEmail",
      "predicate": "isEmail",
      "fields": ["email"]
    }
  }
}`)

// defaultSchema has a field with a default value. Tests Store population for
// field defaults.
var defaultSchema = []byte(`{
  "name": "Config",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "retries", "type": "integer", "default": 3 }
  }
}`)

// ── Invalid fixtures ───────────────────────────────────────────────────────

var invalidMissingName = []byte(`{
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "x", "type": "string" }
  }
}`)

var invalidMissingVersion = []byte(`{
  "name": "Broken",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "x", "type": "string" }
  }
}`)

var invalidUnknownFieldType = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7000-0000-000000000001": { "name": "x", "type": "bogustype" }
  }
}`)

var invalidUnresolvedRef = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7000-0000-000000000030": {
      "name": "addr",
      "type": "object",
      "schema": { "id": "deadbeef-dead-7ead-dead-deaddeadbeef" }
    }
  }
}`)

var invalidBadJSON = []byte(`{ not valid json`)

// ── Helpers ────────────────────────────────────────────────────────────────

// mustParse parses src and panics on error. For use in test setup only.
func mustParse(src []byte) *SourceSchema {
	ss, err := Parse(src)
	if err != nil {
		panic("mustParse: " + err.Error())
	}
	return ss
}

// mustCompile parses and compiles src with the given predicates, panicking on
// error. For use in test setup only.
func mustCompile(src []byte, predicates PredicateMap) *CompiledSchema {
	ss := mustParse(src)
	cs, err := Compile(ss, predicates)
	if err != nil {
		panic("mustCompile: " + err.Error())
	}
	return cs
}

// firstError extracts the first CompileError from an error returned by Parse
// or Compile, or panics if the error is not a CompileErrors.
func firstError(err error) CompileError {
	ce, ok := err.(CompileErrors)
	if !ok || len(ce) == 0 {
		panic("firstError: not a CompileErrors or empty")
	}
	return ce[0]
}

// allErrors extracts all CompileErrors from an error returned by Parse or
// Compile, or panics if the type is wrong.
func allErrors(err error) []CompileError {
	ce, ok := err.(CompileErrors)
	if !ok {
		panic("allErrors: not a CompileErrors")
	}
	return []CompileError(ce)
}

// descriptorRange returns the [start, end) descriptor positions for a schema
// index using SchemaOffsets.
func descriptorRange(cs *CompiledSchema, schemaIdx uint8) (start, end int) {
	packed := cs.SchemaOffsets[schemaIdx]
	return int(uint16(packed)), int(uint16(packed >> 16))
}

// descriptorsFor returns the descriptor slice for one schema.
func descriptorsFor(cs *CompiledSchema, schemaIdx uint8) []uint32 {
	start, end := descriptorRange(cs, schemaIdx)
	return cs.Descriptors[start:end]
}

// findDescriptor returns the first descriptor in cs whose owner_schema ==
// schemaIdx and whose FieldMeta.Name == fieldName, or 0 if not found.
func findDescriptor(cs *CompiledSchema, schemaIdx uint8, fieldName string) uint32 {
	m, ok := cs.Meta[schemaIdx]
	if !ok {
		return 0
	}
	for fd, fm := range m.Fields {
		if fm.Name == fieldName {
			return fd
		}
	}
	return 0
}
