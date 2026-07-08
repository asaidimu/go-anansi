package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewPersistence(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)

	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestPersistence_CreateAndGetCollection(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schemaDef := newTestSchema("my_collection")

	// Create the collection
	createdCollection, err := p.CreateCollection(context.Background(), schemaDef)
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
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schemaDef := newTestSchema("my_collection")

	// Create the collection
	_, err = p.CreateCollection(context.Background(), schemaDef)
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
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// var receivedEvent base.PersistenceEvent
	callback := func(ctx context.Context, event base.PersistenceEvent) error {
		// receivedEvent = event
		return nil
	}

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
	_, err = p.CreateCollection(context.Background(), newTestSchema("another_collection"))
	require.NoError(t, err)

	// Unregister the subscription
	p.Unsubscribe(context.Background(), subID)

	// Verify the subscription is gone
	subs, err = p.Subscriptions(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 0)
}

func TestPersistence_Transact(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("accounts")
	sc.BaseSchema.Fields["balance"] = definition.Field{
		Name: "balance",
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeNumber,
		},
	}

	// Create the collection and some initial data outside the transaction
	accounts, err := p.CreateCollection(context.Background(), sc)
	require.NoError(t, err)

	r, err := accounts.CreateMany(context.Background(), []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Alice", "balance": 100.0}),
		data.MustNewDocument(map[string]any{"name": "Bob", "balance": 50.0}),
	})
	if err != nil {
		t.Logf("Error creating multiple accounts: %v", err)
	}
	require.NoError(t, err)

	ida := r[0].Data.ID()
	idb := r[1].Data.ID()

	// Perform a successful transfer within a transaction
	_, err = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
		// Read Alice's document to get metadata
		aliceQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(ida).Build()
		aliceResult, err := accounts.Read(tctx, &aliceQuery)
		if err != nil {
			return nil, err
		}

		require.Equal(t, 1, aliceResult.Count)
		aliceDoc := aliceResult.Data[0]

		// Subtract 20 from Alice
		aliceDoc.Set("balance", 80.0)
		filterAlice := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(ida).Build().Filters

		// we can nest transactions, but don't
		_, err = p.Transact(tctx, func(ctx context.Context, p base.BasePersistence) (any, error) {
			return accounts.Update(tctx, &base.CollectionUpdate{Set: aliceDoc, Filter: filterAlice})
		})

		if err != nil {
			return nil, err
		}

		// or run methods asynchronously
		tx.Async(tctx, func(ctx context.Context) (any, error) { // this runs in a go function
			// Read Bob's document to get metadata
			bobQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(idb).Build()
			bobResult, err := accounts.Read(ctx, &bobQuery)
			if err != nil {
				return nil, err
			}
			require.Equal(t, 1, bobResult.Count)
			bobDoc := bobResult.Data[0]

			// Add 20 to Bob
			bobDoc.Set("balance", 70.0)
			filterBob := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(idb).Build().Filters
			_, err = accounts.Update(ctx, &base.CollectionUpdate{Set: bobDoc, Filter: filterBob})
			if err != nil {
				return nil, err
			}
			return nil, nil
		})

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
		balances[doc.ID()] = doc.Must().GetFloat64("balance")
	}

	assert.Equal(t, 80.0, balances[ida])
	assert.Equal(t, 70.0, balances[idb])

	// Perform a failing transaction
	_, err = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
		acc, err := tx.Collection(tctx, "accounts")
		if err != nil {
			return nil, err
		}

		// Read Alice's document to get metadata
		aliceQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(ida).Build()
		aliceResult, err := acc.Read(tctx, &aliceQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, aliceResult.Count)
		aliceDoc := aliceResult.Data[0]

		// Subtract 10 from Alice
		aliceDoc.Set("balance", 70.0)
		filterAlice := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(ida).Build().Filters
		_, err = acc.Update(tctx, &base.CollectionUpdate{Set: aliceDoc, Filter: filterAlice})
		if err != nil {
			return nil, err
		}

		// This will fail because of a non-existent field, causing a rollback
		updateBob := data.MustNewDocument(map[string]any{"non_existent_field": "error"})

		// We still need metadata for the update to pass the initial check
		bobQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(idb).Build()
		bobResult, err := acc.Read(tctx, &bobQuery)
		if err != nil {
			return nil, err
		}
		require.Equal(t, 1, bobResult.Count)
		bobDoc := bobResult.Data[0]
		meta := bobDoc.Metadata()
		updateBob.SetMetadata(meta)

		filterBob := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(idb).Build().Filters
		_, err = acc.Update(tctx, &base.CollectionUpdate{Set: updateBob, Filter: filterBob})

		return nil, err // Propagate the error to trigger rollback
	})

	require.Error(t, err)

	// Verify the balances were rolled back and remain unchanged
	rollbackResult, err := finalAccounts.Read(context.Background(), &query.Query{})
	require.NoError(t, err)

	rollbackBalances := make(map[string]any)
	for _, doc := range rollbackResult.Data {
		rollbackBalances[doc.ID()] = doc.Must().GetFloat64("balance")
	}

	assert.Equal(t, 80.0, rollbackBalances[ida])
	assert.Equal(t, 70.0, rollbackBalances[idb])
}

