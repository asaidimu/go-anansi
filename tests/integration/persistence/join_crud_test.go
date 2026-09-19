package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	pevents "github.com/asaidimu/go-anansi/v8/core/persistence/events"
	"github.com/asaidimu/go-anansi/v8/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	rootutils "github.com/asaidimu/go-anansi/v8/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupJoinTest(t *testing.T) (base.Persistence, func()) {
	t.Helper()
	interactor, cleanup := createNativeInteractor(t)
	logger, _ := zap.NewDevelopment()
	bus, err := rootutils.NewInMemoryGoEventsBus("test")
	require.NoError(t, err)
	p, err := persistence.NewPersistence(interactor, pevents.NewGoEventsBusAdapter[base.PersistenceEvent](bus), logger, nil)
	require.NoError(t, err)
	return p, cleanup
}

func newUsersSchema() *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":   {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
}

func newOrdersSchema() *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"id":      {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"user_id": {Name: "user_id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"amount":  {Name: "amount", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
		},
	}
}

func TestJoin_LeftJoin_UsersOrders(t *testing.T) {
	p, cleanup := setupJoinTest(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create collections
	usersColl, err := p.CreateCollection(ctx, newUsersSchema())
	require.NoError(t, err)
	ordersColl, err := p.CreateCollection(ctx, newOrdersSchema())
	require.NoError(t, err)

	// 2. Insert data: 3 users, 3 orders (user3 has no orders)
	_, err = usersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice"}),
		data.MustNewDocument(map[string]any{"id": "u2", "name": "Bob"}),
		data.MustNewDocument(map[string]any{"id": "u3", "name": "Charlie"}),
	})
	require.NoError(t, err)

	_, err = ordersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "o1", "user_id": "u1", "amount": 100.50}),
		data.MustNewDocument(map[string]any{"id": "o2", "user_id": "u1", "amount": 25.00}),
		data.MustNewDocument(map[string]any{"id": "o3", "user_id": "u2", "amount": 75.25}),
	})
	require.NoError(t, err)

	// 3. Build LEFT JOIN query: users LEFT JOIN orders ON users.id = orders.user_id
	joinQuery := query.NewQueryBuilder().
		LeftJoin("orders").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					FieldRefVal: &query.FieldReference{
						Type:  "field",
						Field: "orders.user_id",
					},
				},
			},
		}).
		End().
		Build()

	// 4. Execute query on users collection
	result, err := usersColl.Read(ctx, &joinQuery)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 5. LEFT JOIN produces one row per match. u1 has 2 orders,
	//    u2 has 1, u3 has 0 → 2 + 1 + 1 = 4 rows.
	assert.Len(t, result.Data, 4)

	// 6. Verify flat document structure: fields from both tables merged.
	//    SQL columns are "users.id", "users.name", "orders.id", "orders.amount"
	//    etc. — the flat map uses these as keys directly.
	userCounts := map[string]int{}
	for _, doc := range result.Data {
		userID, err := doc.Get("users.id")
		require.NoError(t, err)
		require.NotNil(t, userID)

		uid := userID.(string)
		userCounts[uid]++

		userName, err := doc.Get("users.name")
		require.NoError(t, err)

		amount, _ := doc.Get("orders.amount")

		switch uid {
		case "u1":
			assert.Equal(t, "Alice", userName)
			// u1's rows have order amounts
			assert.NotNil(t, amount)
		case "u2":
			assert.Equal(t, "Bob", userName)
			assert.NotNil(t, amount)
		case "u3":
			assert.Equal(t, "Charlie", userName)
			// u3 has no orders — LEFT JOIN null-pads
			assert.Nil(t, amount)
		default:
			t.Errorf("unexpected user id: %v", uid)
		}
	}
	assert.Equal(t, 2, userCounts["u1"], "u1 should appear twice (2 orders)")
	assert.Equal(t, 1, userCounts["u2"], "u2 should appear once")
	assert.Equal(t, 1, userCounts["u3"], "u3 should appear once (null-padded)")
}

