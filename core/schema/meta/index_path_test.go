package meta_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
	"github.com/stretchr/testify/require"
)

const nestedSchemaID = "019d7775-6564-0000-8000-000000000001"

// indexDoc builds a schema document whose top-level index is idx1, with a
// nested schema reachable via the dotted path "f1.<seg>".
func indexDoc(index map[string]any) map[string]any {
	return map[string]any{
		"name":    "Test",
		"version": "1.0.0",
		"fields": map[string]any{
			"f1": map[string]any{
				"name":   "f1",
				"type":   "object",
				"schema": map[string]any{"id": nestedSchemaID},
			},
		},
		"schemas": map[string]any{
			nestedSchemaID: map[string]any{
				"name": "Address",
				"fields": map[string]any{
					"email": map[string]any{"name": "email", "type": "string"},
					"geom":  map[string]any{"name": "geom", "type": "geometry"},
				},
			},
		},
		"indexes": map[string]any{"idx1": index},
	}
}

func hasCode(issues []common.Issue, code string) bool {
	for _, is := range issues {
		if is.Code == code {
			return true
		}
	}
	return false
}

func TestIndexFieldReferences_DottedPaths(t *testing.T) {
	vd := meta.DevelopmentSchemaValidator()

	t.Run("valid dotted path accepted", func(t *testing.T) {
		issues, ok := vd.Validate(indexDoc(map[string]any{
			"name": "idx1", "type": "normal", "fields": []any{"f1.email"},
		}))
		require.True(t, ok, "dotted nested field reference should validate, got %v", issues)
	})

	t.Run("missing field still rejected with non-doubled path", func(t *testing.T) {
		issues, ok := vd.Validate(map[string]any{
			"name":    "Test",
			"version": "1.0.0",
			"fields": map[string]any{
				"f1": map[string]any{"name": "f1", "type": "string"},
			},
			"indexes": map[string]any{
				"idx1": map[string]any{
					"name":   "idx1",
					"type":   "normal",
					"fields": []any{"non-existent-field"},
				},
			},
		})
		require.False(t, ok)
		require.True(t, hasCode(issues, "INDEX_FIELD_NOT_FOUND"), "got %v", issues)
		for _, is := range issues {
			if is.Code == "INDEX_FIELD_NOT_FOUND" {
				// The constraint's contextPath is empty; the predicate emits the
				// absolute path and it must not be prefixed twice.
				require.Equal(t, "indexes.idx1.fields[0]", is.Path)
			}
		}
	})

	t.Run("missing nested segment still rejected", func(t *testing.T) {
		issues, ok := vd.Validate(indexDoc(map[string]any{
			"name": "idx1", "type": "normal", "fields": []any{"f1.missing"},
		}))
		require.False(t, ok)
		require.True(t, hasCode(issues, "INDEX_FIELD_NOT_FOUND"), "got %v", issues)
	})
}

func TestSpatialIndex_DottedGeometryPaths(t *testing.T) {
	vd := meta.DevelopmentSchemaValidator()

	t.Run("resolves dotted geometry field", func(t *testing.T) {
		issues, ok := vd.Validate(indexDoc(map[string]any{
			"name": "idx1", "type": "spatial", "fields": []any{"f1.geom"},
		}))
		require.True(t, ok, "spatial index on nested geometry should validate, got %v", issues)
	})

	t.Run("flags dotted non-geometry field", func(t *testing.T) {
		issues, ok := vd.Validate(indexDoc(map[string]any{
			"name": "idx1", "type": "spatial", "fields": []any{"f1.email"},
		}))
		require.False(t, ok)
		require.True(t, hasCode(issues, "SPATIAL_INDEX_NON_GEOMETRY"), "got %v", issues)
	})
}

func TestIndexCondition_DottedFieldPaths(t *testing.T) {
	vd := meta.DevelopmentSchemaValidator()

	t.Run("mismatched value type flagged", func(t *testing.T) {
		issues, ok := vd.Validate(indexDoc(map[string]any{
			"name": "idx1", "type": "normal", "fields": []any{"f1.email"},
			"condition": map[string]any{
				"field":    "f1.email",
				"operator": "eq",
				"value":    123,
			},
		}))
		require.False(t, ok)
		require.True(t, hasCode(issues, "INDEX_CONDITION_VALUE_TYPE_MISMATCH"), "got %v", issues)
	})

	t.Run("matching value type accepted", func(t *testing.T) {
		issues, ok := vd.Validate(indexDoc(map[string]any{
			"name": "idx1", "type": "normal", "fields": []any{"f1.email"},
			"condition": map[string]any{
				"field":    "f1.email",
				"operator": "eq",
				"value":    "alice@test.com",
			},
		}))
		require.True(t, ok, "condition on nested string field should validate, got %v", issues)
	})
}
