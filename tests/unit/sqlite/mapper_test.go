package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	sqlite "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTableTree_Value(t *testing.T) {
	schemaDef := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f3": {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
		},
	}

	q := query.Query{
		Target: &query.QueryTarget{
			Name:   schemaDef.Name,
			Schema: schemaDef,
		},
	}

	builder := sqlite.NewSQLiteFactory()

	nq, err := builder.Build(&q, native.StmtCreateCollection, nil)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	require.NoError(t, err)
	assert.Empty(t, nq.Raw().Params)
	expectedSQL := `CREATE TABLE IF NOT EXISTS users (
    "age" INTEGER,
    "id" TEXT,
    "name" TEXT
);`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
}

func TestDropTableTree_Value(t *testing.T) {
	schemaDef := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
		},
	}

	q := query.Query{
		Target: &query.QueryTarget{
			Name:   schemaDef.Name,
			Schema: schemaDef,
		},
	}

	builder := sqlite.NewSQLiteFactory()

	nq, err := builder.Build(&q, native.StmtDropCollection, nil)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	require.NoError(t, err)
	assert.Empty(t, nq.Raw().Params)
	assert.Equal(t, "DROP TABLE IF EXISTS users;", nq.Raw().SQL)
}

func TestCreateIndexTree_Value(t *testing.T) {
	schemaDef := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
		},
	}
	indexDef := definition.Index{
		Name:   "idx_users_name",
		Fields: []definition.FieldId{"name"},
	}

	q := query.Query{
		Target: &query.QueryTarget{
			Name:   schemaDef.Name,
			Schema: schemaDef,
		},
	}

	builder := sqlite.NewSQLiteFactory()

	nq, err := builder.Build(&q, native.StmtCreateIndex, indexDef)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	require.NoError(t, err)
	assert.Empty(t, nq.Raw().Params)
	assert.Equal(t, `CREATE INDEX IF NOT EXISTS "idx_users_name" ON users ("name");`, nq.Raw().SQL)
}

func TestDropIndexTree_Value(t *testing.T) {
	schemaDef := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
		},
	}
	indexDef := definition.Index{
		Name: "idx_users_name",
	}

	q := query.Query{
		Target: &query.QueryTarget{
			Name:   schemaDef.Name,
			Schema: schemaDef,
		},
	}

	builder := sqlite.NewSQLiteFactory()

	nq, err := builder.Build(&q, native.StmtDropIndex, indexDef)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	require.NoError(t, err)
	assert.Empty(t, nq.Raw().Params)
	assert.Equal(t, `DROP INDEX IF EXISTS idx_users_name;`, nq.Raw().SQL)
}

