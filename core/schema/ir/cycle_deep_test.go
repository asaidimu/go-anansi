package ir

import "testing"

// cycle_deep_test.go tests complex cyclic schemas: multiple paths to the same
// target, nested cycles, and deep back-region traversals.

var complexCycleSchema = []byte(`{
  "name": "Graph",
  "version": "1.0.0",
  "fields": {
    "019ca000-0001-7001-810d-141b22293037": {
      "name": "a",
      "type": "object",
      "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" }
    }
  },
  "schemas": {
    "019ca000-0010-7010-90d0-d7dee5ecf3fa": {
      "name": "NodeA",
      "fields": {
        "019ca000-0011-7011-91dd-e4ebf2f90007": { "name": "valA", "type": "string" },
        "019ca000-0012-7012-92ea-f1f8ff060d14": {
          "name": "toB",
          "type": "object",
          "schema": { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" }
        }
      }
    },
    "019ca000-0020-7020-a0a0-a7aeb5bcc3ca": {
      "name": "NodeB",
      "fields": {
        "019ca000-0021-7021-a1ad-b4bbc2c9d0d7": { "name": "valB", "type": "integer" },
        "019ca000-0022-7022-a2ba-c1c8cfd6dde4": {
          "name": "toA",
          "type": "object",
          "schema": { "id": "019ca000-0010-7010-90d0-d7dee5ecf3fa" }
        },
        "019ca000-0023-7023-a3c7-ced5dce3eaf1": {
          "name": "toB",
          "type": "object",
          "schema": { "id": "019ca000-0020-7020-a0a0-a7aeb5bcc3ca" }
        }
      }
    }
  }
}`)

func TestAddress_DeepComplexCycle(t *testing.T) {
	cs := mustCompile(complexCycleSchema, nil)
	as := cs.AddressSpace

	// Verify NodeA and NodeB are cyclic targets.
	var idxA, idxB uint8
	for i, m := range cs.Meta {
		if m != nil {
			if m.Name == "NodeA" {
				idxA = i
			} else if m.Name == "NodeB" {
				idxB = i
			}
		}
	}

	if as.BlockBases[idxA] == 0 {
		t.Error("NodeA should be a cyclic target")
	}
	if as.BlockBases[idxB] == 0 {
		t.Error("NodeB should be a cyclic target")
	}

	// ── Test Paths ──────────────────────────────────────────────────────────

	paths := []string{
		"a.valA",            // Front region
		"a.toB.valB",        // Front region
		"a.toB.toA.valA",    // Depth 1 back-edge (NodeB.toA → NodeA)
		"a.toB.toB.valB",    // Depth 1 back-edge (NodeB.toB → NodeB)
		"a.toB.toA.toB.valB", // Depth 2 back-edge (NodeB.toA.toB → NodeB)
	}

	seenIDs := make(map[int32]string)
	for _, p := range paths {
		dp, err := cs.Address(p)
		if err != nil {
			t.Errorf("Address(%s) failed: %v", p, err)
			continue
		}
		if prev, exists := seenIDs[dp.ID()]; exists {
			t.Errorf("Ordinal collision: %s and %s share %d", prev, p, dp.ID())
		}
		seenIDs[dp.ID()] = p
	}

	// ── Verify Depth Accumulation ───────────────────────────────────────────

	// "a.toB.valB" is front region.
	dpFront, _ := cs.Address("a.toB.valB")
	if uint32(dpFront.ID()) > as.FrontSize {
		t.Errorf("a.toB.valB should be in front region (ID=%d, FrontSize=%d)", dpFront.ID(), as.FrontSize)
	}

	// "a.toB.toA.valA" enters NodeA block at depth 1.
	dpD1, _ := cs.Address("a.toB.toA.valA")
	if uint32(dpD1.ID()) <= as.FrontSize {
		t.Errorf("a.toB.toA.valA should be in back region")
	}

	// "a.toB.toA.toB.toA.valA" enters NodeA block at depth 2.
	dpD2, _ := cs.Address("a.toB.toA.toB.toA.valA")
	if dpD2.ID() == dpD1.ID() {
		t.Errorf("Depth 1 and Depth 2 back-edge addresses must differ: %d", dpD1.ID())
	}
}

func TestTerminalWalk_Cycles(t *testing.T) {
	cs := mustCompile(complexCycleSchema, nil)

	// NodeA has toB which is NOT a back-edge from NodeA's perspective in DFS.
	// But NodeB has toA which IS a back-edge.
	// TerminalWalk should visit NodeA, NodeB, valA, valB, but NOT recurse
	// into back-edges.

	fieldsSeen := make(map[string]bool)
	TerminalWalk(cs, 0, func(fd uint32) {
		owner := ExtractOwnerSchema(fd)
		if m := cs.Meta[owner]; m != nil {
			for ffd, fm := range m.Fields {
				if ffd == fd {
					fieldsSeen[fm.Name] = true
				}
			}
		}
	})

	expected := []string{"a", "valA", "toB", "valB", "toA", "toB"}
	for _, exp := range expected {
		if !fieldsSeen[exp] {
			t.Errorf("TerminalWalk missed field %q", exp)
		}
	}
}
