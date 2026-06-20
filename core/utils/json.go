package utils

import (
	"encoding/json"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// Clonable is an interface for objects that can be cloned.
type Clonable[T any] interface {
	Clone() (T, error)
}

// ToJSONer is an interface for objects that can be converted to JSON.
type ToJSONer interface {
	ToJSON() (string, error)
	ToJSONBytes() ([]byte, error)
}

// FromJSON populates an object from a JSON byte slice.
func FromJSON[T any](data []byte, target *T) error {
	if err := json.Unmarshal(data, target); err != nil {
		return common.SystemErrorFrom(err).WithOperation("FromJSON")
	}
	return nil
}

// Unmarshal parses a JSON byte slice and returns a new object of type T.
func Unmarshal[T any](data []byte) (T, error) {
	var target T
	if err := json.Unmarshal(data, &target); err != nil {
		return target, common.SystemErrorFrom(err).WithOperation("Unmarshal")
	}
	return target, nil
}

// ToJSON marshals an object to a JSON string.
func ToJSON(v any) (string, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return "", common.SystemErrorFrom(err).WithOperation("ToJSON")
	}
	return string(bytes), nil
}

// ToJSONBytes marshals an object to a JSON byte slice.
func ToJSONBytes(v any) ([]byte, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation("ToJSONBytes")
	}
	return bytes, nil
}

// ToJSONIndent marshals an object to a pretty-printed JSON byte slice.
func ToJSONIndent(v any) ([]byte, error) {
	bytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation("ToJSONIndent")
	}
	return bytes, nil
}

// Clone creates a deep copy of a Clonable object by marshalling and unmarshalling it.
func Clone[T any](from T, to *T) error {
	bytes, err := json.Marshal(from)
	if err != nil {
		return common.SystemErrorFrom(err).WithOperation("Clone")
	}
	return json.Unmarshal(bytes, to)
}
