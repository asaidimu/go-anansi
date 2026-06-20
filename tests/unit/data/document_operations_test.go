package data_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/require"
)

func TestDocument_GetSet(t *testing.T) {
	doc, err := data.NewDocument(nil)
	require.NoError(t, err)

	// Test Set and Get
	err = doc.Set("name", "John Doe")
	require.NoError(t, err)
	val, err := doc.Get("name")
	require.NoError(t, err)
	require.Equal(t, "John Doe", val)

	// Test GetOr
	val = doc.GetOr("name", "Jane Doe")
	require.Equal(t, "John Doe", val)
	val = doc.GetOr("age", 30)
	require.Equal(t, 30, val)

	// Test MustGet
	val = doc.MustGet("name")
	require.Equal(t, "John Doe", val)
	require.Panics(t, func() {
		doc.MustGet("non_existent")
	})

	// Test SetIfNotExists
	set := doc.SetIfNotExists("name", "New Name")
	require.False(t, set)
	require.Equal(t, "John Doe", doc.MustGet("name"))

	set = doc.SetIfNotExists("city", "New York")
	require.True(t, set)
	require.Equal(t, "New York", doc.MustGet("city"))

	// Test Set with empty key
	err = doc.Set("", "invalid")
	require.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyEmpty.Code, sysErr.Code)
}

func TestDocument_GetSetNested(t *testing.T) {
	doc, err := data.NewDocument(nil)
	require.NoError(t, err)

	// Test SetNested
	err = doc.SetNested("address.street", "123 Main St")
	require.NoError(t, err)
	err = doc.SetNested("address.city", "Anytown")
	require.NoError(t, err)

	// Test GetNested
	val, err := doc.GetNested("address.street")
	require.NoError(t, err)
	require.Equal(t, "123 Main St", val)

	val, err = doc.GetNested("address.city")
	require.NoError(t, err)
	require.Equal(t, "Anytown", val)

	// Test GetNested non-existent path
	_, err = doc.GetNested("address.zip")
	require.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrPathSegmentNotFound.Code, sysErr.Code)

	_, err = doc.GetNested("non_existent.path")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrPathSegmentNotFound.Code, sysErr.Code)

	// Test SetNested with empty path
	err = doc.SetNested("", "invalid")
	require.Error(t, err)

	sysErr, ok := err.(*common.SystemError)
	require.True(t, ok)
	require.Equal(t, sysErr.Code, data.ErrKeyEmpty.Code)

	// Test GetNested with empty path
	_, err = doc.GetNested("")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyEmpty.Code, sysErr.Code)

	// Test SetNested into non-map type
	doc.Set("foo", "bar")
	err = doc.SetNested("foo.baz", "qux")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrCannotTraverse.Code, sysErr.Code)
}

func TestDocument_DeleteNested_NonMapParent(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{
		"user": map[string]any{
			"name": "Test User", // This is a string
		},
	})
	require.NoError(t, err)

	err = doc.Delete("user.name.first")
	require.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrParentNotMap.Code, sysErr.Code)
}

func TestDocument_DeleteNested(t *testing.T) {
	initialDoc, err := data.NewDocument(map[string]any{
		"user": map[string]any{
			"name": "Test User",
			"address": map[string]any{
				"street": "123 Main St",
				"city":   "Anytown",
			},
		},
		"product": "Laptop",
	})
	require.NoError(t, err)

	// Test case 1: Delete top-level field
	t.Run("Delete top-level field", func(t *testing.T) {
		doc := initialDoc.Clone()
		err := doc.Delete("product")
		require.NoError(t, err)
		_, err = doc.Get("product")
		require.Error(t, err)
		var sysErr *common.SystemError
		require.ErrorAs(t, err, &sysErr)
		require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)
	})

	// Test case 2: Delete nested field
	t.Run("Delete nested field", func(t *testing.T) {
		doc := initialDoc.Clone()
		err := doc.Delete("user.address.city")
		require.NoError(t, err)
		_, err = doc.GetNested("user.address.city")
		require.Error(t, err)
		var sysErr *common.SystemError
		require.ErrorAs(t, err, &sysErr)
		require.Equal(t, data.ErrPathSegmentNotFound.Code, sysErr.Code)

		// Verify other nested fields remain
		val, err := doc.GetNested("user.address.street")
		require.NoError(t, err)
		require.Equal(t, "123 Main St", val)
	})

	// Test case 3: Delete non-existent field
	t.Run("Delete non-existent field", func(t *testing.T) {
		doc := initialDoc.Clone()
		err := doc.Delete("user.address.zip")
		require.NoError(t, err)
	})

	// Test case 4: Delete with empty path
	t.Run("Delete with empty path", func(t *testing.T) {
		doc := initialDoc.Clone()
		err := doc.Delete("")
		require.Error(t, err)
		var sysErr *common.SystemError
		require.ErrorAs(t, err, &sysErr)
		require.Equal(t, data.ErrKeyEmpty.Code, sysErr.Code)
	})

	// Test case 5: Delete from non-map parent
	t.Run("Delete from non-map parent", func(t *testing.T) {
		doc := initialDoc.Clone()
		err := doc.Delete("user.name.first")
		require.Error(t, err)
		var sysErr *common.SystemError
		require.ErrorAs(t, err, &sysErr)
		require.Equal(t, data.ErrParentNotMap.Code, sysErr.Code)
	})
}

func TestDocument_SetID_ReturnsError(t *testing.T) {
	doc, err := data.NewDocument(nil)
	require.NoError(t, err)

	err = doc.Set(data.DocumentIDField, "some-value")
	require.Error(t, err)

	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, "data.Document.Set", sysErr.Operation)
	require.Equal(t, data.DocumentIDField, sysErr.Path)
	require.Equal(t, data.ErrReadOnlyField.Code, sysErr.Code)
}
