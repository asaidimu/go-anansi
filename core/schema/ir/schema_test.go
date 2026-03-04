package ir_test

import (
	"strings"
	"testing"
	"time"
	"unsafe"

	ir "github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

// =============================================================================
// TEST HELPERS
// =============================================================================

// mustCompile compiles the given JSON and fatals the test on any error.
func mustCompile(t *testing.T, src string) *ir.CompiledEntry {
	t.Helper()
	entry, err := ir.CompileJSON(strings.NewReader(src))
	if err != nil {
		t.Fatalf("CompileJSON: %v", err)
	}
	return entry
}

// mustFail asserts that CompileJSON returns an error for the given JSON.
func mustFail(t *testing.T, src string) {
	t.Helper()
	_, err := ir.CompileJSON(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected compilation error, got nil")
	}
}

// fieldsByName returns a map from field name to FieldDescriptor for all fields
// in a given slot, using Meta for the name lookup.
func fieldsByName(entry *ir.CompiledEntry, slot *ir.SchemaSlot) map[string]ir.FieldDescriptor {
	m := make(map[string]ir.FieldDescriptor, len(slot.Fields))
	for _, fd := range slot.Fields {
		name := entry.Meta.Fields[fd.SchemaIdx()][fd.FieldIdx()].Name
		m[name] = fd
	}
	return m
}

// dagVisit does a full DAG walk from root, calling fn for each FieldDescriptor
// encountered.  Slot-visit deduplication prevents looping on recursive schemas.
func dagVisit(entry *ir.CompiledEntry, fn func(ir.FieldDescriptor)) {
	core := entry.Core
	visited := make(map[uint8]bool)

	var walk func(*ir.SchemaSlot)
	walk = func(slot *ir.SchemaSlot) {
		if visited[slot.Idx] {
			return
		}
		visited[slot.Idx] = true
		for _, fd := range slot.Fields {
			fn(fd)
			if fd.IsLeaf() {
				continue
			}
			switch fd.Kind() {
			case ir.FieldKindObject, ir.FieldKindArray:
				walk(core.Child(fd))
			case ir.FieldKindComplex:
				cx := core.ComplexOf(slot, fd)
				for i := range cx.Variants {
					walk(core.Variant(cx, i))
				}
			}
		}
	}
	walk(core.Root())
}

// countFields counts all FieldDescriptors reachable from root via dagVisit.
func countFields(entry *ir.CompiledEntry) int {
	n := 0
	dagVisit(entry, func(_ ir.FieldDescriptor) { n++ })
	return n
}

// =============================================================================
// SCHEMA FIXTURES
//
// Every field/schema map key is a valid UUID v7; the compiler rejects anything
// else.  Each fixture is minimal and self-contained, exercising one aspect.
// =============================================================================

// One required string field at root.
const singleStringField = `{
  "version": "1.0",
  "name":    "Single",
  "schemas": {},
  "fields": {
    "019ca98f-1001-7000-8000-000000000001": {
      "name": "title", "type": "string", "required": true
    }
  }
}`

// One field carrying every boolean flag plus a default value.
const allFlags = `{
  "version": "1.0",
  "name":    "Flags",
  "schemas": {},
  "fields": {
    "019ca98f-1002-7000-8000-000000000001": {
      "name": "code", "type": "string",
      "required": true, "unique": true, "deprecated": true, "default": "n/a"
    }
  }
}`

// Exercises FieldKindSimple (enum), FieldKindObject, FieldKindArray, FieldKindComplex (union).
const allKinds = `{
  "version": "1.0",
  "name":    "Kinds",
  "schemas": {
    "019ca98f-2001-7000-8000-000000000001": {
      "name": "Status", "type": "enum", "values": ["active","inactive"]
    },
    "019ca98f-2002-7000-8000-000000000002": {
      "name": "Address",
      "fields": {
        "019ca98f-2003-7000-8000-000000000003": {
          "name": "street", "type": "string", "required": true
        }
      }
    },
    "019ca98f-2004-7000-8000-000000000004": {
      "name": "TagList", "type": "array",
      "schema": { "id": "019ca98f-2001-7000-8000-000000000001" }
    },
    "019ca98f-2005-7000-8000-000000000005": {
      "name": "PhoneOrEmail", "type": "union",
      "schema": [
        { "id": "019ca98f-2002-7000-8000-000000000002" }
      ]
    }
  },
  "fields": {
    "019ca98f-2010-7000-8000-000000000010": {
      "name": "status", "type": "enum",
      "schema": { "id": "019ca98f-2001-7000-8000-000000000001" }
    },
    "019ca98f-2011-7000-8000-000000000011": {
      "name": "address", "type": "object",
      "schema": { "id": "019ca98f-2002-7000-8000-000000000002" }
    },
    "019ca98f-2012-7000-8000-000000000012": {
      "name": "tags", "type": "array",
      "schema": { "id": "019ca98f-2004-7000-8000-000000000004" }
    },
    "019ca98f-2013-7000-8000-000000000013": {
      "name": "contact", "type": "union",
      "schema": [
        { "id": "019ca98f-2002-7000-8000-000000000002" }
      ]
    }
  }
}`

// A composite field merging two constituent schemas.
const compositeTwo = `{
  "version": "1.0",
  "name":    "Composite",
  "schemas": {
    "019ca98f-3001-7000-8000-000000000001": {
      "name": "Part1",
      "fields": {
        "019ca98f-3002-7000-8000-000000000002": {
          "name": "alpha", "type": "string"
        }
      }
    },
    "019ca98f-3003-7000-8000-000000000003": {
      "name": "Part2",
      "fields": {
        "019ca98f-3004-7000-8000-000000000004": {
          "name": "beta", "type": "integer"
        }
      }
    }
  },
  "fields": {
    "019ca98f-3010-7000-8000-000000000010": {
      "name": "merged", "type": "composite",
      "schema": [
        { "id": "019ca98f-3001-7000-8000-000000000001" },
        { "id": "019ca98f-3003-7000-8000-000000000003" }
      ]
    }
  }
}`

// A Node schema that references itself via its children field.
const recursiveTree = `{
  "version": "1.0",
  "name":    "Tree",
  "schemas": {
    "019ca98f-4001-7000-8000-000000000001": {
      "name": "Node",
      "fields": {
        "019ca98f-4002-7000-8000-000000000002": {
          "name": "value", "type": "string"
        },
        "019ca98f-4003-7000-8000-000000000003": {
          "name": "children", "type": "array",
          "schema": { "id": "019ca98f-4001-7000-8000-000000000001" }
        }
      }
    }
  },
  "fields": {
    "019ca98f-4010-7000-8000-000000000010": {
      "name": "root", "type": "object",
      "schema": { "id": "019ca98f-4001-7000-8000-000000000001" }
    }
  }
}`

// One root constraint whose field path references one root field.
const withConstraint = `{
  "version": "1.0",
  "name":    "Constrained",
  "schemas": {},
  "fields": {
    "019ca98f-5001-7000-8000-000000000001": {
      "name": "email", "type": "string", "required": true
    }
  },
  "constraints": {
    "019ca98f-5002-7000-8000-000000000002": {
      "name": "emailFormat",
      "description": "Email must be valid.",
      "predicate": "isEmail",
      "fields": ["email"]
    }
  }
}`

// Two indexes with different types and sort orders.
const withIndexes = `{
  "version": "1.0",
  "name":    "Indexed",
  "schemas": {},
  "fields": {
    "019ca98f-6001-7000-8000-000000000001": {
      "name": "username", "type": "string", "required": true, "unique": true
    },
    "019ca98f-6002-7000-8000-000000000002": {
      "name": "score", "type": "integer"
    }
  },
  "indexes": {
    "019ca98f-6003-7000-8000-000000000003": {
      "name": "usernameIdx", "type": "unique",
      "fields": ["username"], "order": "asc"
    },
    "019ca98f-6004-7000-8000-000000000004": {
      "name": "scoreIdx", "type": "normal",
      "fields": ["score"], "order": "desc"
    }
  }
}`

// A constraint defined on a nested schema slot.
const nestedConstraint = `{
  "version": "1.0",
  "name":    "NestedConstrained",
  "schemas": {
    "019ca98f-7001-7000-8000-000000000001": {
      "name": "Profile",
      "fields": {
        "019ca98f-7002-7000-8000-000000000002": {
          "name": "handle", "type": "string", "required": true
        }
      },
      "constraints": {
        "019ca98f-7003-7000-8000-000000000003": {
          "name": "handleFormat",
          "predicate": "isSlug",
          "fields": ["handle"]
        }
      }
    }
  },
  "fields": {
    "019ca98f-7010-7000-8000-000000000010": {
      "name": "profile", "type": "object",
      "schema": { "id": "019ca98f-7001-7000-8000-000000000001" }
    }
  }
}`

// Root schema and a nested schema each carry a metadata map.
const withMetadata = `{
  "version": "1.0",
  "name":    "WithMeta",
  "metadata": { "owner": "team-alpha" },
  "schemas": {
    "019ca98f-8001-7000-8000-000000000001": {
      "name": "Sub",
      "metadata": { "tag": "internal" },
      "fields": {
        "019ca98f-8002-7000-8000-000000000002": {
          "name": "x", "type": "integer"
        }
      }
    }
  },
  "fields": {
    "019ca98f-8010-7000-8000-000000000010": {
      "name": "sub", "type": "object",
      "schema": { "id": "019ca98f-8001-7000-8000-000000000001" }
    }
  }
}`

// Full-feature schema: nested objects, arrays, union, composite, recursion,
// root constraint, root index, nested constraint, schema metadata.
const integrationSchema = `{
  "version": "2.0",
  "name":    "Integration",
  "concrete": true,
  "metadata": { "owner": "core-team" },
  "schemas": {
    "019ca98f-e001-7000-8000-000000000001": {
      "name": "TagEnum", "type": "enum",
      "values": ["personal","work","vip"]
    },
    "019ca98f-e002-7000-8000-000000000002": {
      "name": "HomeAddress",
      "fields": {
        "019ca98f-e003-7000-8000-000000000003": {
          "name": "street", "type": "string", "required": true
        },
        "019ca98f-e004-7000-8000-000000000004": {
          "name": "postcode", "type": "string"
        }
      }
    },
    "019ca98f-e005-7000-8000-000000000005": {
      "name": "WorkAddress",
      "fields": {
        "019ca98f-e006-7000-8000-000000000006": {
          "name": "office", "type": "string", "required": true
        }
      }
    },
    "019ca98f-e007-7000-8000-000000000007": {
      "name": "NamePart",
      "fields": {
        "019ca98f-e008-7000-8000-000000000008": {
          "name": "givenName", "type": "string", "required": true
        }
      }
    },
    "019ca98f-e009-7000-8000-000000000009": {
      "name": "IDPart",
      "fields": {
        "019ca98f-e00a-7000-8000-00000000000a": {
          "name": "nationalId", "type": "string", "required": true
        }
      }
    },
    "019ca98f-e00b-7000-8000-00000000000b": {
      "name": "Contact",
      "fields": {
        "019ca98f-e00c-7000-8000-00000000000c": {
          "name": "email", "type": "string", "required": true, "unique": true
        },
        "019ca98f-e00d-7000-8000-00000000000d": {
          "name": "phone", "type": "string"
        }
      }
    },
    "019ca98f-e00e-7000-8000-00000000000e": {
      "name": "Profile",
      "metadata": { "tier": "user" },
      "fields": {
        "019ca98f-e00f-7000-8000-00000000000f": {
          "name": "handle", "type": "string", "required": true, "unique": true
        },
        "019ca98f-e010-7000-8000-000000000010": {
          "name": "contact", "type": "object",
          "schema": { "id": "019ca98f-e00b-7000-8000-00000000000b" }
        },
        "019ca98f-e011-7000-8000-000000000011": {
          "name": "tags", "type": "array",
          "schema": { "id": "019ca98f-e001-7000-8000-000000000001" }
        },
        "019ca98f-e012-7000-8000-000000000012": {
          "name": "address", "type": "union",
          "schema": [
            { "id": "019ca98f-e002-7000-8000-000000000002" },
            { "id": "019ca98f-e005-7000-8000-000000000005" }
          ]
        },
        "019ca98f-e013-7000-8000-000000000013": {
          "name": "identity", "type": "composite",
          "schema": [
            { "id": "019ca98f-e007-7000-8000-000000000007" },
            { "id": "019ca98f-e009-7000-8000-000000000009" }
          ]
        }
      },
      "constraints": {
        "019ca98f-e020-7000-8000-000000000020": {
          "name": "handleMinLength",
          "predicate": "minLength",
          "parameters": { "min": 3 },
          "fields": ["handle"]
        }
      }
    },
    "019ca98f-e014-7000-8000-000000000014": {
      "name": "Node",
      "fields": {
        "019ca98f-e015-7000-8000-000000000015": {
          "name": "value", "type": "string"
        },
        "019ca98f-e016-7000-8000-000000000016": {
          "name": "children", "type": "array",
          "schema": { "id": "019ca98f-e014-7000-8000-000000000014" }
        }
      }
    }
  },
  "fields": {
    "019ca98f-e030-7000-8000-000000000030": {
      "name": "id", "type": "string", "required": true, "unique": true
    },
    "019ca98f-e031-7000-8000-000000000031": {
      "name": "profile", "type": "object",
      "schema": { "id": "019ca98f-e00e-7000-8000-00000000000e" }
    },
    "019ca98f-e032-7000-8000-000000000032": {
      "name": "tree", "type": "object",
      "schema": { "id": "019ca98f-e014-7000-8000-000000000014" }
    }
  },
  "constraints": {
    "019ca98f-e040-7000-8000-000000000040": {
      "name": "idFormat",
      "predicate": "isUuidV7",
      "fields": ["id"]
    }
  },
  "indexes": {
    "019ca98f-e050-7000-8000-000000000050": {
      "name": "primaryId", "type": "primary",
      "fields": ["id"], "order": "asc", "unique": true
    }
  }
}`

const deepPathSchema = `
{
  "version": "1.0",
  "name":    "DeepPath",
  "schemas": {
    "019ca98f-f001-7000-8000-000000000001": {
      "name": "Level3",
      "fields": {
        "019ca98f-f002-7000-8000-000000000002": { "name": "leaf", "type": "string" }
      }
    },
    "019ca98f-f003-7000-8000-000000000003": {
      "name": "Level2",
      "fields": {
        "019ca98f-f004-7000-8000-000000000004": {
          "name": "l3", "type": "object",
          "schema": { "id": "019ca98f-f001-7000-8000-000000000001" }
        }
      }
    },
    "019ca98f-f005-7000-8000-000000000005": {
      "name": "Level1",
      "fields": {
        "019ca98f-f006-7000-8000-000000000006": {
          "name": "l2", "type": "object",
          "schema": { "id": "019ca98f-f003-7000-8000-000000000003" }
        }
      },
      "constraints": {
        "019ca98f-f010-7000-8000-000000000010": {
          "name": "deepConstraint",
          "predicate": "notEmpty",
          "fields": ["l2.l3.leaf"]
        }
      },
      "indexes": {
        "019ca98f-f020-7000-8000-000000000020": {
          "name": "deepIndex",
          "fields": ["l2.l3.leaf"]
        }
      }
    }
  },
  "fields": {
    "019ca98f-f030-7000-8000-000000000030": {
      "name": "root", "type": "object",
      "schema": { "id": "019ca98f-f005-7000-8000-000000000005" }
    }
  },
  "constraints": {
    "019ca98f-f040-7000-8000-000000000040": {
      "name": "veryDeepConstraint",
      "predicate": "notEmpty",
      "fields": ["root.l2.l3.leaf"]
    }
  }
}`

func TestDeepPathResolution(t *testing.T) {
	entry := mustCompile(t, deepPathSchema)

	// Verify root constraint
	if len(entry.Constraints) != 1 {
		t.Fatalf("root constraints: want 1, got %d", len(entry.Constraints))
	}
	rc := entry.Constraints[0]
	if len(rc.Fields) != 1 {
		t.Fatalf("root constraint fields: want 1, got %d", len(rc.Fields))
	}
	if len(rc.Fields[0]) != 4 {
		t.Errorf("root constraint path length: want 4, got %d", len(rc.Fields[0]))
	}

	// Verify nested constraint on Level1
	l1fd := fieldsByName(entry, entry.Core.Root())["root"]
	l1Slot := entry.Core.Child(l1fd)
	nestedC, ok := entry.NestedConstraints[l1Slot.Idx]
	if !ok || len(nestedC) != 1 {
		t.Fatalf("Level1 nested constraints missing or count != 1")
	}
	nc := nestedC[0]
	if len(nc.Fields[0]) != 3 {
		t.Errorf("nested constraint path length: want 3, got %d", len(nc.Fields[0]))
	}

	// Verify nested index on Level1
	nestedI, ok := entry.NestedIndexes[l1Slot.Idx]
	if !ok || len(nestedI) != 1 {
		t.Fatalf("Level1 nested indexes missing or count != 1")
	}
	ni := nestedI[0]
	if len(ni.Fields[0]) != 3 {
		t.Errorf("nested index path length: want 3, got %d", len(ni.Fields[0]))
	}
}

func TestIndexCondition_LogicalGroup(t *testing.T) {
	const schema = `
	{
	  "version": "1.0",
	  "name":    "IndexCond",
	  "fields": {
		"019ca98f-a001-7000-8000-000000000001": { "name": "a", "type": "integer" },
		"019ca98f-a002-7000-8000-000000000002": { "name": "b", "type": "string" }
	  },
	  "indexes": {
		"019ca98f-a010-7000-8000-000000000010": {
		  "name": "complexIdx",
		  "fields": ["a"],
		  "condition": {
			"operator": "or",
			"conditions": [
			  { "field": "a", "operator": "gt", "value": 10 },
			  { "field": "b", "operator": "eq", "value": "active" }
			]
		  }
		}
	  }
	}`

	entry := mustCompile(t, schema)
	if len(entry.Indexes) != 1 {
		t.Fatalf("indexes: want 1, got %d", len(entry.Indexes))
	}
	idx := entry.Indexes[0]
	if idx.Condition == nil {
		t.Fatalf("index should have condition")
	}
	cond := idx.Condition
	// LogicalOr = 2 (from common/logical_operators.go)
	if !cond.IsGroup || cond.Op != 2 {
		t.Errorf("expected OR group (2), got %v (%q)", cond.Op, cond.Op.String())
	}
	if len(cond.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(cond.Children))
	}

	// Verify child 0 (a gt 10)
	c0 := cond.Children[0]
	if c0.Field.FieldIdx() != 0 {
		t.Errorf("child 0 field: want 0, got %d", c0.Field.FieldIdx())
	}
	// GreaterThan = 5 (from common/comparison_operators.go)
	if c0.Operator != 5 {
		t.Errorf("child 0 op: want 5 (gt), got %v (%q)", c0.Operator, c0.Operator.String())
	}
	v, _ := ir.LiteralValueAs[int64](c0.Value)
	if v != 10 {
		t.Errorf("child 0 value: want 10, got %v", v)
	}

	// Verify child 1 (b eq "active")
	c1 := cond.Children[1]
	if c1.Field.FieldIdx() != 1 {
		t.Errorf("child 1 field: want 1, got %d", c1.Field.FieldIdx())
	}
	// Equal = 1 (from common/comparison_operators.go)
	if c1.Operator != 1 {
		t.Errorf("child 1 op: want 1 (eq), got %v (%q)", c1.Operator, c1.Operator.String())
	}
	s, _ := ir.LiteralValueAs[string](c1.Value)
	if s != "active" {
		t.Errorf("child 1 value: want \"active\", got %q", s)
	}
}

