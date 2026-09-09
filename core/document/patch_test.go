package document

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/stretchr/testify/require"
)

func TestCollection_NewIsFullyInitialized(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.New()
	require.NoError(t, err)

	// Identity.
	if d.ID() == "" {
		t.Fatalf("New did not generate an ID")
	}

	// Metadata defaults (created/updated/version).
	meta := d.Metadata()
	if len(meta) == 0 {
		t.Fatalf("New produced no metadata defaults: %v", meta)
	}
	for _, k := range []string{data.MetadataCreated, data.MetadataUpdated, data.MetadataVersion} {
		if _, ok := meta[k]; !ok {
			t.Fatalf("New metadata missing %q: %v", k, meta)
		}
	}

	// Checksum.
	checksum, err := d.Checksum()
	require.NoError(t, err)
	if checksum == "" {
		t.Fatalf("New did not compute a checksum")
	}
	d.Release()
}

func TestCollection_PatchIsCompletelyUninitialized(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.Patch()
	require.NoError(t, err)

	// Identity: none.
	if d.ID() != "" {
		t.Fatalf("Patch generated an ID: %q", d.ID())
	}

	// Metadata: no defaults, no checksum.
	if m := d.Metadata(); len(m) != 0 {
		t.Fatalf("Patch produced metadata: %v", m)
	}
	if _, err := d.Checksum(); err == nil {
		t.Fatalf("Patch has a checksum; expected an error")
	}

	// User data is empty and writable.
	if !d.IsEmpty() {
		t.Fatalf("Patch not empty")
	}
	require.NoError(t, d.Set("id", "user-1"))
	if got, _ := d.Get("id"); got != "user-1" {
		t.Fatalf("patch field not set: got %v", got)
	}
	d.Release()
}

func TestCollection_PatchReleasesToPool(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.Patch()
	require.NoError(t, err)
	require.NoError(t, d.Set("id", "x"))

	d.Release()
	if d.c != nil || d.pool != nil {
		t.Fatalf("resources not cleared after Patch release")
	}

	d2, err := col.Patch()
	require.NoError(t, err)
	if d2.c.Length() != 0 {
		t.Fatalf("reused patch container retained data")
	}
	d2.Release()
}
