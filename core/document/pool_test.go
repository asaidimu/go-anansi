package document

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const poolParentSchemaJSON = `{
  "version": "1.0.0",
  "name": "testpoolparent",
  "fields": {
    "items": {
      "name": "items",
      "type": "array",
      "schema": { "id": "poolitem" }
    }
  },
  "schemas": {
    "poolitem": {
      "name": "poolitem",
      "fields": {
        "label": { "name": "label", "type": "string" }
      }
    }
  }
}`

func newPoolParentCollection(t *testing.T) *DocumentPool {
	t.Helper()
	s, err := definition.FromJSON([]byte(poolParentSchemaJSON))
	require.NoError(t, err)
	col, err := NewDocumentPool(s)
	require.NoError(t, err)
	return col
}

func TestDocument_ReleaseReturnsContainers(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if d.pool == nil {
		t.Fatalf("root document missing schema pool")
	}
	_ = d.Set("id", "pooled")

	d.Release()
	if d.c != nil || d.pool != nil {
		t.Fatalf("resources not cleared after Release")
	}

	// Same-collection allocation must reuse a cleared container.
	d2, err := col.New()
	if err != nil {
		t.Fatalf("New after release: %v", err)
	}
	// New writes the system fields (_id_, _metadata_) by design, so the
	// container is never empty; the previous document's user data must be gone.
	if len(d2.Data()) != 0 {
		t.Fatalf("reused container retained user data: %v", d2.Data())
	}
	d2.Release()
}

func TestDocument_ReleaseIsIdempotent(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.Release()
	d.Release() // must not panic
}

func TestDocument_RecordViewReleaseIsNoOp(t *testing.T) {
	view := newRecordView(map[string]any{"k": "v"}, nil)
	view.Release() // must not panic
	if view.record == nil {
		t.Fatalf("record view data cleared by Release")
	}
}

func TestDocument_ClonePooledAndIndependent(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = d.Set("id", "original")

	clone := d.Clone().(*Document)
	if clone.pool == nil {
		t.Fatalf("clone did not inherit schema pool")
	}
	_ = clone.Set("id", "changed")

	if got, _ := d.Get("id"); got != "original" {
		t.Fatalf("source mutated by clone: got %v", got)
	}
	d.Release()
	if got, _ := clone.Get("id"); got != "changed" {
		t.Fatalf("clone lost data after source release: got %v", got)
	}
	clone.Release()
}

func TestDocument_StripMetadataPooled(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = d.Set("id", "x")

	stripped := d.StripMetadata().(*Document)
	if m := stripped.Metadata(); len(m) != 0 {
		t.Fatalf("expected empty metadata, got %v", m)
	}
	d.Release()
	stripped.Release()
}

func TestDocument_ReleaseWithArrayObjectChildren(t *testing.T) {
	col := newPoolParentCollection(t)

	d, err := col.FromMap(map[string]any{
		"items": []any{
			map[string]any{"label": "first"},
			map[string]any{"label": "second"},
		},
	})
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}

	arr, err := d.GetDocumentArray("items")
	if err != nil {
		t.Fatalf("GetDocumentArray: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 children, got %d", len(arr))
	}
	// Views over pooled children must no-op on Release.
	for _, v := range arr {
		v.Release()
	}

	d.Release()

	d2, err := col.New()
	if err != nil {
		t.Fatalf("New after release: %v", err)
	}
	if len(d2.Data()) != 0 {
		t.Fatalf("reused container retained array data: %v", d2.Data())
	}
	d2.Release()
}

func TestDocument_CollectionsHaveIndependentPools(t *testing.T) {
	colA := newTestCollection(t)
	colB := newPoolParentCollection(t)

	dA, err := colA.New()
	if err != nil {
		t.Fatalf("New A: %v", err)
	}
	dB, err := colB.New()
	if err != nil {
		t.Fatalf("New B: %v", err)
	}
	if dA.pool == dB.pool {
		t.Fatalf("distinct collections share a pool")
	}

	_ = dA.Set("id", "A")
	dA.Release()
	dB.Release()

	dA2, err := colA.New()
	if err != nil {
		t.Fatalf("New A after release: %v", err)
	}
	if len(dA2.Data()) != 0 {
		t.Fatalf("collection A container retained data: %v", dA2.Data())
	}
	dA2.Release()
}

func TestDocument_ReleaseSatisfiesDocumenter(t *testing.T) {
	var _ data.Documenter = (*Document)(nil)
}

func TestDocument_FromJSONGeneratesIDWhenAbsent(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.FromJSON([]byte(`{"id":"user-1"}`))
	require.NoError(t, err)
	require.Len(t, d.ID(), 32)
	require.True(t, isValidID(d.ID()))
}

func TestDocument_FromJSONHonorsValidID(t *testing.T) {
	col := newTestCollection(t)
	const id = "019fc29f444b736986075644a478fb92"
	d, err := col.FromJSON([]byte(`{"_id_":"` + id + `","id":"user-1"}`))
	require.NoError(t, err)
	require.Equal(t, id, d.ID())
}

func TestDocument_FromJSONReplacesInvalidID(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.FromJSON([]byte(`{"_id_":"not-a-uuid","id":"user-1"}`))
	require.NoError(t, err)
	require.Len(t, d.ID(), 32)
	require.True(t, isValidID(d.ID()))
}
