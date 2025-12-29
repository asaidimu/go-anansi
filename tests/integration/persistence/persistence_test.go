package persistence_test

import (
	// "context"
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// setupTestDB creates a unique, in-memory SQLite database for each test.
// The database is automatically cleaned up when the returned function is called.
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	// The DSN `file:%s?mode=memory&cache=shared` creates a unique, named in-memory
	// database. The `cache=shared` part allows multiple connections within the
	// same test to access the same in-memory database. The database is destroyed
	// when the last connection to it is closed.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())

	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
	}

	var version string
	err = db.QueryRow("SELECT sqlite_version()").Scan(&version)
	require.NoError(t, err)
	t.Logf("SQLite Version: %s", version)

	return db, cleanup
}

func newTestSchema(name ...string) *schema.SchemaDefinition {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	return &schema.SchemaDefinition{
		Name:        sname,
		Version:     "8.0.0",
		Description: "test collection",
		Fields: map[string]*schema.FieldDefinition{
			"name":      {Name: "name", Type: "string", Required: utils.BoolPtr(true)},
			"age":       {Name: "age", Type: "integer"},
			"is_active": {Name: "is_active", Type: "boolean"},
			"price":     {Name: "price", Type: "number"},
		},
	}
}

func createNativeInteractor(t *testing.T) (query.DatabaseInteractor, func()) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory()

	i, err := native.NewNativeInteractor(executor, queryFactory, logger)
	require.NoError(t, err)
	return i, cleanup
}

func TestNewPersistence(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)

	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestPersistence_CreateAndGetCollection(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)


	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("my_collection")

	logger.Debug("Started create collection")

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
	// Create the collection
	createdCollection, err := p.CreateCollection(timeoutCtx, *schema)
	require.NoError(t, err)
	assert.NotNil(t, createdCollection)
	logger.Debug("Completed create collection")

	// Get the collection
	retrievedCollection, err := p.Collection(context.Background(), "my_collection")
	require.NoError(t, err)
	assert.NotNil(t, retrievedCollection)

	// Ensure they are the same instance
	assert.Equal(t, createdCollection, retrievedCollection)
}

func TestPersistence_DeleteCollection(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("my_collection")

	// Create the collection
	_, err = p.CreateCollection(context.Background(), *schema)
	require.NoError(t, err)

	// Delete the collection
	deleted, err := p.Delete(context.Background(), "my_collection")
	require.NoError(t, err)
	assert.True(t, deleted)

	// Verify the collection is gone
	_, err = p.Collection(context.Background(), "my_collection")
	assert.Error(t, err)
}

func TestPersistence_Subscriptions(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	var receivedEvent base.PersistenceEvent
	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		receivedEvent = event
		return nil
	}
	assert.NotNil(t, receivedEvent)
	// Register a subscription
	subID := p.Subscribe(context.Background(), base.SubscriptionOptions{
		Event:    base.CollectionCreateSuccess,
		Callback: callback,
	})

	// Verify the subscription is active
	subs, err := p.Subscriptions(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	// Trigger an event
	_, err = p.CreateCollection(context.Background(), *newTestSchema("another_collection"))
	require.NoError(t, err)

	// Unregister the subscription
	p.Unsubscribe(context.Background(), subID)

	// Verify the subscription is gone
	subs, err = p.Subscriptions(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 0)
}

