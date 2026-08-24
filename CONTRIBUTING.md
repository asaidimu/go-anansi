# Contributing

Thanks for considering a contribution to go-anansi!

## Process

1. Open an issue describing the change (bug, feature, drift report between
   the Go and TypeScript codecs — all welcome).
2. Fork / branch from `main`.
3. Make the change with tests.
4. Open a PR. Accepting the [CLA](CLA.md) is part of the PR checklist — this
   is what allows the project to stay dual-licensed (AGPLv3 + commercial).

## Testing

```sh
# Go (development env is required: fixtures use plain field IDs)
ANANSI_ENV=development make test

# TypeScript package (@asaidimu/anansi)
cd packages/anansi && bun install && bun test && bunx tsc --noEmit
```

If you touched anything that affects encoded bytes, regenerate golden
vectors and verify both languages still agree:

```sh
GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors
cd packages/anansi && bun test
```

The TS suite replays Go's packets byte-for-byte; a mismatch fails CI before
a release can cut.

## Code style

Go: follow the surrounding code; `gofmt` must be clean.
TypeScript: `strict` tsc, no `any` leaks in public signatures.

## Licensing

Contributions are licensed to the project under the
[CLA](CLA.md); outbound licensing remains AGPLv3-or-later with commercial
availability, at the maintainer's discretion.
