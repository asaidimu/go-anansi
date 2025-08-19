package persistence_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

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
		Version:     "1.0.0",
		Description: "test collection",
		Fields: map[string]*schema.FieldDefinition{
			"id":        {Name: "id", Type: "string", Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
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
	executor, err := sqliteExecutor.NewSQLiteInteractor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory()

	i, err := native.NewNativeInteractor(executor, queryFactory)
	require.NoError(t, err)
	return i, cleanup
}

func TestNewPersistence(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)

	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestPersistence_CreateAndGetCollection(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("my_collection")

	// Create the collection
	createdCollection, err := p.Create(context.Background(), *schema)
	require.NoError(t, err)
	assert.NotNil(t, createdCollection)

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
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("my_collection")

	// Create the collection
	_, err = p.Create(context.Background(), *schema)
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
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	var receivedEvent base.PersistenceEvent
	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		receivedEvent = event
		return nil
	}
	assert.NotNil(t, receivedEvent)
	// Register a subscription
	subID := p.RegisterSubscription(context.Background(), base.RegisterSubscriptionOptions{
		Event:    base.CollectionCreateSuccess,
		Callback: callback,
	})

	// Verify the subscription is active
	subs, err := p.Subscriptions(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	// Trigger an event
	_, err = p.Create(context.Background(), *newTestSchema("another_collection"))
	require.NoError(t, err)

	// Unregister the subscription
	p.UnregisterSubscription(context.Background(), subID)

	// Verify the subscription is gone
	subs, err = p.Subscriptions(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 0)
}

/* func TestPersistence_Transact(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("accounts")
	sc.Fields["balance"] = &schema.FieldDefinition{Name: "balance", Type: "number"}

	// Create the collection and some initial data outside the transaction
	accounts, err := p.Create(context.Background(), *sc)
	require.NoError(t, err)

	_, err = accounts.CreateMany(context.Background(), []data.Document{
		{"id": "A", "name": "Alice", "balance": 100.0},
		{"id": "B", "name": "Bob", "balance": 50.0},
	})

	// Perform a successful transfer within a transaction
	_, err = p.Transact(context.Background(), func(tx base.BasePersistence) (any, error) {
		acc, err := tx.Collection(context.Background(), "accounts")
		if err != nil {
			return nil, err
		}

		// Read Alice's document to get metadata
		aliceQuery := query.NewQueryBuilder().Where("id").Eq("A").Build()
		aliceResult, err := acc.Read(context.Background(), &aliceQuery)

		fmt.Printf("aliceDoc \n %v \n", aliceResult.Data)
		if err != nil {
			return nil, err
		}

		require.Equal(t, 1, aliceResult.Count)
		aliceDoc := aliceResult.Data.(data.Document)

		// Subtract 20 from Alice
		aliceDoc["balance"] = 80.0
		filterAlice := query.NewQueryBuilder().Where("id").Eq("A").Build().Filters
		_, err = acc.Update(context.Background(), &base.CollectionUpdate{Data: aliceDoc, Filter: filterAlice})
		if err != nil {
			return nil, fmt.Errorf("%w, \n %v \n", err, aliceDoc)
		}

		// Read Bob's document to get metadata
		bobQuery := query.NewQueryBuilder().Where("id").Eq("B").Build()
		bobResult, err := acc.Read(context.Background(), &bobQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, bobResult.Count)
		bobDoc := bobResult.Data.(data.Document)

		// Add 20 to Bob
		bobDoc["balance"] = 70.0
		filterBob := query.NewQueryBuilder().Where("id").Eq("B").Build().Filters
		_, err = acc.Update(context.Background(), &base.CollectionUpdate{Data: bobDoc, Filter: filterBob})
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
	for _, doc := range result.Data.([]data.Document) {
		balances[doc["id"].(string)] = doc["balance"]
	}

	assert.Equal(t, 80.0, balances["A"])
	assert.Equal(t, 70.0, balances["B"])

	// Perform a failing transaction
	_, err = p.Transact(context.Background(), func(tx base.BasePersistence) (any, error) {
		acc, err := tx.Collection(context.Background(), "accounts")
		if err != nil {
			return nil, err
		}

		// Read Alice's document to get metadata
		aliceQuery := query.NewQueryBuilder().Where("id").Eq("A").Build()
		aliceResult, err := acc.Read(context.Background(), &aliceQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, aliceResult.Count)
		aliceDoc := aliceResult.Data.(data.Document)

		// Subtract 10 from Alice
		aliceDoc["balance"] = 70.0
		filterAlice := query.NewQueryBuilder().Where("id").Eq("A").Build().Filters
		_, err = acc.Update(context.Background(), &base.CollectionUpdate{Data: aliceDoc, Filter: filterAlice})
		if err != nil {
			return nil, err
		}

		// This will fail because of a non-existent field, causing a rollback
		updateBob := data.Document{"non_existent_field": "error"}

		// We still need metadata for the update to pass the initial check
		bobQuery := query.NewQueryBuilder().Where("id").Eq("B").Build()
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

	// Verify the balances were rolled back and remain unchanged
	rollbackResult, err := finalAccounts.Read(context.Background(), &query.Query{})
	require.NoError(t, err)

	rollbackBalances := make(map[string]any)
	for _, doc := range rollbackResult.Data.([]data.Document) {
		rollbackBalances[doc["id"].(string)] = doc["balance"]
	}

	assert.Equal(t, 80.0, rollbackBalances["A"])
	assert.Equal(t, 70.0, rollbackBalances["B"])
}

func TestPersistence_Schema(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	// Create a schema and a collection based on it
	testSchema := newTestSchema("my_schema_collection")
	testSchema.Version = "1.0.0"
	_, err = p.Create(context.Background(), *testSchema)
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
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
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
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	// Close the persistence instance
	p.Close(context.Background())

	// Attempt an operation after closing, it should return an error
	_, err = p.Create(context.Background(), *newTestSchema("closed_collection"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "persistence instance is closed") // Assuming the event bus closure causes subsequent errors
}

func TestPersistence_CollectionNonExistent(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	// Try to get a collection that doesn't exist
	_, err = p.Collection(context.Background(), "non_existent_collection")
	assert.Error(t, err)
}

func TestPersistence_CreateWithInvalidSchema(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	// Create an invalid schema (e.g., missing name)
	invalidSchema := newTestSchema("")
	_, err = p.Create(context.Background(), *invalidSchema)
	assert.Error(t, err)
}

func TestPersistence_MetadataOnEmptyDB(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	// Get metadata from an empty database
	meta, err := p.Metadata(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, int64(0), *meta.CollectionCount)
	assert.Len(t, meta.Collections, 0)
	assert.Len(t, meta.Schemas, 0)
}

func TestPersistence_TransactWithPanic(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("accounts")
	sc.Fields["balance"] = &schema.FieldDefinition{Name: "balance", Type: "number"}

	accounts, err := p.Create(context.Background(), *sc)
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
		aliceDoc := aliceResult.Data.(data.Document)

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
		bobQuery := query.NewQueryBuilder().Where("id").Eq("B").Build()
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
} */

func TestPersistence_Metadata(t *testing.T) {

	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
	require.NoError(t, err)

	// Create some collections
	_, err = p.Create(context.Background(), *newTestSchema("coll1"))
	require.NoError(t, err)
	_, err = p.Create(context.Background(), *newTestSchema("coll2"))
	require.NoError(t, err)

	// Get metadata
	meta, err := p.Metadata(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, int64(2), *meta.CollectionCount)
	assert.Len(t, meta.Collections, 2)
	assert.Len(t, meta.Schemas, 2)
}

/*
func TestPersistence_SimpleLeftJoin(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger := zap.NewNop()
	options := base.MetadataOptions{
		HmacSecretKey: []byte("test-secret"),
	}

	p, err := persistence.NewPersistence(interactor, options, logger, nil)
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
	d := result.Data.([]data.Document)

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