func TestPersistence_Transact(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("accounts")
	sc.Fields["balance"] = &schema.FieldDefinition{Name: "balance", Type: "number"}

	// Create the collection and some initial data outside the transaction
	accounts, err := p.CreateCollection(context.Background(), *sc)
	require.NoError(t, err)

	_, err = accounts.CreateMany(context.Background(), []data.Document{
		{"name": "Alice", "balance": 100.0},
		{"name": "Bob", "balance": 50.0},
	})

	// Perform a successful transfer within a transaction
	_, err = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
		acc, err := tx.Collection(tctx, "accounts")
		if err != nil {
			return nil, err
		}

		// Read Alice's document to get metadata
		aliceQuery := query.NewQueryBuilder().Where("name").Eq("Alice").Build()
		aliceResult, err := acc.Read(tctx, &aliceQuery)

		if err != nil {
			return nil, err
		}

		require.Equal(t, 1, aliceResult.Count)

		// Subtract 20 from Alice
		aliceDoc := data.Document{ "balance": 80 }
		filterAlice := query.NewQueryBuilder().Where("name").Eq("Alice").Build().Filters
		_, err = acc.Update(tctx, &base.CollectionUpdate{Set: aliceDoc, Filter: filterAlice})
		if err != nil {
			return nil, fmt.Errorf("%w, \n %v \n", err, aliceDoc)
		}

		// Read Bob's document to get metadata
		bobQuery := query.NewQueryBuilder().Where("name").Eq("Bob").Build()
		bobResult, err := acc.Read(tctx, &bobQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, bobResult.Count)
		bobDoc := bobResult.Data[0]

		// Add 20 to Bob
		bobDoc["balance"] = 70.0
		filterBob := query.NewQueryBuilder().Where("name").Eq("Bob").Build().Filters
		_, err = acc.Update(tctx, &base.CollectionUpdate{Set: bobDoc, Filter: filterBob})
		if err != nil {
			return nil, fmt.Errorf("%w, \n %v \n", err, bobDoc)
		}

		return nil, nil
	})

	require.NoError(t, err)

	// Verify the balances outside the transaction
	finalAccounts, err := p.Collection(context.Background(), "accounts")
	require.NoError(t, err)
	result, err := finalAccounts.Read(context.Background(), &query.Query{})
	require.NoError(t, err)

	balances := make(map[string]any)
	for _, doc := range result.Data {
		balances[doc["name"].(string)] = doc["balance"]
	}

	assert.Equal(t, 80.0, balances["Alice"])
	assert.Equal(t, 70.0, balances["Bob"])

	// Perform a failing transaction
	_, err = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
		// Read Alice's document to get metadata
		aliceQuery := query.NewQueryBuilder().Where("name").Eq("Alice").Build()
		aliceResult, err := accounts.Read(tctx, &aliceQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, aliceResult.Count)
		aliceDoc := aliceResult.Data[0]

		// Subtract 10 from Alice
		aliceDoc["balance"] = 70.0
		filterAlice := query.NewQueryBuilder().Where("name").Eq("Alice").Build().Filters
		_, err = accounts.Update(tctx, &base.CollectionUpdate{Set: aliceDoc, Filter: filterAlice})
		if err != nil {
			return nil, err
		}

		// This will fail because of a non-existent field, causing a rollback
		updateBob := data.Document{"non_existent_field": "error"}

		// We still need metadata for the update to pass the initial check
		bobQuery := query.NewQueryBuilder().Where("name").Eq("Bob").Build()
		bobResult, err := accounts.Read(tctx, &bobQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, bobResult.Count)
		bobDoc := bobResult.Data[0]
		meta, _ := bobDoc.Metadata()
		updateBob.SetMetadata(meta)

		filterBob := query.NewQueryBuilder().Where("name").Eq("Bob").Build().Filters
		_, err = accounts.Update(tctx, &base.CollectionUpdate{Set: updateBob, Filter: filterBob})

		return nil, err // Propagate the error to trigger rollback
	})

	require.Error(t, err)

	// Verify the balances were rolled back and remain unchanged
	rollbackResult, err := finalAccounts.Read(context.Background(), &query.Query{})
	require.NoError(t, err)

	rollbackBalances := make(map[string]any)
	for _, doc := range rollbackResult.Data {
		rollbackBalances[doc["name"].(string)] = doc["balance"]
	}

	assert.Equal(t, 80.0, rollbackBalances["Alice"])
	assert.Equal(t, 70.0, rollbackBalances["Bob"])
}

func TestPersistence_Schema(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Create a schema and a collection based on it
	testSchema := newTestSchema("my_schema_collection")
	testSchema.Version = "1.0.0"
	_, err = p.CreateCollection(context.Background(), *testSchema)
	require.NoError(t, err)

	// Retrieve the schema by ID
	retrievedSchema, err := p.Schema(context.Background(), "my_schema_collection")
	require.NoError(t, err)
	assert.NotNil(t, retrievedSchema)
	assert.Equal(t, testSchema.Description, retrievedSchema.Description)
	assert.Equal(t, testSchema.Version, retrievedSchema.Version)

	// Retrieve the schema by ID and version
	retrievedSchemaWithVersion, err := p.Schema(context.Background(), "my_schema_collection", "1.0.0")
	require.NoError(t, err)
	assert.NotNil(t, retrievedSchemaWithVersion)
	assert.Equal(t, testSchema.Description, retrievedSchemaWithVersion.Description)
	assert.Equal(t, testSchema.Version, retrievedSchemaWithVersion.Version)

	// Try to retrieve a non-existent schema
	_, err = p.Schema(context.Background(), "non_existent_schema")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collection not found")

	// Try to retrieve an existent schema with a non-existent version
	_, err = p.Schema(context.Background(), "my_schema_collection", "2.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version '2.0.0' not found for collection 'my_schema_collection'")
}

