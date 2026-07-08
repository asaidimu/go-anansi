package query_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/sqlite/query"
	"github.com/stretchr/testify/assert"
)

func TestSQLiteCapabilities_SchemaEvolution(t *testing.T) {
	factory := query.NewSQLiteFactory(nil)
	caps := factory.Capabilities()

	assert.True(t, caps.SchemaEvolution.AddColumn)
	assert.True(t, caps.SchemaEvolution.DropColumn)
	assert.False(t, caps.SchemaEvolution.RenameColumn)
	assert.False(t, caps.SchemaEvolution.AlterColumnType)
	assert.False(t, caps.SchemaEvolution.AddConstraint)
	assert.False(t, caps.SchemaEvolution.DropConstraint)
}
