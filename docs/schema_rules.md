# Schema Definition Semantics

## Overview

This document defines the structural rules and semantics for the schema definition system. These rules ensure deterministic data modeling and type resolution.

---

## Core Rules

### **Rule 1: Global Field ID Uniqueness**

All `FieldId` keys must be unique within a `Schema`, including across all nested schemas in the hierarchy.

**Example:**
```json
{
  "name": "UserSchema",
  "version": "1.0.0",
  "fields": {
    "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
      "name": "email",
      "type": "string"
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
      "name": "address",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f": {
          "name": "street",
          "type": "string"
        }
        // ❌ Cannot reuse "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f" here
      }
    }
  }
}
```

---

### **Rule 2: Nested Schema Mode Exclusivity**

A `NestedSchema` **must use exactly one mode**:

- **Schema mode**: `BaseSchema` properties are populated (Fields, Indexes, Constraints, Metadata). `FieldProperties` must be zero/empty.
- **Type mode**: `FieldProperties` are populated (Type, Default, Schema). `BaseSchema` collections (Fields, Indexes, Constraints) must be empty.
- **Enum mode** (special case): Type is set to `enum`, and `Values` is populated. This is a variant of Type mode.

**A nested schema with both modes populated is invalid.**

**Schema Mode Example:**
```json
{
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
      "name": "address",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
          "name": "street",
          "type": "string"
        },
        "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
          "name": "city",
          "type": "string"
        }
      }
    }
  }
}
```

**Type Mode Example:**
```json
{
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
      "name": "email_type",
      "type": "string"
    }
  }
}
```

**Enum Mode Example:**
```json
{
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "status_values",
      "type": "string",
      "values": ["active", "inactive", "pending"]
    }
  }
}
```

**❌ Invalid Example (Both Modes):**
```json
{
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
      "name": "invalid",
      "type": "string",              // Type mode
      "fields": {                    // Schema mode
        "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
          "name": "something",
          "type": "string"
        }
      }
    }
  }
}
```

---

### **Rule 3: Primitive Schema References**

Primitive types (`string`, `number`, `integer`, `decimal`, `boolean`) **cannot** have a `Schema` reference.

**Note:** `enum` is **not** a primitive type.

**✅ Valid:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
      "name": "username",
      "type": "string"
    }
  }
}
```

**Valid Data:**
```json
{
  "username": "john_doe"
}
```

**❌ Invalid:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
      "name": "username",
      "type": "string",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f"}
    }
  }
}
```

---

### **Rule 4: Enum Requirements**

An `enum` field:
- **Must** have a `Schema` reference
- The referenced schema **must** declare a type that is either:
  - Numerical (`number`, `integer`, `decimal`), or
  - `string`
- The referenced schema **must** have a populated `Values` entry
- **Only enum-type schemas can have Values populated**

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
      "name": "status",
      "type": "enum",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
      "name": "status_enum",
      "type": "string",
      "values": ["active", "inactive", "suspended"]
    }
  }
}
```

**Valid Data:**
```json
{
  "status": "active"
}
```

**Invalid Data:**
```json
{
  "status": "deleted"
}
```

---

### **Rule 5: Composite Field Semantics**

A `composite` field:
- Must reference multiple schemas ([]SchemaReference)
- Data must match ALL referenced schemas (logical AND)
- Each referenced schema must effectively represent an object type:

- Schema mode: Schema has Fields defined (explicit object), OR
- Type mode with Object: Type is FieldTypeObject, OR
- Type mode with Record: Type is FieldTypeRecord, OR
- Type mode with Union: Type is FieldTypeUnion AND all union variants are themselves effectively objects



The rationale is that composition only makes semantic sense when merging object structures. You can compose `{a, b}` AND `{c, d}` into `{a, b, c, d}`, but you cannot meaningfully compose `{a, b}` AND `"string"`.

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
      "name": "timestamped_entity",
      "type": "composite",
      "schema": [
        {"id": "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f"},
        {"id": "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f"}
      ]
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
      "name": "entity",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
          "name": "id",
          "type": "string"
        }
      }
    },
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "timestamps",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
          "name": "created_at",
          "type": "string"
        },
        "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
          "name": "updated_at",
          "type": "string"
        }
      }
    }
  }
}
```

