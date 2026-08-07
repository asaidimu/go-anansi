# Configuring the anansi tooling

Anansi is driven by a single `anansi.json` at a project root (found by walking
up to the nearest one) plus CLI flags. The config controls where schemas live,
how types are generated, and where output lands. The scaffold defaults shown
below are just defaults — you can organise a project however you like.

## `anansi.json`

```jsonc
{
  "schema": {
    "glob": "schemas/**/*.schema.json",   // schema file location
    "lockfile": "schemas.lock.json",      // where version+ID history is tracked
    "migrations_dir": "migrations/"       // where migrations + registry are written
  },
  "tsgen": { "out": "types.ts" },        // TypeScript output path
  "gogen": {
    "tags": null,                          // struct tag config (see below)
    "scoped": false,              // generate unexported (low-level) accessors
    "name_rules": [],             // collection-name prefixes, e.g. [{"pattern":"^usr$","prefix":"User"}]
    "mode": ""                    // ""|"full"|"structs"|"model" (zero = full)
  },
  "metadata": {
    "schema_path": "metadata.schema.json", // declarative user-defined metadata
    "out_dir": ""                 // "" = generated metadata.go/providers.go go in migrations_dir
  }
}
```

### Layout knobs (`schema.*`)

- **`schema.glob`** — where `*.schema.json` files live. `anansi migrate
  generate` finds schemas here, and generated `.model.go` files land next to
  each schema file (package = that file's directory). Changing the glob lets
  you move schemas anywhere (e.g. `contracts/*.schema.json`,
  `pkg/store/**/*.schema.json`).
- **`schema.lockfile`** — JSON tracking each collection's current version, its
  hash, embedded schema, and migration history. Field **IDs are keyed off
  this**; never hand-delete entries or you break the linkage.
- **`schema.migrations_dir`** — where generated migrations, `registry.go`,
  `metadata.go`, and `providers.go` are written.

All three are stored **relative** to the project root so tooling works from any
CWD. `anansi scaffold --schemas-dir/--migrations-dir/--lockfile` sets them for
a new project; they can equally be edited by hand.

### Codegen config (`gogen.*`, `tsgen.*`)

- **`gogen.tags`** — optional Go struct-tag override map, e.g. `{"field":"gorm:\"column:foo\""}`.
- **`gogen.scoped`** — emit scoped (unexported) accessor functions instead of
  the full exported API.
- **`gogen.name_rules`** — prefix/rename rules applied to collection type names,
  matching by regexp pattern with a prefix. E.g.
  `[{"pattern":"^usr$","prefix":"User"}]`.
- **`gogen.mode`** — "full" (default: structs + collection + singleton),
  "structs" (DTOs only), or "model" (model + projections, no collection).
- **`tsgen.out`** — output file for `anansi codegen typescript`.

### Metadata config (`metadata.*`)

- **`metadata.schema_path`** — the declarative metadata file that defines the
  injected `_metadata_` nested schema and custom providers. 
- **`metadata.out_dir`** — where `metadata.go`/`providers.go` are generated;
  empty means the `schema.migrations_dir`.

## CLI flags

`anansi codegen golang`:

| Flag | Meaning |
| --- | --- |
| `--glob` | override the schema glob |
| `--mode structs\|model\|full` | what to emit (default full) |
| `--scoped` | emit unexported accessors |
| `--no-tags` | skip tag emission |
| `--dry-run` | print what would be written |

`anansi codegen typescript` / `anansi codegen faker`: `--glob`, `--out`
(typescript only), `--dry-run`.

`anansi migrate generate`: `--glob`, `--lockfile`, `--out`, `--check`,
`--dry-run`. Add `--check` to fail when migrations are needed instead of
writing them (CI).

`anansi migrate squash <collection>`: consolidate a collection's history.

`anansi scaffold`:

| Flag | Meaning |
| --- | --- |
| `--no-interactive` | never prompt; apply defaults (agents/CI) |
| `--existing` | add anansi to an existing module (no go.mod/main.go) |
| `--schemas-dir` | schemas directory (default `schemas`) |
| `--migrations-dir` | migrations directory (default `migrations`) |
| `--lockfile` | lockfile path (default `schemas.lock.json`) |
| `--dry-run` | print what would be created without writing |

## Recs

- Match the existing project's conventions before adding codegen options; read
  its `anansi.json` rather than assuming defaults.
- Explicit flags plus `--no-interactive` is the safe, reproducible way for
  agents to scaffold.