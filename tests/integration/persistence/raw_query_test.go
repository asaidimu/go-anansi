package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPersistence_RawQuery_Update(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	v1, _ := common.NewVersion("1.0.0")
	productSchema := definition.Schema{
		Version: v1,
		BaseSchema: definition.BaseSchema{
			Name: "products",
			Fields: map[definition.FieldId]definition.Field{
				"pid":   {Name: "pid", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}, Required: true},
				"name":  {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"price": {Name: "price", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
		},
	}

	ctx := context.Background()
	productsCollection, err := p.CreateCollection(ctx, &productSchema)
	require.NoError(t, err)

	_, err = productsCollection.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"pid": "prod1", "name": "Laptop", "price": 1200.0}),
		data.MustNewDocument(map[string]any{"pid": "prod2", "name": "Mouse", "price": 25.0}),
	})
	require.NoError(t, err)

	rawUpdateQuery := &query.RawQuery{
		Template:   `UPDATE {{collections.products}} SET price = ? WHERE name = ?`,
		Parameters: []any{1300.0, "Laptop"},
		Collections: map[string]query.RawQueryTarget{
			"products": {Collection: "products"},
		},
	}

	result, err := p.Query(ctx, rawUpdateQuery)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, int64(1), result.AffectedRows)

	readQuery := query.NewQueryBuilder().Where("name").Eq("Laptop").Build()
	readResult, err := productsCollection.Read(ctx, &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)
	readDoc := readResult.Data[0]
	assert.Equal(t, 1300.0, readDoc.MustGet("price"))
}

func TestPersistence_RawQuery_LeftJoin(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	v1, _ := common.NewVersion("1.0.0")
	userSchema := definition.Schema{
		Version: v1,
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"uid":  {Name: "uid", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}, Required: true},
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	profileSchema := definition.Schema{
		Version: v1,
		BaseSchema: definition.BaseSchema{
			Name: "profiles",
			Fields: map[definition.FieldId]definition.Field{
				"user_id": {Name: "user_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}, Required: true},
				"bio":     {Name: "bio", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	ctx := context.Background()
	usersCollection, err := p.CreateCollection(ctx, &userSchema)
	require.NoError(t, err)
	profilesCollection, err := p.CreateCollection(ctx, &profileSchema)
	require.NoError(t, err)

	_, err = usersCollection.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"uid": "user1", "name": "Alice"}),
		data.MustNewDocument(map[string]any{"uid": "user2", "name": "Bob"}),
		data.MustNewDocument(map[string]any{"uid": "user3", "name": "Charlie"}),
	})
	require.NoError(t, err)

	_, err = profilesCollection.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"user_id": "user1", "bio": "Engineer"}),
		data.MustNewDocument(map[string]any{"user_id": "user3", "bio": "Artist"}),
	})
	require.NoError(t, err)

	rawLeftJoinQuery := &query.RawQuery{
		Template: `
			SELECT u.name, p.bio
			FROM {{collections.users}} u
			LEFT JOIN {{collections.profiles}} p ON u.uid = p.user_id
			ORDER BY u.name
		`,
		Collections: map[string]query.RawQueryTarget{
			"users":    {Collection: "users"},
			"profiles": {Collection: "profiles"},
		},
		Options: map[string]any{
			"expectRows": true,
		},
	}

	result, err := p.Query(ctx, rawLeftJoinQuery)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 3, result.Count)

	joinedDocs := result.Data.([]map[string]any)
	assert.Equal(t, "Alice", joinedDocs[0]["name"])
	assert.Equal(t, "Engineer", joinedDocs[0]["bio"])
	assert.Equal(t, "Bob", joinedDocs[1]["name"])
	assert.Nil(t, joinedDocs[1]["bio"])
	assert.Equal(t, "Charlie", joinedDocs[2]["name"])
	assert.Equal(t, "Artist", joinedDocs[2]["bio"])
}

func TestPersistence_RawQuery_SyntaxError(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	rawSyntaxErrorQuery := &query.RawQuery{
		Template: `SELECT FROM table_that_does_not_exist`,
	}

	_, err = p.Query(context.Background(), rawSyntaxErrorQuery)
	assert.Error(t, err)
}

