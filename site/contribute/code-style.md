---
title: Code style
description: "Go and TypeScript conventions for go-anansi contributions. gofmt must be clean; no any leaks in public TS signatures."
---

# Code style

## Go

- Follow the surrounding code; `gofmt` must be clean.
- Exported symbols should have a `// Name ...` godoc comment. The project's
  godoc coverage is currently around 31% — adding godoc to exported symbols
  you touch is a welcome improvement.
- Prefer returning `error` over panicking in library code.
- Use `common.SystemError` for richer error context in low-level utilities
  (per `.devnotes/`).
- Generic methods on concrete types are fair game — Anansi requires Go 1.27
  and uses them throughout.

## TypeScript

- `strict` tsc, no `any` leaks in public signatures.
- Use `type` for unions and intersections; `interface` for object shapes that
  may be extended.
- Prefer `unknown` over `any` when you genuinely don't know the shape.

## Schema files

- Field map keys are the field **name** while authoring (e.g.
  `"shipping_address"`). `anansi migrate generate` rewrites them to UUIDv7.
- Never hand-edit an existing UUIDv7 field ID.
- Use snake_case for field names.

## Generated files

- The generated file is overwritten on every codegen run — never edit it
  directly. Extend models in separate files (`<model>_utils.go`).
- Golden codegen outputs in `codegen/golang/testdata/` are regenerated with
  `go test ./codegen/golang/ -update`.

## Commit messages

The project uses
[semantic-release](https://github.com/asaidimu/go-anansi/blob/main/.releaserc.json)
with conventional commits. Format:

```
type(scope): subject

body
```

Common types: `feat`, `fix`, `refactor`, `perf`, `docs`, `test`, `chore`.
Common scopes: `core`, `encoding`, `query`, `persistence`, `codegen`,
`reflect`.

## Read next

- [Testing](/contribute/testing)
- [Getting started](/contribute/getting-started)