func TestPersistence_Schema(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Create a schema and a collection based on it
	testSchemaDef := newTestSchema("my_schema_collection")
	_, err = p.CreateCollection(context.Background(), testSchemaDef)
	require.NoError(t, err)

	// Retrieve the schema by ID
	retrievedSchema, err := p.Schema(context.Background(), "my_schema_collection")
	require.NoError(t, err)
	assert.NotNil(t, retrievedSchema)
	assert.Equal(t, testSchemaDef.Description, retrievedSchema.Description)
	assert.Equal(t, testSchemaDef.Version.String(), retrievedSchema.Version.String())

	// Retrieve the schema by ID and version
	retrievedSchemaWithVersion, err := p.Schema(context.Background(), "my_schema_collection", "1.0.0")
	require.NoError(t, err)
	assert.NotNil(t, retrievedSchemaWithVersion)
	assert.Equal(t, testSchemaDef.Description, retrievedSchemaWithVersion.Description)
	assert.Equal(t, testSchemaDef.Version.String(), retrievedSchemaWithVersion.Version.String())

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
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Try to delete a collection that doesn't exist
	deleted, err := p.Delete(context.Background(), "non_existent_collection")
	require.Error(t, err)
	assert.False(t, deleted)
}

func TestPersistence_Close(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Close the persistence instance
	p.Close(context.Background())

	// Attempt an operation after closing, it should return an error
	_, err = p.CreateCollection(context.Background(), newTestSchema("closed_collection"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "persistence instance is closed")
}

func TestPersistence_CollectionNonExistent(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Try to get a collection that doesn't exist
	_, err = p.Collection(context.Background(), "non_existent_collection")
	assert.Error(t, err)
}

func TestPersistence_CreateWithInvalidSchema(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Create an invalid schema (e.g., missing name)
	invalidSchema := newTestSchema("")
	_, err = p.CreateCollection(context.Background(), invalidSchema)
	assert.Error(t, err)
}

func TestPersistence_MetadataOnEmptyDB(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Get metadata from an empty database
	meta, err := p.Metadata(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, int64(0), *meta.CollectionCount)
	assert.Len(t, meta.Collections, 0)
	assert.Len(t, meta.Schemas, 0)
}

func TestPersistence_TransactWithPanic(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	sc := newTestSchema("accounts")
	sc.BaseSchema.Fields["balance"] = definition.Field{
		Name: "balance",
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeNumber,
		},
	}

	accounts, err := p.CreateCollection(context.Background(), sc)
	require.NoError(t, err)

	r, err := accounts.CreateMany(context.Background(), []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Alice", "balance": 100.0}),
	})
	if err != nil {
		t.Logf("Error creating single account for panic test: %v", err)
	}
	require.NoError(t, err)

	id := r[0].Data.ID()
	// Perform a transaction that panics
	assert.Panics(t, func() {
		_, _ = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
			acc, err := tx.Collection(tctx, "accounts")
			if err != nil {
				return nil, err
			}

			// Read Alice's document
			aliceQuery := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
			aliceResult, err := acc.Read(context.Background(), &aliceQuery)
			if err != nil {
				return nil, err
			}
			require.Equal(t, 1, aliceResult.Count)
			aliceDoc := aliceResult.Data[0]

			// Update Alice's balance
			aliceDoc.Set("balance", 50.0)
			filterAlice := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters
			_, err = acc.Update(tctx, &base.CollectionUpdate{Set: aliceDoc, Filter: filterAlice})
			if err != nil {
				return nil, err
			}

			// Panic!
			panic("something went wrong")
		})
	})

	// Verify that the balance was rolled back
	finalAccounts, err := p.Collection(context.Background(), "accounts")
	require.NoError(t, err)
	result, err := finalAccounts.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)
	doc := result.Data[0]
	balances := make(map[string]any)
	balances[doc.ID()] = doc.Must().GetFloat64("balance")

	assert.Equal(t, 100.0, balances[id])
}

