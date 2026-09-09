package anansi

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const aliasChildSchema = `{
	"version": "1.0.0",
	"name": "alias_child",
	"fields": {
		"lines": { "name": "lines", "type": "array", "schema": { "id": "ln" } }
	},
	"schemas": {
		"ln": {
			"name": "ln",
			"fields": {
				"sku": { "name": "sku", "type": "string" },
				"qty": { "name": "qty", "type": "integer" }
			}
		}
	}
}`

// TestAliasing_ChildCarriesBacking verifies nested TypeArrayObject element
// containers receive their own reference to the root backing buffer, so an
// extracted child remains valid independently of its parent.
func TestAliasing_ChildCarriesBacking(t *testing.T) {
	s, err := definition.FromJSON([]byte(aliasChildSchema))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := definition.Compile(s)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		t.Fatal(err)
	}

	doc := container.NewDataContainer()
	payload := `{"lines":[{"sku":"A1","qty":1},{"sku":"B2","qty":2}]}`
	if err := cjson.DecodeJSONInto(cs, []byte(payload), doc, nil); err != nil {
		t.Fatal(err)
	}

	wire, err := EncodeDense(cs, doc, 0)
	if err != nil {
		t.Fatal(err)
	}

	out := container.NewDataContainer()
	if _, err := DecodeAnansiInto(cs, wire, out, nil); err != nil {
		t.Fatal(err)
	}

	fields, err := collectWireFields(cs, rootSlot, nil)
	if err != nil {
		t.Fatal(err)
	}
	var linesKey container.DataContainerKey
	found := false
	for _, wf := range fields {
		if wf.name == "lines" && wf.fd.DataType() == container.TypeArrayObject {
			linesKey = wf.key
			found = true
		}
	}
	if !found {
		t.Fatal("lines field not found in wire fields")
	}

	children, ok, err := out.GetArrayObject(linesKey)
	if err != nil || !ok || len(children) == 0 {
		t.Fatalf("GetArrayObject: children=%d ok=%v err=%v", len(children), ok, err)
	}

	rootBacking := out.Backing()
	if len(rootBacking) == 0 {
		t.Fatal("root has no backing")
	}
	child := children[0]
	if len(child.Backing()) == 0 {
		t.Fatal("child container missing backing reference")
	}
	if &child.Backing()[0] != &rootBacking[0] {
		t.Fatal("child backing must be the same buffer as the root's")
	}

	// And a string on the child itself must view that buffer in place.
	childDump, err := cjson.Dump(cs, child)
	if err != nil {
		t.Fatal(err)
	}
	var probed bool
	for _, v := range childDump {
		s, ok := v.(string)
		if !ok || len(s) == 0 {
			continue
		}
		p := uintptr(unsafe.Pointer(unsafe.StringData(s)))
		lo := uintptr(unsafe.Pointer(&rootBacking[0]))
		hi := uintptr(unsafe.Pointer(&rootBacking[len(rootBacking)-1]))
		if p < lo || p > hi {
			t.Fatalf("child string %#x outside root backing [%#x,%#x]", p, lo, hi)
		}
		probed = true
		break
	}
	if !probed {
		t.Log("first child carried no strings; pointer check skipped")
	}
}
