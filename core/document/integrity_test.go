package document

import (
	"bytes"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/stretchr/testify/require"
)

// TestCanonicalBytesMatchLegacySerialization pins the container-direct
// canonical serializer against the previous map-based canonicalization
// (json.Marshal(canonicalize(ToMap minus signature/checksum))). The two must
// produce byte-identical output for root container-backed documents so stored
// checksums remain verifiable after the refactor.
func TestCanonicalBytesMatchLegacySerialization(t *testing.T) {
	d := testDocument(t)

	// The document carries generated metadata incl. a checksum, so traversal
	// must skip exactly _metadata_.checksum/_metadata_.signature.
	v, err := d.GetMetadataValue(data.MetadataChecksum)
	require.NoError(t, err)
	require.NotEmpty(t, v)

	legacy, err := canonicalMarshal(d.canonicalHashInput())
	require.NoError(t, err)

	got, err := d.canonicalBytes()
	require.NoError(t, err)

	if !bytes.Equal(got, legacy) {
		t.Fatalf("canonical bytes mismatch\nlegacy: %s\n   got: %s", legacy, got)
	}

	// Canonical form must not leak the checksum/signature fields themselves.
	if bytes.Contains(got, []byte("checksum")) {
		t.Fatalf("canonical bytes unexpectedly contain checksum: %s", got)
	}
	if bytes.Contains(got, []byte(`"signature"`)) {
		t.Fatalf("canonical bytes unexpectedly contain signature: %s", got)
	}

	// Hash/verify still round-trips through the container-direct path.
	require.NoError(t, d.Hash())
	ok, err := d.VerifyHash()
	require.NoError(t, err)
	require.True(t, ok)
}

// TestCanonicalFloatNormalization ensures a whole-number float serializes as an
// integer in the canonical form, matching legacy float64->int64 normalization.
func TestCanonicalFloatNormalization(t *testing.T) {
	d := newTestCollection(t).MustFromMap(map[string]any{
		"score": 8.0,
	})

	legacy, err := canonicalMarshal(d.canonicalHashInput())
	require.NoError(t, err)
	got, err := d.canonicalBytes()
	require.NoError(t, err)

	if !bytes.Equal(got, legacy) {
		t.Fatalf("canonical bytes mismatch for whole float\nlegacy: %s\n   got: %s", legacy, got)
	}
}
