package data_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/require"
)

func TestDocument_Transform(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{
		"name":  "John Doe",
		"age":   30,
		"city":  "New York",
		"email": "john.doe@example.com",
	})
	require.NoError(t, err)

	// Test Map
	mappedDoc := doc.Transform().Map(func(key string, value any) any {
		if key == "name" {
			return "Jane Doe"
		}
		return value
	}).Execute()

	require.Equal(t, "Jane Doe", mappedDoc.MustGet("name"))
	require.Equal(t, 30, mappedDoc.MustGet("age"))

	// Test Filter
	filteredDoc := doc.Transform().Filter(func(key string, value any) bool {
		return key == "name" || key == "age"
	}).Execute()

	require.Equal(t, 2, filteredDoc.Len())
	require.True(t, filteredDoc.HasKey("name"))
	require.True(t, filteredDoc.HasKey("age"))
	require.False(t, filteredDoc.HasKey("city"))

	// Test Pick
	pickedDoc := doc.Transform().Pick("name", "email").Execute()

	require.Equal(t, 2, pickedDoc.Len())
	require.True(t, pickedDoc.HasKey("name"))
	require.True(t, pickedDoc.HasKey("email"))
	require.False(t, pickedDoc.HasKey("age"))

	// Test Omit
	omittedDoc := doc.Transform().Omit("age", "city").Execute()

	require.Equal(t, 3, omittedDoc.Len())
	require.True(t, omittedDoc.HasKey("name"))
	require.True(t, omittedDoc.HasKey("email"))
	require.False(t, omittedDoc.HasKey("age"))

	// Test chaining transformations
	chainedDoc := doc.Transform().
		Pick("name", "age", "city").
		Map(func(key string, value any) any {
			if key == "age" {
				return value.(int) + 1
			}
			return value
		}).
		Omit("city").
		Execute()

	require.Equal(t, 2, chainedDoc.Len())
	require.Equal(t, "John Doe", chainedDoc.MustGet("name"))
	require.Equal(t, 31, chainedDoc.MustGet("age"))
	require.False(t, chainedDoc.HasKey("city"))
}