func TestJoin_InnerJoin_UsersOrders(t *testing.T) {
	p, cleanup := setupJoinTest(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create collections
	usersColl, err := p.CreateCollection(ctx, newUsersSchema())
	require.NoError(t, err)
	ordersColl, err := p.CreateCollection(ctx, newOrdersSchema())
	require.NoError(t, err)

	// 2. Insert data: 3 users, 2 orders (only u1 and u2 have orders)
	_, err = usersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice"}),
		data.MustNewDocument(map[string]any{"id": "u2", "name": "Bob"}),
		data.MustNewDocument(map[string]any{"id": "u3", "name": "Charlie"}),
	})
	require.NoError(t, err)

	_, err = ordersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "o1", "user_id": "u1", "amount": 100.00}),
		data.MustNewDocument(map[string]any{"id": "o2", "user_id": "u2", "amount": 50.00}),
	})
	require.NoError(t, err)

	// 3. Build INNER JOIN query
	joinQuery := query.NewQueryBuilder().
		InnerJoin("orders").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					FieldRefVal: &query.FieldReference{
						Type:  "field",
						Field: "orders.user_id",
					},
				},
			},
		}).
		End().
		Build()

	// 4. Execute
	result, err := usersColl.Read(ctx, &joinQuery)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 5. INNER JOIN: only 2 documents (u3 excluded)
	assert.Len(t, result.Data, 2)

	// 6. Each result has flat fields from both tables
	for _, doc := range result.Data {
		userID, err := doc.Get("users.id")
		require.NoError(t, err)
		require.NotNil(t, userID)

		_, errOrder := doc.Get("orders.id")
		require.NoError(t, errOrder, "INNER JOIN must have orders fields")

		id := userID.(string)
		assert.Contains(t, []string{"u1", "u2"}, id)
	}
}

func TestJoin_LeftJoin_WithFilter(t *testing.T) {
	p, cleanup := setupJoinTest(t)
	defer cleanup()

	ctx := context.Background()

	usersColl, err := p.CreateCollection(ctx, newUsersSchema())
	require.NoError(t, err)
	ordersColl, err := p.CreateCollection(ctx, newOrdersSchema())
	require.NoError(t, err)

	_, err = usersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice"}),
		data.MustNewDocument(map[string]any{"id": "u2", "name": "Bob"}),
		data.MustNewDocument(map[string]any{"id": "u3", "name": "Charlie"}),
	})
	require.NoError(t, err)

	_, err = ordersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "o1", "user_id": "u1", "amount": 100.00}),
		data.MustNewDocument(map[string]any{"id": "o2", "user_id": "u2", "amount": 200.00}),
		data.MustNewDocument(map[string]any{"id": "o3", "user_id": "u2", "amount": 50.00}),
	})
	require.NoError(t, err)

	// LEFT JOIN + filter: only users with orders.amount > 80
	joinQuery := query.NewQueryBuilder().
		LeftJoin("orders").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					FieldRefVal: &query.FieldReference{
						Type:  "field",
						Field: "orders.user_id",
					},
				},
			},
		}).
		End().
		Where("orders.amount").Gt(80.0).
		Build()

	result, err := usersColl.Read(ctx, &joinQuery)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Filter on joined field should narrow results
	for _, doc := range result.Data {
		amount, _ := doc.Get("orders.amount")
		if amount != nil {
			assert.Greater(t, amount, 80.0, "filtered orders should have amount > 80")
		}
	}
}

func TestJoin_EffectiveSchema(t *testing.T) {
	p, cleanup := setupJoinTest(t)
	defer cleanup()

	ctx := context.Background()

	usersColl, err := p.CreateCollection(ctx, newUsersSchema())
	require.NoError(t, err)
	ordersColl, err := p.CreateCollection(ctx, newOrdersSchema())
	require.NoError(t, err)

	_, err = usersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice"}),
	})
	require.NoError(t, err)

	_, err = ordersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "o1", "user_id": "u1", "amount": 100.00}),
	})
	require.NoError(t, err)

	// Join query
	joinQuery := query.NewQueryBuilder().
		InnerJoin("orders").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					FieldRefVal: &query.FieldReference{
						Type:  "field",
						Field: "orders.user_id",
					},
				},
			},
		}).
		End().
		Build()

	result, err := usersColl.Read(ctx, &joinQuery)
	require.NoError(t, err)
	require.Len(t, result.Data, 1)

	// The document should carry the effective schema
	doc := result.Data[0]
	sc := doc.EffectiveSchema()
	require.NotNil(t, sc, "join result should have an effective schema")
	// For join queries the effective schema is derived from SchemaFromQuery,
	// which generates a name like "<target>_joined_result".
	assert.Contains(t, sc.Name, "joined_result", "effective schema should be the derived join result schema")

	// Plain reads should carry the collection schema via the same
	// context-propagation path.
	plainQuery := query.NewQueryBuilder().Build()
	plainResult, err := usersColl.Read(ctx, &plainQuery)
	require.NoError(t, err)
	require.NotEmpty(t, plainResult.Data)
	plainSc := plainResult.Data[0].EffectiveSchema()
	require.NotNil(t, plainSc, "plain read should have an effective schema")
	assert.Contains(t, string(plainSc.Name), "users", "plain read schema should derive from the collection schema")
}

