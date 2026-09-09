package query

import (
        "fmt"
        "strings"

        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ftsTableSuffix is appended to the collection name to derive the FTS5
// virtual table name. e.g. collection "Products" -> fts table "Products_fts".
const ftsTableSuffix = "_fts"

// ftsRowIDColumn is the column on the FTS5 virtual table that mirrors the
// underlying collection's rowid, used to JOIN search results back to the
// main table.
const ftsRowIDColumn = "rowid"

// ftsPrefixIndexes lists the prefix lengths indexed for FTS5 prefix queries.
// Contains searches always emit trailing-* prefix queries ("term"*), which
// without prefix indexes degrade to full index scans over the vocabulary.
// '2 3 4' is the standard balance: prefixes of 2+ characters use the index
// (single-character prefixes still scan). Longer sets bloat the index for
// rapidly diminishing returns.
const ftsPrefixIndexes = "2 3 4"

// ftsIndexName derives the FTS5 virtual table name from a base collection
// name. Exported for use by the SQLite executor / helpers and tests.
func ftsIndexName(collection string) string {
        return collection + ftsTableSuffix
}

// collectFullTextIndexes returns all IndexTypeFullText indexes declared on
// the schema. Each returned index names the set of TEXT-typed fields that
// should be searchable via FTS5. Returns nil if no full-text indexes exist.
func collectFullTextIndexes(sc *definition.Schema) []definition.Index {
        var out []definition.Index
        for _, idx := range sc.Indexes {
                if idx.Type == definition.IndexTypeFullText {
                        out = append(out, idx)
                }
        }
        return out
}

func (t *createFTSTree) Value() (string, []any, error) {
        if t.collection == "" {
                return "", nil, ErrCollectionSchemaNotDefined
        }
        if len(t.index.Fields) == 0 {
                return "", nil, ErrIndexIndexNotDefined
        }

        collection := quoteIdentifier(t.collection)
        ftsName := quoteIdentifier(ftsIndexName(t.collection))

        // When the schema is nil (e.g., when CreateIndex was called via the
        // SchemaManager interface which doesn't carry schema), we skip field-type
        // validation and emit FTS5 columns directly from the index's Fields list.
        // The DB will reject unknown columns at execution time.
        var ftsColumns []string
        if t.schema != nil {
                sc := *t.schema
                for _, fieldName := range t.index.Fields {
                        _, field, ok := sc.GetFieldByName(fieldName)
                        if !ok {
                                return "", nil, ErrCollectionFieldError.WithCause(
                                        fmt.Errorf("fulltext index %q references unknown field %q", t.index.Name, fieldName))
                        }
                        if field.Type != definition.FieldTypeString && field.Type != definition.FieldTypeEnum && field.Type != definition.FieldTypeBytes {
                                return "", nil, ErrCollectionFieldError.WithCause(
                                        fmt.Errorf("fulltext index %q field %q must be a string/enum/bytes column; got %s", t.index.Name, fieldName, field.Type))
                        }
                        ftsColumns = append(ftsColumns, quoteIdentifier(string(fieldName)))
                }
        } else {
                // Schema not available — emit columns from the index's Fields list
                // directly. This path is taken when SchemaManager.CreateIndex is
                // invoked without a schema context (e.g., from the migration layer
                // calling CreateIndex directly). The caller is responsible for
                // ensuring the field names match the underlying collection's columns.
                for _, fieldName := range t.index.Fields {
                        ftsColumns = append(ftsColumns, quoteIdentifier(string(fieldName)))
                }
        }

        var sb strings.Builder

        // 1. FTS5 virtual table. We use external-content mode
        //    (content='<table>', content_rowid='rowid') so the FTS index never
        //    duplicates row data — only the tokenized index. The 'rowid'
        //    column on the underlying collection table is the implicit
        //    integer primary key in SQLite. prefix='2 3 4' keeps prefix
        //    indexes so Contains ("term"*) queries seek instead of scanning.
        sb.WriteString(fmt.Sprintf(
                "CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(%s, content='%s', content_rowid='%s', prefix='%s');\n",
                ftsName,
                strings.Join(ftsColumns, ", "),
                t.collection,
                ftsRowIDColumn,
                ftsPrefixIndexes,
        ))

        // Trigger name prefix — sanitized to be SQLite-safe (no quotes here;
        // we'll quote the full name when emitting the CREATE TRIGGER).
        triggerPrefix := "fts_" + sanitizeTriggerName(t.collection) + "_" + sanitizeTriggerName(t.index.Name)

        // Comma-separated column lists reused across triggers.
        colList := strings.Join(ftsColumns, ", ")
        newCols := joinPrefixed("new.", ftsColumns)
        oldCols := joinPrefixed("old.", ftsColumns)

        // 2. AFTER INSERT trigger — populate FTS columns from the new row.
        sb.WriteString(fmt.Sprintf(
                "CREATE TRIGGER IF NOT EXISTS %s AFTER INSERT ON %s BEGIN\n"+
                        "  INSERT INTO %s(%s, %s) VALUES (new.rowid, %s);\n"+
                        "END;\n",
                quoteIdentifier(triggerPrefix+"_ai"),
                collection,
                ftsName, ftsRowIDColumn, colList,
                newCols,
        ))

        // 3. AFTER DELETE trigger — issue the FTS5 'delete' command.
        //    The 'delete' command syntax for external-content FTS5 tables is:
        //      INSERT INTO <fts_table>(<fts_table>, rowid, col1, col2)
        //        VALUES('delete', old.rowid, old.col1, old.col2);
        //    The first column is the FTS table name itself (the special
        //    command column); the remaining columns mirror the FTS schema.
        sb.WriteString(fmt.Sprintf(
                "CREATE TRIGGER IF NOT EXISTS %s AFTER DELETE ON %s BEGIN\n"+
                        "  INSERT INTO %s(%s, %s, %s) VALUES('delete', old.rowid, %s);\n"+
                        "END;\n",
                quoteIdentifier(triggerPrefix+"_ad"),
                collection,
                ftsName, ftsName, ftsRowIDColumn, colList,
                oldCols,
        ))

        // 4. AFTER UPDATE trigger — delete the old FTS row, insert the new one.
        sb.WriteString(fmt.Sprintf(
                "CREATE TRIGGER IF NOT EXISTS %s AFTER UPDATE ON %s BEGIN\n"+
                        "  INSERT INTO %s(%s, %s, %s) VALUES('delete', old.rowid, %s);\n"+
                        "  INSERT INTO %s(%s, %s) VALUES (new.rowid, %s);\n"+
                        "END;\n",
                quoteIdentifier(triggerPrefix+"_au"),
                collection,
                ftsName, ftsName, ftsRowIDColumn, colList,
                oldCols,
                ftsName, ftsRowIDColumn, colList,
                newCols,
        ))

        return sb.String(), nil, nil
}

// dropFTSTree emits DROP TABLE IF EXISTS for the FTS5 virtual table and the
// three sync triggers. It must run before the underlying collection table
// is dropped (otherwise the triggers can't be cleaned up).
// (Struct definition lives in types.go alongside the other SQLNode types.)

func (t *dropFTSTree) Value() (string, []any, error) {
        if t.collection == "" {
                return "", nil, ErrCollectionSchemaNotDefined
        }

        var sb strings.Builder
        triggerPrefix := "fts_" + sanitizeTriggerName(t.collection) + "_" + sanitizeTriggerName(t.index.Name)

        // Drop triggers first to avoid the FTS5 delete-on-drop firing the
        // triggers we are about to remove.
        for _, suffix := range []string{"_ai", "_ad", "_au"} {
                sb.WriteString(fmt.Sprintf("DROP TRIGGER IF EXISTS %s;\n", quoteIdentifier(triggerPrefix+suffix)))
        }
        sb.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", quoteIdentifier(ftsIndexName(t.collection))))
        return sb.String(), nil, nil
}

// joinPrefixed joins a slice of identifier strings, each prefixed with the
// given prefix. Used to produce "new.col1, new.col2" lists.
func joinPrefixed(prefix string, parts []string) string {
        out := make([]string, len(parts))
        for i, p := range parts {
                out[i] = prefix + p
        }
        return strings.Join(out, ", ")
}

// sanitizeTriggerName converts arbitrary collection / index names into a
// form safe for use in generated SQLite trigger names. SQLite identifiers
// for triggers cannot contain double quotes but can contain most other
// printable ASCII; we conservatively replace anything that is not
// alphanumeric or underscore with an underscore.
func sanitizeTriggerName(s string) string {
        var b strings.Builder
        for _, r := range s {
                if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
                        b.WriteRune(r)
                } else {
                        b.WriteByte('_')
                }
        }
        if b.Len() == 0 {
                return "anon"
        }
        return b.String()
}
