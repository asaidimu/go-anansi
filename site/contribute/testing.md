---
title: Testing
description: "The dev env flag (ANANSI_ENV=development), golden vectors, and cross-language conformance. What to run before opening a PR."
---

# Testing

## Run the Go tests

```bash
ANANSI_ENV=development make test
```

Integration and e2e tests **require** the development environment mode:

```bash
ANANSI_ENV=development make test
```

Production mode (the default) requires UUIDv7 field IDs; without the dev
env, schema fixtures that use plain field IDs fail with
`ERR_REGISTRY_INVALID_SCHEMA` ("Field ID '...' is not a valid UUIDv7").

## Run the TypeScript tests

```bash
cd packages/anansi && bun install && bun test && bunx tsc --noEmit
```

## Cross-language conformance

If you touched anything that affects encoded bytes, regenerate golden
vectors and verify both languages still agree:

```bash
# Regenerate golden vectors (Go side)
GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors

# Verify TS replays them
cd packages/anansi && bun test
```

The TS suite replays Go's packets byte-for-byte; a mismatch fails CI before
a release can cut. This is the strongest possible cross-language guarantee —
**a wire-format drift breaks the build**.

## Golden codegen outputs

Golden codegen outputs live in `codegen/golang/testdata/`. Regenerate when
you change codegen output:

```bash
go test ./codegen/golang/ -update
```

Commit the regenerated files alongside the codegen change.

## What to run before a PR

1. `ANANSI_ENV=development make test` — Go unit + integration.
2. `cd packages/anansi && bun test && bunx tsc --noEmit` — if you touched
   the TS package.
3. `GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors`
   then `cd packages/anansi && bun test` — if you touched the wire format.
4. `go test ./codegen/golang/ -update` — if you touched codegen output.

## Bug fixing precedence

Should you discover a bug while working on the codebase, **fixing the bug
takes precedence over whatever else you are doing.** Bugs don't wait for
their own PR — fix them in the same branch you're already working in,
with a test that demonstrates the fix.

## Read next

- [Code style](/contribute/code-style)
- [Getting started](/contribute/getting-started)
