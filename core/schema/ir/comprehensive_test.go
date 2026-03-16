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
			"019ca000-0100-7100-8000-070e151c232a": {
				"name": "ptr",
				"type": "object",
				"schema": { "id": "019ca000-0110-7110-90d0-d7dee5ecf3fa" }
			}
		},
		"schemas": {
			"019ca000-0110-7110-90d0-d7dee5ecf3fa": {
				"name": "NamedUnion",
				"type": "union",
				"schema": [
					{ "id": "019ca000-0120-7120-a0a0-a7aeb5bcc3ca" },
					{ "id": "019ca000-0121-7121-a1ad-b4bbc2c9d0d7" }
				]
			},
			"019ca000-0120-7120-a0a0-a7aeb5bcc3ca": {
				"name": "V1",
				"fields": {
					"019ca000-0130-7130-b070-777e858c939a": { "name": "f1", "type": "string" }
				}
			},
			"019ca000-0121-7121-a1ad-b4bbc2c9d0d7": {
				"name": "V2",
				"fields": {
					"019ca000-0131-7131-b17d-848b9299a0a7": { "name": "f2", "type": "integer" }
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
