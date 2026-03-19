package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

// testdata_test.go provides shared JSON fixtures and helper functions used
// across all test files.

const (
	nestedAddressSchemaUUID = "019ca000-0010-7010-90d0-d7dee5ecf3fa"
)

var flatSchema = []byte(`{
  "name": "Flat",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "name",    "type": "string",  "required": true },
    "019ca000-0002-7002-821a-21282f363d44": { "name": "desc",    "type": "string" },
    "019ca000-0003-7003-8327-2e353c434a51": { "name": "version", "type": "string",  "required": true }
  }
}`)

var nestedObjectSchema = []byte(`{
  "name": "Person",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7030-b070-777e858c939a": {
      "name": "address",
      "type": "object",
      "required": true,
      "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" }
    }
  },
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "Address",
      "fields": {
        "019ca000-0011-7011-91dd-e4ebf2f90007": { "name": "street", "type": "string", "required": true },
        "019ca000-0012-7012-92ea-f1f8ff060d14": { "name": "city",   "type": "string", "required": true }
      }
    }
  }
}`)

var enumSchema = []byte(`{
  "name": "Order",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7030-b070-777e858c939a": {
      "name": "status",
      "type": "enum",
      "schema": { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" }
    }
  },
  "schemas": {
    "019ca000-0020-7020-a0a0-a7aeb5bcc3ca": {
      "name": "StatusEnum",
      "type": "enum",
      "values": ["pending", "active", "closed"]
    }
  }
}`)

var cycleSchema = []byte(`{
  "name": "Tree",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "label", "type": "string" },
    "019ca000-0030-7030-b070-777e858c939a": {
      "name": "node",
      "type": "object",
      "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" }
    }
  },
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "Node",
      "fields": {
        "019ca000-0011-7011-91dd-e4ebf2f90007": { "name": "value", "type": "string" },
        "019ca000-0012-7012-92ea-f1f8ff060d14": {
          "name": "next",
          "type": "object",
          "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" }
        }
      }
    }
  }
}`)

var unionSchema = []byte(`{
  "name": "Event",
  "version": "1.0.0",
  "fields": {
    "019ca000-0030-7030-b070-777e858c939a": {
      "name": "payload",
      "type": "union",
      "schema": [
        { "id": "019ca000-0040-7040-8040-474e555c636a" },
        { "id": "019ca000-0041-7041-814d-545b62697077" }
      ]
    }
  },
  "schemas": {
    "019ca000-0040-7040-8040-474e555c636a": {
      "name": "VariantA",
      "fields": {
        "019ca000-0001-7001-810d-141b22293037": { "name": "typeA", "type": "string" }
      }
    },
    "019ca000-0041-7041-814d-545b62697077": {
      "name": "VariantB",
      "fields": {
        "019ca000-0002-7002-821a-21282f363d44": { "name": "typeB", "type": "integer" }
      }
    }
  }
}`)

var compositeSchema = []byte(`{
  "name": "CompositeTest",
  "version": "1.0.0",
  "fields": {
    "019ca000-0000-7000-b000-000000000001": {
      "name": "comp", "type": "composite", "schema": [{ "id": "019ca000-0000-7000-b000-000000000002" }]
    }
  },
  "schemas": {
    "019ca000-0000-7000-b000-000000000002": {
      "name": "S",
      "fields": {
        "019ca000-0000-7000-b000-000000000003": { "name": "f1", "type": "string" },
        "019ca000-0000-7000-b000-000000000004": { "name": "f2", "type": "integer" }
      }
    }
  }
}`)

var indexedSchema = []byte(`{
  "name": "Product",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "sku",   "type": "string", "required": true }
  },
  "indexes": {
    "019ca000-0050-7050-9010-171e252c333a": {
      "name": "sku_index",
      "type": "unique",
      "fields": ["sku"]
    }
  }
}`)

var constrainedSchema = []byte(`{
  "name": "Account",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": { "name": "email", "type": "string", "required": true }
  },
  "constraints": {
    "019ca000-0060-7060-a0e0-e7eef5fc030a": {
      "name": "validEmail",
      "predicate": "isEmail",
      "fields": ["email"]
    }
  }
}`)

var complexCycleSchema = []byte(`{
  "name": "Cycle3",
  "version": "1.0.0",
  "fields": {
    "019ca000-0000-7000-b000-000000000001": { "name": "start", "type": "object", "schema": { "id": "019ca000-0000-7000-b000-00000000000A" } }
  },
  "schemas": {
    "019ca000-0000-7000-b000-00000000000A": {
      "name": "A",
      "fields": {
        "019ca000-0000-7000-b000-000000000002": { "name": "value", "type": "string" },
        "019ca000-0000-7000-b000-000000000003": { "name": "next",  "type": "object", "schema": { "id": "019ca000-0000-7000-b000-00000000000B" } }
      }
    },
    "019ca000-0000-7000-b000-00000000000B": {
      "name": "B",
      "fields": {
        "019ca000-0000-7000-b000-000000000004": { "name": "value", "type": "string" },
        "019ca000-0000-7000-b000-000000000005": { "name": "next",  "type": "object", "schema": { "id": "019ca000-0000-7000-b000-00000000000C" } }
      }
    },
    "019ca000-0000-7000-b000-00000000000C": {
      "name": "C",
      "fields": {
        "019ca000-0000-7000-b000-000000000006": { "name": "value", "type": "string" },
        "019ca000-0000-7000-b000-000000000007": { "name": "next",  "type": "object", "schema": { "id": "019ca000-0000-7000-b000-00000000000A" } }
      }
    }
  }
}`)

