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

// usersTestSchema returns a schema for a "Users" collection with name, email,
// and active fields. Used as the underlying collection for view tests.
func usersTestSchema() *definition.Schema {
        return &definition.Schema{
                BaseSchema: definition.BaseSchema{
                        Name: "Users",
                        Fields: map[definition.FieldId]definition.Field{
                                "id":     {Name: "id",     Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "name":   {Name: "name",   Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "email":  {Name: "email", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                                "active": {Name: "active", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
                        },
                        Indexes: map[definition.IndexID]definition.Index{
                                "pk": {Name: "pk_id", Type: definition.IndexTypePrimary, Fields: []definition.FieldName{"id"}},
                        },
                },
                Version: common.MustNewVersion("1.0.0"),
        }
}

func TestSQLite_View_RegistrationPersists(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        // Underlying collection must exist before the view can be created
        // (the view's query targets it).
        _, err := p.CreateCollection(ctx, usersTestSchema())
        require.NoError(t, err)

        // Build the view query: select active users only, projected to
        // name + email.
        sc := usersTestSchema()
        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Select().Include("name", "email").End().
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: sc}

        coll, err := p.CreateView(ctx, "ActiveUsers", &viewQuery)
        require.NoError(t, err, "expected CreateView to succeed")
        require.NotNil(t, coll)

        // Verify the view is registered by checking the registry has an entry.
        has, err := p.HasCollection(ctx, "ActiveUsers")
        require.NoError(t, err)
        assert.True(t, has, "expected ActiveUsers view to be registered")
}

func TestSQLite_View_ReadReturnsFilteredResults(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        // Create the underlying Users collection.
        users, err := p.CreateCollection(ctx, usersTestSchema())
        require.NoError(t, err)

        // Insert 3 users, 2 active and 1 inactive.
        _, err = users.CreateMany(ctx, []data.Documenter{
                data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice",  "email": "alice@example.com",  "active": true}),
                data.MustNewDocument(map[string]any{"id": "u2", "name": "Bob",    "email": "bob@example.com",    "active": true}),
                data.MustNewDocument(map[string]any{"id": "u3", "name": "Charlie","email": "charlie@example.com","active": false}),
        })
        require.NoError(t, err)

        // Create the view: active users only, projected to name + email.
        sc := usersTestSchema()
        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Select().Include("name", "email").End().
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: sc}

        _, err = p.CreateView(ctx, "ActiveUsers", &viewQuery)
        require.NoError(t, err)

        // Read the view — should return only the 2 active users.
        view, err := p.Collection(ctx, "ActiveUsers")
        require.NoError(t, err)

        readQuery := query.NewQueryBuilder().Build()
        res, err := view.Read(ctx, &readQuery)
        require.NoError(t, err)
        assert.Equal(t, 2, res.Count, "expected view to return only active users")

        // Confirm the projected fields are present (name + email); the
        // underlying active/id fields should NOT be returned because the view
        // projects only name+email.
        if res.Count > 0 {
                _, nameErr := res.Data[0].Get("name")
                _, emailErr := res.Data[0].Get("email")
                assert.NoError(t, nameErr, "expected 'name' field in view result")
                assert.NoError(t, emailErr, "expected 'email' field in view result")
        }
}

func TestSQLite_View_RejectsWrites(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, err := p.CreateCollection(ctx, usersTestSchema())
        require.NoError(t, err)

        // Need at least one row so the view has something to read.
        _, err = users.CreateOne(ctx, data.MustNewDocument(map[string]any{
                "id": "u1", "name": "Alice", "email": "alice@example.com", "active": true,
        }))
        require.NoError(t, err)

        sc := usersTestSchema()
        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: sc}

        _, err = p.CreateView(ctx, "ActiveUsers", &viewQuery)
        require.NoError(t, err)

        view, err := p.Collection(ctx, "ActiveUsers")
        require.NoError(t, err)

        // CreateOne on a view must fail with ErrReadOnly.
        _, err = view.CreateOne(ctx, data.MustNewDocument(map[string]any{
                "id": "x1", "name": "X", "email": "x@x", "active": true,
        }))
        require.Error(t, err)
        assert.Contains(t, err.Error(), "READ_ONLY")

        // Update on a view must fail.
        filter := query.NewQueryBuilder().From("Users").Where("id").Eq("u1").Build()
        _, err = view.Update(ctx, &base.CollectionUpdate{
                Filter: filter.Filters,
                Set:    data.MustNewDocument(map[string]any{"name": "NewName"}),
        })
        require.Error(t, err)
        assert.Contains(t, err.Error(), "READ_ONLY")

        // Delete on a view must fail.
        _, err = view.Delete(ctx, filter.Filters, false)
        require.Error(t, err)
        assert.Contains(t, err.Error(), "READ_ONLY")
}

func TestSQLite_View_ReadComposesWithUserFilters(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, err := p.CreateCollection(ctx, usersTestSchema())
        require.NoError(t, err)

        // Insert 4 active users with different names.
        _, err = users.CreateMany(ctx, []data.Documenter{
                data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice",  "email": "alice@example.com",  "active": true}),
                data.MustNewDocument(map[string]any{"id": "u2", "name": "Bob",    "email": "bob@example.com",    "active": true}),
                data.MustNewDocument(map[string]any{"id": "u3", "name": "Charlie","email": "charlie@example.com","active": true}),
                data.MustNewDocument(map[string]any{"id": "u4", "name": "Alice",  "email": "alice2@example.com", "active": true}),
        })
        require.NoError(t, err)

        // View: all active users.
        sc := usersTestSchema()
        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: sc}

        _, err = p.CreateView(ctx, "ActiveUsers", &viewQuery)
        require.NoError(t, err)

        view, err := p.Collection(ctx, "ActiveUsers")
        require.NoError(t, err)

        // User-side filter: name = "Alice". Should compose with view's
        // active=true filter, returning only the 2 Alices.
        userQuery := query.NewQueryBuilder().Where("name").Eq("Alice").Build()
        res, err := view.Read(ctx, &userQuery)
        require.NoError(t, err)
        assert.Equal(t, 2, res.Count, "expected view + user filter to return 2 Alices")
}

func TestSQLite_View_ValidatorIsNoOp(t *testing.T) {
        p := newTestPersistence(t)
        ctx := context.Background()

        users, err := p.CreateCollection(ctx, usersTestSchema())
        require.NoError(t, err)
        _, err = users.CreateOne(ctx, data.MustNewDocument(map[string]any{
                "id": "u1", "name": "Alice", "email": "alice@example.com", "active": true,
        }))
        require.NoError(t, err)

        sc := usersTestSchema()
        viewQuery := query.NewQueryBuilder().
                From("Users").
                Where("active").Eq(true).
                Build()
        viewQuery.Target = &query.QueryTarget{Name: "Users", Schema: sc}

        _, err = p.CreateView(ctx, "ActiveUsers", &viewQuery)
        require.NoError(t, err)

        view, err := p.Collection(ctx, "ActiveUsers")
        require.NoError(t, err)

        // Validate any document — should always succeed on a view because
        // views are read-only and validation is a no-op for them.
        issues, ok := view.Validate(ctx, data.MustNewDocument(map[string]any{
                "id": "any", "name": "any", "email": "any", "active": true,
        }), false)
        assert.True(t, ok, "expected view Validate to always return ok=true")
        assert.Empty(t, issues, "expected view Validate to return no issues")
}
