---
title: Complex schema
description: "Nested schemas, projections, enums, composites, and unions in one schema. The reference for non-trivial data models."
---

# Complex schema

The [`example/complex`](https://github.com/asaidimu/go-anansi/tree/main/example/complex)
directory shows a non-trivial schema with nested types, projections, and
composite fields.

## What it shows

- Nested schemas (`user.json` with embedded address, profile).
- Composite and union types.
- Multiple projections on the same collection (`UserSummary`, `UserCreate`,
  `UserUpdate`).
- Codegen output for all of the above.

## How to run

```bash
cd example/complex
ANANSI_ENV=development go run .
```

## Read next

- [Reference: Schema format](/reference/schema-format) — the full type
  table including nested schemas and projections.
- [Reference: Struct tag spec](/reference/struct-tag-spec) — including
  idiomatic Go unions and composites.
- [Guide: Domain modeling](/guides/domain-modeling) — the six-question
  decision matrix for schema design.