**Valid Data:**
```json
{
  "timestamped_entity": {
    "id": "user_123",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T14:45:00Z"
  }
}
```

**Invalid Data (Missing required field from one schema):**
```json
{
  "timestamped_entity": {
    "id": "user_123",
    "created_at": "2024-01-15T10:30:00Z"
  }
}
```

---

### **Rule 6: Union Field Semantics**

A `union` field:
- **Must** reference multiple schemas (`[]SchemaReference`)
- Data **must match ONE** of the referenced schemas (logical OR)
- Referenced schemas **can be in either Schema mode or Type mode** interchangeably
- Allows modeling types like: `string | Array<string> | Record<string, string>`

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
      "name": "flexible_value",
      "type": "union",
      "schema": [
        {"id": "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f"},
        {"id": "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f"},
        {"id": "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f"}
      ]
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f": {
      "name": "string_type",
      "type": "string"
    },
    "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
      "name": "string_array",
      "type": "array",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f"}
    },
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
      "name": "string_record",
      "type": "record",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f"}
    }
  }
}
```

**Valid Data (string variant):**
```json
{
  "flexible_value": "hello"
}
```

**Valid Data (array variant):**
```json
{
  "flexible_value": ["tag1", "tag2", "tag3"]
}
```

**Valid Data (record variant):**
```json
{
  "flexible_value": {
    "key1": "value1",
    "key2": "value2"
  }
}
```

**Invalid Data (doesn't match any variant):**
```json
{
  "flexible_value": 42
}
```

---

### **Rule 7: Array Field Semantics**

An `array` field:
- **Must** reference a single schema that defines the element type
- All elements must conform to the referenced schema
- Referenced schema **can be in either Schema mode or Type mode**

**Type Mode Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
      "name": "tags",
      "type": "array",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
      "name": "tag_type",
      "type": "string"
    }
  }
}
```

**Valid Data:**
```json
{
  "tags": ["javascript", "golang", "python"]
}
```

**Schema Mode Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "addresses",
      "type": "array",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
      "name": "address",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
          "name": "street",
          "type": "string"
        },
        "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
          "name": "city",
          "type": "string"
        }
      }
    }
  }
}
```

**Valid Data:**
```json
{
  "addresses": [
    {
      "street": "123 Main St",
      "city": "Nairobi"
    },
    {
      "street": "456 Oak Ave",
      "city": "Mombasa"
    }
  ]
}
```

---

### **Rule 8: Set Field Semantics**

A `set` field:
- Behaves like an array with unique elements enforced
- **Must** reference a single schema that defines the element type
- Referenced schema **can be in either Schema mode or Type mode**
- Uniqueness determination is implementation-defined

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
      "name": "unique_tags",
      "type": "set",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f": {
      "name": "tag_type",
      "type": "string"
    }
  }
}
```

**Valid Data:**
```json
{
  "unique_tags": ["javascript", "golang", "python"]
}
```

**Invalid Data (duplicate elements):**
```json
{
  "unique_tags": ["javascript", "golang", "javascript"]
}
```

---

### **Rule 9: Object Field Semantics**

