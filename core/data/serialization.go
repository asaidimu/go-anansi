package data

import (
	"encoding/json"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

// ToStruct converts to a struct with better error handling.
func (d *Document) ToStruct(target any) error {
	data, err := json.Marshal(d)
	if err != nil {
		return common.SystemErrorFrom(ErrFailedToMarshalJSON).WithOperation("data.Document.ToStruct").WithCause(err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return common.SystemErrorFrom(ErrFailedToUnmarshalStruct).WithOperation("data.Document.ToStruct").WithCause(err)
	}

	return nil
}
