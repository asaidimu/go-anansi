package sqlite_test

import (
        "context"
        "testing"

        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/data"
        "github.com/asaidimu/go-anansi/v8/core/persistence/base"
        "github.com/asaidimu/go-anansi/v8/core/query"
        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
        "github.com/stretchr/testify/assert"
        "github.com/stretchr/testify/require"
)

// usersAndProfilesSchemas returns two schemas: "Users" (id, name, email, active)
// and "Profiles" (id, user_id, bio). Used to test materialized views that join
// the two collections.
func usersAndProfilesSchemas() (*definition.Schema, *definition.Schema) {
        users := &definition.Schema{
                BaseSchema: definition.BaseSchema{
                        Name: "Users",
                        Fields: map[definition.FieldId]definition.Field{
                                "id":     {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "name":   {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "email":  {Name: "email", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "active": {Name: "active", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
                        },
                        Indexes: map[definition.IndexID]definition.Index{
                                "pk": {Name: "pk_id", Type: definition.IndexTypePrimary, Fields: []definition.FieldName{"id"}},
                        },
                },
                Version: common.MustNewVersion("1.0.0"),
        }
        profiles := &definition.Schema{
                BaseSchema: definition.BaseSchema{
                        Name: "Profiles",
                        Fields: map[definition.FieldId]definition.Field{
                                "id":      {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "user_id": {Name: "user_id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "bio":     {Name: "bio", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                        },
                        Indexes: map[definition.IndexID]definition.Index{
                                "pk": {Name: "pk_id", Type: definition.IndexTypePrimary, Fields: []definition.FieldName{"id"}},
                        },
                },
                Version: common.MustNewVersion("1.0.0"),
        }
        return users, profiles
}

func TestSQLite_MaterializedView_CreateAndRead(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, profiles := usersAndProfilesSchemas()
        err := p.CreateCollections(ctx, []*definition.Schema{users, profiles})
        require.NoError(t, err)

        usersColl, _ := p.Collection(ctx, "Users")
        profilesColl, _ := p.Collection(ctx, "Profiles")

        // Insert users and profiles.
        _, err = usersColl.CreateMany(ctx, []data.Documenter{
                data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice", "email": "alice@x.com", "active": true}),
                data.MustNewDocument(map[string]any{"id": "u2", "name": "Bob", "email": "bob@x.com", "active": false}),
        })
        require.NoError(t, err)
        _, err = profilesColl.CreateMany(ctx, []data.Documenter{
                data.MustNewDocument(map[string]any{"id": "p1", "user_id": "u1", "bio": "Alice's bio"}),
                data.MustNewDocument(map[string]any{"id": "p2", "user_id": "u2", "bio": "Bob's bio"}),
        })
        require.NoError(t, err)

        // Create a materialized view that joins Users + Profiles, filtered to active users.
        viewQuery := query.NewQueryBuilder().
                From("Users").
                InnerJoin("Profiles").End().
                Where("active").Eq(true).
                Build()
        // Wire up the join's ON clause and schemas.
        viewQuery.Joins[0].On = &query.QueryFilter{
                Condition: &query.FilterCondition{
                        Field:    "Profiles.user_id",
                        Operator: query.ComparisonOperatorEq,
                        Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "Users.id"}},
                },
        }
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: users}
        viewQuery.Joins[0].Target = query.QueryTarget{Name: "Profiles", Schema: profiles}

        _, err = p.CreateView(ctx, "ActiveUsersWithProfiles", &viewQuery, true)
        require.NoError(t, err, "expected materialized view creation to succeed")

        // Read the materialized view — should return only Alice (active=true).
        view, err := p.Collection(ctx, "ActiveUsersWithProfiles")
        require.NoError(t, err)

        readQuery := query.NewQueryBuilder().Build()
        res, err := view.Read(ctx, &readQuery)
        require.NoError(t, err)
        assert.Equal(t, 1, res.Count, "expected materialized view to return 1 active user with profile")
}

func TestSQLite_MaterializedView_RefreshPicksUpNewData(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, _ := usersAndProfilesSchemas()
        err := p.CreateCollections(ctx, []*definition.Schema{users})
        require.NoError(t, err)

        usersColl, _ := p.Collection(ctx, "Users")
        _, err = usersColl.CreateMany(ctx, []data.Documenter{
                data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice", "email": "alice@x.com", "active": true}),
        })
        require.NoError(t, err)

        // Create a materialized view of active users.
        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: users}

        _, err = p.CreateView(ctx, "ActiveUsers", &viewQuery, true)
        require.NoError(t, err)

        view, err := p.Collection(ctx, "ActiveUsers")
        require.NoError(t, err)

        // Initial read: 1 active user.
        readQ := query.NewQueryBuilder().Build()
        res, err := view.Read(ctx, &readQ)
        require.NoError(t, err, "expected read to succeed")
        require.Equal(t, 1, res.Count)

        // Insert another active user into the underlying collection.
        _, err = usersColl.CreateOne(ctx, data.MustNewDocument(map[string]any{
                "id": "u2", "name": "Bob", "email": "bob@x.com", "active": true,
        }))
        require.NoError(t, err)

        // Before refresh, the materialized view still shows 1 user (stale).
        res, _ = view.Read(ctx, &readQ)
        assert.Equal(t, 1, res.Count, "expected stale materialized view before refresh")

        // Refresh the materialized view.
        err = view.Refresh(ctx)
        require.NoError(t, err)

        // After refresh, the materialized view shows 2 users.
        res, _ = view.Read(ctx, &readQ)
        assert.Equal(t, 2, res.Count, "expected refreshed materialized view to show 2 users")
}

func TestSQLite_MaterializedView_RejectsWrites(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, _ := usersAndProfilesSchemas()
        err := p.CreateCollections(ctx, []*definition.Schema{users})
        require.NoError(t, err)

        usersColl, _ := p.Collection(ctx, "Users")
        _, err = usersColl.CreateOne(ctx, data.MustNewDocument(map[string]any{
                "id": "u1", "name": "Alice", "email": "alice@x.com", "active": true,
        }))
        require.NoError(t, err)

        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: users}

        _, err = p.CreateView(ctx, "ActiveUsers", &viewQuery, true)
        require.NoError(t, err)

        view, err := p.Collection(ctx, "ActiveUsers")
        require.NoError(t, err)

        // CreateOne must fail with ErrReadOnly.
        _, err = view.CreateOne(ctx, data.MustNewDocument(map[string]any{
                "id": "x1", "name": "X", "email": "x@x", "active": true,
        }))
        require.Error(t, err)
        assert.Contains(t, err.Error(), "READ_ONLY")

        // Update must fail.
        filter := query.NewQueryBuilder().From("Users").Where("id").Eq("u1").Build()
        _, err = view.Update(ctx, &base.CollectionUpdate{
                Filter: filter.Filters,
                Set:    data.MustNewDocument(map[string]any{"name": "NewName"}),
        })
        require.Error(t, err)
        assert.Contains(t, err.Error(), "READ_ONLY")

        // Delete must fail.
        _, err = view.Delete(ctx, filter.Filters, false)
        require.Error(t, err)
        assert.Contains(t, err.Error(), "READ_ONLY")
}

func TestSQLite_MaterializedView_RefreshOnNonMaterializedReturnsError(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, _ := usersAndProfilesSchemas()
        err := p.CreateCollections(ctx, []*definition.Schema{users})
        require.NoError(t, err)

        usersColl, _ := p.Collection(ctx, "Users")
        _, err = usersColl.CreateOne(ctx, data.MustNewDocument(map[string]any{
                "id": "u1", "name": "Alice", "email": "alice@x.com", "active": true,
        }))
        require.NoError(t, err)

        // Create a VIRTUAL view (materialized=false).
        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: users}

        _, err = p.CreateView(ctx, "ActiveUsers", &viewQuery, false)
        require.NoError(t, err)

        view, err := p.Collection(ctx, "ActiveUsers")
        require.NoError(t, err)

        // Refresh on a virtual view must fail with ErrNotMaterialized.
        err = view.Refresh(ctx)
        require.Error(t, err)
        assert.Contains(t, err.Error(), "NOT_MATERIALIZED")
}

func TestSQLite_MaterializedView_RefreshOnRegularCollectionReturnsError(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, _ := usersAndProfilesSchemas()
        err := p.CreateCollections(ctx, []*definition.Schema{users})
        require.NoError(t, err)

        // Refresh on a regular (non-view) collection must fail.
        usersColl, err := p.Collection(ctx, "Users")
        require.NoError(t, err)

        err = usersColl.Refresh(ctx)
        require.Error(t, err)
        assert.Contains(t, err.Error(), "NOT_MATERIALIZED")
}
