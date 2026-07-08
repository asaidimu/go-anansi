package migration_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/migration"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSchema(name string) *definition.Schema {
	return &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: name,
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
		},
	}
}

func init() {
	testutils.ConfigureDocumentFactory()
}

func TestDefaultDataMigrator_Migrate(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	sm := interactor.SchemaManager()

	srcSchema := newTestSchema("users_src")
	err := sm.CreateCollection(ctx, *srcSchema)
	require.NoError(t, err)

	_, err = interactor.InsertDocuments(ctx, srcSchema, []map[string]any{
		{"name": "Alice", "age": 30},
		{"name": "Bob", "age": 25},
	})
	require.NoError(t, err)

	dstSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users_dst",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"f3": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	err = sm.CreateCollection(ctx, *dstSchema)
	require.NoError(t, err)

	store := make(map[string]map[string]*definition.Schema)
	store["users"] = map[string]*definition.Schema{
		"1.0.0": srcSchema,
		"2.0.0": dstSchema,
	}

	registry := &testRegistry{store: store}

	transformer := func(_ context.Context, doc data.Document) (data.Document, error) {
		d := doc.ToMap()
		d["email"] = d["name"].(string) + "@example.com"
		return *data.MustNewDocument(d), nil
	}

	migrator := migration.NewDefaultDataMigrator(interactor, registry)
	jobID, err := migrator.Migrate(ctx, "users", "1.0.0", "2.0.0", transformer)
	require.NoError(t, err)
	assert.NotEmpty(t, jobID)

	rows, _, err := interactor.SelectDocuments(ctx, dstSchema, &query.Query{})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	for _, row := range rows {
		email, ok := row["email"]
		require.True(t, ok, "email field should exist in migrated data")
		name := row["name"]
		assert.Equal(t, name.(string)+"@example.com", email)
	}
}

func TestDefaultDataMigrator_NilTransformer(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()

	store := make(map[string]map[string]*definition.Schema)
	registry := &testRegistry{store: store}

	migrator := migration.NewDefaultDataMigrator(interactor, registry)
	_, err := migrator.Migrate(ctx, "users", "1.0.0", "2.0.0", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transformer is required")
}

func TestDefaultDataMigrator_EmptySource(t *testing.T) {
	ctx := context.Background()
	interactor := ephemeral.NewEphemeral()
	sm := interactor.SchemaManager()

	srcSchema := newTestSchema("users_empty_src")
	err := sm.CreateCollection(ctx, *srcSchema)
	require.NoError(t, err)

	dstSchema := newTestSchema("users_empty_dst")
	err = sm.CreateCollection(ctx, *dstSchema)
	require.NoError(t, err)

	store := make(map[string]map[string]*definition.Schema)
	store["users"] = map[string]*definition.Schema{
		"1.0.0": srcSchema,
		"2.0.0": dstSchema,
	}
	registry := &testRegistry{store: store}

	transformer := func(_ context.Context, doc data.Document) (data.Document, error) {
		return doc, nil
	}

	migrator := migration.NewDefaultDataMigrator(interactor, registry)
	jobID, err := migrator.Migrate(ctx, "users", "1.0.0", "2.0.0", transformer)
	require.NoError(t, err)
	assert.NotEmpty(t, jobID)
}

type testRegistry struct {
	base.CollectionRegistry
	store map[string]map[string]*definition.Schema
}

func (r *testRegistry) GetSchema(_ context.Context, name string, version ...string) (*definition.Schema, error) {
	ver := "1.0.0"
	if len(version) > 0 {
		ver = version[0]
	}
	return r.store[name][ver], nil
}
