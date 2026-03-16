package ir

// validate_testdata_test.go provides fixtures for Pass 1.5 (validate.go) tests
// and TypeDecimal tests. All fixtures with the "invalid" prefix are expected to
// be rejected by Parse with a PassValidate error unless otherwise noted.
//
// Reuses the UUID constants from testdata_test.go where applicable.
// All map keys are valid UUID v7 values.

// ── TypeDecimal fixtures ───────────────────────────────────────────────────

// decimalSchema has a single decimal field. Tests that TypeDecimal parses,
// compiles, maps to document.TypeInt in the store, and serializes back as
// "decimal".
var decimalSchema = []byte(`{
  "name": "Price",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "amount", "type": "decimal", "required": true }
  }
}`)

// decimalEnumSchema has a decimal enum field with a named enum schema whose
// values are scaled integers. Tests that enum values for decimal fields are
// stored as TypeArrayInt.
var decimalEnumSchema = []byte(`{
  "name": "Rating",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7030-b070-777e858c939a": {
      "name": "tier",
      "type": "enum",
      "schema": { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" }
    }
  },
  "schemas": {
    "019ca000-0020-7020-a0a0-a7aeb5bcc3ca": {
      "name": "TierEnum",
      "type": "enum",
      "values": [100, 200, 300]
    }
  }
}`)

// ── Field validation — invalid fixtures ───────────────────────────────────

var invalidFieldMissingName = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "type": "string" }
  }
}`)

var invalidFieldMissingType = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "x" }
  }
}`)

// invalidScalarWithSchema: a string field must not carry a schema reference.
var invalidScalarWithSchema = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "x",
      "type": "string",
      "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" }
    }
  }
}`)

// invalidArrayMissingSchema: an array field requires a schema reference.
var invalidArrayMissingSchema = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "items", "type": "array" }
  }
}`)

// invalidUnionWithSingleSchema: a union field requires an array of refs, not
// a single object.
var invalidUnionWithSingleSchema = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "payload",
      "type": "union",
      "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" }
    }
  }
}`)

// invalidSingleTypeWithArraySchema: an object field requires a single ref, not
// an array.
var invalidSingleTypeWithArraySchema = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "addr",
      "type": "object",
      "schema": [
        { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" },
        { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" }
      ]
    }
  }
}`)

// invalidInlineStructuralType: inline schema ref uses "object", which is not
// in InlinePrimitiveTypeEnum.
var invalidInlineStructuralType = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "items",
      "type": "array",
      "schema": { "type": "object" }
    }
  }
}`)

// invalidSchemaRefBothIdAndType: a FieldSchema must not have both id and type.
var invalidSchemaRefBothIdAndType = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "items",
      "type": "array",
      "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa", "type": "string" }
    }
  }
}`)

// invalidInlineRecordWithValues: an inline schema of type "record" must not
// carry values (values are only valid on scalar primitives).
var invalidInlineRecordWithValues = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "meta",
      "type": "record",
      "schema": { "type": "record", "values": ["a", "b"] }
    }
  }
}`)

// invalidUniqueOnStructural: unique:true on an array field is an error.
var invalidUniqueOnStructural = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "tags",
      "type": "array",
      "unique": true,
      "schema": { "type": "string" }
    }
  }
}`)

// ── Nested schema validation — invalid fixtures ───────────────────────────

var invalidNestedMissingName = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "fields": {
        "019ca000-0001-7001-810d-141b22293037": { "name": "x", "type": "string" }
      }
    }
  }
}`)

// invalidNestedBothFormsAmbiguous: nested schema has both fields and type.
var invalidNestedBothFormsAmbiguous = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "Ambiguous",
      "type": "enum",
      "values": ["a", "b"],
      "fields": {
        "019ca000-0001-7001-810d-141b22293037": { "name": "x", "type": "string" }
      }
    }
  }
}`)

// invalidNestedNeitherForm: nested schema has neither fields nor type.
var invalidNestedNeitherForm = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "Empty"
    }
  }
}`)

// invalidNestedScalarType: a nested type schema with type "string" is invalid
// (scalars are not valid named type schemas).
var invalidNestedScalarType = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "BadType",
      "type": "string"
    }
  }
}`)

