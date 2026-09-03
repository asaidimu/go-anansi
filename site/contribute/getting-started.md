---
title: Getting started
description: "Fork, branch, write tests, run make test, open a PR. The CLA is part of the PR checklist. Read this before your first contribution."
---

# Getting started

Thanks for considering a contribution to go-anansi!

## Process

1. Open an issue describing the change (bug, feature, drift report between
   the Go and TypeScript codecs — all welcome).
2. Fork / branch from `main`.
3. Make the change with tests.
4. Open a PR. Accepting the [CLA](/contribute/cla) is part of the PR
   checklist — this is what allows the project to stay dual-licensed
   (AGPLv3 + commercial).

## Setup

```bash
git clone https://github.com/asaidimu/go-anansi.git
cd go-anansi
go mod tidy
make build    # compile the project
make test     # run all unit + integration tests
```

Notes for contributors:

- The module currently requires **Go 1.27** (generic methods). CI pins the
  matching `rc` toolchain.
- Golden codegen outputs live in `codegen/golang/testdata/`; regenerate with
  `go test ./codegen/golang/ -update`.
- Wire-format changes require regenerating the cross-language golden
  vectors:

  ```bash
  GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors
  cd packages/anansi && bun test
  ```

- `bin/bump.sh` upgrades the module's major version across the codebase —
  run with `--dry-run` first.

## Where things live

See [Internals map](/contribute/internals-map) for a guided tour of the
repository layout, which directories contain what, and where to start when
fixing a specific kind of bug.

## Reporting bugs and features

Report bugs and feature requests via
[GitHub issues](https://github.com/asaidimu/go-anansi/issues). The
maintainers use `todo/` and `.devnotes/` for working notes — those aren't
user-facing, but they're useful context if you're picking up an issue.

## Read next

- [Testing](/contribute/testing) — the dev env flag, golden vectors,
  cross-language conformance.
- [Code style](/contribute/code-style) — Go and TS conventions.
- [CLA](/contribute/cla) — the dual-licensing mechanism.
- [Internals map](/contribute/internals-map) — where things live.