func TestConstraint_ComplexVariants(t *testing.T) {
	const schema = `
	{
	  "version": "1.0",
	  "name":    "ComplexVar",
	  "schemas": {
		"019ca98f-c001-7000-8000-000000000001": {
		  "name": "Addr",
		  "fields": { "019ca98f-c002-7000-8000-000000000002": { "name": "city", "type": "string" } }
		},
		"019ca98f-c005-7000-8000-000000000005": {
		  "name": "Name",
		  "fields": { "019ca98f-c006-7000-8000-000000000006": { "name": "last", "type": "string" } }
		}
	  },
	  "fields": {
		"019ca98f-c011-7000-8000-000000000011": {
		  "name": "c", "type": "composite",
		  "schema": [
			{ "id": "019ca98f-c001-7000-8000-000000000001" },
			{ "id": "019ca98f-c005-7000-8000-000000000005" }
		  ]
		}
	  },
	  "constraints": {
		"019ca98f-c021-7000-8000-000000000021": {
		  "name": "compPath", "predicate": "notEmpty", "fields": ["c.last"]
		}
	  }
	}`

	entry := mustCompile(t, schema)
	if len(entry.Constraints) != 1 {
		t.Fatalf("constraints count: want 1, got %d", len(entry.Constraints))
	}

	cc := entry.Constraints[0]
	if len(cc.Fields[0]) != 2 {
		t.Errorf("constraint path length: want 2, got %d", len(cc.Fields[0]))
	}
}

