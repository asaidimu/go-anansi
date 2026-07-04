package data

import (
	"encoding/json"
	"fmt"
)

// MarshalJSON implements the json.Marshaler interface.
func (d *Document) MarshalJSON() ([]byte, error) {
	if d == nil || d.data == nil {
		return []byte("null"), nil
	}
	val := d.ToMap()
	return json.Marshal(val)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *Document) UnmarshalJSON(data []byte) error {
	if d == nil {
		return fmt.Errorf("cannot unmarshal into nil document")
	}
	if d.data == nil {
		d.data = make(map[string]any)
	}
	var val map[string]any
	json.Unmarshal(data, &val)
	doc, err := NewDocument(val)
	if err != nil {
		return err
	}
	*d = *doc
	return nil
}
