package ir

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

func TestAddress_ThroughUnionField(t *testing.T) {
	cs := mustCompile(unionSchema, nil)

	// VariantA has field "typeA"
	dpA, err := cs.Address("payload.typeA")
	if err != nil {
		t.Fatalf("Address(payload.typeA) failed: %v", err)
	}
	if dpA.Type() != document.TypeString {
		t.Errorf("expected TypeString, got %v", dpA.Type())
	}

	// VariantB has field "typeB"
	dpB, err := cs.Address("payload.typeB")
	if err != nil {
		t.Fatalf("Address(payload.typeB) failed: %v", err)
	}
	if dpB.Type() != document.TypeInt {
		t.Errorf("expected TypeInt, got %v", dpB.Type())
	}
}

func TestFullWalk_IncludesNamedTypeSchemas(t *testing.T) {
	// We need a schema where a field points to a named union schema.
	// unionSchema in testdata uses type: union directly on the field.
	// We'll create a custom fixture.
	srcJSON := `{
		"name": "Custom",
		"version": "1.0.0",
		"fields": {
			"f1": {
				"name": "ptr",
				"type": "object",
				"schema": { "id": "u1" }
			}
		},
		"schemas": {
			"u1": {
				"name": "NamedUnion",
				"type": "union",
				"schema": [
					{ "id": "v1" },
					{ "id": "v2" }
				]
			},
			"v1": {
				"name": "V1",
				"fields": {
					"vf1": { "name": "f1", "type": "string" }
				}
			},
			"v2": {
				"name": "V2",
				"fields": {
					"vf2": { "name": "f2", "type": "integer" }
				}
			}
		}
	}`

	src, err := Parse([]byte(srcJSON))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	cs, err := Compile(src, nil)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// FullWalk should visit fields in V1 and V2.
	fieldsSeen := make(map[string]bool)
	FullWalk(cs, 0, func(fd uint32) {
		owner := ExtractOwnerSchema(fd)
		meta := cs.Meta[owner]
		for fdInMeta, fm := range meta.Fields {
			if fdInMeta == fd {
				fieldsSeen[fm.Name] = true
			}
		}
	})

	if !fieldsSeen["f1"] {
		t.Error("FullWalk missed field f1 in variant V1")
	}
	if !fieldsSeen["f2"] {
		t.Error("FullWalk missed field f2 in variant V2")
	}

	// Address() should also work through the named union.
	dp, err := cs.Address("ptr.f1")
	if err != nil {
		t.Fatalf("Address(ptr.f1) failed: %v", err)
	}
	if dp.Type() != document.TypeString {
		t.Errorf("expected TypeString for ptr.f1, got %v", dp.Type())
	}
}