var complexConstraintSchema = []byte(`{
  "name": "Complex",
  "version": "1.0.0",
  "fields": {
    "019ca000-0000-7000-b000-000000000001": { "name": "a", "type": "integer" },
    "019ca000-0000-7000-b000-000000000002": { "name": "b", "type": "boolean" },
    "019ca000-0000-7000-b000-000000000003": { "name": "c", "type": "string" }
  },
  "constraints": {
    "019ca000-0000-7000-b000-000000000004": {
      "name": "complex",
      "predicate": "isEmail",
      "fields": ["c"]
    }
  }
}`)

// ── Helpers ────────────────────────────────────────────────────────────────

// binaryTreeSchema exercises the multi-back-edge case: a BTree schema where
// both "left" and "right" are back-edges to BTree itself. This verifies that
// BackEdgeOrdinal assigns distinct 1-based slots (sorted by field UUID) and
// that Address() produces different ordinals for each back-edge field.
//
// Field UUID lex order within BTree: left(010) < right(011) < value(012)
// → BackEdgeOrdinal[BTree][left]=1, BackEdgeOrdinal[BTree][right]=2
// → BlockSize[BTree] = AcyclicSubtreeSize(3) × 2 back-edges = 6
var binaryTreeSchema = []byte(`{
  "name": "BinaryTree",
  "version": "1.0.0",
  "fields": {
    "019ca000-0000-7000-b000-000000000001": {
      "name": "root", "type": "object",
      "schema": { "id": "019ca000-0000-7000-b000-000000000020" }
    }
  },
  "schemas": {
    "019ca000-0000-7000-b000-000000000020": {
      "name": "BTree",
      "fields": {
        "019ca000-0000-7000-b000-000000000010": {
          "name": "left",  "type": "object",
          "schema": { "id": "019ca000-0000-7000-b000-000000000020" }
        },
        "019ca000-0000-7000-b000-000000000011": {
          "name": "right", "type": "object",
          "schema": { "id": "019ca000-0000-7000-b000-000000000020" }
        },
        "019ca000-0000-7000-b000-000000000012": { "name": "value", "type": "string" }
      }
    }
  }
}`)

// twoCycleSchema exercises the multiple-cyclic-targets case: two independent
// self-cycles (P and Q), each with their own back block. This verifies that
// block base addresses are strictly decreasing and non-overlapping when two
// cyclic target schemas exist.
//
// Schema UUID lex order: ...0000a0 < ...0000b0 (a < b)
// → P gets the higher block base (allocated first from addressSpaceMax downward)
// → BlockBases[P] > BlockBases[Q] and BlockBases[P] - BlockSize[P] == BlockBases[Q]
var twoCycleSchema = []byte(`{
  "name": "TwoCycles",
  "version": "1.0.0",
  "fields": {
    "019ca000-0000-7000-b000-000000000001": {
      "name": "p", "type": "object",
      "schema": { "id": "019ca000-0000-7000-b000-0000000000a0" }
    },
    "019ca000-0000-7000-b000-000000000002": {
      "name": "q", "type": "object",
      "schema": { "id": "019ca000-0000-7000-b000-0000000000b0" }
    }
  },
  "schemas": {
    "019ca000-0000-7000-b000-0000000000a0": {
      "name": "P",
      "fields": {
        "019ca000-0000-7000-b000-0000000000a1": {
          "name": "next", "type": "object",
          "schema": { "id": "019ca000-0000-7000-b000-0000000000a0" }
        },
        "019ca000-0000-7000-b000-0000000000a2": { "name": "val", "type": "string" }
      }
    },
    "019ca000-0000-7000-b000-0000000000b0": {
      "name": "Q",
      "fields": {
        "019ca000-0000-7000-b000-0000000000b1": {
          "name": "next", "type": "object",
          "schema": { "id": "019ca000-0000-7000-b000-0000000000b0" }
        },
        "019ca000-0000-7000-b000-0000000000b2": { "name": "val", "type": "string" }
      }
    }
  }
}`)



func mustParse(src []byte) *ir.SourceSchema {
	ss, err := ir.Parse(src)
	if err != nil {
		panic("mustParse: " + err.Error())
	}
	return ss
}

func mustCompile(src []byte, predicates ir.PredicateMap) *ir.Schema {
	ss := mustParse(src)
	cs, err := ir.Compile(ss, predicates)
	if err != nil {
		panic("mustCompile: " + err.Error())
	}
	return cs
}

func mustCompileAny(t *testing.T, src []byte) *ir.Schema {
	return mustCompile(src, nil)
}

func mustCompileWithStubPredicate(src []byte, name string) *ir.Schema {
	return mustCompile(src, ir.PredicateMap{name: func(_ *document.Document, _ []document.DocumentKey, _ any) bool { return true }})
}
