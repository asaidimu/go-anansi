package document

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/sanitize"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const sanitizeTestSchemaJSON = `{
  "version": "1.0.0",
  "name": "sdoc",
  "fields": {
    "name":      { "name": "name",      "type": "string" },
    "password":  { "name": "password",  "type": "string" },
    "email":     { "name": "email",     "type": "string" },
    "address":   { "name": "address",   "type": "object", "schema": { "id": "addr" } },
    "profile":   { "name": "profile",   "type": "record", "schema": { "id": "addr" } },
    "members":   { "name": "members",   "type": "array",  "schema": { "id": "addr" } }
  },
  "schemas": {
    "addr": {
      "name": "addr",
      "fields": {
        "street":   { "name": "street",   "type": "string" },
        "password": { "name": "password", "type": "string" }
      }
    }
  }
}`

func newSanitizeCollection(t *testing.T) *DocumentPool {
	t.Helper()
	s, err := definition.FromJSON([]byte(sanitizeTestSchemaJSON))
	require.NoError(t, err)
	col, err := NewDocumentPool(s)
	require.NoError(t, err)
	return col
}

// sanitizeTestInput returns a document exercising every container leaf shape:
// flat strings, a flattened object, a record, and an array of objects.
func sanitizeTestInput() map[string]any {
	return map[string]any{
		"name":     "Al",
		"password": "hunter2",
		"email":    "al@example.com",
		"address":  map[string]any{"street": "1 Main", "password": "objpass"},
		"profile":  map[string]any{"street": "2 Side", "password": "recpass"},
		"members": []any{
			map[string]any{"street": "3 Leaf", "password": "arrpass"},
		},
	}
}

func setupSanitizeFor(t *testing.T, fn func(s *sanitize.SanitizationRegistry)) {
	t.Helper()
	sanitize.ResetForTesting()
	reg := sanitize.Registry()
	fn(reg)
}

// TestSanitizeContainerMasksStringLeaves verifies the container path masks
// string leaves in place without a map round-trip: flat fields, flattened
// objects, records, and array-of-object children.
func TestSanitizeContainerMasksStringLeaves(t *testing.T) {
	setupSanitizeFor(t, func(reg *sanitize.SanitizationRegistry) {
		require.NoError(t, reg.Register("app", &sanitize.FieldMaskConfig{
			Fields: map[string]sanitize.MaskedFieldPolicy{
				"password": sanitize.MaskRedact,
				"email":    sanitize.MaskObscure,
			},
			DefaultPolicy: sanitize.MaskPreserve,
			ObscureConfig: sanitize.ObscureConfig{PrefixLength: 2, SuffixLength: 2, Replacement: "*"},
		}))
	})

	d := newSanitizeCollection(t).MustFromMap(sanitizeTestInput())
	ctx := common.ContextWithSanitizationScope(context.Background(), "app")

	sanitized, err := d.Sanitize(ctx)
	require.NoError(t, err)

	s, err := sanitized.Get("password")
	require.NoError(t, err)
	assert.Equal(t, "***", s)

	// Masked copies never mutate the source.
	orig, err := d.Get("password")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", orig)

	email, err := sanitized.Get("email")
	require.NoError(t, err)
	assert.Equal(t, "al**********om", email)

	name, err := sanitized.Get("name")
	require.NoError(t, err)
	assert.Equal(t, "Al", name)
}

// TestSanitizeContainerNestedShapes verifies recursive containment masking
// mirrors the map pipeline for objects, records, and object arrays.
func TestSanitizeContainerNestedShapes(t *testing.T) {
	setupSanitizeFor(t, func(reg *sanitize.SanitizationRegistry) {
		require.NoError(t, reg.Register("app", &sanitize.FieldMaskConfig{
			Fields: map[string]sanitize.MaskedFieldPolicy{
				"password": sanitize.MaskRedact,
			},
			DefaultPolicy: sanitize.MaskPreserve,
		}))
	})

	d := newSanitizeCollection(t).MustFromMap(sanitizeTestInput())
	ctx := common.ContextWithSanitizationScope(context.Background(), "app")

	sanitized, err := d.Sanitize(ctx)
	require.NoError(t, err)

	// Flattened object child.
	addrVal, err := sanitized.Get("address")
	require.NoError(t, err)
	addr := addrVal.(map[string]any)
	assert.Equal(t, "***", addr["password"])
	assert.Equal(t, "1 Main", addr["street"])

	// Record-typed field (TypeRecord, SanitizeNestedMap path).
	profVal, err := sanitized.Get("profile")
	require.NoError(t, err)
	prof := profVal.(map[string]any)
	assert.Equal(t, "***", prof["password"])

	// Array-of-object children.
	membersVal, err := sanitized.Get("members")
	require.NoError(t, err)
	members := membersVal.([]any)
	require.Len(t, members, 1)
	first := members[0].(map[string]any)
	assert.Equal(t, "***", first["password"])
	assert.Equal(t, "3 Leaf", first["street"])
}

// TestSanitizeContainerPreservesReservedMetadata verifies _metadata_ system
// fields survive even when a rule would otherwise mask them, matching the map
// pipeline's SanitizeMetadata.
func TestSanitizeContainerPreservesReservedMetadata(t *testing.T) {
	setupSanitizeFor(t, func(reg *sanitize.SanitizationRegistry) {
		require.NoError(t, reg.SetGlobal(&sanitize.FieldMaskConfig{
			Fields: map[string]sanitize.MaskedFieldPolicy{
				"password": sanitize.MaskRedact,
				"checksum": sanitize.MaskRedact,
				"version":  sanitize.MaskRedact,
				"created":  sanitize.MaskRedact,
			},
			DefaultPolicy: sanitize.MaskPreserve,
		}))
	})

	d := newSanitizeCollection(t).MustFromMap(sanitizeTestInput())
	require.NoError(t, d.Hash())

	sanitized, err := d.Sanitize()
	require.NoError(t, err)

	s, err := sanitized.Get("password")
	require.NoError(t, err)
	assert.Equal(t, "***", s)

	// System metadata preserved despite redact rules; the checksum was
	// recomputed over the sanitized data so it stays fresh.
	version, err := sanitized.GetMetadataInt("version")
	require.NoError(t, err)
	assert.Equal(t, 1, version)
	checksum, err := sanitized.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, checksum)
	created, err := sanitized.CreatedAt()
	require.NoError(t, err)
	assert.False(t, created.IsZero())

	// Original integrity is untouched.
	origChecksum, err := d.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, origChecksum)
}

// TestSanitizeContainerNoConfigClones verifies no sanitizer configured yields
// an untampered copy with identical data.
func TestSanitizeContainerNoConfigClones(t *testing.T) {
	sanitize.ResetForTesting()

	d := newSanitizeCollection(t).MustFromMap(sanitizeTestInput())
	sanitized, err := d.Sanitize()
	require.NoError(t, err)

	got, err := sanitized.Get("password")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", got)

	orig, err := d.Get("password")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", orig)
}