func TestPersistence_Metadata(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Create some collections
	_, err = p.CreateCollection(context.Background(), newTestSchema("coll1"))
	require.NoError(t, err)
	_, err = p.CreateCollection(context.Background(), newTestSchema("coll2"))
	require.NoError(t, err)

	// Get metadata
	meta, err := p.Metadata(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, int64(2), *meta.CollectionCount)
	assert.Len(t, meta.Collections, 2)
	assert.Len(t, meta.Schemas, 2)
}

func TestPersistence_SimpleLeftJoin(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// 1. Define Schemas
	userSchema := definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"idi":  {Name: "idi", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	profileSchema := definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "profiles",
			Fields: map[definition.FieldId]definition.Field{
				"user": {Name: "user", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"bio":  {Name: "bio", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	// 2. Create Collections
	usersCollection, err := p.CreateCollection(context.Background(), &userSchema)
	require.NoError(t, err)
	profilesCollection, err := p.CreateCollection(context.Background(), &profileSchema)
	require.NoError(t, err)

	// 3. Insert Data
	_, err = usersCollection.CreateMany(context.Background(), []*data.Document{
		data.MustNewDocument(map[string]any{"idi": "user1", "name": "Alice"}),
		data.MustNewDocument(map[string]any{"idi": "user2", "name": "Bob"}),
		data.MustNewDocument(map[string]any{"idi": "user3", "name": "Charlie"}),
	})
	if err != nil {
		t.Logf("Error creating multiple users: %v", err)
	}

	require.NoError(t, err)

	_, err = profilesCollection.CreateMany(context.Background(), []*data.Document{
		data.MustNewDocument(map[string]any{"user": "user1", "bio": "Loves Go programming"}),
		data.MustNewDocument(map[string]any{"user": "user2", "bio": "Enjoys testing"}),
	})
	if err != nil {
		t.Logf("Error creating multiple profiles: %v", err)
	}

	require.NoError(t, err)

	// 4. Construct LEFT JOIN Query
	joinQuery := query.NewQueryBuilder().
		LeftJoin("profiles"). // Logical name for the right side
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.idi", // Physical name of left collection
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
		userData, errUser := doc.GetDocument("users")
		require.NoError(t, errUser) // We expect "users" to always exist

		profileData, errProfile := doc.GetDocument("profiles") // profileData can be nil if not found

		switch userData.Must().GetString("idi") {
		case "user1":
			assert.NoError(t, errProfile) // We expect a profile here
			assert.Equal(t, "Alice", userData.Must().GetString("name"))
			assert.Equal(t, "Loves Go programming", profileData.Must().GetString("bio"))
		case "user2":
			assert.NoError(t, errProfile) // We expect a profile here
			assert.Equal(t, "Bob", userData.Must().GetString("name"))
			assert.Equal(t, "Enjoys testing", profileData.Must().GetString("bio"))
		case "user3":
			assert.NoError(t, errProfile)         // We expect NO profile here
			assert.True(t, profileData.IsEmpty()) // Ensure profileData is nil
			assert.Equal(t, "Charlie", userData.Must().GetString("name"))
		default:
			t.Errorf("Unexpected user ID: %v", userData.Must().GetString("idi"))
		}
	}
}

func TestPersistence_Migrate_PhaseSchemaOnly(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("migrate_schema_only")
	_, err = p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "migrate_schema_only",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	plan := base.NewSchemaOnlyMigration(target, "add email field")
	_, err = p.Migrate(ctx, "migrate_schema_only", plan, nil)
	require.NoError(t, err)

	sc, err := p.Schema(ctx, "migrate_schema_only")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", sc.Version.String())
}

func TestPersistence_Migrate_PhaseSchemaOnly_ReactivePropagation(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("migrate_reactive")
	coll, err := p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	// Create a document with the original schema
	_, err = coll.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Alice"}),
	})
	require.NoError(t, err)

	// Migrate to add a field via PhaseSchemaOnly
	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "migrate_reactive",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	plan := base.NewSchemaOnlyMigration(target, "add email field")
	newColl, err := p.Migrate(ctx, "migrate_reactive", plan, nil)
	require.NoError(t, err)

	// The old collection reference should see the new schema reactively
	_, err = coll.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Bob", "email": "bob@test.com"}),
	})
	assert.NoError(t, err, "old collection reference should accept new fields after reactive propagation")

	// Verify the returned collection can also use the new field
	_, err = newColl.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Charlie", "email": "charlie@test.com"}),
	})
	assert.NoError(t, err)

	// Verify all data is intact (no orphaned physical collection)
	result, err := coll.Read(ctx, &query.Query{})
	require.NoError(t, err)
	assert.Len(t, result.Data, 3, "should have 3 documents — data not lost")

	// Verify data preserved from before migration
	sc, err := p.Schema(ctx, "migrate_reactive")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", sc.Version.String())
}

