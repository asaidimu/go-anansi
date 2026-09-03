---
title: "@asaidimu/anansi"
description: "The TypeScript package — a self-contained implementation of the Anansi wire format. Schema compile/link/address, packet codecs (Dense / Sparse / Batch), ZSTD + BLAKE3 + AES-256-GCM, validation. No Go runtime, no codegen."
---

# @asaidimu/anansi

A self-contained TypeScript implementation of the Anansi wire format ships in
this repository at [`packages/anansi`](https://github.com/asaidimu/go-anansi/tree/main/packages/anansi)
and is published to npm in lockstep with the Go module (same version numbers).

## Install

```bash
bun add @asaidimu/anansi
# or: npm install @asaidimu/anansi
```

## Usage

```ts
import {
  parseSchema,
  link,
  buildManifest,
  encodeDocument,
  decodeDocument,
} from "@asaidimu/anansi";

const fields = buildManifest(link(parseSchema(schemaJSON)));
const wire   = encodeDocument(fields, order);          // auto dense/sparse
const back   = decodeDocument(wire, fields);
```

## Cross-language guarantee

CI replays Go-generated golden packets through the TS codec byte-for-byte. A
drift between the two implementations fails the build before a release can
cut. See the [wire format explainer](/explanations/wire-format) for the
design and the [Go ⇄ TS round trip example](/examples/encoding-roundtrip)
for a working demo.

## What's implemented

- **Schema compile/link/addressing** — bit-exact with the Go linker (verified
  by cross-language conformance tests).
- **Packet codecs** — Dense, Sparse, Batch (row + columnar), all 16 data
  types, full duplex.
- **Transforms** — ZSTD compression, BLAKE3-128 integrity hashing, and
  AES-256-GCM encryption. Browser-compatible backends (WebCrypto, hash-wasm,
  fzstd).
- **Validation** — documents against schemas, schemas against the meta-schema.

> **TODO:** document the per-API surface (`parseSchema`, `link`,
> `buildManifest`, `encodeDocument`, `decodeDocument`, `validate`) with full
> signatures and examples.