func TestConstraint_UnionPathTerminal(t *testing.T) {
	const schema = `
	{
	  "version": "1.0",
	  "name":    "UnionTerm",
	  "schemas": {
		"019ca98f-d001-7000-8000-000000000001": { "name": "A", "fields": {} }
	  },
	  "fields": {
		"019ca98f-d002-7000-8000-000000000002": {
		  "name": "u", "type": "union", "schema": [{ "id": "019ca98f-d001-7000-8000-000000000001" }]
		}
	  },
	  "constraints": {
		"019ca98f-d003-7000-8000-000000000003": {
		  "name": "unionTerminal", "predicate": "isSet", "fields": ["u"]
		}
	  }
	}`

	entry := mustCompile(t, schema)
	if len(entry.Constraints) != 1 {
		t.Fatalf("constraints count: want 1, got %d", len(entry.Constraints))
	}
	if len(entry.Constraints[0].Fields[0]) != 1 {
		t.Errorf("path length: want 1, got %d", len(entry.Constraints[0].Fields[0]))
	}
}

// =============================================================================
// FIELD DESCRIPTOR — BIT PACKING
// =============================================================================

func TestFieldDescriptor_FieldKindAndIdentity(t *testing.T) {
	entry := mustCompile(t, singleStringField)
	root := entry.Core.Root()

	if len(root.Fields) != 1 {
		t.Fatalf("root field count: want 1, got %d", len(root.Fields))
	}
	fd := root.Fields[0]

	if fd.Kind() != ir.FieldKindSimple {
		t.Errorf("Kind: want FieldKindSimple, got %v", fd.Kind())
	}
	if fd.SchemaIdx() != 0 {
		t.Errorf("SchemaIdx: want 0 (root), got %d", fd.SchemaIdx())
	}
	if fd.FieldIdx() != 0 {
		t.Errorf("FieldIdx: want 0, got %d", fd.FieldIdx())
	}
}

