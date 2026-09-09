package query

import (
        "strings"
        "testing"

        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
        "github.com/stretchr/testify/assert"
        "github.com/stretchr/testify/require"
)

// mustBuildSchema builds a minimal schema with the given fields and indexes
// for testing. The schema name is "Articles".
func mustBuildSchema(t *testing.T, fields map[definition.FieldId]definition.Field, indexes map[definition.IndexID]definition.Index) *definition.Schema {
        t.Helper()
        sc := &definition.Schema{
                BaseSchema: definition.BaseSchema{
                        Name:    "Articles",
                        Fields:  fields,
                        Indexes: indexes,
                },
                Version: common.MustNewVersion("1.0.0"),
        }
        return sc
}

func TestCreateFTSTree_EmitsVirtualTableAndTriggers(t *testing.T) {
        sc := mustBuildSchema(t,
                map[definition.FieldId]definition.Field{
                        "f1": {Name: "title", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                        "f2": {Name: "body",  FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                        "f3": {Name: "id",    FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
                },
                map[definition.IndexID]definition.Index{
                        "i1": {
                                Name:   "articles_fts",
                                Type:   definition.IndexTypeFullText,
                                Fields: []definition.FieldName{"title", "body"},
                        },
                },
        )

        tree := &createFTSTree{
                collection: "Articles",
                schema:     sc,
                index:      sc.Indexes["i1"],
        }
        sql, params, err := tree.Value()
        require.NoError(t, err)
        require.Empty(t, params)

        // 1. FTS5 virtual table with external-content mode.
        assert.Contains(t, sql, `CREATE VIRTUAL TABLE IF NOT EXISTS "Articles_fts" USING fts5(`)
        assert.Contains(t, sql, `"title", "body"`)
        assert.Contains(t, sql, `content='Articles'`)
        assert.Contains(t, sql, `content_rowid='rowid'`)
        // Prefix indexes so Contains ("term"*) queries seek instead of scanning.
        assert.Contains(t, sql, `prefix='2 3 4'`)

        // 2. AFTER INSERT trigger.
        assert.Contains(t, sql, `CREATE TRIGGER IF NOT EXISTS "fts_Articles_articles_fts_ai" AFTER INSERT ON "Articles"`)
        assert.Contains(t, sql, `INSERT INTO "Articles_fts"(rowid, "title", "body") VALUES (new.rowid, new."title", new."body")`)

        // 3. AFTER DELETE trigger — uses 'delete' special command.
        assert.Contains(t, sql, `CREATE TRIGGER IF NOT EXISTS "fts_Articles_articles_fts_ad" AFTER DELETE ON "Articles"`)
        assert.Contains(t, sql, `INSERT INTO "Articles_fts"("Articles_fts", rowid, "title", "body") VALUES('delete', old.rowid, old."title", old."body")`)

        // 4. AFTER UPDATE trigger — delete old, insert new.
        assert.Contains(t, sql, `CREATE TRIGGER IF NOT EXISTS "fts_Articles_articles_fts_au" AFTER UPDATE ON "Articles"`)
}

func TestCreateFTSTree_RejectsNonStringField(t *testing.T) {
        sc := mustBuildSchema(t,
                map[definition.FieldId]definition.Field{
                        "f1": {Name: "title", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                        "f2": {Name: "count", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
                },
                map[definition.IndexID]definition.Index{
                        "i1": {
                                Name:   "bad_fts",
                                Type:   definition.IndexTypeFullText,
                                Fields: []definition.FieldName{"title", "count"}, // count is integer
                        },
                },
        )

        tree := &createFTSTree{
                collection: "Articles",
                schema:     sc,
                index:      sc.Indexes["i1"],
        }
        _, _, err := tree.Value()
        require.Error(t, err)
        assert.Contains(t, err.Error(), "must be a string/enum/bytes column")
}

func TestCreateFTSTree_RejectsUnknownField(t *testing.T) {
        sc := mustBuildSchema(t,
                map[definition.FieldId]definition.Field{
                        "f1": {Name: "title", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                },
                map[definition.IndexID]definition.Index{
                        "i1": {
                                Name:   "bad_fts",
                                Type:   definition.IndexTypeFullText,
                                Fields: []definition.FieldName{"nonexistent"},
                        },
                },
        )

        tree := &createFTSTree{
                collection: "Articles",
                schema:     sc,
                index:      sc.Indexes["i1"],
        }
        _, _, err := tree.Value()
        require.Error(t, err)
        assert.Contains(t, err.Error(), "unknown field")
}

func TestDropFTSTree_DropsTriggersBeforeTable(t *testing.T) {
        tree := &dropFTSTree{
                collection: "Articles",
                index: definition.Index{
                        Name:   "articles_fts",
                        Type:   definition.IndexTypeFullText,
                        Fields: []definition.FieldName{"title", "body"},
                },
        }
        sql, _, err := tree.Value()
        require.NoError(t, err)

        // Triggers must be dropped before the table.
        dropTriggerIdx := strings.Index(sql, "DROP TRIGGER IF EXISTS")
        dropTableIdx := strings.Index(sql, "DROP TABLE IF EXISTS")
        require.NotEqual(t, -1, dropTriggerIdx, "no DROP TRIGGER in: %s", sql)
        require.NotEqual(t, -1, dropTableIdx, "no DROP TABLE in: %s", sql)
        assert.Less(t, dropTriggerIdx, dropTableIdx, "triggers should be dropped before table")

        assert.Contains(t, sql, `"fts_Articles_articles_fts_ai"`)
        assert.Contains(t, sql, `"fts_Articles_articles_fts_ad"`)
        assert.Contains(t, sql, `"fts_Articles_articles_fts_au"`)
        assert.Contains(t, sql, `"Articles_fts"`)
}

func TestCollectFullTextIndexes_ReturnsOnlyFullText(t *testing.T) {
        sc := mustBuildSchema(t,
                map[definition.FieldId]definition.Field{
                        "f1": {Name: "title", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                },
                map[definition.IndexID]definition.Index{
                        "i1": {Name: "normal_idx", Type: definition.IndexTypeNormal,   Fields: []definition.FieldName{"title"}},
                        "i2": {Name: "fts_idx",    Type: definition.IndexTypeFullText, Fields: []definition.FieldName{"title"}},
                        "i3": {Name: "unique_idx", Type: definition.IndexTypeUnique,   Fields: []definition.FieldName{"title"}},
                },
        )

        out := collectFullTextIndexes(sc)
        require.Len(t, out, 1)
        assert.Equal(t, "fts_idx", out[0].Name)
}

func TestCreateTableTree_AutoEmitsFTSForFullTextIndexes(t *testing.T) {
        sc := mustBuildSchema(t,
                map[definition.FieldId]definition.Field{
                        "f1": {Name: "title", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                        "f2": {Name: "body",  FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                },
                map[definition.IndexID]definition.Index{
                        "i1": {
                                Name:   "articles_fts",
                                Type:   definition.IndexTypeFullText,
                                Fields: []definition.FieldName{"title", "body"},
                        },
                },
        )

        tree := &createTableTree{schema: sc}
        sql, _, err := tree.Value()
        require.NoError(t, err)

        // The CREATE TABLE statement must come first, followed by FTS5 DDL.
        createTableIdx := strings.Index(sql, "CREATE TABLE IF NOT EXISTS")
        createFTSIdx := strings.Index(sql, "CREATE VIRTUAL TABLE IF NOT EXISTS")
        require.NotEqual(t, -1, createTableIdx)
        require.NotEqual(t, -1, createFTSIdx, "expected FTS5 virtual table to be emitted, got: %s", sql)
        assert.Less(t, createTableIdx, createFTSIdx, "CREATE TABLE should come before CREATE VIRTUAL TABLE")
}

func TestCreateTableTree_NoFTSWhenNoFullTextIndex(t *testing.T) {
        sc := mustBuildSchema(t,
                map[definition.FieldId]definition.Field{
                        "f1": {Name: "title", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                },
                map[definition.IndexID]definition.Index{
                        "i1": {Name: "normal_idx", Type: definition.IndexTypeNormal, Fields: []definition.FieldName{"title"}},
                },
        )

        tree := &createTableTree{schema: sc}
        sql, _, err := tree.Value()
        require.NoError(t, err)
        assert.NotContains(t, sql, "CREATE VIRTUAL TABLE")
        assert.NotContains(t, sql, "USING fts5")
}

func TestDropTableTree_DropsFTSTableAndTriggersBeforeCollection(t *testing.T) {
        sc := mustBuildSchema(t,
                map[definition.FieldId]definition.Field{
                        "f1": {Name: "title", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                        "f2": {Name: "body",  FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
                },
                map[definition.IndexID]definition.Index{
                        "i1": {
                                Name:   "articles_fts",
                                Type:   definition.IndexTypeFullText,
                                Fields: []definition.FieldName{"title", "body"},
                        },
                },
        )

        tree := &dropTableTree{name: "Articles", schema: sc}
        sql, _, err := tree.Value()
        require.NoError(t, err)

        dropFTSTriggerIdx := strings.Index(sql, "DROP TRIGGER IF EXISTS")
        dropFTSTableIdx := strings.Index(sql, `DROP TABLE IF EXISTS "Articles_fts"`)
        dropCollectionIdx := strings.Index(sql, `DROP TABLE IF EXISTS "Articles"`)
        require.NotEqual(t, -1, dropFTSTriggerIdx, "expected DROP TRIGGER in: %s", sql)
        require.NotEqual(t, -1, dropFTSTableIdx, "expected DROP TABLE for FTS in: %s", sql)
        require.NotEqual(t, -1, dropCollectionIdx, "expected DROP TABLE for collection in: %s", sql)

        // Triggers → FTS table → collection table.
        assert.Less(t, dropFTSTriggerIdx, dropFTSTableIdx, "triggers should drop before FTS table")
        assert.Less(t, dropFTSTableIdx, dropCollectionIdx, "FTS table should drop before collection table")
}