An `object` field:
- Represents a typed map with string keys
- **Must** reference a schema that has `Fields` defined (Schema mode)
- Values must match the field definitions from the referenced schema
- Field optionality is determined by the `Required` property

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
      "name": "contact_info",
      "type": "object",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
      "name": "contact",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
          "name": "email",
          "type": "string",
          "required": true
        },
        "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
          "name": "phone",
          "type": "string",
          "required": false
        }
      }
    }
  }
}
```

**Valid Data (all required fields present):**
```json
{
  "contact_info": {
    "email": "john@example.com",
    "phone": "+254-712-345678"
  }
}
```

**Valid Data (optional field omitted):**
```json
{
  "contact_info": {
    "email": "john@example.com"
  }
}
```

**Invalid Data (missing required field):**
```json
{
  "contact_info": {
    "phone": "+254-712-345678"
  }
}
```

---

### **Rule 10: Record Field Semantics**

A `record` field:
- Without schema reference: untyped map (`map[string]any`)
- With schema reference: typed map (`map[string]Shape`)
- Referenced schema **can be in either Schema mode or Type mode**:
  - **Type mode**: Simple maps like `Record<string, string>` (schema has Type set)
  - **Schema mode**: Complex maps like `Record<string, Address>` (schema has Fields set)
- **Must** reference exactly one schema or none (cannot reference multiple schemas)

**Type Mode Example (Simple Map):**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "metadata",
      "type": "record",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
      "name": "string_value",
      "type": "string"
    }
  }
}
```

**Valid Data:**
```json
{
  "metadata": {
    "author": "John Doe",
    "version": "1.0.0",
    "description": "Sample project"
  }
}
```

**Schema Mode Example (Complex Map):**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
      "name": "address_book",
      "type": "record",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
      "name": "address",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
          "name": "street",
          "type": "string"
        },
        "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f": {
          "name": "city",
          "type": "string"
        }
      }
    }
  }
}
```

**Valid Data:**
```json
{
  "address_book": {
    "home": {
      "street": "123 Main St",
      "city": "Nairobi"
    },
    "work": {
      "street": "456 Business Rd",
      "city": "Mombasa"
    }
  }
}
```

**Untyped Record Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
      "name": "flexible_data",
      "type": "record"
    }
  }
}
```

**Valid Data (any structure):**
```json
{
  "flexible_data": {
    "key1": "string value",
    "key2": 42,
    "key3": true,
    "key4": ["array", "values"]
  }
}
```

---

### **Rule 11: Geometry Field Semantics**

A `geometry` field:
- Represents an array of numerical tuples: `Array<Array<number>>`
- Inner arrays contain numerical values
- `number` includes: `number`, `integer`, or `decimal` types

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
      "name": "polygon",
      "type": "geometry"
    }
  }
}
```

**Valid Data:**
```json
{
  "polygon": [
    [0.0, 0.0],
    [10.5, 0.0],
    [10.5, 10.5],
    [0.0, 10.5],
    [0.0, 0.0]
  ]
}
```

---

### **Rule 12: Index Field References**

- Indexes can only reference existing `FieldId`s within the schema
- **Spatial indexes** (`IndexTypeSpatial`) can only reference fields of type `geometry`

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
      "name": "email",
      "type": "string"
    },
    "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
      "name": "location",
      "type": "geometry"
    }
  },
  "indexes": {
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "email_idx",
      "type": "unique",
      "fields": ["01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f"]
    },
    "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
      "name": "location_idx",
      "type": "spatial",
      "fields": ["01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f"]
    }
  }
}
```

---

### **Rule 13: Index Value Type Matching**

Index condition values must match the type of the field being indexed.

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
      "name": "age",
      "type": "integer"
    }
  },
  "indexes": {
    "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
      "name": "adult_idx",
      "type": "normal",
      "fields": ["01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f"],
      "condition": {
        "field": "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f",
        "operator": "gte",
        "value": 18
      }
    }
  }
}
```

**❌ Invalid (Type Mismatch):**
```json
{
  "condition": {
    "field": "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f",
    "operator": "gte",
    "value": "18"
  }
}
```

---

### **Rule 14: Constraint Field References**

Constraints reference fields by path notation and must reference existing fields.

**Path notation:** Concatenation of field names, e.g., `user.address.street`

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
      "name": "user",
      "type": "object",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f": {
      "name": "user_schema",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
          "name": "email",
          "type": "string"
        }
      }
    }
  },
  "constraints": {
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
      "name": "valid_email",
      "predicate": "email_format",
      "fields": ["user.email"]
    }
  }
}
```