func TestFieldDescriptor_AllFlagsSet(t *testing.T) {
	entry := mustCompile(t, allFlags)
	fd := entry.Core.Root().Fields[0]

	for _, tc := range []struct {
		name string
		got  bool
	}{
		{"Required", fd.Required()},
		{"Unique", fd.Unique()},
		{"Deprecated", fd.Deprecated()},
		{"HasDefault", fd.HasDefault()},
	} {
		if !tc.got {
			t.Errorf("%s: want true, got false", tc.name)
		}
	}
}

func TestFieldDescriptor_AllFlagsClear(t *testing.T) {
	// title has required=true; everything else must be false.
	entry := mustCompile(t, singleStringField)
	fd := entry.Core.Root().Fields[0]

	if fd.Unique() {
		t.Error("Unique: want false")
	}
	if fd.Deprecated() {
		t.Error("Deprecated: want false")
	}
	if fd.HasDefault() {
		t.Error("HasDefault: want false")
	}
	if fd.Terminal() {
		t.Error("Terminal: want false for non-recursive simple field")
	}
}

func TestFieldDescriptor_IsLeafForSimple(t *testing.T) {
	entry := mustCompile(t, singleStringField)
	if !entry.Core.Root().Fields[0].IsLeaf() {
		t.Error("FieldKindSimple: IsLeaf want true")
	}
}

func TestFieldDescriptor_IsLeafForObjectFalse(t *testing.T) {
	entry := mustCompile(t, allKinds)
	fd := fieldsByName(entry, entry.Core.Root())["address"]
	if fd.IsLeaf() {
		t.Error("FieldKindObject: IsLeaf want false")
	}
}

func TestFieldDescriptor_TargetIdx_Object(t *testing.T) {
	entry := mustCompile(t, allKinds)
	fd := fieldsByName(entry, entry.Core.Root())["address"]

	if fd.Kind() != ir.FieldKindObject {
		t.Fatalf("address Kind: want FieldKindObject, got %v", fd.Kind())
	}
	child := entry.Core.Child(fd)
	// TargetIdx must equal the child slot's Idx.
	if fd.TargetIdx() != child.Idx {
		t.Errorf("address TargetIdx %d != child.Idx %d", fd.TargetIdx(), child.Idx)
	}
}

func TestFieldDescriptor_TargetIdx_Array(t *testing.T) {
	entry := mustCompile(t, allKinds)
	fd := fieldsByName(entry, entry.Core.Root())["tags"]

	if fd.Kind() != ir.FieldKindArray {
		t.Fatalf("tags Kind: want FieldKindArray, got %v", fd.Kind())
	}
	child := entry.Core.Child(fd)
	if fd.TargetIdx() != child.Idx {
		t.Errorf("tags TargetIdx %d != child.Idx %d", fd.TargetIdx(), child.Idx)
	}
}

func TestFieldDescriptor_DescriptorCarriesOwnerIdentity(t *testing.T) {
	// Every descriptor in a slot must report SchemaIdx == slot.Idx and
	// FieldIdx == its position within slot.Fields.
	entry := mustCompile(t, allKinds)
	core := entry.Core

	seen := map[uint8]bool{}
	var check func(*ir.SchemaSlot)
	check = func(slot *ir.SchemaSlot) {
		if seen[slot.Idx] {
			return
		}
		seen[slot.Idx] = true
		for i, fd := range slot.Fields {
			if fd.SchemaIdx() != slot.Idx {
				t.Errorf("slot %d field %d: SchemaIdx %d != slot.Idx %d",
					slot.Idx, i, fd.SchemaIdx(), slot.Idx)
			}
			if fd.FieldIdx() != uint8(i) {
				t.Errorf("slot %d field %d: FieldIdx %d != %d",
					slot.Idx, i, fd.FieldIdx(), i)
			}
		}
		for _, fd := range slot.Fields {
			if fd.IsLeaf() {
				continue
			}
			switch fd.Kind() {
			case ir.FieldKindObject, ir.FieldKindArray:
				check(core.Child(fd))
			case ir.FieldKindComplex:
				cx := core.ComplexOf(slot, fd)
				for i := range cx.Variants {
					check(core.Variant(cx, i))
				}
			}
		}
	}
	check(core.Root())
}

// =============================================================================
// FIELD KINDS
// =============================================================================

func TestFieldKinds_AllFour(t *testing.T) {
	entry := mustCompile(t, allKinds)
	byName := fieldsByName(entry, entry.Core.Root())

	cases := []struct {
		field string
		kind  ir.FieldKind
	}{
		// enum is stored as an integer ordinal — no child schema traversal.
		{"status", ir.FieldKindSimple},
		{"address", ir.FieldKindObject},
		{"tags", ir.FieldKindArray},
		{"contact", ir.FieldKindComplex},
	}
	for _, tc := range cases {
		fd, ok := byName[tc.field]
		if !ok {
			t.Errorf("field %q not found in root slot", tc.field)
			continue
		}
		if fd.Kind() != tc.kind {
			t.Errorf("field %q: Kind want %v, got %v", tc.field, tc.kind, fd.Kind())
		}
	}
}

// =============================================================================
// SCHEMA SLOT STRUCTURE
// =============================================================================

func TestSchemaSlot_RootIsIdx0(t *testing.T) {
	entry := mustCompile(t, allKinds)
	if entry.Core.Root().Idx != 0 {
		t.Errorf("Root().Idx: want 0, got %d", entry.Core.Root().Idx)
	}
}

func TestSchemaSlot_NestedSlotIdxMatchesTargetIdx(t *testing.T) {
	entry := mustCompile(t, allKinds)
	fd := fieldsByName(entry, entry.Core.Root())["address"]
	child := entry.Core.Child(fd)

	if child.Idx == 0 {
		t.Error("nested slot Idx must not be 0 (root)")
	}
	if child.Idx != fd.TargetIdx() {
		t.Errorf("child.Idx %d != fd.TargetIdx() %d", child.Idx, fd.TargetIdx())
	}
}

func TestSchemaSlot_DirectAccessViaSlot(t *testing.T) {
	// core.Slot(slot.Idx) must return the same slot reached by traversal.
	entry := mustCompile(t, allKinds)
	core := entry.Core

	seen := map[uint8]bool{}
	var check func(*ir.SchemaSlot)
	check = func(slot *ir.SchemaSlot) {
		if seen[slot.Idx] {
			return
		}
		seen[slot.Idx] = true
		direct := core.Slot(slot.Idx)
		if direct.Idx != slot.Idx {
			t.Errorf("Slot(%d).Idx = %d", slot.Idx, direct.Idx)
		}
		for _, fd := range slot.Fields {
			if fd.IsLeaf() {
				continue
			}
			switch fd.Kind() {
			case ir.FieldKindObject, ir.FieldKindArray:
				check(core.Child(fd))
			case ir.FieldKindComplex:
				cx := core.ComplexOf(slot, fd)
				for i := range cx.Variants {
					check(core.Variant(cx, i))
				}
			}
		}
	}
	check(core.Root())
}