// TestJoin_WithProjection_NoAmbiguousMetadata is the end-to-end regression
// test for #metadata-ambiguous-on-joins: a join query with an explicit
// projection goes through ensureMetadataProjection (which injects a bare
// _metadata_ include). Both tables define _metadata_, so the generated SQL
// must qualify it with the primary table instead of failing with
// "ambiguous column name: _metadata_".
func TestJoin_WithProjection_NoAmbiguousMetadata(t *testing.T) {
	p, cleanup := setupJoinTest(t)
	defer cleanup()

	ctx := context.Background()

	usersColl, err := p.CreateCollection(ctx, newUsersSchema())
	require.NoError(t, err)
	ordersColl, err := p.CreateCollection(ctx, newOrdersSchema())
	require.NoError(t, err)

	_, err = usersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice"}),
	})
	require.NoError(t, err)

	_, err = ordersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "o1", "user_id": "u1", "amount": 100.00}),
	})
	require.NoError(t, err)

	joinQuery := query.NewQueryBuilder().
		Select().Include("name").End().
		InnerJoin("orders").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					FieldRefVal: &query.FieldReference{
						Type:  "field",
						Field: "orders.user_id",
					},
				},
			},
		}).
		End().
		Build()

	result, err := usersColl.Read(ctx, &joinQuery)
	require.NoError(t, err, "join with explicit projection must not raise ambiguous _metadata_")
	require.Len(t, result.Data, 1)

	doc := result.Data[0]
	name, err := doc.Get("name")
	require.NoError(t, err)
	assert.Equal(t, "Alice", name)
	// The injected _metadata_ include resolves to the primary table.
	meta, err := doc.Get("_metadata_")
	require.NoError(t, err)
	assert.NotNil(t, meta, "primary table _metadata_ must be present")
}

// TestJoin_JoinLevelProjection_LimitsColumns is the end-to-end regression
// test for #join-projection-ignored: a join WithProjection(Include) must
// restrict the joined table's columns in the executed SQL.
func TestJoin_JoinLevelProjection_LimitsColumns(t *testing.T) {
	p, cleanup := setupJoinTest(t)
	defer cleanup()

	ctx := context.Background()

	usersColl, err := p.CreateCollection(ctx, newUsersSchema())
	require.NoError(t, err)
	ordersColl, err := p.CreateCollection(ctx, newOrdersSchema())
	require.NoError(t, err)

	_, err = usersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice"}),
	})
	require.NoError(t, err)

	_, err = ordersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "o1", "user_id": "u1", "amount": 100.00}),
	})
	require.NoError(t, err)

	joinQuery := query.NewQueryBuilder().
		InnerJoin("orders").
		WithProjection(&query.ProjectionConfiguration{
			Include: []query.ProjectionField{{Name: "amount"}},
		}).
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					FieldRefVal: &query.FieldReference{
						Type:  "field",
						Field: "orders.user_id",
					},
				},
			},
		}).
		End().
		Build()

	result, err := usersColl.Read(ctx, &joinQuery)
	require.NoError(t, err)
	require.Len(t, result.Data, 1)

	doc := result.Data[0]
	m := doc.ToMap()
	assert.Contains(t, m, "orders.amount", "projected join column must be present")

	// Columns outside the join projection must be absent.
	for _, key := range []string{"orders.user_id", "orders.id"} {
		assert.NotContains(t, m, key, "non-projected join column %s must be absent", key)
	}

	// Primary-table columns are unaffected by the join projection.
	userID, err := doc.Get("users.id")
	require.NoError(t, err)
	assert.Equal(t, "u1", userID)
}
