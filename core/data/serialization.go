package data

import (
	"context"
	"encoding/json"
	"fmt"
)

// FromJSON creates a Document from JSON bytes with enhanced error handling.
func FromJSON(data []byte) (Document, error) {
	if len(data) == 0 {
		return getFactory().newDocument(context.Background(), make(map[string]any))
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, &DocumentError{
			Operation: "FromJSON",
			Message:   ErrFailedToUnmarshalJSON.Error(),
			Cause:     fmt.Errorf("%w: %w", ErrFailedToUnmarshalJSON, err),
		}
	}
	return getFactory().newDocument(context.Background(), doc)
}

// FromStruct creates a Document from any struct using JSON marshaling.
func FromStruct(s any) (Document, error) {
	if s == nil {
		return make(Document), nil
	}

	data, err := json.Marshal(s)
	if err != nil {
		return nil, &DocumentError{
			Operation: "FromStruct",
			Message:   ErrFailedToMarshalStruct.Error(),
			Cause:     fmt.Errorf("%w: %w", ErrFailedToMarshalStruct, err),
		}
	}

	return FromJSON(data)
}

// ToJSON with pretty printing option.
func (d Document) ToJSON(pretty ...bool) ([]byte, error) {
	if len(pretty) > 0 && pretty[0] {
		return json.MarshalIndent(d, "", "  ")
	}
	return json.Marshal(d)
}

// ToStruct converts to a struct with better error handling.
func (d Document) ToStruct(target any) error {
	data, err := d.ToJSON()
	if err != nil {
		return &DocumentError{
			Operation: "ToStruct",
			Message:   ErrFailedToMarshalJSON.Error(),
			Cause:     fmt.Errorf("%w: %w", ErrFailedToMarshalJSON, err),
		}
	}

	if err := json.Unmarshal(data, target); err != nil {
		return &DocumentError{

			Operation: "ToStruct",
			Message:   ErrFailedToUnmarshalStruct.Error(),
			Cause:     fmt.Errorf("%w: %w", ErrFailedToUnmarshalStruct, err),
		}
	}

	return nil
}
