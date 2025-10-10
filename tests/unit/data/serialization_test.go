package data_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/require"
)

func TestDocument_ToJSON(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{
		"name": "Test",
		"age":  30,
	})
	require.NoError(t, err)

	// Test without pretty print
	jsonBytes, err := doc.ToJSON()
	require.NoError(t, err)
	var result map[string]any
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)
	require.Contains(t, result, "name")
	require.Contains(t, result, "age")
	require.Contains(t, result, data.MetadataField)

	// Test with pretty print
	prettyBytes, err := doc.ToJSON(true)
	require.NoError(t, err)
	require.True(t, len(prettyBytes) > len(jsonBytes))
	require.Contains(t, string(prettyBytes), "  ") // Check for indentation
}

func TestDocument_ToStruct(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	doc, err := data.NewDocument(map[string]any{
		"name": "John Doe",
		"age":  25,
	})
	require.NoError(t, err)

	var person Person
	err = doc.ToStruct(&person)
	require.NoError(t, err)
	require.Equal(t, "John Doe", person.Name)
	require.Equal(t, 25, person.Age)

	// Test with missing field in struct
	type PartialPerson struct {
		Name string `json:"name"`
	}
	var partialPerson PartialPerson
	err = doc.ToStruct(&partialPerson)
	require.NoError(t, err)
	require.Equal(t, "John Doe", partialPerson.Name)

	// Test with type mismatch
	type InvalidPerson struct {
		Age string `json:"age"`
	}
	var invalidPerson InvalidPerson
	err = doc.ToStruct(&invalidPerson)
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrFailedToUnmarshalStruct)
}

func TestFromStruct(t *testing.T) {
	type Product struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Price float64 `json:"price"`
	}

	product := Product{
		ID:    "prod1",
		Name:  "Laptop",
		Price: 1200.50,
	}

	doc, err := data.FromStruct(product)
	require.NoError(t, err)

	require.Equal(t, "Laptop", doc.MustGet("name"))
	require.Equal(t, 1200.50, doc.MustGet("price"))

	// Test with nil struct
	doc, err = data.FromStruct(nil)
	require.NoError(t, err)
	require.True(t, doc.IsEmpty())

	// Test with struct that cannot be marshaled (e.g., contains a channel)
	type InvalidStruct struct {
		Ch chan int
	}
	invalid := InvalidStruct{Ch: make(chan int)}
	doc, err = data.FromStruct(invalid)
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrFailedToMarshalStruct)
}
