---
title: "Installation"
description: "Install the anansi CLI, add the module to a Go project, scaffold a baseline, and verify the toolchain works end-to-end."
---

# Installation

## Prerequisites

- **Go 1.27 or later.** Run `go version` to check. The generated collection
  wrapper relies on generic methods on concrete types, which require Go 1.27.
- **SQLite.** The driver is bundled with the Go module — no system install
  required.
- **(Optional) Bun.** Only needed if you'll work with the TypeScript package
  `@asaidimu/anansi`.

## Install the CLI

```bash
go install github.com/asaidimu/go-anansi/v8/cmd/anansi@latest
anansi version
```

If `anansi version` prints a version string, you're done. If not, check that
`$GOPATH/bin` is on your `$PATH`.

## Add Anansi to a Go module

```bash
go get github.com/asaidimu/go-anansi/v8@latest
```

The module is versioned at `v8` (module path
`github.com/asaidimu/go-anansi/v8`). Older major versions (`v7` and below)
are unmaintained.

## Scaffold a project

For a new standalone app:

```bash
anansi scaffold myapp
cd myapp
```

This creates the project baseline:

- `anansi.json` — config (schema glob, lockfile path, migrations dir).
- `schemas/` — a starter `example.schema.json`.
- `migrations/` — empty until you run `anansi migrate generate`.
- `schemas.lock.json` — field ID stability tracker.
- `metadata.schema.json` — the document metadata sub-schema.
- `AGENTS.md` — context for AI coding assistants.
- `go.mod` and `main.go` — a SQLite `Playground` with a demo create/read.

For adding Anansi to an existing Go module (library mode):

```bash
anansi scaffold --existing --schemas-dir src/defs --migrations-dir src/migrations
```

Library mode is **non-destructive**: no `go.mod`/`main.go` is written, and
existing files are never overwritten. An existing `AGENTS.md`,
`metadata.schema.json`, schema files, lockfile, or generated
migration/registry files are left exactly as they are. If the project already
owns a schema, scaffold skips generating its example migration/lockfile/
registry entirely; run `anansi migrate generate && anansi codegen golang`
afterwards.

## Verify the toolchain

From the scaffolded project root:

```bash
ANANSI_ENV=development go run .
```

You should see log lines confirming the schema was registered, a demo
document was created, and a read returned it. The `ANANSI_ENV=development`
flag is required because scaffolded fixtures use plain field-name keys;
production mode requires UUIDv7 field IDs. See
[Testing](/contribute/testing) for why.

## Verify the CLI

```bash
anansi version          # prints version
anansi codegen --help   # lists codegen subcommands
anansi migrate --help   # lists migrate subcommands
anansi schema --help    # lists schema subcommands
```

If all four commands respond without error, the toolchain is ready.

## Common pitfalls

- **`anansi: command not found`.** `$GOPATH/bin` is not on your `$PATH`. Add
  `export PATH="$PATH:$(go env GOPATH)/bin"` to your shell config.
- **`go: cannot find module`**. You're on Go 1.26 or earlier. Update to 1.27.
- **`ERR_REGISTRY_INVALID_SCHEMA: Field ID 'name' is not a valid UUIDv7`.**
  You're running in production mode against fixtures that use plain field
  names. Set `ANANSI_ENV=development` or run `anansi migrate generate` to
  normalize first.

## Next

[Your first schema →](/tutorial/first-schema) — declare a `Products`
collection from scratch.
