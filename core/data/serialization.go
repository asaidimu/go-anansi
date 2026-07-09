package data

import (
	"encoding/json"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

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
		data, err = json.MarshalIndent(d, "", "  ")
	} else {
		data, err = json.Marshal(d)
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
