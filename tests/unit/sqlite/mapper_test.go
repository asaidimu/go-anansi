package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	sqlite "github.com/asaidimu/go-anansi/v7/sqlite/query"
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

	builder := sqlite.NewSQLiteFactory(nil)

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

	builder := sqlite.NewSQLiteFactory(nil)

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
		Fields: []definition.FieldName{"name"},
	}

	q := query.Query{
		Target: &query.QueryTarget{
			Name:   schemaDef.Name,
			Schema: schemaDef,
		},
	}

	builder := sqlite.NewSQLiteFactory(nil)

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

	builder := sqlite.NewSQLiteFactory(nil)

	nq, err := builder.Build(&q, native.StmtDropIndex, indexDef)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	require.NoError(t, err)
	assert.Empty(t, nq.Raw().Params)
	assert.Equal(t, `DROP INDEX IF EXISTS idx_users_name;`, nq.Raw().SQL)
}

func TestCreateTableTree_EnumValueConsistency(t *testing.T) {
	// 1. Define the enum schema for the 'policy' field
	enumSchemaId := definition.SchemaId("policy_enum")
	schemaDef := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "sanitization_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {
					Name:     "policy",
					Required: false,
					FieldProperties: definition.FieldProperties{
						Type:    definition.FieldTypeEnum,
						Schema:  definition.NewSchemaReference(definition.SchemaReference{ID: enumSchemaId}),
						Default: mustNewLiteralValue("preserve"),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			enumSchemaId: {
				BaseSchema: definition.BaseSchema{
					Name: "PolicyValues",
				},
				Values: []definition.LiteralValue{
					mustNewLiteralValue("obscure"),
					mustNewLiteralValue("preserve"),
					mustNewLiteralValue("redact"),
					mustNewLiteralValue("hash"),
				},
			},
		},
	}

	q := query.Query{
		Target: &query.QueryTarget{
			Name:   schemaDef.Name,
			Schema: schemaDef,
		},
	}

	builder := sqlite.NewSQLiteFactory(nil)
	nq, err := builder.Build(&q, native.StmtCreateCollection, nil)

	require.NoError(t, err)
	assert.NotNil(t, nq)

	sql := nq.Raw().SQL

	// 2. ASSERTIONS:
	// The core issue is that SQLite CHECK constraints and DEFAULT values should
	// contain 'preserve', NOT '"preserve"'.

	// Check for correct DEFAULT formatting (no double quotes)
	assert.Contains(t, sql, "DEFAULT 'preserve'", "Default value should not be double-quoted inside single quotes")

	// Check for correct CHECK constraint formatting (no double quotes)
	assert.Contains(t, sql, "CHECK(\"policy\" IN ('obscure', 'preserve', 'redact', 'hash'))",
		"Enum values in CHECK constraint should not be double-quoted")
}

// Helper to match the one in your meta package
func mustNewLiteralValue(value string) definition.LiteralValue {
	val, err := definition.NewLiteralValue(value)
	if err != nil {
		panic(err)
	}
	return val
}
