package ephemeral_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestEphemeralSchemaManager_CreateCollection(t *testing.T) {
	_, manager := ephemeral.NewEphemeral()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(schemaDef)
	assert.NoError(t, err)

	exists, err := manager.CollectionExists("users")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestEphemeralSchemaManager_CreateCollection_AlreadyExists(t *testing.T) {
	_, manager := ephemeral.NewEphemeral()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(schemaDef)
	assert.NoError(t, err)

	err = manager.CreateCollection(schemaDef)
	assert.Error(t, err)
}

func TestEphemeralSchemaManager_DropCollection(t *testing.T) {
	_, manager := ephemeral.NewEphemeral()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(schemaDef)
	assert.NoError(t, err)

	err = manager.DropCollection("users")
	assert.NoError(t, err)

	exists, err := manager.CollectionExists("users")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestEphemeralSchemaManager_CreateIndex(t *testing.T) {
	_, manager := ephemeral.NewEphemeral()
	schemaDef := schema.SchemaDefinition{Name: "users"}
	err := manager.CreateCollection(schemaDef)
	assert.NoError(t, err)

	index := schema.IndexDefinition{Name: "my_index", Fields: []string{"name"}}
	err = manager.CreateIndex("users", index)
	assert.NoError(t, err)
}