func TestPersistence_DeleteNonExistentCollection(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Try to delete a collection that doesn't exist
	deleted, err := p.Delete(context.Background(), "non_existent_collection")
	require.Error(t, err)
	assert.False(t, deleted)
}

func TestPersistence_Close(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Close the persistence instance
	p.Close(context.Background())

	// Attempt an operation after closing, it should return an error
	_, err = p.CreateCollection(context.Background(), *newTestSchema("closed_collection"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "persistence instance is closed") // Assuming the event bus closure causes subsequent errors
}

func TestPersistence_CollectionNonExistent(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Try to get a collection that doesn't exist
	_, err = p.Collection(context.Background(), "non_existent_collection")
	assert.Error(t, err)
}

func TestPersistence_CreateWithInvalidSchema(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Create an invalid schema (e.g., missing name)
	invalidSchema := newTestSchema("")
	_, err = p.CreateCollection(context.Background(), *invalidSchema)
	assert.Error(t, err)
}

func TestPersistence_MetadataOnEmptyDB(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil,  logger, nil)
	require.NoError(t, err)

	// Get metadata from an empty database
	meta, err := p.Metadata(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, int64(0), *meta.CollectionCount)
	assert.Len(t, meta.Collections, 0)
	assert.Len(t, meta.Schemas, 0)
}

