# Schema Migration Plan: `schema.SchemaDefinition` → `definition.Schema`

## Objective

Execute a total replacement of the legacy `schema.SchemaDefinition` with
`definition.Schema` across the entire codebase.

- **Target**: `core/schema/definition`
- **Constraint — Zero-Bridge Policy**: Do not author programmatic converters,
  adapter functions, or shim layers. The legacy type must be purged entirely,
  not wrapped or adapted. If you are tempted to write a converter, stop and
  rethink the approach.

---

## System Context

This is a data platform that abstracts persistence. The schema is a
foundational mechanism used across multiple interconnected layers:

- **SQL / Native Query Generation Layer**
- **Query Optimization Layer**
- **Collection Management Layer**
- **Data Validation & Sanitization Layer**

Each layer has its own package-level unit tests and a second layer of
integration tests. These layers are interconnected: lower-level packages are
depended upon by higher-level ones.

---

## Rationale for Migration

- **Type Safety**: Replace string-based maps and `any` types with specialized
  IDs (`FieldId`, `SchemaId`) and structured `LiteralValue` types.
- **Performance**: Use byte-based `FieldType` and optimized map lookups.
  Prioritize zero-allocation paths. Avoid reflection and unnecessary heap
  escapes. Align with the mechanical sympathy of the go-anansi engine.
- **Consistency**: Align with the new architecture's document-centric
  persistence model.

---

## Phase 0: Code Review & Migration Planning (Do This First — Do Not Skip)

**Before touching a single line of production code**, you must perform a full
reconnaissance of the codebase. The output of this phase is a detailed
migration plan, not working code. This is the most important phase.

### 0.1 — Dependency Graph Analysis

Read the codebase and produce a complete picture of how `schema.SchemaDefinition`
is used:

- Which packages import or reference `schema.SchemaDefinition`?
- What is the dependency order of those packages (bottom-up)?
- Which packages are leaves (no dependents) vs. roots (many dependents)?
- Are there any circular dependencies or particularly tangled call sites?

Present this as an ordered list of packages, from lowest-level to
highest-level, which defines the migration sequence.

### 0.2 — Symbol & Call Site Inventory

For each package in the dependency graph, enumerate:

- Every type, function, method, and interface that references
  `schema.SchemaDefinition`.
- Every location where the type is constructed, passed, stored, or returned.
- Any legacy helper functions that exist solely to support the old schema
  (these are candidates for deletion under the Clean Sweep Rule, not
  migration).

### 0.3 — Risk & Impact Analysis Per Layer

For each of the four system layers (query generation, query optimization,
collection management, data validation), assess:

- **Migration complexity**: How deeply is `schema.SchemaDefinition` embedded?
  Is it a thin wrapper or a core invariant?
- **Risk surface**: What breaks if the migration is done incorrectly? What
  are the semantic differences between the old and new schema types that could
  introduce subtle bugs?
- **Test coverage**: Are the existing unit and integration tests sufficient to
  detect regressions? Flag any areas where test coverage is thin before
  migration begins.

### 0.4 — Recommended TDD Migration Sequence

Produce an ordered, package-by-package migration sequence following the
bottom-up TDD strategy described below. For each package in the sequence,
specify:

1. Which tests need to be updated first to reflect the new type signatures.
2. Which production symbols change.
3. What the expected compiler error surface looks like before the fix.
4. What "green" looks like for that package before moving to the next.

Do not proceed to Phase 1 until this plan is reviewed and agreed upon.

---

## Migration Strategy: Bottom-Up, Package-Local TDD

The guiding principle is strategic, not tactical. We move layer by layer, starting from the lowest-level packages and working upward.

**THE STRICT ISOLATION IMPERATIVE**
**EACH MODULE MUST PASS ITS OWN UNIT TESTS BEFORE TOUCHING ANY OTHER PACKAGE.** > 
You are strictly forbidden from jumping to dependent packages to "fix" them just because they are failing due to changes in the current module. If a downstream package is broken, it stays broken until its specific turn in the migration sequence. Focus exclusively on making the current package Green within its own boundary.

### Why This Order Matters

When a low-level package is migrated correctly, higher-level packages that
depend on it will surface compiler errors — but those errors are expected and
are a signal that the lower layer is now correct. We do not attempt to silence
upstream errors by patching high-level code prematurely. We let the compiler
errors accumulate upward naturally as we climb the dependency graph.

