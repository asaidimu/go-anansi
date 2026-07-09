package persistence_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"encoding/json"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	sqliteExecutor "github.com/asaidimu/go-anansi/v8/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v8/sqlite/query"
	"github.com/asaidimu/go-anansi/v8/tests/testutils"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newMigrationTestSchema(name string, fields map[definition.FieldId]definition.Field) *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name:   name,
			Fields: fields,
		},
	}
}

// fieldID returns a new UUID v7 formatted as a definition.FieldId.
func fieldID() definition.FieldId {
	return definition.FieldId(uuid.Must(uuid.NewV7()).String())
}

// fieldIDByName returns the first FieldId whose field name matches.
func fieldIDByName(fields map[definition.FieldId]definition.Field, name string) definition.FieldId {
	for id, f := range fields {
		if string(f.Name) == name {
			return id
		}
	}
	return ""
}

func copyFields(fields map[definition.FieldId]definition.Field) map[definition.FieldId]definition.Field {
	out := make(map[definition.FieldId]definition.Field, len(fields))
	for k, v := range fields {
		out[k] = v
	}
	return out
}

// cloneSchemaViaJSON round-trips a schema through JSON to produce a clean copy
// that matches what the generated migration code produces.
func cloneSchemaViaJSON(sc *definition.Schema) *definition.Schema {
	b, err := json.Marshal(sc)
	if err != nil {
		panic(err)
	}
	clone, err := definition.FromJSON(b)
	if err != nil {
		panic(err)
	}
	return clone
}

func setupSQLitePersistence(t *testing.T) (base.Persistence, func()) {
	testutils.ConfigureDocumentFactory()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)

	logger := zap.NewNop()
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory(nil)

	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	return p, func() { db.Close() }
}

// TestE2E_MultipleMigrations_WithSQLite exercises the full migration lifecycle
// on a real SQLite backend: create → migrate → migrate again → rollback → rollback.
func TestE2E_MultipleMigrations_WithSQLite(t *testing.T) {
	ctx := context.Background()
	p, cleanup := setupSQLitePersistence(t)
	defer cleanup()

	// ─── Step 1: Create initial collection ────────────────────────────────
	v1Fields := map[definition.FieldId]definition.Field{
		fieldID(): {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
		fieldID(): {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
	}
	schema := newMigrationTestSchema("multi_mig", v1Fields)
	coll, err := p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	// Seed data
	docs := []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Alice", "age": 30}),
		data.MustNewDocument(map[string]any{"name": "Bob", "age": 25}),
		data.MustNewDocument(map[string]any{"name": "Charlie", "age": 35}),
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	// ─── Step 2: First migration — add "email" field ─────────────────────
	currentSchema, err := p.Schema(ctx, "multi_mig")
	require.NoError(t, err)

	// Clone the enriched schema via JSON (mimics generated migration code)
	targetV2 := cloneSchemaViaJSON(currentSchema)
	targetV2.Version = common.MustNewVersion("1.1.0")
	targetV2.Fields[fieldID()] = definition.Field{Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}}

	plan := &base.MigrationPlan{
		Description: "add email column",
		Target:      targetV2,
		VersionBump: definition.BumpMinor,
	}
	_, err = p.Migrate(ctx, "multi_mig", plan, nil)
	require.NoError(t, err)

	// Verify version bumped
	sc, err := p.Schema(ctx, "multi_mig")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", sc.Version.String())

	// Verify data is still accessible
	coll, err = p.Collection(ctx, "multi_mig")
	require.NoError(t, err)
	result, err := coll.Read(ctx, &query.Query{})
	require.NoError(t, err)
	assert.Len(t, result.Data, 3)

	// ─── Step 3: Second migration — add "status" field ──────────────────
	currentSchemaV2, err := p.Schema(ctx, "multi_mig")
	require.NoError(t, err)

	targetV3 := cloneSchemaViaJSON(currentSchemaV2)
	targetV3.Version = common.MustNewVersion("1.2.0")
	targetV3.Fields[fieldID()] = definition.Field{Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}}

	planV3 := &base.MigrationPlan{
		Description: "add status field",
		Target:      targetV3,
		VersionBump: definition.BumpMinor,
	}
	_, err = p.Migrate(ctx, "multi_mig", planV3, nil)
	require.NoError(t, err)

	// Verify version bumped
	sc, err = p.Schema(ctx, "multi_mig")
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", sc.Version.String())

	// Verify data on all 3 fields
	coll, err = p.Collection(ctx, "multi_mig")
	require.NoError(t, err)
	result, err = coll.Read(ctx, &query.Query{})
	require.NoError(t, err)
	assert.Len(t, result.Data, 3)

	// ─── Step 4: Rollback V3 → V2 ───────────────────────────────────────
	rolledBack, err := p.Rollback(ctx, "multi_mig", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, rolledBack)

	sc, err = p.Schema(ctx, "multi_mig")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", sc.Version.String())

	coll, err = p.Collection(ctx, "multi_mig")
	require.NoError(t, err)
	result, err = coll.Read(ctx, &query.Query{})
	require.NoError(t, err)
	assert.Len(t, result.Data, 3)

	// ─── Step 5: Rollback V2 → V1 ───────────────────────────────────────
	rolledBack2, err := p.Rollback(ctx, "multi_mig", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, rolledBack2)

	sc, err = p.Schema(ctx, "multi_mig")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", sc.Version.String())

	coll, err = p.Collection(ctx, "multi_mig")
	require.NoError(t, err)
	result, err = coll.Read(ctx, &query.Query{})
	require.NoError(t, err)
	assert.Len(t, result.Data, 3)
}

