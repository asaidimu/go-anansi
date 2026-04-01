package meta_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/asaidimu/go-anansi/v6/core/schema/meta"
	"github.com/stretchr/testify/require"
)

func TestMetaSchema_MarshalUnmarshalJSON(t *testing.T) {
	marshaledData := meta.MetaSchema.ToJSON()
	mapData := meta.MetaSchema.AsMap()

	// Unmarshal the schema back into a new struct
	var schemaData map[string]any
	err := json.Unmarshal(marshaledData, &schemaData)
	require.NoError(t, err)

	vd, err := definition.NewDocumentValidator(&meta.MetaSchema, make(definition.PredicateMap))
	require.NoError(t,err)

	issues, ok := vd.Validate(mapData)
	require.True(t, ok)
	require.Empty(t,issues)
}

