package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetaSchema_MarshalUnmarshalJSON(t *testing.T) {
	// Marshal the schema
	marshaledData, err := json.MarshalIndent(definition.MetaSchema, "", "  ")
	require.NoError(t, err)

	// Unmarshal the schema back into a new struct
	var unmarshaledMetaSchema definition.Schema
	err = json.Unmarshal(marshaledData, &unmarshaledMetaSchema)
	require.NoError(t, err)

	var data map[string]any
	err = json.Unmarshal(marshaledData, &data)
	require.NoError(t, err)

	vd, err := definition.NewDocumentValidator(&definition.MetaSchema, nil)
	require.NoError(t, err)

	issues, valid := vd.Validate(data)
	assert.True(t, valid, "validation failed, issues: %v", issues)
	assert.Empty(t, issues)
	// Assert deep equality between the original and unmarshaled schema
	assert.Equal(t, definition.MetaSchema, unmarshaledMetaSchema)
}
