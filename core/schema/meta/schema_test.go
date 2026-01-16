package meta_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/asaidimu/go-anansi/v6/core/schema/meta"
	"github.com/stretchr/testify/require"
)

func TestMetaSchema_MarshalUnmarshalJSON(t *testing.T) {
	mapData := meta.MetaSchema.AsMap()
	vd, err := definition.NewDocumentValidator(&meta.MetaSchema, make(definition.PredicateMap))
	require.NoError(t,err)

	issues, ok := vd.Validate(mapData)
	require.True(t, ok)
	require.Empty(t,issues)
}

