package meta_test

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/asaidimu/go-anansi/v6/core/schema/meta"
	"github.com/stretchr/testify/require"
)

func TestMetaSchema_MarshalUnmarshalJSON(t *testing.T) {
	// Marshal the schema
	/* marshaledData, err := json.MarshalIndent(meta.MetaSchema, "", "  ")
	require.NoError(t, err) */

	marshaledData := meta.MetaSchema.ToJSON()
	mapData := meta.MetaSchema.AsMap()

	fmt.Printf("schema \n%s\n", marshaledData)
	/*
	// Unmarshal the schema back into a new struct
	var schemaData map[string]any
	err := json.Unmarshal(marshaledData, &schemaData)
	require.Error(t, err) */

	vd, err := definition.NewDocumentValidator(&meta.MetaSchema, make(definition.PredicateMap))
	require.NoError(t,err)

	issues, ok := vd.Validate(mapData)
	require.True(t, ok)
	require.Empty(t,issues)
}