---

### **Rule 15: Schema Reference Integrity**

All schema references (in `FieldSchemaReference`, constraints, etc.) must resolve to existing schemas in the schema hierarchy.

**✅ Valid:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
      "name": "profile",
      "type": "object",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
      "name": "profile_schema",
      "fields": {}
    }
  }
}
```

**❌ Invalid (Dangling Reference):**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "profile",
      "type": "object",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-nonexistent"}
    }
  }
}
```

---

### **Rule 16: Default Value Constraints**

Default values **must** match the field's declared type.

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
      "name": "status",
      "type": "string",
      "default": "active"
    },
    "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
      "name": "count",
      "type": "integer",
      "default": 0
    },
    "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
      "name": "tags",
      "type": "array",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f"},
      "default": []
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
      "name": "string_type",
      "type": "string"
    }
  }
}
```

**Valid Data (using defaults):**
```json
{
  "status": "active",
  "count": 0,
  "tags": []
}
```

**Valid Data (overriding defaults):**
```json
{
  "status": "pending",
  "count": 5,
  "tags": ["important", "urgent"]
}
```

---

### **Rule 17: Circular References**

Schemas **may** reference themselves directly or transitively. Depth validation is a runtime/validation concern, not a schema definition concern.

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f": {
      "name": "student",
      "type": "object",
      "schema": {"id": "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f"}
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
      "name": "person",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
          "name": "name",
          "type": "string"
        },
        "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
          "name": "emergency_contact",
          "type": "object",
          "schema": {"id": "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f"}
        }
      }
    }
  }
}
```

**Valid Data (nested circular reference):**
```json
{
  "student": {
    "name": "Alice",
    "emergency_contact": {
      "name": "Bob",
      "emergency_contact": {
        "name": "Carol",
        "emergency_contact": null
      }
    }
  }
}
```

---

### **Rule 18: Unknown Field Type**

`FieldTypeUnknown` is an escape hatch with no structural rules. Validation is handled exclusively through user-defined constraints.

**Example:**
```json
{
  "fields": {
    "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
      "name": "custom_data",
      "type": "unknown"
    }
  },
  "constraints": {
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "custom_validator",
      "predicate": "validate_custom",
      "fields": ["custom_data"]
    }
  }
}
```

**Valid Data (depends on custom constraint):**
```json
{
  "custom_data": {
    "any": "structure",
    "is": ["valid"],
    "as": {
      "long": {
        "as": "constraint passes"
      }
    }
  }
}
```

---

### **Rule 19: Constraint Specificity and Override**

Constraints follow a specificity hierarchy where more specific constraints override less specific ones. Constraint names determine override behavior:

**Specificity Hierarchy (lowest to highest):**
1. **Nested Schema Constraints** (least specific) - Defined in the nested schema itself
2. **Schema Reference Constraints** (medium specific) - Defined in the `SchemaReference` when referencing a schema
3. **Top-Level Field Constraints** (most specific) - Defined at the top level schema

**Override Rule:** When constraints share the same name across different specificity levels, the more specific constraint completely replaces the less specific one.

**Example:**

```json
{
  "name": "UserSchema",
  "version": "1.0.0",
  "fields": {
    "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
      "name": "email",
      "type": "object",
      "schema": {
        "id": "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f",
        "constraints": {
          "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
            "name": "email_validation",
            "predicate": "email_strict",
            "fields": ["address"],
            "parameters": {
              "require_company_domain": true
            }
          }
        }
      }
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
      "name": "email_contact",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
          "name": "address",
          "type": "string"
        },
        "01934d8a-7c24-7b3e-9f12-5a6b7c8d9e0f": {
          "name": "verified",
          "type": "boolean"
        }
      },
      "constraints": {
        "01934d8a-7c24-7b3e-9f12-6a7b8c9d0e1f": {
          "name": "email_validation",
          "predicate": "email_format",
          "fields": ["address"]
        },
        "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f": {
          "name": "verified_check",
          "predicate": "is_true",
          "fields": ["verified"]
        }
      }
    }
  },
  "constraints": {
    "01934d8a-7c24-7b3e-9f12-8a9b0c1d2e3f": {
      "name": "email_validation",
      "predicate": "email_relaxed",
      "fields": ["email.address"],
      "parameters": {
        "allow_subaddressing": true
      }
    }
  }
}
```