func TestPersistence_RawQuery_Delete(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	v1, _ := common.NewVersion("1.0.0")
	productSchema := definition.Schema{
		Version: v1,
		BaseSchema: definition.BaseSchema{
			Name: "products",
			Fields: map[definition.FieldId]definition.Field{
				"pid":   {Name: "pid", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}, Required: true},
				"name":  {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"price": {Name: "price", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
		},
	}

	ctx := context.Background()
	productsCollection, err := p.CreateCollection(ctx, &productSchema)
	require.NoError(t, err)

	_, err = productsCollection.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"pid": "prod1", "name": "Laptop", "price": 1200.0}),
		data.MustNewDocument(map[string]any{"pid": "prod2", "name": "Mouse", "price": 25.0}),
	})
	require.NoError(t, err)

	rawDeleteQuery := &query.RawQuery{
		Template:   `DELETE FROM {{collections.products}} WHERE name = ?`,
		Parameters: []any{"Mouse"},
		Collections: map[string]query.RawQueryTarget{
			"products": {Collection: "products"},
		},
	}

	result, err := p.Query(ctx, rawDeleteQuery)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, int64(1), result.AffectedRows)

	readQuery := query.NewQueryBuilder().Build()
	readResult, err := productsCollection.Read(ctx, &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)
	readDoc := readResult.Data[0]
	assert.Equal(t, "Laptop", readDoc.MustGet("name"))
}

func TestPersistence_RawQuery_GroupConcat(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	v1, _ := common.NewVersion("1.0.0")
	orderSchema := definition.Schema{
		Version: v1,
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"order_id": {Name: "order_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}, Required: true},
				"user_id":  {Name: "user_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"product":  {Name: "product", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	ctx := context.Background()
	ordersCollection, err := p.CreateCollection(ctx, &orderSchema)
	require.NoError(t, err)

	_, err = ordersCollection.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"order_id": "orderA", "user_id": "user1", "product": "Laptop"}),
		data.MustNewDocument(map[string]any{"order_id": "orderB", "user_id": "user2", "product": "Mouse"}),
		data.MustNewDocument(map[string]any{"order_id": "orderC", "user_id": "user1", "product": "Keyboard"}),
	})
	require.NoError(t, err)

	rawGroupConcatQuery := &query.RawQuery{
		Template: `
			SELECT user_id, GROUP_CONCAT(product, ',') as products
			FROM {{collections.orders}}
			GROUP BY user_id
			ORDER BY user_id
		`,
		Collections: map[string]query.RawQueryTarget{
			"orders": {Collection: "orders"},
		},
		Options: map[string]any{
			"expectRows": true,
		},
	}

	result, err := p.Query(ctx, rawGroupConcatQuery)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 2, result.Count)

	aggDocs := result.Data.([]map[string]any)
	assert.Equal(t, "user1", aggDocs[0]["user_id"])
	assert.Contains(t, aggDocs[0]["products"].(string), "Laptop")
	assert.Contains(t, aggDocs[0]["products"].(string), "Keyboard")
	assert.Equal(t, "user2", aggDocs[1]["user_id"])
	assert.Equal(t, "Mouse", aggDocs[1]["products"])
}

func TestPersistence_RawQuery_MissingPlaceholder(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	rawMissingPlaceholderQuery := &query.RawQuery{
		Template: `SELECT * FROM {{collections.non_existent}}`,
		Collections: map[string]query.RawQueryTarget{
			"products": {Collection: "products"},
		},
	}

	_, err = p.Query(context.Background(), rawMissingPlaceholderQuery)
	assert.Error(t, err)
}

func TestPersistence_RawQuery_CollectionRead(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	v1, _ := common.NewVersion("1.0.0")
	productSchema := definition.Schema{
		Version: v1,
		BaseSchema: definition.BaseSchema{
			Name: "products",
			Fields: map[definition.FieldId]definition.Field{
				"pid":   {Name: "pid", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}, Required: true},
				"name":  {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"price": {Name: "price", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
		},
	}

	ctx := context.Background()
	productsCollection, err := p.CreateCollection(ctx, &productSchema)
	require.NoError(t, err)

	_, err = productsCollection.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"pid": "prod1", "name": "Laptop", "price": 1200.0}),
		data.MustNewDocument(map[string]any{"pid": "prod2", "name": "Mouse", "price": 25.0}),
		data.MustNewDocument(map[string]any{"pid": "prod3", "name": "Keyboard", "price": 75.0}),
	})
	require.NoError(t, err)

	rawReadQuery := &query.Query{
		Raw: &query.RawQuery{
			Template:   `SELECT * FROM {{collections.products}} WHERE price > ?`,
			Parameters: []any{50.0},
			Collections: map[string]query.RawQueryTarget{
				"products": {Collection: "products"},
			},
		},
	}

	readResult, err := productsCollection.Read(ctx, rawReadQuery)
	require.NoError(t, err)
	assert.Equal(t, 2, len(readResult.Data))

	readDocs := readResult.Data
	assert.Len(t, readDocs, 2)
}