func TestSchemaSlot_NestedFieldsPopulated(t *testing.T) {
	entry := mustCompile(t, allKinds)
	fd := fieldsByName(entry, entry.Core.Root())["address"]
	child := entry.Core.Child(fd)

	if len(child.Fields) == 0 {
		t.Error("Address slot must have at least one field (street)")
	}
	streetFD := child.Fields[0]
	name := entry.Meta.Fields[streetFD.SchemaIdx()][streetFD.FieldIdx()].Name
	if name != "street" {
		t.Errorf("Address.Fields[0]: want street, got %q", name)
	}
}

// =============================================================================
// SHARED BACKING ARRAYS
// =============================================================================

// TestBackingArrays_FieldSlicesInSingleSpan verifies that all slot.Fields
// sub-slices lie within a single contiguous address span, confirming they
// share the SchemaBuilder backing array.
func TestBackingArrays_FieldSlicesInSingleSpan(t *testing.T) {
	entry := mustCompile(t, allKinds)
	core := entry.Core

	// Collect all distinct slots reachable from root.
	slots := map[uint8]*ir.SchemaSlot{}
	var gather func(*ir.SchemaSlot)
	gather = func(slot *ir.SchemaSlot) {
		if _, ok := slots[slot.Idx]; ok {
			return
		}
		slots[slot.Idx] = slot
		for _, fd := range slot.Fields {
			if fd.IsLeaf() {
				continue
			}
			switch fd.Kind() {
			case ir.FieldKindObject, ir.FieldKindArray:
				gather(core.Child(fd))
			case ir.FieldKindComplex:
				cx := core.ComplexOf(slot, fd)
				for i := range cx.Variants {
					gather(core.Variant(cx, i))
				}
			}
		}
	}
	gather(core.Root())

	// Compute the address span across all non-empty Fields slices.
	var lo, hi uintptr
	first := true
	for _, slot := range slots {
		if len(slot.Fields) == 0 {
			continue
		}
		p := uintptr(unsafe.Pointer(&slot.Fields[0]))
		end := p + uintptr(len(slot.Fields))*4 // FieldDescriptor is uint32 = 4 bytes
		if first {
			lo, hi = p, end
			first = false
		} else {
			if p < lo {
				lo = p
			}
			if end > hi {
				hi = end
			}
		}
	}
	if first {
		t.Skip("no non-empty slots to check")
	}

	// Every slot's Fields pointer must fall within [lo, hi).
	for _, slot := range slots {
		if len(slot.Fields) == 0 {
			continue
		}
		p := uintptr(unsafe.Pointer(&slot.Fields[0]))
		end := p + uintptr(len(slot.Fields))*4
		if p < lo || end > hi {
			t.Errorf("slot %d Fields [%x, %x) outside global span [%x, %x)",
				slot.Idx, p, end, lo, hi)
		}
	}
}

func TestBackingArrays_ComplexVariantsCorrect(t *testing.T) {
	entry := mustCompile(t, compositeTwo)
	root := entry.Core.Root()

	if len(root.Fields) != 1 {
		t.Fatalf("compositeTwo root: want 1 field, got %d", len(root.Fields))
	}
	cx := entry.Core.ComplexOf(root, root.Fields[0])
	if len(cx.Variants) < 2 {
		t.Fatalf("composite must have >=2 variants, got %d", len(cx.Variants))
	}
	// Each variant schemaIdx must round-trip through Variant().
	for i, vIdx := range cx.Variants {
		v := entry.Core.Variant(cx, i)
		if v.Idx != vIdx {
			t.Errorf("Variant(%d).Idx %d != Variants[%d] %d", i, v.Idx, i, vIdx)
		}
	}
}

// =============================================================================
// COMPLEX FIELDS — UNION AND COMPOSITE
// =============================================================================

func TestComplex_UnionKind(t *testing.T) {
	entry := mustCompile(t, allKinds)
	root := entry.Core.Root()
	fd := fieldsByName(entry, root)["contact"]
	cx := entry.Core.ComplexOf(root, fd)

	if cx.Kind != ir.ComplexUnion {
		t.Errorf("contact ComplexKind: want ComplexUnion, got %v", cx.Kind)
	}
	if len(cx.Variants) == 0 {
		t.Error("union must have at least one variant")
	}
}

func TestComplex_CompositeKind(t *testing.T) {
	entry := mustCompile(t, compositeTwo)
	root := entry.Core.Root()
	cx := entry.Core.ComplexOf(root, root.Fields[0])

	if cx.Kind != ir.ComplexComposite {
		t.Errorf("merged ComplexKind: want ComplexComposite, got %v", cx.Kind)
	}
}

func TestComplex_CompositeVariantCount(t *testing.T) {
	entry := mustCompile(t, compositeTwo)
	root := entry.Core.Root()
	cx := entry.Core.ComplexOf(root, root.Fields[0])

	if len(cx.Variants) != 2 {
		t.Errorf("compositeTwo: want 2 constituents, got %d", len(cx.Variants))
	}
}

func TestComplex_VariantSlotsAccessible(t *testing.T) {
	entry := mustCompile(t, compositeTwo)
	root := entry.Core.Root()
	cx := entry.Core.ComplexOf(root, root.Fields[0])

	for i := range cx.Variants {
		v := entry.Core.Variant(cx, i)
		if len(v.Fields) == 0 {
			t.Errorf("composite variant %d has no fields", i)
		}
	}
}

// =============================================================================
// DAG TRAVERSAL
// =============================================================================

func TestTraversal_CountFlat(t *testing.T) {
	entry := mustCompile(t, singleStringField)
	if n := countFields(entry); n != 1 {
		t.Errorf("singleStringField: want 1 reachable field, got %d", n)
	}
}

func TestTraversal_CountNested(t *testing.T) {
	// allKinds: 4 root fields + Address.street = 5 total.
	// tags → TagList → StatusEnum (no fields) and contact → Address (already
	// visited) add no new fields.
	entry := mustCompile(t, allKinds)
	if n := countFields(entry); n != 5 {
		t.Errorf("allKinds: want 5 reachable fields, got %d", n)
	}
}

func TestTraversal_CountComposite(t *testing.T) {
	// compositeTwo: merged + Part1.alpha + Part2.beta = 3.
	entry := mustCompile(t, compositeTwo)
	if n := countFields(entry); n != 3 {
		t.Errorf("compositeTwo: want 3 reachable fields, got %d", n)
	}
}

