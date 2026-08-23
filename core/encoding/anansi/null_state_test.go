package anansi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func loadTestSchema(t *testing.T) *definition.CompiledSchema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	s, err := definition.FromJSON(data)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	rs, err := definition.Compile(s)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		t.Fatalf("link schema: %v", err)
	}
	return cs
}

// TestNullFieldStateRoundTrip verifies that explicit null (as opposed to
// merely-absent) fields survive Dense and Sparse encoding, exercising
// fieldStateOf/SetNull directly at the container level to avoid
// core/encoding/json's JSON `null`-literal parsing bug (see the
// TestDocumentIntegration comment in anansi_test.go for the equivalent
// black-box note).
func TestNullFieldStateRoundTrip(t *testing.T) {
	cs := loadTestSchema(t)

	build := func() *container.DataContainer {
		doc := container.NewDataContainer()
		if err := cjson.DecodeJSONInto(cs, []byte(`{"count": 7, "title": "present"}`), doc, nil); err != nil {
			t.Fatalf("DecodeJSONInto: %v", err)
		}
		fields, err := collectWireFields(cs, rootSlot, nil)
		if err != nil {
			t.Fatalf("collectWireFields: %v", err)
		}
		var priceField, nicknameField wireField
		found := 0
		for _, wf := range fields {
			switch wf.name {
			case "price":
				priceField = wf
				found++
			case "nickname":
				nicknameField = wf
				found++
			}
		}
		if found != 2 {
			t.Fatalf("expected to find both price and nickname fields, found %d", found)
		}
		doc.SetNull(priceField.key)
		doc.SetNull(nicknameField.key)
		return doc
	}

	assertStates := func(t *testing.T, doc *container.DataContainer) {
		t.Helper()
		fields, err := collectWireFields(cs, rootSlot, nil)
		if err != nil {
			t.Fatalf("collectWireFields: %v", err)
		}
		for _, wf := range fields {
			state := fieldStateOf(doc, wf.key)
			switch wf.name {
			case "price", "nickname":
				if state != stateNull {
					t.Errorf("field %q: want stateNull, got %v", wf.name, state)
				}
			case "count", "title":
				if state != stateHasValue {
					t.Errorf("field %q: want stateHasValue, got %v", wf.name, state)
				}
			default:
				if state != stateNotSet {
					t.Errorf("field %q: want stateNotSet, got %v", wf.name, state)
				}
			}
		}
	}

	t.Run("dense", func(t *testing.T) {
		doc := build()
		wire, err := EncodeDense(cs, doc, 0)
		if err != nil {
			t.Fatalf("EncodeDense: %v", err)
		}
		decoded, _, err := DecodeAnansi(cs, wire)
		if err != nil {
			t.Fatalf("DecodeAnansi: %v", err)
		}
		assertStates(t, decoded)
	})

	t.Run("sparse", func(t *testing.T) {
		doc := build()
		wire, err := EncodeSparse(cs, doc, 0)
		if err != nil {
			t.Fatalf("EncodeSparse: %v", err)
		}
		decoded, _, err := DecodeAnansi(cs, wire)
		if err != nil {
			t.Fatalf("DecodeAnansi: %v", err)
		}
		assertStates(t, decoded)
	})
}

// TestDenseVsSparseFieldOrderStable verifies that collectWireFields (and
// therefore both the Dense state map's bit order and the Sparse decoder's
// key lookup) produce the same field set/order across repeated calls for
// the same compiled schema, which the wire format's correctness depends on.
func TestDenseVsSparseFieldOrderStable(t *testing.T) {
	cs := loadTestSchema(t)
	a, err := collectWireFields(cs, rootSlot, nil)
	if err != nil {
		t.Fatalf("collectWireFields: %v", err)
	}
	b, err := collectWireFields(cs, rootSlot, nil)
	if err != nil {
		t.Fatalf("collectWireFields: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("field count differs across calls: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].name != b[i].name || a[i].key != b[i].key {
			t.Fatalf("field %d differs across calls: %+v vs %+v", i, a[i], b[i])
		}
	}
}
