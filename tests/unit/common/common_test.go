package common_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/assert"
)

func TestStripMetadata(t *testing.T) {
	data.ConfigureDocumentFactory(data.DocumentFactoryConfig{}, nil)
	// Create document with metadata
	doc, err := data.NewDocument(
		map[string]any{
			"name": "Test1",
			data.MetadataField: map[string]any{
				"created": 1754420381,
				"hash":    "a1ed91f3783c2a4f4de392df95f8d1c245be66bf68f9f098c7a5745d74b4c107",
				"updated": 1754420381,
				"version": 1,
			},
		})

	assert.NoError(t, err)
	// Strip metadata
	cp := doc.StripMetadata()

	// Expected result without metadata
	expected, err := data.NewDocument(map[string]any{"name": "Test1"})
	assert.NoError(t, err)

	// Assert equality
	assert.True(t, expected.Equals(cp))
}