### The Red-Green Discipline

For each package:

1. **Red**: Update the type signatures and references in the package to use definition.Schema. The package and its dependents will no longer compile. **This is the desired state**.
2. **Fix**: Resolve all type mismatches within the current package boundary only.
3. **Green**: Run `go test ./tests/unit/package` The package must pass 100% of its internal tests.

4. **Halt & Verify**: Even if the rest of the project is a "sea of red" in the compiler output, if the current package is green, it is done. Do not touch the next package until this one is verified.

5. **Commit**: Move to the next package in the sequence.

---

## Execution Principles

- **Package-Local Green**: The unit of progress is the package, not the codebase. When a package is the current migration focus, it must be left in a fully passing state before work moves to the next package. Do not attempt to silence upstream errors by patching high-level code prematurely. Downstream packages that consequently break are expected—they are signals of progress, not triggers for context-switching.
- **Zero-Bridge Policy**: No converters. No shims. No adapter types. If you find yourself writing one, stop—it means the migration at a lower level is incomplete or the current package's logic hasn't been fully refactored to the new types.
- **The "One-at-a-Time" Rule**: Under no circumstances should you have uncommitted changes in multiple packages simultaneously. Finish the current package, verify it with go test ./path/to/package/..., and only then proceed to the next item in the dependency graph.
- **Correctness Above All**: The goal is a correct system, not a compiling
  one. Passing tests achieved by weakening assertions or bypassing type checks
  are worse than failing tests. A failing test is honest; a dishonest passing
  test is a latent bug.

---

## What "Done" Means

A package migration is complete when:

- [ ] All references to `schema.SchemaDefinition` are removed from the package.
- [ ] All type signatures use `definition.Schema` and its associated types.
- [ ] `go test ./tests/unit/package/...` passes with no skipped or weakened assertions.
- [ ] No bridges, shims, or converters have been introduced.
- [ ] Any legacy-only helper functions have been deleted.

The full migration is complete when:

- [ ] `schema.SchemaDefinition` has zero references in the codebase.
- [ ] `make test ./...` passes cleanly.
- [ ] `schema.SchemaDefinition` is removed or marked deprecated.

---

## Phases

### Phase 1: Core Interface Breaking Changes

Starting from the lowest-level package identified in Phase 0, update type
signatures to use `definition.Schema`. Let compiler errors surface naturally
upward. Do not chase upstream errors — resolve only the current package.

### Phase 2: Layer-by-Layer Migration

Following the sequence established in Phase 0, migrate each package in
dependency order:

- SQL / Query Generation Layer
- Query Optimization Layer
- Collection Management Layer
- Data Validation & Sanitization Layer

Apply the Red-Green discipline for each package. Do not move to the next
package until the current one is green.

### Phase 3: Cleanup & Validation

- Run the full test suite: `make test ./...`
- Fix any remaining regressions.
- Delete or deprecate `schema.SchemaDefinition`.
- Confirm zero references remain.

---

## Session State

Update these sections at the end of every interaction. This is the
authoritative record of migration progress.

### Current Progress

- [x] Phase 0 complete — migration plan agreed upon
- [ ] Phase 1 complete — core interfaces migrated
- [ ] Phase 2 complete — all layers migrated
- [ ] Phase 3 complete — cleanup done, full suite green

### Package Migration Checklist

| Package | Status | Notes |
|---------|--------|-------|
| `core/storage` | [ ] | Core interface breaking changes |
| `core/query` | [ ] | Interface and DSL |
| `core/data` | [ ] | Metadata and Factory |
| `core/schema/validator` | [ ] | Migration to `definition.Schema` |
| `core/schema/migration` | [ ] | Migration to `definition.Schema` |
| `sqlite/query` | [ ] | SQLite backend |
| `core/persistence/registry` | [ ] | Schema registry |
| `core/persistence/collection` | [ ] | Managed collections |
| `core/persistence/persistence` | [ ] | Top-level persistence |
| `tests/` | [ ] | All unit and integration tests |

### TODO

- [ ] Migrate `core/data` (Metadata, Factory)
- [ ] Migrate `core/query` internals
- [ ] Migrate `core/persistence` stack
- [ ] Full test suite validation

### IN PROGRESS
- [ ] Migrate `sqlite/query`. Test using `go test ./tests/unit/sqlite`

