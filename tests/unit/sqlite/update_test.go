package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqlite_query "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateQueryTranslation(t *testing.T) {
	factory := sqlite_query.NewSQLiteFactory()

	schemaDef := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":      {Name: "id", Type: schema.FieldTypeString},
			"name":    {Name: "name", Type: schema.FieldTypeString},
			"version": {Name: "version", Type: schema.FieldTypeInteger},
			"metadata": {
				Name: "metadata",
				Type: schema.FieldTypeObject,
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
