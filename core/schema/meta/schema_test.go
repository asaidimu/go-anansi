package meta_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/asaidimu/go-anansi/v6/core/schema/meta"
	"github.com/stretchr/testify/require"
)

func TestMetaSchema_MarshalUnmarshalJSON(t *testing.T) {
	// Marshal the schema
	marshaledData, err := json.MarshalIndent(meta.MetaSchema, "", "  ")
	require.NoError(t, err)

	// Unmarshal the schema back into a new struct
	var schemaData map[string]any
	err = json.Unmarshal(marshaledData, &schemaData)
	require.NoError(t, err)

	fmt.Printf("schema \n%s\n", schemaData["schema"])
	vd, err := definition.NewDocumentValidator(&meta.MetaSchema, make(definition.PredicateMap))
	require.NoError(t,err)

	issues, ok := vd.Validate(schemaData)
	require.True(t, ok)
	require.Empty(t,issues)
}

