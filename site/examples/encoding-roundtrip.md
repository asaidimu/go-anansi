---
title: Go ⇄ TS round trip
description: "Encode a document in Go, send it over the wire, decode it in TypeScript. Verifies byte-for-byte equivalence between the two implementations."
---

# Go ⇄ TS round trip

The [`example/encoding`](https://github.com/asaidimu/go-anansi/tree/main/example/encoding)
directory is the canonical demo of the cross-language wire format guarantee.

## What it shows

- A Go server that encodes a document using the Anansi wire format.
- A TypeScript client that decodes the same bytes.
- A `run.sh` script that orchestrates the round trip end-to-end.
- Verification that the round-tripped document matches the original.

This is the example that CI uses to verify the Go ⇄ TypeScript
byte-for-byte equivalence. If a wire-format change breaks the cross-language
contract, this example fails before a release can cut.

## How to run

```bash
cd example/encoding
./run.sh
```

The script starts the Go server, runs the TS client against it, and prints
the round-tripped document.

## Read next

- [Explanation: Wire format](/explanations/wire-format) — the design and
  packet shapes.
- [Reference: @asaidimu/anansi](/reference/ts-package) — the TS API surface.
- [Reference: Schema IR](/reference/schema-ir) — the compiled form both
  codecs consume.