// invalidEnumSchemaMissingValues: an enum type schema must have a non-empty
// values array.
var invalidEnumSchemaMissingValues = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "EmptyEnum",
      "type": "enum"
    }
  }
}`)

// invalidEnumSchemaWithSchemaRef: an enum type schema must not have a schema
// reference (values are its contract, not a nested schema).
var invalidEnumSchemaWithSchemaRef = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "BadEnum",
      "type": "enum",
      "values": ["a", "b"],
      "schema": { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" }
    }
  }
}`)

// invalidArraySchemaMissingRef: an array type schema requires a schema
// reference for its element type.
var invalidArraySchemaMissingRef = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "BadArray",
      "type": "array"
    }
  }
}`)

// invalidUnionSchemaWithValues: a union type schema must not have values.
var invalidUnionSchemaWithValues = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "BadUnion",
      "type": "union",
      "values": ["a", "b"],
      "schema": [
        { "id": "019ca000-0040-7040-8040-474e555c636a" },
        { "id": "019ca000-0041-7041-814d-545b62697077" }
      ]
    }
  }
}`)

// invalidUnionSchemaNotArray: a union type schema requires an array of refs,
// not a single object ref.
var invalidUnionSchemaNotArray = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "BadUnion",
      "type": "union",
      "schema": { "id": "019ca000-0040-7040-8040-474e555c636a" }
    }
  }
}`)

// invalidObjectSchemaWithValues: an object schema must not have values.
var invalidObjectSchemaWithValues = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "BadObject",
      "values": ["a", "b"],
      "fields": {
        "019ca000-0001-7001-810d-141b22293037": { "name": "x", "type": "string" }
      }
    }
  }
}`)

// invalidObjectSchemaWithSchemaRef: an object schema must not have a schema
// reference at the top level.
var invalidObjectSchemaWithSchemaRef = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "BadObject",
      "schema": { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" },
      "fields": {
        "019ca000-0001-7001-810d-141b22293037": { "name": "x", "type": "string" }
      }
    }
  }
}`)

// ── Index validation — invalid fixtures ───────────────────────────────────

var invalidIndexMissingName = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "sku", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "type": "unique",
      "fields": ["sku"]
    }
  }
}`)

var invalidIndexMissingType = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "sku", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_sku",
      "fields": ["sku"]
    }
  }
}`)

var invalidIndexBadType = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "sku", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_sku",
      "type": "badtype",
      "fields": ["sku"]
    }
  }
}`)

var invalidIndexMissingFields = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "sku", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_sku",
      "type": "unique"
    }
  }
}`)

var invalidIndexBadOrder = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "sku", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_sku",
      "type": "normal",
      "fields": ["sku"],
      "order": "sideways"
    }
  }
}`)

// Index condition fixtures — all share a valid base structure and only corrupt
// the condition portion.

var invalidConditionLeafMissingField = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "status", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_active",
      "type": "normal",
      "fields": ["status"],
      "condition": { "operator": "eq", "value": "active" }
    }
  }
}`)

var invalidConditionLeafMissingOp = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "status", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_active",
      "type": "normal",
      "fields": ["status"],
      "condition": { "field": "status", "value": "active" }
    }
  }
}`)

var invalidConditionLeafBadOp = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "status", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_active",
      "type": "normal",
      "fields": ["status"],
      "condition": { "field": "status", "operator": "like", "value": "active" }
    }
  }
}`)

var invalidConditionLeafMissingValue = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "status", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_active",
      "type": "normal",
      "fields": ["status"],
      "condition": { "field": "status", "operator": "eq" }
    }
  }
}`)

var invalidConditionGroupMissingOp = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "status", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_active",
      "type": "normal",
      "fields": ["status"],
      "condition": {
        "conditions": [
          { "field": "status", "operator": "eq", "value": "active" }
        ]
      }
    }
  }
}`)

var invalidConditionGroupBadOp = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "status", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_active",
      "type": "normal",
      "fields": ["status"],
      "condition": {
        "operator": "maybe",
        "conditions": [
          { "field": "status", "operator": "eq", "value": "active" }
        ]
      }
    }
  }
}`)