func TestTraversal_TerminatesOnRecursiveSchema(t *testing.T) {
	// Node.children → Node is a back-edge and must be marked terminal.
	// Without that the traversal would loop; a 2s deadline catches the bug.
	entry := mustCompile(t, recursiveTree)

	done := make(chan int, 1)
	go func() { done <- countFields(entry) }()

	select {
	case n := <-done:
		// root + Node.value + Node.children = 3.
		if n != 3 {
			t.Errorf("recursiveTree: want 3 reachable fields, got %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DAG traversal did not terminate — back-edge not marked terminal")
	}
}

func TestTraversal_BackEdgeIsTerminal(t *testing.T) {
	entry := mustCompile(t, recursiveTree)
	nodeSlot := entry.Core.Child(entry.Core.Root().Fields[0])

	children, ok := fieldsByName(entry, nodeSlot)["children"]
	if !ok {
		t.Fatal("Node.children not found")
	}
	if !children.Terminal() {
		t.Error("children (back-edge): Terminal want true")
	}
	if !children.IsLeaf() {
		t.Error("children (terminal): IsLeaf want true")
	}
}

func TestTraversal_NonRecursiveFieldNotTerminal(t *testing.T) {
	entry := mustCompile(t, recursiveTree)
	nodeSlot := entry.Core.Child(entry.Core.Root().Fields[0])

	value, ok := fieldsByName(entry, nodeSlot)["value"]
	if !ok {
		t.Fatal("Node.value not found")
	}
	if value.Terminal() {
		t.Error("value (non-recursive): Terminal want false")
	}
}

// =============================================================================
// RANDOM ACCESS
// =============================================================================

func TestRandomAccess_FieldMetaByDescriptor(t *testing.T) {
	entry := mustCompile(t, allKinds)
	dagVisit(entry, func(fd ir.FieldDescriptor) {
		meta := entry.Meta.Fields[fd.SchemaIdx()][fd.FieldIdx()]
		if meta.Name == "" {
			t.Errorf("schemaIdx=%d fieldIdx=%d: empty FieldMeta.Name",
				fd.SchemaIdx(), fd.FieldIdx())
		}
	})
}

func TestRandomAccess_SchemaMetaRootName(t *testing.T) {
	entry := mustCompile(t, allKinds)
	if entry.Meta.Schemas[0].Name != "Kinds" {
		t.Errorf("root SchemaMeta.Name: want Kinds, got %q", entry.Meta.Schemas[0].Name)
	}
}

func TestRandomAccess_SchemaMetaNestedNamesPopulated(t *testing.T) {
	entry := mustCompile(t, allKinds)
	core := entry.Core

	seen := map[uint8]bool{0: true}
	var check func(*ir.SchemaSlot)
	check = func(slot *ir.SchemaSlot) {
		for _, fd := range slot.Fields {
			if fd.IsLeaf() {
				continue
			}
			var visit func(*ir.SchemaSlot)
			visit = func(s *ir.SchemaSlot) {
				if seen[s.Idx] {
					return
				}
				seen[s.Idx] = true
				if entry.Meta.Schemas[s.Idx].Name == "" {
					t.Errorf("slot %d: empty SchemaMeta.Name", s.Idx)
				}
				check(s)
			}
			switch fd.Kind() {
			case ir.FieldKindObject, ir.FieldKindArray:
				visit(core.Child(fd))
			case ir.FieldKindComplex:
				cx := core.ComplexOf(slot, fd)
				for i := range cx.Variants {
					visit(core.Variant(cx, i))
				}
			}
		}
	}
	check(core.Root())
}

// =============================================================================
// METADATA
// =============================================================================

func TestMeta_RootSchemaMetadata(t *testing.T) {
	entry := mustCompile(t, withMetadata)
	md := entry.Meta.Schemas[0].Metadata
	if md == nil {
		t.Fatal("root SchemaMeta.Metadata: want non-nil")
	}
	if md["owner"] != "team-alpha" {
		t.Errorf("root metadata[owner]: want team-alpha, got %v", md["owner"])
	}
}

func TestMeta_NestedSchemaMetadata(t *testing.T) {
	entry := mustCompile(t, withMetadata)
	fd := fieldsByName(entry, entry.Core.Root())["sub"]
	child := entry.Core.Child(fd)
	md := entry.Meta.Schemas[child.Idx].Metadata
	if md == nil {
		t.Fatal("Sub SchemaMeta.Metadata: want non-nil")
	}
	if md["tag"] != "internal" {
		t.Errorf("Sub metadata[tag]: want internal, got %v", md["tag"])
	}
}

func TestMeta_RootVersion(t *testing.T) {
	entry := mustCompile(t, singleStringField)
	if entry.Meta.Schemas[0].Version != "1.0" {
		t.Errorf("SchemaMeta.Version: want 1.0, got %q", entry.Meta.Schemas[0].Version)
	}
}

func TestMeta_FieldUUIDPreserved(t *testing.T) {
	// The FieldMeta.ID must equal the UUID v7 used as the field key in JSON.
	entry := mustCompile(t, singleStringField)
	fd := entry.Core.Root().Fields[0]
	m := entry.Meta.Fields[fd.SchemaIdx()][fd.FieldIdx()]
	// UUID "019ca98f-1001-7000-8000-000000000001" → non-zero [16]byte.
	var zero [16]byte
	if m.ID == zero {
		t.Error("FieldMeta.ID: want non-zero UUID, got zero")
	}
}

// =============================================================================
// CONSTRAINTS
// =============================================================================

func TestConstraint_CompiledAtRoot(t *testing.T) {
	entry := mustCompile(t, withConstraint)
	if len(entry.Constraints) != 1 {
		t.Fatalf("root constraints: want 1, got %d", len(entry.Constraints))
	}
}

func TestConstraint_IsLeafNotGroup(t *testing.T) {
	entry := mustCompile(t, withConstraint)
	if entry.Constraints[0].IsGroup {
		t.Error("top-level constraint must not be a group")
	}
}

func TestConstraint_Predicate(t *testing.T) {
	entry := mustCompile(t, withConstraint)
	if entry.Constraints[0].Predicate != "isEmail" {
		t.Errorf("Predicate: want isEmail, got %q", entry.Constraints[0].Predicate)
	}
}

func TestConstraint_FieldPathResolvesToEmailField(t *testing.T) {
	entry := mustCompile(t, withConstraint)
	cc := entry.Constraints[0]

	if len(cc.Fields) != 1 {
		t.Fatalf("constraint Fields: want 1 path, got %d", len(cc.Fields))
	}
	path := cc.Fields[0]
	if len(path) != 1 {
		t.Fatalf("email path depth: want 1, got %d", len(path))
	}
	name := entry.Meta.Fields[path[0].SchemaIdx()][path[0].FieldIdx()].Name
	if name != "email" {
		t.Errorf("constraint path terminal: want email, got %q", name)
	}
}

func TestConstraint_MetaParallel(t *testing.T) {
	entry := mustCompile(t, withConstraint)
	if len(entry.Meta.Constraints) != len(entry.Constraints) {
		t.Errorf("Meta.Constraints len %d != Constraints len %d",
			len(entry.Meta.Constraints), len(entry.Constraints))
	}
	if entry.Meta.Constraints[0].Name != "emailFormat" {
		t.Errorf("ConstraintMeta.Name: want emailFormat, got %q",
			entry.Meta.Constraints[0].Name)
	}
}

func TestConstraint_NestedOnSlot(t *testing.T) {
	entry := mustCompile(t, nestedConstraint)

	if len(entry.Constraints) != 0 {
		t.Errorf("root constraints: want 0, got %d", len(entry.Constraints))
	}

	fd := fieldsByName(entry, entry.Core.Root())["profile"]
	profileSlot := entry.Core.Child(fd)

	nested, ok := entry.NestedConstraints[profileSlot.Idx]
	if !ok {
		t.Fatalf("no nested constraint for Profile slot %d", profileSlot.Idx)
	}
	if len(nested) != 1 {
		t.Fatalf("Profile nested constraints: want 1, got %d", len(nested))
	}
	if nested[0].Predicate != "isSlug" {
		t.Errorf("Profile constraint Predicate: want isSlug, got %q", nested[0].Predicate)
	}
}

// =============================================================================
// INDEXES
// =============================================================================

func TestIndex_CompiledCount(t *testing.T) {
	entry := mustCompile(t, withIndexes)
	if len(entry.Indexes) != 2 {
		t.Fatalf("index count: want 2, got %d", len(entry.Indexes))
	}
}

func TestIndex_TypeAndOrder(t *testing.T) {
	entry := mustCompile(t, withIndexes)

	byName := make(map[string]ir.CompiledIndex)
	for i, ci := range entry.Indexes {
		byName[entry.Meta.Indexes[i].Name] = ci
	}

	uid, ok := byName["usernameIdx"]
	if !ok {
		t.Fatal("usernameIdx not found")
	}
	if uid.Type != ir.IndexTypeUnique {
		t.Errorf("usernameIdx Type: want Unique, got %v", uid.Type)
	}
	if uid.Order != ir.IndexOrderAsc {
		t.Errorf("usernameIdx Order: want Asc, got %v", uid.Order)
	}

	sid, ok := byName["scoreIdx"]
	if !ok {
		t.Fatal("scoreIdx not found")
	}
	if sid.Order != ir.IndexOrderDesc {
		t.Errorf("scoreIdx Order: want Desc, got %v", sid.Order)
	}
}

func TestIndex_FieldPathsResolved(t *testing.T) {
	entry := mustCompile(t, withIndexes)
	for i, ci := range entry.Indexes {
		name := entry.Meta.Indexes[i].Name
		if len(ci.Fields) == 0 {
			t.Errorf("index %q: no field paths", name)
			continue
		}
		for _, rp := range ci.Fields {
			if len(rp) == 0 {
				t.Errorf("index %q: empty ResolvedPath", name)
			}
		}
	}
}

func TestIndex_MetaParallel(t *testing.T) {
	entry := mustCompile(t, withIndexes)
	if len(entry.Meta.Indexes) != len(entry.Indexes) {
		t.Errorf("Meta.Indexes len %d != Indexes len %d",
			len(entry.Meta.Indexes), len(entry.Indexes))
	}
}

// =============================================================================
// REGISTRY
// =============================================================================

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := ir.NewRegistry()
	entry := mustCompile(t, singleStringField)

	var id [16]byte
	id[0], id[15] = 0xAB, 0xCD
	reg.Register(id, entry)

	got, ok := reg.Get(id)
	if !ok {
		t.Fatal("Get after Register: not found")
	}
	if got != entry {
		t.Error("Get returned a different pointer than was registered")
	}
}