/*
func TestPersistence_TransactWithPanic(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("accounts")
	sc.Fields["balance"] = &schema.FieldDefinition{Name: "balance", Type: "number"}

	accounts, err := p.CreateCollection(context.Background(), *sc)
	require.NoError(t, err)

	_, err = accounts.CreateMany(context.Background(), []data.Document{
		{"id": "A", "name": "Alice", "balance": 100.0},
	})
	require.NoError(t, err)

	// Perform a transaction that fails and should rollback
	_, err = p.Transact(context.Background(), func(tx base.BasePersistence) (any, error) {
		acc, err := tx.Collection(context.Background(), "accounts")
		if err != nil {
			return nil, err
		}

		// Read Alice's document
		aliceQuery := query.NewQueryBuilder().Where("id").Eq("A").Build()
		aliceResult, err := acc.Read(context.Background(), &aliceQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, aliceResult.Count)
		aliceDoc := aliceResult.Data[0]

		// Subtract 10 from Alice
		aliceDoc["balance"] = 50.0
		filterAlice := query.NewQueryBuilder().Where("id").Eq("A").Build().Filters
		_, err = acc.Update(context.Background(), &base.CollectionUpdate{Data: aliceDoc, Filter: filterAlice})
		if err != nil {
			return nil, err
		}

		// This will fail because of a non-existent field, causing a rollback
		updateBob := data.Document{"non_existent_field": "error"}

		// We still need metadata for the update to pass the initial check
		bobQuery := query.NewQueryBuilder().Where("id").Eq("A").Build()
		bobResult, err := acc.Read(context.Background(), &bobQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, bobResult.Count)
		bobDoc := bobResult.Data.(data.Document)
		meta, _ := bobDoc.Metadata()
		updateBob.SetMetadata(meta)

		filterBob := query.NewQueryBuilder().Where("id").Eq("B").Build().Filters
		_, err = acc.Update(context.Background(), &base.CollectionUpdate{Data: updateBob, Filter: filterBob})

		return nil, err // Propagate the error to trigger rollback
	})

	require.Error(t, err)

	// Verify that the balance was rolled back
	finalAccounts, err := p.Collection(context.Background(), "accounts")
	require.NoError(t, err)
	result, err := finalAccounts.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)
	doc := result.Data.(data.Document)
	balances := make(map[string]any)
	balances[doc["id"].(string)] = doc["balance"]

	assert.Equal(t, 100.0, balances["A"])
}

func TestPersistence_Metadata(t *testing.T) {

	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, logger, nil)
	require.NoError(t, err)

	// Create some collections
	_, err = p.CreateCollection(context.Background(), *newTestSchema("coll1"))
	require.NoError(t, err)
	_, err = p.CreateCollection(context.Background(), *newTestSchema("coll2"))
	require.NoError(t, err)

	// Get metadata
	meta, err := p.Metadata(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, int64(2), *meta.CollectionCount)
	assert.Len(t, meta.Collections, 2)
	assert.Len(t, meta.Schemas, 2)
}

func TestPersistence_SimpleLeftJoin(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()


	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// 1. Define Schemas
	userSchema := schema.SchemaDefinition{
		Name:    "users",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"name": {Name: "name", Type: schema.FieldTypeString},
		},
	}

	profileSchema := schema.SchemaDefinition{
		Name:    "profiles",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"user": {Name: "user", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"bio":  {Name: "bio", Type: schema.FieldTypeString},
		},
	}

	// 2. Create Collections
	usersCollection, err := p.Create(context.Background(), userSchema)
	require.NoError(t, err)
	profilesCollection, err := p.Create(context.Background(), profileSchema)
	require.NoError(t, err)

	// 3. Insert Data
	_, err = usersCollection.CreateMany(context.Background(), []data.Document{
		{"id": "user1", "name": "Alice"},
		{"id": "user2", "name": "Bob"},
		{"id": "user3", "name": "Charlie"},
	})

	require.NoError(t, err)

	_, err = profilesCollection.CreateMany(context.Background(), []data.Document{
		{"user": "user1", "bio": "Loves Go programming"},
		{"user": "user2", "bio": "Enjoys testing"},
	})

	require.NoError(t, err)

	// 4. Construct LEFT JOIN Query
	joinQuery := query.NewQueryBuilder().
		LeftJoin("profiles"). // Logical name for the right side
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id", // Physical name of left collection
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					FieldRefVal: &query.FieldReference{
						Type:  "field",
						Field: "profiles.user", // Logical name of right collection
					},
				},
			},
		}).
		End().
		Build()

	// 5. Execute Query on the 'users' collection
	result, err := usersCollection.Read(context.Background(), &joinQuery)
	require.NoError(t, err)
	assert.NotNil(t, result)
	d := result.Data

	// 6. Assert Results
	assert.Len(t, d, 3) // Expecting 3 documents (all users)

	// Verify content
	for _, doc := range d {
		userData, userOk := doc["users"].(data.Document)
		profileData, profileOk := doc["profiles"].(data.Document)

		assert.True(t, userOk)

		switch userData["id"] {
		case "user1":
			assert.True(t, profileOk)
			assert.Equal(t, "Alice", userData["name"])
			assert.Equal(t, "Loves Go programming", profileData["bio"])
		case "user2":
			assert.True(t, profileOk)
			assert.Equal(t, "Bob", userData["name"])
			assert.Equal(t, "Enjoys testing", profileData["bio"])
		case "user3":
			assert.False(t, profileOk)          // No profile for user3
			assert.Nil(t, doc["user_profiles"]) // Ensure it's explicitly nil
			assert.Equal(t, "Charlie", userData["name"])
		default:
			t.Errorf("Unexpected user ID: %v", userData["id"])
		}
	}
} */

