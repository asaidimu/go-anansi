package ir

import "testing"

func TestWalk_Divergence(t *testing.T) {
	// A schema with a back-edge. TerminalWalk should stop at the back-edge,
	// while FullWalk should visit every schema once.
	src := []byte(`{
	  "name": "WalkTest",
	  "version": "1.0.0",
	  "fields": {
	    "f1": {
	      "name": "node",
	      "type": "object",
	      "schema": { "id": "node" }
	    }
	  },
	  "schemas": {
	    "node": {
	      "name": "Node",
	      "fields": {
	        "v1": { "name": "val", "type": "string" },
	        "n1": {
	          "name": "next",
	          "type": "object",
	          "schema": { "id": "node" }
	        }
	      }
	    }
	  }
	}`)

	cs := mustCompile(src, nil)

	// --- TerminalWalk ---
	terminalFields := make(map[string]int)
	TerminalWalk(cs, 0, func(fd uint32) {
		owner := ExtractOwnerSchema(fd)
		if m := cs.Meta[owner]; m != nil {
			for ffd, fm := range m.Fields {
				if ffd == fd {
					terminalFields[fm.Name]++
				}
			}
		}
	})

	// Expected visits for TerminalWalk:
	// 1. root.node (terminal=1 from root) -> Recurse into Node
	// 2. Node.val  (scalar)
	// 3. Node.next (terminal=0 because it points back to Node which is on DFS path) -> NO Recurse
	// Total: node, val, next each seen exactly once.
	if terminalFields["node"] != 1 {
		t.Errorf("TerminalWalk: expected node once, got %d", terminalFields["node"])
	}
	if terminalFields["val"] != 1 {
		t.Errorf("TerminalWalk: expected val once, got %d", terminalFields["val"])
	}
	if terminalFields["next"] != 1 {
		t.Errorf("TerminalWalk: expected next once, got %d", terminalFields["next"])
	}

	// --- FullWalk ---
	fullFields := make(map[string]int)
	FullWalk(cs, 0, func(fd uint32) {
		owner := ExtractOwnerSchema(fd)
		if m := cs.Meta[owner]; m != nil {
			for ffd, fm := range m.Fields {
				if ffd == fd {
					fullFields[fm.Name]++
				}
			}
		}
	})

	// FullWalk should visit each schema exactly once.
	// 1. Root: field 'node' -> Recurse Node
	// 2. Node: fields 'val', 'next' -> 'next' is schema-bearing but 'node' is already visited -> Stop
	if fullFields["node"] != 1 {
		t.Errorf("FullWalk: expected node once, got %d", fullFields["node"])
	}
	if fullFields["val"] != 1 {
		t.Errorf("FullWalk: expected val once, got %d", fullFields["val"])
	}
}

func TestWalk_TypeSchemaPassthrough(t *testing.T) {
	// Tests that walks correctly "jump" through type schemas (like unions
	// that don't have fields themselves).
	src := []byte(`{
	  "name": "JumpTest",
	  "version": "1.0.0",
	  "fields": {
	    "f1": { "name": "u", "type": "union", "schema": [ { "id": "v1" } ] }
	  },
	  "schemas": {
	    "v1": {
	      "name": "Variant1",
	      "fields": { "vf1": { "name": "target", "type": "string" } }
	    }
	  }
	}`)

	cs := mustCompile(src, nil)

	seen := false
	TerminalWalk(cs, 0, func(fd uint32) {
		owner := ExtractOwnerSchema(fd)
		if fm, ok := cs.Meta[owner].Fields[fd]; ok && fm.Name == "target" {
			seen = true
		}
	})

	if !seen {
		t.Error("TerminalWalk failed to jump through union type schema to find 'target'")
	}
}
