package definition

import (
	"encoding/json"
	"fmt"
)

// ConstraintId represents the id of a constraint in a schema.
// IT IS A UUID v7 AND IS DISTINCT FROM NAME
type ConstraintId string

// SchemaId represents the id of a nested schema in a schema.
// IT IS A UUID v7 AND IS DISTINCT FROM NAME
type SchemaId string

// ResourceReference defines a reference to a component in the registry.
type ResourceReference struct {
	ID string `json:"id"`
}

// SchemaReference defines a reference to a nested schema.
type SchemaReference struct {
	ID          SchemaId                    `json:"id"`
	Indexes     map[IndexID]Index           `json:"indexes,omitempty"`
	Constraints map[ConstraintId]Constraint `json:"constraints,omitempty"`
	Type        FieldType                   `json:"type,omitempty"`
	Values      []LiteralValue              `json:"values,omitempty"`
}

type FieldSchemaKind byte

const (
	FieldSchemaKindSingle FieldSchemaKind = iota + 1
	FieldSchemaKindMultiple
)

type FieldSchemaReference struct {
	kind    FieldSchemaKind
	payload any // Either SchemaReference or []SchemaReference
}

// UnmarshalJSON implements json.Unmarshaler interface
// It tries to unmarshal as a single schema first, then as an array
func (fr *FieldSchemaReference) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	// Sniff the first character to determine if it's an Object { or Array [
	switch data[0] {
	case '{':
		var s SchemaReference
		if err := json.Unmarshal(data, &s); err != nil {
			return ErrUnmarshalFailed.WithCause(err).WithOperation("FieldSchemaReference.UnmarshalJSON")
		}
		fr.kind = FieldSchemaKindSingle
		fr.payload = s // Value-based storage
	case '[':
		var s []SchemaReference
		if err := json.Unmarshal(data, &s); err != nil {
			return ErrUnmarshalFailed.WithCause(err)
		}
		fr.kind = FieldSchemaKindMultiple
		fr.payload = s
	default:
		return ErrUnmarshalFailed.WithMessage("invalid schema reference format").WithOperation("FieldSchemaReference.UnmarshalJSON")
	}
	return nil
}

func (fr FieldSchemaReference) MarshalJSON() ([]byte, error) {
	if fr.payload == nil {
		return []byte("null"), nil
	}

	val, err := json.Marshal(fr.payload)
	if err != nil {
		return nil, ErrMarshalFailed.WithCause(err).WithOperation("FieldSchemaReference.MarshalJSON")
	}
	return val, nil
}

type SchemaReferenceType interface {
	SchemaReference | []SchemaReference
}

func (fr FieldSchemaReference) Value() any {
	// Either SchemaReference or []SchemaReference
	return fr.payload
}

func FieldSchemaAs[T SchemaReferenceType](fr FieldSchemaReference) (T, error) {
	var zero T
	val, ok := fr.payload.(T)
	if !ok {
		return zero, ErrTypeMismatch.WithMessage(
			fmt.Sprintf("type mismatch: requested %T but union contains kind %d", zero, fr.kind),
		)
	}
	return val, nil
}

func (ref SchemaReference) IsInline() bool {
	return len(ref.ID) == 0 && ( ref.Type != 0 || len(ref.Values) > 0)
}

// IsSingle returns true if this reference holds a single schema
func (fr FieldSchemaReference) IsSingle() bool {
	return fr.kind == FieldSchemaKindSingle
}

// IsMultiple returns true if this reference holds multiple schemas
func (fr FieldSchemaReference) IsMultiple() bool {
	return fr.kind == FieldSchemaKindMultiple
}

// IsZero returns true if the reference is empty
func (fr FieldSchemaReference) IsZero() bool {
	return fr.payload == nil
}

func NewSchemaReference[T SchemaReferenceType](payload T) FieldSchemaReference {
	fr := FieldSchemaReference{
		payload: payload,
	}

	switch any(payload).(type) {
	case SchemaReference:
		fr.kind = FieldSchemaKindSingle
	case []SchemaReference:
		fr.kind = FieldSchemaKindMultiple
	}
	return fr
}