func TestPersistence_Migrate_PhaseFull(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("migrate_full")
	coll, err := p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	_, err = coll.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Alice"}),
		data.MustNewDocument(map[string]any{"name": "Bob"}),
	})
	require.NoError(t, err)

	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "migrate_full",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "name_upper", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	transformer := func(_ context.Context, doc data.Document) (data.Document, error) {
		d := doc.ToMap()
		name, _ := d["name"].(string)
		d["name_upper"] = name
		delete(d, "name")
		return *data.MustNewDocument(d), nil
	}

	plan := base.NewFullMigration(target, "rename name to name_upper", transformer)
	_, err = p.Migrate(ctx, "migrate_full", plan, nil)
	require.NoError(t, err)

	sc, err := p.Schema(ctx, "migrate_full")
	require.NoError(t, err)
	require.NotNil(t, sc.Version)
	assert.Equal(t, "2.0.0", sc.Version.String())

	coll, err = p.Collection(ctx, "migrate_full")
	require.NoError(t, err)

	result, err := coll.Read(ctx, &query.Query{})
	require.NoError(t, err)
	require.Len(t, result.Data, 2)
}

func TestPersistence_Migrate_PhaseDDL_FallbackToFull(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("migrate_ddl_fallback")
	coll, err := p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	_, err = coll.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Alice"}),
	})
	require.NoError(t, err)

	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "migrate_ddl_fallback",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "full_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	transformer := func(_ context.Context, doc data.Document) (data.Document, error) {
		d := doc.ToMap()
		d["full_name"] = d["name"]
		delete(d, "name")
		return *data.MustNewDocument(d), nil
	}

	plan := &base.MigrationPlan{
		Description: "rename field with fallback",
		Target:      target,
		Phase:       base.PhaseDDL,
		Transformer: transformer,
	}
	_, err = p.Migrate(ctx, "migrate_ddl_fallback", plan, nil)
	require.NoError(t, err)
}

