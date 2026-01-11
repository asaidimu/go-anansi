package data_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
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
	require.Contains(t, result, data.MetadataField) // Top-level metadata should now be present

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
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrFailedToUnmarshalStruct.Code, sysErr.Code)
}