func TestPersistence_RawQueryWithJoin(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// 1. Define Schemas
	userSchema := schema.SchemaDefinition{
		Name:    "users",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"uid":   {Name: "uid", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"name": {Name: "name", Type: schema.FieldTypeString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "ix_uid", Fields: []string{"uid"}, Type: schema.IndexTypeNormal},
		},
	}

	orderSchema := schema.SchemaDefinition{
		Name:    "orders",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"order_id": {Name: "order_id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"user_id":  {Name: "user_id", Type: schema.FieldTypeString},
			"amount":   {Name: "amount", Type: schema.FieldTypeNumber},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "order_id_pk", Fields: []string{"order_id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()

	// 2. Create Collections
	_, err = p.CreateCollection(ctx, userSchema)
	require.NoError(t, err)
	_, err = p.CreateCollection(ctx, orderSchema)
	require.NoError(t, err)

	// 3. Insert Data
	usersCollection, err := p.Collection(ctx, "users")
	require.NoError(t, err)
	_, err = usersCollection.CreateMany(ctx, []data.Document{
		data.MustNewDocument(map[string]any{"uid": "user1", "name": "Alice"}),
		data.MustNewDocument(map[string]any{"uid": "user2", "name": "Bob"}),
	})
	require.NoError(t, err)

	ordersCollection, err := p.Collection(ctx, "orders")
	require.NoError(t, err)
	_, err = ordersCollection.CreateMany(ctx, []data.Document{
		data.MustNewDocument(map[string]any{"order_id": "orderA", "user_id": "user1", "amount": 100.0}),
		data.MustNewDocument(map[string]any{"order_id": "orderB", "user_id": "user2", "amount": 200.0}),
		data.MustNewDocument(map[string]any{"order_id": "orderC", "user_id": "user1", "amount": 150.0}),
	})
	require.NoError(t, err)

	// 4. Construct and execute a raw JOIN query
	rawJoinQuery := &query.RawQuery{
		Template: `
            SELECT u.uid AS user_id, u.name AS user_name, o.order_id, o.amount
            FROM {{collections.users}} u
            JOIN {{collections.orders}} o ON u.uid = o.user_id
            WHERE u.name = ?
            ORDER BY o.amount DESC
        `,
		Parameters: []any{"Alice"},
		Collections: map[string]query.RawQueryTarget{
			"users":  {Collection: "users"},
			"orders": {Collection: "orders"},
		},
		Options: map[string]any{
			"expectRows": true,
		},
	}

	result, err := p.Query(ctx, rawJoinQuery)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 2, result.Count)
	assert.Len(t, result.Data, 2)

	// 5. Assert Results
	joinedDocs := result.Data.([]data.Document)

	// Expecting two orders for Alice, sorted by amount DESC
	assert.Equal(t, "user1", joinedDocs[0]["user_id"])
	assert.Equal(t, "Alice", joinedDocs[0]["user_name"])
	assert.Equal(t, "orderC", joinedDocs[0]["order_id"])
	assert.Equal(t, float64(150), joinedDocs[0]["amount"])

	assert.Equal(t, "user1", joinedDocs[1]["user_id"])
	assert.Equal(t, "Alice", joinedDocs[1]["user_name"])
	assert.Equal(t, "orderA", joinedDocs[1]["order_id"])
	assert.Equal(t, float64(100), joinedDocs[1]["amount"])
}

func TestPersistence_CollectionReadWithRawQuery(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// 1. Define Schema for products
	productSchema := schema.SchemaDefinition{
		Name:    "products",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"pid":   {Name: "pid", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"name":  {Name: "name", Type: schema.FieldTypeString},
			"price": {Name: "price", Type: schema.FieldTypeNumber},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "ix_pid", Fields: []string{"pid"}, Type: schema.IndexTypeNormal},
		},
	}

	ctx := context.Background()

	// 2. Create Collection
	productsCollection, err := p.CreateCollection(ctx, productSchema)
	require.NoError(t, err)

	// 3. Insert Data
	_, err = productsCollection.CreateMany(ctx, []data.Document{
		data.MustNewDocument(map[string]any{"pid": "prod1", "name": "Laptop", "price": 1200.0}),
		data.MustNewDocument(map[string]any{"pid": "prod2", "name": "Mouse", "price": 25.0}),
		data.MustNewDocument(map[string]any{"pid": "prod3", "name": "Keyboard", "price": 75.0}),
		data.MustNewDocument(map[string]any{"pid": "prod4", "name": "Monitor", "price": 300.0}),
	})
	require.NoError(t, err)

	// 4. Construct a query.Query with a RawQuery for collection.Read()
	rawReadQuery := &query.Query{
		Raw: &query.RawQuery{
			Template: `SELECT pid, name, price FROM {{collections.products}} WHERE price > ? ORDER BY price DESC`,
			Parameters: []any{float64(50.0)},
			Collections: map[string]query.RawQueryTarget{
				"products": {Collection: "products"},
			},
			Options: map[string]any{
				"expectRows": true,
			},
		},
	}

	// 5. Execute Query on the 'products' collection
	result, err := productsCollection.Read(ctx, rawReadQuery)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Count)
	assert.Len(t, result.Data, 3)

	// 6. Assert Results
	readDocs := result.Data

	// Expecting three products, sorted by price DESC
	assert.Equal(t, "prod1", readDocs[0]["pid"])
	assert.Equal(t, "Laptop", readDocs[0]["name"])
	assert.Equal(t, float64(1200.0), readDocs[0]["price"])

	assert.Equal(t, "prod4", readDocs[1]["pid"])
	assert.Equal(t, "Monitor", readDocs[1]["name"])
	assert.Equal(t, float64(300.0), readDocs[1]["price"])

	assert.Equal(t, "prod3", readDocs[2]["pid"])
	assert.Equal(t, "Keyboard", readDocs[2]["name"])
	assert.Equal(t, float64(75.0), readDocs[2]["price"])
}
