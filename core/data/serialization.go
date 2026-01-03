package data

import (
	"context"
	"encoding/json"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// FromJSON creates a Document from JSON bytes with enhanced error handling.
func FromJSON(data []byte) (*Document, error) {
	if len(data) == 0 {
		return getFactory().newDocument(context.Background(), make(map[string]any))
	}

	var docMap map[string]any
	if err := json.Unmarshal(data, &docMap); err != nil {
		return nil, common.SystemErrorFrom(ErrFailedToUnmarshalJSON).WithOperation("data.FromJSON").WithCause(err)
	}
	return getFactory().newDocument(context.Background(), docMap)
}

// FromStruct creates a Document from any struct using JSON marshaling.
func FromStruct(s any) (*Document, error) {
	if s == nil {
		return MustNewDocument(nil), nil
	}

	data, err := json.Marshal(s)
	if err != nil {
		return nil, common.SystemErrorFrom(ErrFailedToMarshalStruct).WithOperation("data.FromStruct").WithCause(err)
	}

	return FromJSON(data)
}

// ToJSON with pretty printing option.
func (d *Document) ToJSON(pretty ...bool) ([]byte, error) {
	if d == nil {
		return []byte("null"), nil
	}
	var (
		data []byte
		err  error
	)
	if len(pretty) > 0 && pretty[0] {
		data, err = json.MarshalIndent(d.data, "", "  ")
	} else {
		data, err = json.Marshal(d.data)
	}
	if err != nil {
		return nil, common.SystemErrorFrom(ErrFailedToMarshalJSON).WithOperation("data.Document.ToJSON").WithCause(err)
	}
	return data, nil
}

// ToStruct converts to a struct with better error handling.
func (d *Document) ToStruct(target any) error {
	data, err := d.ToJSON()
	if err != nil {
		return common.SystemErrorFrom(ErrFailedToMarshalJSON).WithOperation("data.Document.ToStruct").WithCause(err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return common.SystemErrorFrom(ErrFailedToUnmarshalStruct).WithOperation("data.Document.ToStruct").WithCause(err)
	}

	return nil
}