func TestRegistry_MissReturnsNotOk(t *testing.T) {
	reg := ir.NewRegistry()
	var id [16]byte
	_, ok := reg.Get(id)
	if ok {
		t.Error("Get on empty registry: want not-ok, got ok")
	}
}

func TestRegistry_OverwriteIsAllowed(t *testing.T) {
	reg := ir.NewRegistry()
	e1 := mustCompile(t, singleStringField)
	e2 := mustCompile(t, allFlags)

	var id [16]byte
	reg.Register(id, e1)
	reg.Register(id, e2)

	got, _ := reg.Get(id)
	if got != e2 {
		t.Error("second Register should overwrite first")
	}
}

// =============================================================================
// COMPILE ERROR CASES
// =============================================================================

func TestCompileError_BadFieldUUID(t *testing.T) {
	mustFail(t, `{
	  "version":"1.0","name":"X","schemas":{},
	  "fields":{"not-a-uuid":{"name":"x","type":"string"}}
	}`)
}

func TestCompileError_BadSchemaUUID(t *testing.T) {
	mustFail(t, `{
	  "version":"1.0","name":"X","fields":{},
	  "schemas":{"not-a-uuid":{"name":"E","type":"enum","values":["a"]}}
	}`)
}

func TestCompileError_DuplicateFieldUUID(t *testing.T) {
	// Same UUID appears both as a field inside a nested schema and as a root field.
	mustFail(t, `{
	  "version":"1.0","name":"X",
	  "schemas":{
	    "019ca98f-9001-7000-8000-000000000001":{
	      "name":"S",
	      "fields":{
	        "019ca98f-9002-7000-8000-000000000002":{"name":"a","type":"string"}
	      }
	    }
	  },
	  "fields":{
	    "019ca98f-9002-7000-8000-000000000002":{"name":"b","type":"string"}
	  }
	}`)
}

func TestCompileError_BothFieldsAndType(t *testing.T) {
	mustFail(t, `{
	  "version":"1.0","name":"X","fields":{},
	  "schemas":{
	    "019ca98f-9003-7000-8000-000000000003":{
	      "name":"Conflict","type":"enum","values":["a"],
	      "fields":{"019ca98f-9004-7000-8000-000000000004":{"name":"x","type":"string"}}
	    }
	  }
	}`)
}

func TestConstraint_TopLevelGroup(t *testing.T) {
	entry := mustCompile(t, `{
	  "version":"1.0","name":"X","schemas":{},
	  "fields":{"019ca98f-9005-7000-8000-000000000005":{"name":"a","type":"string"}},
	  "constraints":{
	    "019ca98f-9006-7000-8000-000000000006":{
	      "name":"validGroup","operator":"and","rules":[
	        {"predicate":"notEmpty","fields":["a"]}
	      ]
	    }
	  }
	}`)
	if len(entry.Constraints) != 1 {
		t.Fatalf("constraints count: want 1, got %d", len(entry.Constraints))
	}
	cc := entry.Constraints[0]
	if !cc.IsGroup {
		t.Error("expected constraint to be a group")
	}
	if len(cc.Children) != 1 {
		t.Fatalf("expected 1 child rule, got %d", len(cc.Children))
	}
}

func TestCompileError_SchemaLimitExceeded(t *testing.T) {
	// 128 nested schemas pushes the total (nested + root) to 129, exceeding the
	// 128-slot maximum. The compiler must reject this.
	hexChar := func(n int) byte {
		if n < 10 {
			return byte('0' + n)
		}
		return byte('a' + n - 10)
	}

	var sb strings.Builder
	sb.WriteString(`{"version":"1.0","name":"Overflow","fields":{},"schemas":{`)
	for i := 0; i < 128; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		// Encode i into two hex digits at positions 12-13 and 33-34 of the UUID.
		uid := []byte("019ca98f-aa00-7000-8000-0000000000ff")
		uid[12] = hexChar((i >> 4) & 0xF)
		uid[13] = hexChar(i & 0xF)
		uid[33] = hexChar((i >> 4) & 0xF)
		uid[34] = hexChar(i & 0xF)
		sb.WriteString(`"` + string(uid) + `":{"name":"S","type":"enum","values":["x"]}`)
	}
	sb.WriteString(`}}`)

	mustFail(t, sb.String())
}

// =============================================================================
// INTEGRATION — FULL FEATURE SCHEMA
// =============================================================================

func TestIntegration_Compiles(t *testing.T) {
	mustCompile(t, integrationSchema)
}

func TestIntegration_RootFieldCount(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	// id, profile, tree = 3.
	if len(entry.Core.Root().Fields) != 3 {
		t.Errorf("root fields: want 3, got %d", len(entry.Core.Root().Fields))
	}
}

func TestIntegration_SchemaVersion(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	if entry.Meta.Schemas[0].Version != "2.0" {
		t.Errorf("version: want 2.0, got %q", entry.Meta.Schemas[0].Version)
	}
}

