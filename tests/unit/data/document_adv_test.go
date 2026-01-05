package data_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocument_DeepMerge(t *testing.T) {
	t.Run("should merge nested documents", func(t *testing.T) {
		doc1 := data.MustNewDocument(map[string]any{
			"user": map[string]any{
				"name": "Alice",
				"details": map[string]any{
					"age": 30,
				},
			},
			"status": "active",
		})
		doc2 := data.MustNewDocument(map[string]any{
			"user": map[string]any{
				"details": map[string]any{
					"city": "New York",
				},
			},
			"status": "inactive",
		})

		doc1.DeepMerge(doc2) // DeepMerge now modifies in-place

		expected := data.MustNewDocument(map[string]any{
			"user": map[string]any{
				"name": "Alice",
				"details": map[string]any{
					"age":  30,
					"city": "New York",
				},
			},
			"status": "inactive",
		})

		// We need to remove metadata and dynamic IDs for a stable comparison in this test
		actualMap := doc1.StripMetadata().ToMap()
		expectedMap := expected.StripMetadata().ToMap()

		// Remove dynamic IDs for comparison as they are generated
		delete(actualMap, "id")
		delete(expectedMap, "id")

		assert.Equal(t, expectedMap, actualMap)
	})

	t.Run("should overwrite non-document values", func(t *testing.T) {
		doc1 := data.MustNewDocument(map[string]any{"a": 1})
		doc2 := data.MustNewDocument(map[string]any{"a": 2})
		doc1.DeepMerge(doc2) // DeepMerge now modifies in-place
		val, err := doc1.GetInt("a")
		require.NoError(t, err)
		assert.Equal(t, 2, val)
	})
}


func TestDocument_DiffAndApply(t *testing.T) {
	doc1 := data.MustNewDocument(map[string]any{
		"a": 1,
		"b": "hello",
		"c": true,
	})
	doc2 := data.MustNewDocument(map[string]any{
		"b": "world",
		"c": true,
		"d": 123,
	})

	// Strip metadata and IDs for consistent diff comparison
	doc1Stripped := doc1.StripMetadata()
	doc2Stripped := doc2.StripMetadata()

	diff := doc1Stripped.Diff(doc2Stripped)

	expectedDiff := data.DocumentDiff{
		Added:   map[string]any{"d": 123},
		Removed: map[string]any{"a": 1},
		Modified: map[string]data.DiffValue{
			"b": {Old: "hello", New: "world"},
		},
	}

	assert.Equal(t, expectedDiff.Added, diff.Added)
	assert.Equal(t, expectedDiff.Removed, diff.Removed)
	assert.Equal(t, expectedDiff.Modified, diff.Modified)
	assert.True(t, diff.HasChanges())

	// Apply the diff
	doc3 := doc1Stripped.Apply(diff)
	assert.True(t, doc2Stripped.StripMetadata().Equals(doc3.StripMetadata()))
}

func TestDocument_JSONPathQuery(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"store": map[string]any{
			"book": []any{
				map[string]any{"title": "Book 1", "price": 10},
				map[string]any{"title": "Book 2", "price": 20},
			},
			"bicycle": map[string]any{
				"color": "red",
				"price": 100,
			},
		},
	})

	doc = doc.StripMetadata()
	json, _ := doc.ToJSON(true)
	t.Logf("Data %s", json)
	// Test nested access
	prices, err := doc.JSONPathQuery("$.store.book[*].price")
	require.NoError(t, err)
	assert.Equal(t, []any{10, 20}, prices)

	// Test wildcard
	storeItems, err := doc.JSONPathQuery("$.store.*")
	require.NoError(t, err)
	assert.Len(t, storeItems, 2)

	// Test single field
	color, err := doc.JSONPathQuery("$.store.bicycle.color")
	require.NoError(t, err)
	assert.Equal(t, []any{"red"}, color)
}

func TestDocument_JSONPathQuery_Simple(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"key": "value",
	}).StripMetadata()

	result, err := doc.JSONPathQuery("$.key")
	require.NoError(t, err)
	assert.Equal(t, []any{"value"}, result)
}

func TestDocument_JSONPathQuery_WildcardAndAccess(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"items": []any{
			map[string]any{"value": 1},
			map[string]any{"value": 2},
		},
	}).StripMetadata()

	result, err := doc.JSONPathQuery("$.items[*].value")
	require.NoError(t, err)
	assert.Equal(t, []any{1, 2}, result)
}

func TestDocument_JSONPathQuery_SingleLevel(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"a": 1,
		"b": 2,
	}).StripMetadata()

	result, err := doc.JSONPathQuery("$.a")
	require.NoError(t, err)
	assert.Equal(t, []any{1}, result)
}

func TestDocument_JSONPathQuery_Book(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"store": map[string]any{
			"book": []any{
				map[string]any{"title": "Book 1", "price": 10},
				map[string]any{"title": "Book 2", "price": 20},
			},
		},
	}).StripMetadata()

	result, err := doc.JSONPathQuery("$.store.book")
	require.NoError(t, err)
	// The JSONPathQuery returns []any, and the nested documents are returned as maps
	// from deepCloneValue during the query processing.
	assert.Equal(t, []any{
		map[string]any{"title": "Book 1", "price": 10},
		map[string]any{"title": "Book 2", "price": 20},
	}, result)
}

func TestDocument_Normalize(t *testing.T) {
	docWithNestedMeta := data.MustNewDocument(map[string]any{
		"level1": data.MustNewDocument(map[string]any{
			"level2": "value",
		}),
	})

	// The MustNewDocument constructor adds metadata, so level1 will have it.
	// Let's check if Normalize removes it.
	normalized := docWithNestedMeta.Normalize()
	require.NotNil(t, normalized)

	// Check that top-level metadata is preserved
	_, hasMeta := normalized.Metadata()
	assert.True(t, hasMeta, "top-level metadata should be preserved")

	// Check that nested metadata is removed
	level1, err := normalized.GetDocument("level1")
	require.NoError(t, err)
	require.NotNil(t, level1)
	_, hasNestedMeta := level1.Metadata()
	assert.False(t, hasNestedMeta, "nested metadata should be removed")
}
