package definition

import (
	"encoding/json"

	"github.com/asaidimu/go-anansi/v7/core/common"
)

// FromJSON parses a byte slice containing JSON into a Schema object.
func FromJSON(data []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, common.SystemErrorFrom(err).WithMessage("failed to unmarshal schema JSON")
	}
	return &s, nil
}
