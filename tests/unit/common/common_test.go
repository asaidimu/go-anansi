package common_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/stretchr/testify/assert"
)

func TestStripMetadata(t *testing.T) {
	// Create document with metadata
	doc := common.Document{
		"id":   "1",
		"name": "Test1",
		common.MetadataFieldName: map[string]any{
			"created": 1754420381,
			"hash":    "a1ed91f3783c2a4f4de392df95f8d1c245be66bf68f9f098c7a5745d74b4c107",
			"updated": 1754420381,
			"version": 1,
		},
	}

	// Strip metadata
	doc = doc.StripMetadata()

	// Expected result without metadata
	expected := common.Document{"id": "1", "name": "Test1"}

	// Assert equality
	assert.Equal(t, expected, doc)
}
