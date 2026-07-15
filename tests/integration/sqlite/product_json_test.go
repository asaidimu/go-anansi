package sqlite_test

import (
	"context"
	"os"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
	sqliteQuery "github.com/asaidimu/go-anansi/v8/sqlite/query"
	"github.com/asaidimu/go-anansi/v8/tests/testutils"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestProductJson_CreateCollectionAndQuery(t *testing.T) {
	testutils.ConfigureDocumentFactory()

	productJSON, err := os.ReadFile("Product.json")
	require.NoError(t, err)
	require.NotEmpty(t, productJSON)

	schemaDef, err := definition.FromJSON(productJSON)
	require.NoError(t, err)
	require.NotNil(t, schemaDef)
	t.Logf("Schema name: %s", schemaDef.Name)
	t.Logf("Schema fields: %d", len(schemaDef.Fields))
	for id, f := range schemaDef.Fields {
		t.Logf("  Field %s: name=%q type=%v required=%v", id, f.Name, f.Type, f.Required)
	}

	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	ctx := context.Background()

	err = interactor.SchemaManager().CreateCollection(ctx, *schemaDef)
	require.NoError(t, err, "CreateCollection should succeed")

	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?;", schemaDef.Name)
	var tableName string
	err = row.Scan(&tableName)
	require.NoError(t, err)
	require.Equal(t, schemaDef.Name, tableName)

	t.Logf("Table '%s' created successfully", tableName)

	rows, err := db.Query("PRAGMA table_info(\"" + schemaDef.Name + "\");")
	require.NoError(t, err)
	defer rows.Close()

	t.Logf("Columns for table %s:", schemaDef.Name)
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue interface{}
		var pk int
		err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk)
		require.NoError(t, err)
		t.Logf("  %s (%s) notnull=%d pk=%d", name, ctype, notnull, pk)
	}

	doc := map[string]any{
		"_id_":       "prod-001",
		"label":      "My Product",
		"type":       "fungible",
		"group":      "software",
		"tags":       []any{"tag1", "tag2"},
		"id":         "SKU-123",
		"description": "A test product",
		"data": map[string]any{
			"version": map[string]any{
				"tag":  "v1.0.0",
				"hash": "abc123",
				"date": "2024-01-15",
				"repo": "github.com/example/repo",
			},
			"artifacts": []any{
				map[string]any{
					"type":         "binary",
					"url":          "https://example.com/pkg.tar.gz",
					"checksum":     "sha256:xyz",
					"platform":     "linux",
					"architecture": "amd64",
				},
			},
			"license": "open-source",
			"status":  "active",
		},
		"_metadata_": map[string]any{
			"sla":            "24h",
			"supportTier":    "standard",
			"lifecycleStage": "active",
			"owner":          "team-a",
			"maintainers":    []any{"alice", "bob"},
		},
	}

	insertedDocs, err := interactor.InsertDocuments(ctx, schemaDef, []map[string]any{doc})
	require.NoError(t, err, "InsertDocuments should succeed")
	require.Len(t, insertedDocs, 1)

	t.Log("Document inserted successfully")

	allDocs, _, err := interactor.SelectDocuments(ctx, schemaDef, &query.Query{
		Target: &query.QueryTarget{Name: schemaDef.Name, Schema: schemaDef},
	})
	require.NoError(t, err, "SelectDocuments should succeed")
	require.Len(t, allDocs, 1)

	t.Logf("Selected document: %+v", allDocs[0])

	filterQuery := &query.Query{
		Target: &query.QueryTarget{Name: schemaDef.Name, Schema: schemaDef},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "label",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{StringVal: utils.StringPtr("My Product")},
			},
		},
	}
	filteredDocs, _, err := interactor.SelectDocuments(ctx, schemaDef, filterQuery)
	require.NoError(t, err, "Filtered SelectDocuments should succeed")
	require.Len(t, filteredDocs, 1)
	require.Equal(t, "My Product", filteredDocs[0]["label"])

	t.Log("Query with filter succeeded")
}

func TestProductJson_CreateTableSQL(t *testing.T) {
	productJSON, err := os.ReadFile("Product.json")
	require.NoError(t, err)

	schemaDef, err := definition.FromJSON(productJSON)
	require.NoError(t, err)

	builder := sqliteQuery.NewSQLiteFactory(nil)
	q := query.Query{
		Target: &query.QueryTarget{
			Name:   schemaDef.Name,
			Schema: schemaDef,
		},
	}
	nq, err := builder.Build(&q, native.StmtCreateCollection, nil)
	require.NoError(t, err)

	sql := nq.Raw().SQL
	t.Logf("Generated CREATE TABLE SQL:\n%s", sql)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS")
	require.Contains(t, sql, `"label"`)
	require.NotEmpty(t, sql)
}
