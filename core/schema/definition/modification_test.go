package definition

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustFieldID returns a fresh UUIDv7 FieldId.
func mustFieldID() FieldId {
	return FieldId(uuid.Must(uuid.NewV7()).String())
}

func TestSchema_WithSchema_ComposesSubSchema(t *testing.T) {
	// Envelope schema that already owns a nested schema with ID "n1".
	envelope := &Schema{
		Name: "Envelope",
		Schemas: map[SchemaId]NestedSchema{
			SchemaId("n1"): {BaseSchema: BaseSchema{Name: "boilerplate"}},
		},
	}

	// DTO schema whose root field references its own nested schema "n1"
	// (a deliberate collision with the envelope's "n1").
	innerID := mustFieldID()
	streetID := mustFieldID()

	dto := &Schema{
		Name: "Address",
		Fields: map[FieldId]Field{
			innerID: {
				Name: "address",
				FieldProperties: FieldProperties{
					Type:   FieldTypeObject,
					Schema: NewSchemaReference(SchemaReference{ID: SchemaId("n1")}),
				},
			},
		},
		Schemas: map[SchemaId]NestedSchema{
			SchemaId("n1"): {
				BaseSchema: BaseSchema{
					Name: "AddressInner",
					Fields: map[FieldId]Field{
						streetID: {Name: "street", FieldProperties: FieldProperties{Type: FieldTypeString}},
					},
				},
			},
		},
	}

	composed, dtoID, err := envelope.WithSchema(dto)
	require.NoError(t, err)
	require.NotEqual(t, SchemaId(""), dtoID)

	// The composed body must be referenceable.
	body, ok := composed.Schemas[dtoID]
	require.True(t, ok, "composed schema must be registered under returned id")
	assert.Equal(t, "Address", body.Name)
	require.Len(t, body.Fields, 1)

	// The envelope's original nested schema must be untouched.
	orig, ok := composed.Schemas[SchemaId("n1")]
	require.True(t, ok)
	assert.Equal(t, "boilerplate", orig.Name)

	// The DTO's colliding nested schema must have been remapped to a fresh id
	// and the root body's reference rewritten to point at the new id.
	bodyField := body.Fields[innerID]
	single, err := FieldSchemaAs[SchemaReference](bodyField.Schema)
	require.NoError(t, err)
	require.NotEqual(t, SchemaId("n1"), single.ID, "reference must be rewritten away from colliding id")
	_, exists := composed.Schemas[single.ID]
	require.True(t, exists, "rewritten reference must point at an existing nested schema")
	assert.Equal(t, "AddressInner", composed.Schemas[single.ID].Name)

	// Composed schema must be usable as a root field via the returned id.
	envelopeWithField := composed.WithField(mustFieldID(), Field{
		Name: "payload",
		FieldProperties: FieldProperties{
			Type:   FieldTypeObject,
			Schema: NewSchemaReference(SchemaReference{ID: dtoID}),
		},
	})
	_, f, ok := envelopeWithField.GetFieldByName("payload")
	require.True(t, ok)
	single2, err := FieldSchemaAs[SchemaReference](f.Schema)
	require.NoError(t, err)
	assert.Equal(t, dtoID, single2.ID)

	// The original envelope must not be mutated.
	require.NotContains(t, envelope.Schemas, dtoID)
	assert.Len(t, envelope.Schemas, 1)
}

func TestSchema_WithSchema_NoCollision_KeepsSubIds(t *testing.T) {
	envelope := &Schema{Name: "Envelope"}
	innerID := mustFieldID()
	dto := &Schema{
		Name: "Payload",
		Fields: map[FieldId]Field{
			innerID: {
				Name: "body",
				FieldProperties: FieldProperties{
					Type:   FieldTypeObject,
					Schema: NewSchemaReference(SchemaReference{ID: SchemaId("sub1")}),
				},
			},
		},
		Schemas: map[SchemaId]NestedSchema{
			SchemaId("sub1"): {BaseSchema: BaseSchema{Name: "Sub"}},
		},
	}

	composed, dtoID, err := envelope.WithSchema(dto)
	require.NoError(t, err)

	// No collision -> nested schema keeps its own id.
	_, ok := composed.Schemas[SchemaId("sub1")]
	require.True(t, ok)

	// Root field reference still points at "sub1".
	body := composed.Schemas[dtoID]
	ref, err := FieldSchemaAs[SchemaReference](body.Fields[innerID].Schema)
	require.NoError(t, err)
	assert.Equal(t, SchemaId("sub1"), ref.ID)
}

func TestSchema_WithSchema_NilSub(t *testing.T) {
	envelope := &Schema{Name: "Envelope"}
	composed, dtoID, err := envelope.WithSchema(nil)
	require.Error(t, err)
	assert.Nil(t, composed)
	assert.Equal(t, SchemaId(""), dtoID)
}