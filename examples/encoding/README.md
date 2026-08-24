# Anansi Encoding — Cross-Language Duplex Example

Proves that a TypeScript client and a Go server exchange **identical bytes**
using the Anansi binary wire format, in both directions.

## Flow

```
ts-client/main.ts            go-server/main.go             ts-client/decode-response.ts
──────────────────           ───────────────────           ──────────────────────────
link(schema)                 DecodeAnansi (dense)          decodeDocument (dense)
encode dense/sparse/batch →  DecodeAnansi (sparse)         decodeBatch (columnar) ←
                             DecodeBatchRows
                             EncodeDense ────────────────→ ✓ verify single
                             EncodeBatchColumnar ────────→ ✓ verify result set
```

- **Client → server**: the same order document encoded three ways (dense,
  sparse, row-batch). The Go server decodes each with the production codec
  (`definition.Compile`/`Link` + `anansi.DecodeAnansiInto`/`DecodeBatchRows`).
- **Server → client**: a query-result set answered with a Dense packet plus a
  **columnar batch** (the server's efficient shape), decoded by the TS client.

All packets are stamped with schema fullVersion 7; both sides resolve the
schema independently from identical JSON — no shared runtime.

## Run

```sh
bun run examples/encoding/ts-client/main.ts        # client encodes request
go run ./examples/encoding/go-server               # server decodes + responds
bun run examples/encoding/ts-client/decode-response.ts   # client verifies response
```

Expected final output:

```
client ← server  (TS codec decoding Go-encoded packets)
  dense   : v7 ✓ {"total":129.99,"order_id":"ORD-2026-0001","paid":true}
  columnar: v7 ✓ 2 records decoded

full duplex verified: TS ⇄ Go on identical bytes.
```

## Notes

- Packets here are plain (no zstd/BLAKE3/AES frames); those transforms are
  flag-gated on the wire and supported by the Go codec, with TS support
  landing alongside `@asaidimu/anansi`'s transform module.
- Bytes fields are represented as base64 strings in the document model;
  int64 values surface as JS numbers and throw past `MAX_SAFE_INTEGER`.
- Why this works: CI pins cross-language parity — Go emits golden packets +
  linker internals (`packages/anansi/testdata/golden.json`), and the TS suite
  fails on any byte drift. A re-encode mismatch anywhere breaks the build
  before it can break a client.
