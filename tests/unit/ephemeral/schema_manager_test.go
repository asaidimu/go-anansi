package ephemeral_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestEphemeralSchemaManager_CreateCollection(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	exists, err := manager.CollectionExists(context.Background(),"users")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestEphemeralSchemaManager_CreateCollection_AlreadyExists(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	err = manager.CreateCollection(context.Background(),schemaDef)
	assert.Error(t, err)
}

func TestEphemeralSchemaManager_DropCollection(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	err = manager.DropCollection(context.Background(),"users")
	assert.NoError(t, err)

	exists, err := manager.CollectionExists(context.Background(),"users")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestEphemeralSchemaManager_CreateIndex(t *testing.T) {
	i := ephemeral.NewEphemeral()
	manager := i.SchemaManager()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	index := schema.IndexDefinition{Name: "my_index", Fields: []string{"name"}}
	err = manager.CreateIndex(context.Background(), schemaDef.Name, index)
	assert.NoError(t, err)
}
