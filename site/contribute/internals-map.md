---
title: Internals map
description: "A guided tour of the repository layout. Where to start when fixing a specific kind of bug, and which directories contain what."
---

# Internals map

A guided tour of the repository layout for new contributors.

## Top-level layout

| Path | What it is |
| --- | --- |
| `anansi.go` | Root package: `Setup` and `Playground` entry points. |
| `anansi.json` | Project config (schema glob, lockfile, migrations dir). |
| `core/` | the library's core packages. |
| `sqlite/` | Reference `DatabaseInteractor` implementation. |
| `codegen/` | Code generators: `golang`, `typescript`, `faker`. |
| `cmd/anansi/` | The CLI. |
| `packages/anansi/` | TypeScript wire-format implementation. |
| `example/` | Seven runnable examples. |
| `tests/` | Cross-cutting tests (conformance, integration, e2e). |
| `docs/` | Legacy spec docs (migrating to `site/`). |
| `todo/` | Working RFCs (migrating to `site/rfc/`). |
| `skills/anansi/` | The AI-agent skill (source of truth for much of this docs site). |
| `.devnotes/` | Automated inline source annotations (issues, todos, decisions). |
| `scripts/` | Project scripts. |
| `utils/` | Top-level utilities (decorators, etc.). |
| `bin/` | Binary helpers (`bump.sh`). |

## Where to start by task

### "I want to fix a bug in codegen"

Start in `codegen/golang/` (or `codegen/typescript/`,
`codegen/faker/`). Golden outputs are in `codegen/golang/testdata/` —
regenerate with `go test ./codegen/golang/ -update`.

### "I want to fix the query engine"

Start in `core/query/`. Note this area is **slated for an overhaul** — see
[the RFC](/rfc/query-engine-overhaul). Build only against the stable
[Query DSL](/reference/query-dsl) surface, not internals.

### "I want to fix the wire format"

Start in `core/encoding/anansi/` (Go) and `packages/anansi/` (TypeScript).
Both sides must stay byte-for-byte equivalent — see
[Testing](/contribute/testing) for the golden-vector workflow.

### "I want to add a new schema type"

Start in `core/schema/definition`. Add the type, update the graph validator
(`core/schema`), update the codegen output (`codegen/golang`), and update
both wire format codecs.

### "I want to add a new backend"

Implement the `DatabaseInteractor` interface from `core/query`. The SQLite
reference is in `sqlite/`. See the [BadgerDB interactor RFC](/rfc/badgerdb-interactor)
for prior art.

### "I want to fix a docs issue"

The docs site is at `site/`. Most content under `site/guides/` and
`site/reference/` was migrated from `skills/anansi/references/` — those
files are still authoritative for AI-agent use. Run `cd site && bun dev`
to preview locally.

## The `.devnotes/` pattern

`.devnotes/index.json` is a living, source-anchored annotation map. Each
entry has an `id`, `category` (issue/todo/status), `status` (open/resolved),
`title`, `body`, and a `location` (file + line range). It's how the
maintainer tracks inline decisions without scattering `// TODO` comments
across the codebase.

When you fix a bug noted in `.devnotes/`, update the entry's `status` to
`resolved` in the same PR.

## CI workflows

`.github/workflows/` contains:

- `test.yaml` — Go unit + integration tests (with `ANANSI_ENV=development`).
- `version.yaml` — semantic-release version bumps.
- `deploy.yaml` — release deployment.
- `docs.yaml` (planned) — VitePress docs build + GitHub Pages deploy.

## Read next

- [Getting started](/contribute/getting-started)
- [Architecture](/explanations/architecture) — the package-level mental model.
- [Data flow](/explanations/data-flow) — the request path.