func TestIntegration_SchemaConcrete(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	if !entry.Meta.Schemas[0].Concrete {
		t.Error("concrete: want true")
	}
}

func TestIntegration_RootMetadata(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	md := entry.Meta.Schemas[0].Metadata
	if md == nil {
		t.Fatal("root metadata: want non-nil")
	}
	if md["owner"] != "core-team" {
		t.Errorf("metadata[owner]: want core-team, got %v", md["owner"])
	}
}

func TestIntegration_NestedSlotMetadata(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	fd := fieldsByName(entry, entry.Core.Root())["profile"]
	slot := entry.Core.Child(fd)
	md := entry.Meta.Schemas[slot.Idx].Metadata
	if md == nil {
		t.Fatal("Profile metadata: want non-nil")
	}
	if md["tier"] != "user" {
		t.Errorf("Profile metadata[tier]: want user, got %v", md["tier"])
	}
}

func TestIntegration_ProfileFieldsAccessible(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	profileSlot := entry.Core.Child(fieldsByName(entry, entry.Core.Root())["profile"])
	byName := fieldsByName(entry, profileSlot)

	for _, name := range []string{"handle", "contact", "tags", "address", "identity"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("Profile.%s not found", name)
		}
	}
}

func TestIntegration_UnionVariantCount(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	profileSlot := entry.Core.Child(fieldsByName(entry, entry.Core.Root())["profile"])
	fd := fieldsByName(entry, profileSlot)["address"]
	cx := entry.Core.ComplexOf(profileSlot, fd)

	if cx.Kind != ir.ComplexUnion {
		t.Errorf("address ComplexKind: want Union, got %v", cx.Kind)
	}
	if len(cx.Variants) != 2 {
		t.Errorf("address union variants: want 2, got %d", len(cx.Variants))
	}
}

func TestIntegration_CompositeConstituentCount(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	profileSlot := entry.Core.Child(fieldsByName(entry, entry.Core.Root())["profile"])
	fd := fieldsByName(entry, profileSlot)["identity"]
	cx := entry.Core.ComplexOf(profileSlot, fd)

	if cx.Kind != ir.ComplexComposite {
		t.Errorf("identity ComplexKind: want Composite, got %v", cx.Kind)
	}
	if len(cx.Variants) != 2 {
		t.Errorf("identity constituents: want 2, got %d", len(cx.Variants))
	}
}

func TestIntegration_RecursiveNodeTerminates(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	done := make(chan struct{}, 1)
	go func() { countFields(entry); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DAG traversal did not terminate")
	}
}

func TestIntegration_RootConstraint(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	if len(entry.Constraints) != 1 {
		t.Fatalf("root constraints: want 1, got %d", len(entry.Constraints))
	}
	if entry.Constraints[0].Predicate != "isUuidV7" {
		t.Errorf("root constraint predicate: want isUuidV7, got %q",
			entry.Constraints[0].Predicate)
	}
}

func TestIntegration_RootConstraintPathTerminal(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	cc := entry.Constraints[0]
	if len(cc.Fields) == 0 || len(cc.Fields[0]) == 0 {
		t.Fatal("root constraint has no field path")
	}
	terminal := cc.Fields[0][len(cc.Fields[0])-1]
	name := entry.Meta.Fields[terminal.SchemaIdx()][terminal.FieldIdx()].Name
	if name != "id" {
		t.Errorf("root constraint path terminal: want id, got %q", name)
	}
}

func TestIntegration_RootIndex(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	if len(entry.Indexes) != 1 {
		t.Fatalf("root indexes: want 1, got %d", len(entry.Indexes))
	}
	ci := entry.Indexes[0]
	if ci.Type != ir.IndexTypePrimary {
		t.Errorf("primaryId Type: want Primary, got %v", ci.Type)
	}
	if ci.Order != ir.IndexOrderAsc {
		t.Errorf("primaryId Order: want Asc, got %v", ci.Order)
	}
	if !ci.Unique {
		t.Error("primaryId Unique: want true")
	}
}

func TestIntegration_RootIndexPathTerminal(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	ci := entry.Indexes[0]
	if len(ci.Fields) == 0 || len(ci.Fields[0]) == 0 {
		t.Fatal("primaryId index has no resolved path")
	}
	terminal := ci.Fields[0][len(ci.Fields[0])-1]
	name := entry.Meta.Fields[terminal.SchemaIdx()][terminal.FieldIdx()].Name
	if name != "id" {
		t.Errorf("primaryId terminal field: want id, got %q", name)
	}
}

func TestIntegration_NestedConstraintOnProfile(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	profileSlot := entry.Core.Child(fieldsByName(entry, entry.Core.Root())["profile"])

	nested, ok := entry.NestedConstraints[profileSlot.Idx]
	if !ok {
		t.Fatalf("no nested constraints for Profile slot %d", profileSlot.Idx)
	}
	if len(nested) != 1 {
		t.Fatalf("Profile nested constraints: want 1, got %d", len(nested))
	}
	if nested[0].Predicate != "minLength" {
		t.Errorf("Profile constraint predicate: want minLength, got %q", nested[0].Predicate)
	}
}

func TestIntegration_AllMetaFieldsNamed(t *testing.T) {
	entry := mustCompile(t, integrationSchema)
	dagVisit(entry, func(fd ir.FieldDescriptor) {
		m := entry.Meta.Fields[fd.SchemaIdx()][fd.FieldIdx()]
		if m.Name == "" {
			t.Errorf("schemaIdx=%d fieldIdx=%d: empty FieldMeta.Name",
				fd.SchemaIdx(), fd.FieldIdx())
		}
	})
}

func TestCompileError_InvalidPathTraversal(t *testing.T) {
	// Trying to traverse into a simple string field "a".
	mustFail(t, `{
	  "version":"1.0","name":"X",
	  "fields":{
	    "019ca98f-b001-7000-8000-000000000001":{"name":"a","type":"string"}
	  },
	  "constraints":{
	    "019ca98f-b002-7000-8000-000000000002":{
	      "name":"badPath","predicate":"notEmpty","fields":["a.invalid"]
	    }
	  }
	}`)
}

func TestConstraint_RecursivePath(t *testing.T) {
	const schema = `{
	  "version": "1.0",
	  "name":    "Recursive",
	  "schemas": {
		"019ca98f-a001-7000-8000-000000000001": {
		  "name": "Node",
		  "fields": {
			"019ca98f-a002-7000-8000-000000000002": { "name": "val", "type": "string" },
			"019ca98f-a003-7000-8000-000000000003": {
			  "name": "child", "type": "object",
			  "schema": { "id": "019ca98f-a001-7000-8000-000000000001" }
			}
		  }
		}
	  },
	  "fields": {
		"019ca98f-a010-7000-8000-000000000010": {
		  "name": "root", "type": "object",
		  "schema": { "id": "019ca98f-a001-7000-8000-000000000001" }
		}
	  },
	  "constraints": {
		"019ca98f-a020-7000-8000-000000000020": {
		  "name": "recursePath", "predicate": "notEmpty", "fields": ["root.child.val"]
		}
	  }
	}`

	entry := mustCompile(t, schema)
	if len(entry.Constraints) != 1 {
		t.Fatalf("constraints count: want 1, got %d", len(entry.Constraints))
	}
	cc := entry.Constraints[0]
	if len(cc.Fields[0]) != 3 {
		t.Errorf("path length: want 3, got %d", len(cc.Fields[0]))
	}
}