func TestPersistence_Rollback(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("rollback_test")
	_, err = p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "rollback_test",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	plan := base.NewSchemaOnlyMigration(target, "add email")
	_, err = p.Migrate(ctx, "rollback_test", plan, nil)
	require.NoError(t, err)

	sc, err := p.Schema(ctx, "rollback_test")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", sc.Version.String())

	rolledBack, err := p.Rollback(ctx, "rollback_test", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, rolledBack)

	sc, err = p.Schema(ctx, "rollback_test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", sc.Version.String())
}

func TestPersistence_Migrate_DryRun(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("dryrun_test")
	_, err = p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "dryrun_test",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	dryRun := true
	plan := base.NewSchemaOnlyMigration(target, "dry run")
	_, err = p.Migrate(ctx, "dryrun_test", plan, &dryRun)
	require.NoError(t, err)

	sc, err := p.Schema(ctx, "dryrun_test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", sc.Version.String())
}

func TestPersistence_Migrate_InvalidParam(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	_, err = p.Migrate(ctx, "test", "not-a-plan", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a *base.MigrationPlan")
}

func TestPersistence_Rollback_NoPreviousVersion(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("rollback_none")
	_, err = p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	_, err = p.Rollback(ctx, "rollback_none", nil, nil)
	assert.Error(t, err)
}

func TestPersistence_E2E_MigrateWithTransformer(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("e2e_transform")
	coll, err := p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	docs := []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Alice", "age": 30}),
		data.MustNewDocument(map[string]any{"name": "Bob", "age": 25}),
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "e2e_transform",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"f3": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f4": {Name: "is_adult", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
			},
		},
	}

	transformer := func(_ context.Context, doc data.Document) (data.Document, error) {
		d := doc.ToMap()
		d["email"] = d["name"].(string) + "@example.com"
		d["is_adult"] = d["age"].(int) >= 18
		return *data.MustNewDocument(d), nil
	}

	plan := base.NewFullMigration(target, "add email and is_adult", transformer)
	_, err = p.Migrate(ctx, "e2e_transform", plan, nil)
	require.NoError(t, err)

	coll, err = p.Collection(ctx, "e2e_transform")
	require.NoError(t, err)

	result, err := coll.Read(ctx, &query.Query{})
	require.NoError(t, err)
	require.Len(t, result.Data, 2)

	for _, doc := range result.Data {
		email, err := doc.Get("email")
		require.NoError(t, err)
		name, err := doc.Get("name")
		require.NoError(t, err)
		isAdult, err := doc.Get("is_adult")
		require.NoError(t, err)

		assert.Equal(t, name.(string)+"@example.com", email)
		age, _ := doc.Get("age")
		assert.Equal(t, age.(int) >= 18, isAdult)
	}
}

func TestPersistence_E2E_RollbackAfterMigrate(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	logger := zap.NewNop()

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	schema := newMigrationTestSchema("e2e_rollback")
	_, err = p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "e2e_rollback",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	plan := base.NewSchemaOnlyMigration(target, "add email")
	_, err = p.Migrate(ctx, "e2e_rollback", plan, nil)
	require.NoError(t, err)

	sc, err := p.Schema(ctx, "e2e_rollback")
	require.NoError(t, err)
	require.NotNil(t, sc)
	assert.Equal(t, "2.0.0", sc.Version.String())

	rolledBack, err := p.Rollback(ctx, "e2e_rollback", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, rolledBack)

	sc, err = p.Schema(ctx, "e2e_rollback")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", sc.Version.String())
}

func newMigrationTestSchema(name string) *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: name,
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				definition.FieldId(uuid.Must(uuid.NewV7()).String()): {
					Name: "age",
					FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger},
				},
			},
		},
	}
}
