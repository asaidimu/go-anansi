package data_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/stretchr/testify/assert"
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

func TestDocument_JSONMarshalUnmarshal_RoundTrip(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{
		"name":   "Alice",
		"age":    30,
		"active": true,
		"score":  99.5,
	})
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(doc)
	require.NoError(t, err)
	require.Contains(t, string(jsonBytes), "Alice")
	require.Contains(t, string(jsonBytes), data.DocumentIDField)
	require.Contains(t, string(jsonBytes), data.MetadataField)

	var restored data.Document
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	name, err := restored.Get("name")
	require.NoError(t, err)
	assert.Equal(t, "Alice", name)

	age, err := restored.Get("age")
	require.NoError(t, err)
	assert.Equal(t, float64(30), age)

	active, err := restored.Get("active")
	require.NoError(t, err)
	assert.Equal(t, true, active)

	score, err := restored.Get("score")
	require.NoError(t, err)
	assert.Equal(t, 99.5, score)

	assert.Equal(t, doc.ID(), restored.ID())
	assert.Equal(t, doc.Len(), restored.Len())
	assert.True(t, restored.HasKey("name"))
	assert.True(t, restored.HasKey("age"))
	assert.True(t, restored.HasKey("active"))
	assert.True(t, restored.HasKey("score"))
}

func TestDocument_JSONMarshal_NilDocument(t *testing.T) {
	var doc *data.Document
	jsonBytes, err := json.Marshal(doc)
	require.NoError(t, err)
	assert.Equal(t, "null", string(jsonBytes))
}

func TestDocument_JSONUnmarshal_NilReceiver(t *testing.T) {
	var doc *data.Document
	err := doc.UnmarshalJSON([]byte(`{"foo":"bar"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil document")
}

func TestDocument_JSONUnmarshal_InvalidJSON(t *testing.T) {
	var doc data.Document
	err := json.Unmarshal([]byte(`{invalid}`), &doc)
	require.Error(t, err)
}

func TestDocument_JSONUnmarshal_NestedData(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{
		"user": map[string]any{
			"name":  "Bob",
			"email": "bob@example.com",
		},
		"tags": []any{"go", "anansi"},
	})
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(doc)
	require.NoError(t, err)

	var restored data.Document
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	email, err := restored.GetNested("user.email")
	require.NoError(t, err)
	assert.Equal(t, "bob@example.com", email)

	tags, err := restored.Get("tags")
	require.NoError(t, err)
	assert.Equal(t, []any{"go", "anansi"}, tags)

	assert.True(t, doc.Equals(&restored))
}

func TestDocument_JSONMarshalUnmarshal_EmptyDocument(t *testing.T) {
	doc, err := data.NewDocument(nil)
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(doc)
	require.NoError(t, err)

	var restored data.Document
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	assert.Equal(t, doc.ID(), restored.ID())
	assert.True(t, doc.Equals(&restored))
}

func TestDocument_JSONUnmarshal_MetadataPreserved(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{"foo": "bar"})
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(doc)
	require.NoError(t, err)

	var restored data.Document
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	assert.Equal(t, doc.ID(), restored.ID())

	v1, err := doc.Version()
	require.NoError(t, err)
	v2, err := restored.Version()
	require.NoError(t, err)
	assert.Equal(t, v1, v2)

	c1, err := doc.Checksum()
	require.NoError(t, err)
	c2, err := restored.Checksum()
	require.NoError(t, err)
	assert.Equal(t, c1, c2)
}