**Effective Constraints Applied:**

For the `email` field, the following constraints are applied in order of specificity:

1. **Top-level constraint** `email_validation` (predicate: `email_relaxed`) - **APPLIED** (most specific, overrides all others with same name)
2. **Schema reference constraint** `email_validation` (predicate: `email_strict`) - **IGNORED** (overridden by top-level)
3. **Nested schema constraint** `email_validation` (predicate: `email_format`) - **IGNORED** (overridden by both above)
4. **Nested schema constraint** `verified_check` - **APPLIED** (no conflicts, unique name)

**Valid Data:**
```json
{
  "email": {
    "address": "user+tag@example.com",
    "verified": true
  }
}
```

**Explanation:**
- The `email_relaxed` predicate from top-level allows subaddressing (`user+tag@...`)
- The nested schema's `email_format` and reference's `email_strict` are overridden
- The `verified_check` constraint still applies (no name conflict)

---

**Another Example - No Overrides:**

```json
{
  "name": "ProductSchema",
  "version": "1.0.0",
  "fields": {
    "01934d8a-7c24-7b3e-9f12-9a0b1c2d3e4f": {
      "name": "price",
      "type": "object",
      "schema": {
        "id": "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f",
        "constraints": {
          "01934d8a-7c24-7b3e-9f12-1a2b3c4d5e6f": {
            "name": "positive_amount",
            "predicate": "greater_than",
            "fields": ["amount"],
            "parameters": {"min": 0}
          }
        }
      }
    }
  },
  "schemas": {
    "01934d8a-7c24-7b3e-9f12-0a1b2c3d4e5f": {
      "name": "money",
      "fields": {
        "01934d8a-7c24-7b3e-9f12-2a3b4c5d6e7f": {
          "name": "amount",
          "type": "decimal"
        },
        "01934d8a-7c24-7b3e-9f12-3a4b5c6d7e8f": {
          "name": "currency",
          "type": "string"
        }
      },
      "constraints": {
        "01934d8a-7c24-7b3e-9f12-4a5b6c7d8e9f": {
          "name": "valid_currency",
          "predicate": "in_set",
          "fields": ["currency"],
          "parameters": {"values": ["KES", "USD", "EUR"]}
        }
      }
    }
  }
}
```

**Effective Constraints Applied:**

1. **Schema reference constraint** `positive_amount` - **APPLIED** (overrides nested schema if it had same name)
2. **Nested schema constraint** `valid_currency` - **APPLIED** (no conflicts)

**Valid Data:**
```json
{
  "price": {
    "amount": 1500.00,
    "currency": "KES"
  }
}
```

**Invalid Data (violates positive_amount):**
```json
{
  "price": {
    "amount": -100.00,
    "currency": "KES"
  }
}
```

**Invalid Data (violates valid_currency):**
```json
{
  "price": {
    "amount": 1500.00,
    "currency": "GBP"
  }
}
```

---

## Summary

These 19 rules provide a complete and deterministic specification for schema definition. Key principles:

1. **Uniqueness**: FieldIds are globally unique within a schema
2. **Mode Exclusivity**: Nested schemas use exactly one mode (Schema/Type/Enum)
3. **Type Safety**: References must match expected types and exist
4. **Flexibility**: Support for primitives, collections, unions, and custom types
5. **Validation**: Structural rules separate from runtime validation concerns
6. **Constraint Hierarchy**: Clear specificity rules for constraint override behavior

The rules enable predictable type resolution and data modeling while maintaining flexibility for complex data structures.
