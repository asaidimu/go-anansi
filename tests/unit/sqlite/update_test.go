package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	sqlite_query "github.com/asaidimu/go-anansi/v7/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateQueryTranslation(t *testing.T) {
	factory := sqlite_query.NewSQLiteFactory(nil)

	schemaDef := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f3": {Name: "version", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"f4": {Name: "metadata", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeObject}},
			},
		},
	}

	t.Run("computed field translation for arithmetic", func(t *testing.T) {
		q := query.NewQueryBuilder().
			From("users").
			Schema(schemaDef).
			Where("id").Eq("123").
			Build()

		version := query.NewQueryBuilder().Select().AddComputed("version", "ADD",
			&query.FieldReference{Field: "version"}, 1).End().Build()

		updatePayload := map[string]any{
			"compute": map[string]query.Query{
				"version": version,
			},
		}

		node, err := factory.Build(&q, native.StmtUpdate, updatePayload)
		require.NoError(t, err)

		rawQuery := node.Raw()
		sql := rawQuery.SQL
		params := rawQuery.Params

		expectedSQL := `UPDATE "users" SET "version" = ("version" + $1) WHERE "id" = $2`
		assert.Equal(t, expectedSQL, sql)
		assert.Equal(t, []any{1.0, "123"}, params)
	})

	t.Run("computed field translation for nested field", func(t *testing.T) {
		q := query.NewQueryBuilder().
			From("users").
			Schema(schemaDef).
			Where("id").Eq("123").
			Build()

		version := query.NewQueryBuilder().Select().AddComputed("version", "ADD",
			&query.FieldReference{Field: "metadata.version"}, 1).End().Build()

		updatePayload := map[string]any{
			"compute": map[string]query.Query{
				"metadata.version": version,
			},
		}

		node, err := factory.Build(&q, native.StmtUpdate, updatePayload)
		require.NoError(t, err)

		rawQuery := node.Raw()
		sql := rawQuery.SQL
		params := rawQuery.Params

		expectedSQL := `UPDATE "users" SET "metadata" = json_set("metadata", '$.version', json_extract("metadata", '$.version') + $1) WHERE "id" = $2`
		assert.Equal(t, expectedSQL, sql)
		assert.Equal(t, []any{1.0, "123"}, params)
	})
}