var invalidConditionMixed = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "status", "type": "string" }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "idx_mixed",
      "type": "normal",
      "fields": ["status"],
      "condition": {
        "field": "status",
        "operator": "eq",
        "value": "active",
        "conditions": [
          { "field": "status", "operator": "eq", "value": "active" }
        ]
      }
    }
  }
}`)

// ── Constraint validation — invalid fixtures ──────────────────────────────

var invalidConstraintMissingName = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string" }
  },
  "constraints": {
    "019ca000-0060-7060-a0e0-e7eef5fc030a": {
      "predicate": "isEmail",
      "fields": ["email"]
    }
  }
}`)

// invalidConstraintNeitherForm: has name but neither predicate nor operator.
var invalidConstraintNeitherForm = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string" }
  },
  "constraints": {
    "019ca000-0060-7060-a0e0-e7eef5fc030a": {
      "name": "orphan"
    }
  }
}`)

// invalidConstraintBothForms: has both predicate and operator — ambiguous leaf
// vs group.
var invalidConstraintBothForms = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string" }
  },
  "constraints": {
    "019ca000-0060-7060-a0e0-e7eef5fc030a": {
      "name": "ambiguous",
      "predicate": "isEmail",
      "operator": "and",
      "rules": []
    }
  }
}`)

var invalidConstraintGroupMissingOp = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string" }
  },
  "constraints": {
    "019ca000-0060-7060-a0e0-e7eef5fc030a": {
      "name": "noOp",
      "rules": [
        { "name": "r1", "predicate": "isEmail", "fields": ["email"] }
      ]
    }
  }
}`)

var invalidConstraintGroupBadOp = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string" }
  },
  "constraints": {
    "019ca000-0060-7060-a0e0-e7eef5fc030a": {
      "name": "badOp",
      "operator": "maybe",
      "rules": [
        { "name": "r1", "predicate": "isEmail", "fields": ["email"] }
      ]
    }
  }
}`)

var invalidConstraintGroupEmptyRules = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string" }
  },
  "constraints": {
    "019ca000-0060-7060-a0e0-e7eef5fc030a": {
      "name": "emptyGroup",
      "operator": "and",
      "rules": []
    }
  }
}`)

// ── UUID key validation — invalid fixtures ────────────────────────────────

// invalidNonUUIDFieldKey uses a plain string as a field map key.
var invalidNonUUIDFieldKey = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "not-a-uuid": { "name": "x", "type": "string" }
  }
}`)

// invalidNonUUIDSchemaKey uses a plain string as a schemas map key.
var invalidNonUUIDSchemaKey = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {},
  "schemas": {
    "not-a-uuid": {
      "name": "BadKey",
      "type": "enum",
      "values": ["a"]
    }
  }
}`)

// invalidNonUUIDIndexKey uses a plain string as an indexes map key.
var invalidNonUUIDIndexKey = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "sku", "type": "string" }
  },
  "indexes": {
    "not-a-uuid": {
      "name": "idx_sku",
      "type": "unique",
      "fields": ["sku"]
    }
  }
}`)

// invalidNonUUIDConstraintKey uses a plain string as a constraints map key.
var invalidNonUUIDConstraintKey = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string" }
  },
  "constraints": {
    "not-a-uuid": {
      "name": "validEmail",
      "predicate": "isEmail",
      "fields": ["email"]
    }
  }
}`)

// invalidUUIDWrongVersion uses a UUID v4 key (version nibble = 4, not 7).
var invalidUUIDWrongVersion = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "550e8400-e29b-41d4-a716-446655440000": { "name": "x", "type": "string" }
  }
}`)

// invalidUUIDWrongVariant uses a UUID with a Microsoft GUID variant
// (variant bits = 11xx instead of 10xx).
var invalidUUIDWrongVariant = []byte(`{
  "name": "Broken",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-c10d-141b22293037": { "name": "x", "type": "string" }
  }
}`)
