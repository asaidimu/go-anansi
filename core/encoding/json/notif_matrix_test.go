package json_test

import (
	stdjson "encoding/json"
	"os"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/document"
)

// TestChildEntryNames decodes the actions payload and resolves entry names the
// same way writeObject does (nameFor) — WITHOUT invoking the encoder — to see
// what names child-container keys resolve to.
func TestDecode_GeneratedNotifSchema(t *testing.T) {
	raw, err := os.ReadFile("testdata/notif_input_schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	pool, err := document.NewDocumentPoolFromJSON(raw)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}

	variants := map[string]string{
		"a-base":    `{"payload":{"user_id":"u","subject":"s"}}`,
		"b-type":    `{"payload":{"user_id":"u","subject":"s","type":"deploy"}}`,
		"c-data":    `{"payload":{"user_id":"u","subject":"s","data":{"version":"1.2.0"}}}`,
		"d-actions": `{"payload":{"user_id":"u","subject":"s","actions":[{"label":"L","url":"https://x.com"}]}}`,
	}
	for name, doc := range variants {
		t.Run(name, func(t *testing.T) {
			d, err := pool.FromJSON([]byte(doc))
			if err != nil {
				t.Fatalf("decode %s: %v", name, err)
			}
			m := d.ToMap()
			if m == nil {
				t.Fatal("nil map")
			}
			if _, err := stdjson.Marshal(m); err != nil {
				t.Fatalf("marshal %s: %v", name, err)
			}
		})
	}
}

// TestEncode_ArrayObjectElements pins the writeChildren fix: serializing a
// document whose array<object> field has content must terminate and emit one
// object per element keyed by the child schema's field names. Before the fix,
// metadata finalization inside pool.FromJSON recursed emitObject forever on
// this shape (unbounded prefix growth), OOM-ing the process.
func TestEncode_ArrayObjectElements(t *testing.T) {
	raw, err := os.ReadFile("testdata/notif_input_schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	pool, err := document.NewDocumentPoolFromJSON(raw)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	d, err := pool.FromJSON([]byte(`{"payload":{"user_id":"u","subject":"s","actions":[{"label":"L","url":"https://x.com"}]}}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	m := d.ToMap()
	payload, ok := m["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload shape: %T", m["payload"])
	}
	acts, ok := payload["actions"].([]any)
	if !ok || len(acts) != 1 {
		t.Fatalf("actions shape: %T", payload["actions"])
	}
	act, ok := acts[0].(map[string]any)
	if !ok || act["label"] != "L" || act["url"] != "https://x.com" {
		t.Fatalf("element round-trip: %+v", acts[0])
	}
}
