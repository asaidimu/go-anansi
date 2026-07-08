package ephemeral_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEphemeralSchemaManager_CreateCollection(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{Name: "users"}}
	err := manager.CreateCollection(context.Background(), schemaDef)
	assert.NoError(t, err)

	exists, err := manager.CollectionExists(context.Background(), "users")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestEphemeralSchemaManager_CreateCollection_AlreadyExists(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{Name: "users"}}
	err := manager.CreateCollection(context.Background(), schemaDef)
	assert.NoError(t, err)

	err = manager.CreateCollection(context.Background(), schemaDef)
	assert.Error(t, err)
}

func TestEphemeralSchemaManager_DropCollection(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{Name: "users"}}
	err := manager.CreateCollection(context.Background(), schemaDef)
	assert.NoError(t, err)

	err = manager.DropCollection(context.Background(), "users")
	assert.NoError(t, err)

	exists, err := manager.CollectionExists(context.Background(), "users")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestEphemeralSchemaManager_CreateIndex(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{Name: "users"}}
	err := manager.CreateCollection(context.Background(), schemaDef)
	assert.NoError(t, err)

	index := definition.Index{Name: "my_index", Fields: []definition.FieldName{"name"}}
	err = manager.CreateIndex(context.Background(), schemaDef.Name, index)
	assert.NoError(t, err)
}

func TestEphemeralSchemaManager_AddColumn(t *testing.T) {
	i := ephemeral.NewEphemeral()
	sm := i.SchemaManager()

	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{Name: "users"}}
	err := sm.CreateCollection(context.Background(), schemaDef)
	require.NoError(t, err)

	err = sm.AddColumn(context.Background(), "users", definition.Field{
		Name: "email",
		FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString},
	})
	assert.NoError(t, err)
}

func TestEphemeralSchemaManager_DropColumn(t *testing.T) {
	i := ephemeral.NewEphemeral()
	sm := i.SchemaManager()

	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{
		Name: "users",
		Fields: map[definition.FieldId]definition.Field{
			"f1": {Name: "email"},
		},
	}}
	err := sm.CreateCollection(context.Background(), schemaDef)
	require.NoError(t, err)

	err = sm.DropColumn(context.Background(), "users", "email")
	assert.NoError(t, err)
}

func TestEphemeralSchemaManager_RenameColumn(t *testing.T) {
	i := ephemeral.NewEphemeral()
	sm := i.SchemaManager()

	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{
		Name: "users",
		Fields: map[definition.FieldId]definition.Field{
			"f1": {Name: "old_name"},
		},
	}}
	err := sm.CreateCollection(context.Background(), schemaDef)
	require.NoError(t, err)

	err = sm.RenameColumn(context.Background(), "users", "old_name", "new_name")
	assert.NoError(t, err)
}

func TestEphemeralSchemaManager_DropColumn_NonExistent(t *testing.T) {
	i := ephemeral.NewEphemeral()
	sm := i.SchemaManager()

	schemaDef := definition.Schema{BaseSchema: definition.BaseSchema{Name: "users"}}
	err := sm.CreateCollection(context.Background(), schemaDef)
	require.NoError(t, err)

	err = sm.DropColumn(context.Background(), "users", "nonexistent")
	assert.NoError(t, err)
}

func TestEphemeralSchemaManager_Capabilities(t *testing.T) {
	i := ephemeral.NewEphemeral()
	caps := i.Capabilities()
	assert.True(t, caps.SchemaEvolution.AddColumn)
	assert.True(t, caps.SchemaEvolution.DropColumn)
	assert.True(t, caps.SchemaEvolution.RenameColumn)
	assert.True(t, caps.SchemaEvolution.AlterColumnType)
	assert.True(t, caps.SchemaEvolution.AddConstraint)
	assert.True(t, caps.SchemaEvolution.DropConstraint)
}

func TestEphemeralSchemaManager_AddColumn_NonExistentCollection(t *testing.T) {
	i := ephemeral.NewEphemeral()
	sm := i.SchemaManager()

	err := sm.AddColumn(context.Background(), "nonexistent", definition.Field{
		Name: "x",
	})
	assert.Error(t, err)
}

func TestEphemeralSchemaManager_DropColumn_NonExistentCollection(t *testing.T) {
	i := ephemeral.NewEphemeral()
	sm := i.SchemaManager()

	err := sm.DropColumn(context.Background(), "nonexistent", "x")
	assert.Error(t, err)
}

func TestEphemeralSchemaManager_RenameColumn_NonExistentCollection(t *testing.T) {
	i := ephemeral.NewEphemeral()
	sm := i.SchemaManager()

	err := sm.RenameColumn(context.Background(), "nonexistent", "x", "y")
	assert.Error(t, err)
}
