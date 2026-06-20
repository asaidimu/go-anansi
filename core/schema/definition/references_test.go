package definition_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldSchemaReference_MarshalJSON_Single(t *testing.T) {
	schemaRef := definition.SchemaReference{
		ID: "schema1",
	}
	fsr := definition.NewSchemaReference(schemaRef)

	data, err := json.Marshal(fsr)
	require.NoError(t, err)

	expected := `{"id":"schema1"}`
	assert.JSONEq(t, expected, string(data))
}

func TestFieldSchemaReference_MarshalJSON_Multiple(t *testing.T) {
	schemasRef := []definition.SchemaReference{
		{ID: "schema1"},
		{ID: "schema2"},
	}
	fsr := definition.NewSchemaReference(schemasRef)

	data, err := json.Marshal(fsr)
	require.NoError(t, err)

	expected := `[{"id":"schema1"},{"id":"schema2"}]`
	assert.JSONEq(t, expected, string(data))
}

func TestFieldSchemaReference_UnmarshalJSON_Single(t *testing.T) {
	jsonStr := `{"id":"schema1"}`
	var fsr definition.FieldSchemaReference
	err := json.Unmarshal([]byte(jsonStr), &fsr)
	require.NoError(t, err)

	singleSchema, errA := definition.FieldSchemaAs[definition.SchemaReference](fsr)
	require.NoError(t, errA)
	require.NotNil(t, singleSchema)
	assert.True(t, fsr.IsSingle())
	assert.Equal(t, definition.SchemaId("schema1"), singleSchema.ID)
}

func TestFieldSchemaReference_UnmarshalJSON_Multiple(t *testing.T) {
	jsonStr := `[{"id":"schema1"},{"id":"schema2"}]`
	var fsr definition.FieldSchemaReference
	err := json.Unmarshal([]byte(jsonStr), &fsr)
	require.NoError(t, err)

	multipleSchemas, errA := definition.FieldSchemaAs[[]definition.SchemaReference](fsr)
	require.NoError(t, errA)
	require.NotNil(t, multipleSchemas)
	assert.True(t, fsr.IsMultiple())
	require.Len(t, multipleSchemas, 2)
	assert.Equal(t, definition.SchemaId("schema1"), multipleSchemas[0].ID)
	assert.Equal(t, definition.SchemaId("schema2"), multipleSchemas[1].ID)
}

func TestFieldSchemaReference_Helpers(t *testing.T) {
	// Test NewSchemaReference (single), IsSingle, IsZero
	singleSchemaRef := definition.SchemaReference{ID: "single"}
	fsrSingle := definition.NewSchemaReference(singleSchemaRef)
	assert.True(t, fsrSingle.IsSingle())
	assert.False(t, fsrSingle.IsMultiple())
	assert.False(t, fsrSingle.IsZero())
	singleSchema, err := definition.FieldSchemaAs[definition.SchemaReference](fsrSingle)
	require.NoError(t, err)
	assert.Equal(t, singleSchemaRef.ID, singleSchema.ID)

	// Test NewSchemaReference (multiple), IsMultiple, IsZero
	multiSchemaRefs := []definition.SchemaReference{{ID: "multi1"}, {ID: "multi2"}}
	fsrMulti := definition.NewSchemaReference(multiSchemaRefs)
	assert.False(t, fsrMulti.IsSingle())
	assert.True(t, fsrMulti.IsMultiple())
	assert.False(t, fsrMulti.IsZero())
	multipleSchemas, errM := definition.FieldSchemaAs[[]definition.SchemaReference](fsrMulti)
	require.NoError(t, errM)
	assert.Equal(t, multiSchemaRefs, multipleSchemas)

	// Test IsZero for an empty FieldSchemaReference
	fsrZero := definition.FieldSchemaReference{}
	assert.False(t, fsrZero.IsSingle())
	assert.False(t, fsrZero.IsMultiple())
	assert.True(t, fsrZero.IsZero())
}

func TestFieldSchemaReference_UnmarshalJSON_InvalidSingleSchema(t *testing.T) {
	jsonStr := `{"id": 123}` // id should be string
	var fsr definition.FieldSchemaReference
	err := json.Unmarshal([]byte(jsonStr), &fsr)
	require.Error(t, err) // Should error on single schema unmarshal
	var sysErr *common.SystemError
	ok := errors.As(err, &sysErr)
	assert.True(t, ok)
	assert.Equal(t, "ERR_SCHEMA_UNMARSHAL_FAILED", sysErr.Code)
}

func TestFieldSchemaReference_UnmarshalJSON_InvalidMultiSchema(t *testing.T) {
	jsonStr := `[{"id": 123}]` // id should be string
	var fsr definition.FieldSchemaReference
	err := json.Unmarshal([]byte(jsonStr), &fsr)
	require.Error(t, err) // Should error on multi schema unmarshal
	var sysErr *common.SystemError
	ok := errors.As(err, &sysErr)
	assert.True(t, ok)
	assert.Equal(t, "ERR_SCHEMA_UNMARSHAL_FAILED", sysErr.Code)
}

func TestFieldSchemaReference_UnmarshalJSON_ArrayPassedAsSingle(t *testing.T) {
	jsonStr := `[{"id":"schema1"}]` // This is an array, but json.Unmarshal can sometimes unmarshal arrays into structs if it's permissive
	var fsr definition.FieldSchemaReference
	err := json.Unmarshal([]byte(jsonStr), &fsr)
	require.NoError(t, err)
	// Expect it to unmarshal as multiple schemas, because the data started with '['
	assert.False(t, fsr.IsSingle())
	assert.True(t, fsr.IsMultiple())
	multipleSchemas, errA := definition.FieldSchemaAs[[]definition.SchemaReference](fsr)
	require.NoError(t, errA)
	require.Len(t, multipleSchemas, 1)
	assert.Equal(t, definition.SchemaId("schema1"), multipleSchemas[0].ID)
}