// TestE2E_Migration_PhaseResolvedAtRuntime verifies that a MigrationPlan
// without an explicit Phase gets its phase resolved at runtime based on
// the backend's DDL capabilities.
func TestE2E_Migration_PhaseResolvedAtRuntime(t *testing.T) {
	ctx := context.Background()
	p, cleanup := setupSQLitePersistence(t)
	defer cleanup()

	v1Fields := map[definition.FieldId]definition.Field{
		fieldID(): {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
		fieldID(): {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
	}
	schema := newMigrationTestSchema("resolve_phase", v1Fields)
	_, err := p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	currentSchema, err := p.Schema(ctx, "resolve_phase")
	require.NoError(t, err)

	// Clone via JSON and add a field — same pattern as generated migration code
	target := cloneSchemaViaJSON(currentSchema)
	target.Version = common.MustNewVersion("1.1.0")
	target.Fields[fieldID()] = definition.Field{Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}}

	// Plan WITHOUT explicit Phase — Relies on ResolvePhase
	plan := &base.MigrationPlan{
		Description: "add status (phase resolved at runtime)",
		Target:      target,
		VersionBump: definition.BumpMinor,
	}
	_, err = p.Migrate(ctx, "resolve_phase", plan, nil)
	require.NoError(t, err)

	sc, err := p.Schema(ctx, "resolve_phase")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", sc.Version.String())
}

// TestE2E_Migration_DryRun_WithSQLite verifies that dry-run mode
// doesn't modify the schema version or data on SQLite.
func TestE2E_Migration_DryRun_WithSQLite(t *testing.T) {
	ctx := context.Background()
	p, cleanup := setupSQLitePersistence(t)
	defer cleanup()

	v1Fields := map[definition.FieldId]definition.Field{
		fieldID(): {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
		fieldID(): {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
	}
	schema := newMigrationTestSchema("dryrun_test", v1Fields)
	_, err := p.CreateCollection(ctx, schema)
	require.NoError(t, err)

	currentSchema, err := p.Schema(ctx, "dryrun_test")
	require.NoError(t, err)

	// Clone via JSON and add a field
	target := cloneSchemaViaJSON(currentSchema)
	target.Version = common.MustNewVersion("1.1.0")
	target.Fields[fieldID()] = definition.Field{Name: "tags", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}}

	dryRun := true
	plan := &base.MigrationPlan{
		Description: "dry run test",
		Target:      target,
		VersionBump: definition.BumpMinor,
	}
	_, err = p.Migrate(ctx, "dryrun_test", plan, &dryRun)
	require.NoError(t, err)

	// Version should NOT have changed
	sc, err := p.Schema(ctx, "dryrun_test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", sc.Version.String())
}
