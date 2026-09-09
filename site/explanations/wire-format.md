---
title: "Wire format (Go ⇄ TypeScript)"
description: "How the Go codec and the TypeScript codec stay byte-for-byte equivalent. CI replays golden vectors across both languages; drift fails the build before a release cuts."
---

# Wire format (Go ⇄ TypeScript)

Go-Anansi ships a TypeScript package (`@asaidimu/anansi`) that implements
the Anansi wire format natively — no Go runtime, no codegen step. The two
implementations are verified **byte-for-byte equivalent** by CI.

## Why a binary wire format

If you're building a Go backend and a TypeScript frontend (or any
TypeScript-side consumer — a worker, a CLI, a browser app), you usually
serialize JSON. JSON is fine, but it has real costs:

- **Slow.** Text parsing, allocation-heavy, no streaming.
- **Loose.** No schema enforcement at the wire level — a missing field is
  null, a number is a float, a large integer silently loses precision.
- **Bulky.** Repeated field names, no compression, no columnar layout for
  batch data.

Anansi's wire format is a binary codec derived from the schema IR. It
supports three packet shapes:

| Shape | Use case |
| --- | --- |
| **Dense** | Every field present, every field required — minimal overhead. |
| **Sparse** | Partial documents (updates, projections) — field presence bitmap up front. |
| **Batch (row)** | Many documents of the same schema — amortized header. |
| **Batch (columnar)** | Many documents, analytical access patterns — column-oriented storage. |

All 16 schema data types are supported, full duplex (encode + decode).

## Transforms

The wire format stacks three transforms, each optional:

- **ZSTD compression** — for batch packets especially, dramatic size
  reduction. TS uses `fzstd`; Go uses the standard library bindings.
- **BLAKE3-128 integrity hashing** — detect corruption before decode. TS uses
  `hash-wasm`; Go uses the native BLAKE3 implementation.
- **AES-256-GCM encryption** — for at-rest or in-flight confidentiality. TS
  uses WebCrypto; Go uses the standard library.

The TypeScript package uses browser-compatible backends so the same code runs
in Node, Bun, and browsers.

## The cross-language guarantee

CI replays Go-generated golden packets through the TS codec byte-for-byte:

```bash
# Regenerate golden vectors (Go side)
GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors

# Verify TS replays them
cd packages/anansi && bun test
```

A drift between the two implementations fails the build before a release can
cut. This is the strongest possible cross-language guarantee — not "the APIs
match" but "the bytes match."

## A round trip

Encode on the Go side:

```go
import "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"

fields := anansi.BuildManifest(anansi.Link(anansi.ParseSchema(schemaJSON)))
wire, err := anansi.EncodeDocument(fields, order)
if err != nil { /* ... */ }
// wire is a []byte — send it over HTTP, write it to a file, push it through a queue.
```

Decode on the TypeScript side:

```ts
import { parseSchema, link, buildManifest, decodeDocument } from "@asaidimu/anansi";

const fields = buildManifest(link(parseSchema(schemaJSON)));
const doc = decodeDocument(wireBytes, fields);
// doc is a plain object matching the schema shape.
```

Both sides consume the same schema. The bytes are identical. The round-trip
is lossless.

See the [Go ⇄ TS round trip example](/examples/encoding-roundtrip) for a
working demo with `run.sh`.

## When to use it

- **Go backend, TS frontend** — encode on the Go side, decode in the browser,
  skip JSON entirely. Saves bandwidth, gives you schema enforcement at the
  wire level, and is faster than JSON parse.
- **Go primary, TS workers** — share the schema, share the wire format, no
  JSON in between.
- **Browser-side validation** — the TS package can validate documents
  against schemas and schemas against the meta-schema, client-side. Catch
  invalid input before it hits the network.
- **At-rest storage** — the wire format + AES-256-GCM is a compact,
  encrypted, integrity-checked persistence format for blobs.

## When NOT to use it

- **Public API** — if your consumers aren't running Anansi, JSON is more
  interoperable. Don't force binary on consumers who can't decode it.
- **Pure Go projects** — the wire format is overhead if you don't need
  cross-language coherence. Use plain Go structs and skip the codec.
- **Streaming workloads** — Anansi is request/response oriented; for
  streaming, use your transport's native framing (gRPC, WebSocket frames,
  etc.).

## Related

- [@asaidimu/anansi reference](/reference/ts-package) — the TS API surface.
- [Go ⇄ TS round trip example](/examples/encoding-roundtrip) — a working
  demo.
- [Schema IR](/reference/schema-ir) — the compiled form both codecs consume.
