package definition_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordArraySchema is meta-schema-valid: collection_requires_schema mandates a
// "schema" on array fields and inline_type_descriptor_valid only allows
// primitives or "record" as the inline item type.
const recordArraySchemaJSON = `{
  "name": "RecordDoc",
  "version": "1.0.0",
  "fields": {
    "tags": {
      "name": "tags",
      "type": "array",
      "schema": { "type": "record" }
    }
  }
}`

// TestInlineRecordArrayValidation guards against the validator rejecting
// map[string]any items of an array declared with the inline descriptor
// { "type": "record" } (TYPE_MISMATCH despite the schema passing the
// meta-schema validator).
func TestInlineRecordArrayValidation(t *testing.T) {
	sc, err := definition.FromJSON([]byte(recordArraySchemaJSON))
	require.NoError(t, err)

	v, err := definition.NewDocumentValidator(sc, nil)
	require.NoError(t, err)

	t.Run("valid records accepted", func(t *testing.T) {
		doc := map[string]any{
			"tags": []any{
				map[string]any{"key": "env", "value": "prod"},
				map[string]any{},
			},
		}
		issues, ok := v.Validate(doc)
		assert.True(t, ok, "unexpected issues: %+v", issues)
	})

	t.Run("non-map item rejected", func(t *testing.T) {
		doc := map[string]any{"tags": []any{"not-a-record"}}
		issues, ok := v.Validate(doc)
		require.False(t, ok)
		for _, issue := range issues {
			assert.Equal(t, "TYPE_MISMATCH", issue.Code)
			assert.Equal(t, "tags[0]", issue.Path)
		}
	})

	t.Run("null item allowed by default", func(t *testing.T) {
		// Inline item descriptors resolve nullable=true when absent, matching
		// the behavior of primitive inline arrays ({ "type": "string" }, ...).
		doc := map[string]any{"tags": []any{nil}}
		issues, ok := v.Validate(doc)
		assert.True(t, ok, "unexpected issues: %+v", issues)
	})
}

// TestInlineRecordFieldValidation covers a top-level record field carrying its
// own inline descriptor: each entry value must itself be a record.
func TestInlineRecordFieldValidation(t *testing.T) {
	schemaJSON := `{
	  "name": "LabelDoc",
	  "version": "1.0.0",
	  "fields": {
	    "labels": {
	      "name": "labels",
	      "type": "record",
	      "schema": { "type": "record" }
	    }
	  }
	}`

	sc, err := definition.FromJSON([]byte(schemaJSON))
	require.NoError(t, err)

	v, err := definition.NewDocumentValidator(sc, nil)
	require.NoError(t, err)

	valid := map[string]any{"labels": map[string]any{"a": map[string]any{"x": 1}}}
	issues, ok := v.Validate(valid)
	assert.True(t, ok, "unexpected issues: %+v", issues)

	invalid := map[string]any{"labels": map[string]any{"a": "scalar"}}
	issues, ok = v.Validate(invalid)
	require.False(t, ok)
	for _, issue := range issues {
		assert.Equal(t, "TYPE_MISMATCH", issue.Code)
	}
}
