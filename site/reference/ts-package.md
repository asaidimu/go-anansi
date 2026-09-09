---
title: "@asaidimu/anansi"
description: "The TypeScript package — codec facade, wire codecs, validation (incl. Standard Schema), streaming migrations, transforms. Self-contained, byte-compatible with Go."
---

# @asaidimu/anansi

A self-contained TypeScript implementation of the Anansi stack ships in this
repository at [`packages/anansi`](https://github.com/asaidimu/go-anansi/tree/main/packages/anansi)
and is published to npm in lockstep with the Go module (same version numbers).
It is not just a codec: schema compile/link, document validation, streaming
schema migrations, and the full transform pipeline all run in TS with no Go
runtime. What it doesn't do is persistence and codegen — there is no query
engine and no `anansi codegen` equivalent; the TS side consumes schemas the
Go side (or hand-written JSON) produces.

```bash
bun add @asaidimu/anansi
# or: npm install @asaidimu/anansi
```

## The facade: `AnansiCodec`

One immutable object per schema — compile once, share across requests:

```ts
import { AnansiCodec } from "@asaidimu/anansi";

const codec = await AnansiCodec.create(schemaJSON);
const wire  = await codec.encode(order);          // Dense/Sparse by heuristic
const { version, doc } = await codec.decode(wire);

const batchWire = await codec.encodeBatch(orders);      // row batch
const colWire   = await codec.encodeColumnar(orders);   // columnar batch
const back      = await codec.decodeBatch(batchWire);
```

Options (`AnansiCodecOptions`):

```ts
const codec = await AnansiCodec.create(schemaJSON, {
  fullVersion: 7,                    // version stamped into packets (0–1023)
  kind: "auto",                      // or force "dense" / "sparse"
  compression: true,                 // ZSTD
  integrity: true,                   // BLAKE3-128 hash
  encryptionKey: key,                // AES-256-GCM
  decryptionKey: key,                // for incoming encrypted packets
});
```

Underneath, the pipeline is `parseSchema` → `link` → `buildManifest`
(compiled descriptors, slots, DataPoints — bit-exact with the Go linker),
then `encodeDocument` / `decodeDocument` (single), `encodeBatchRows` /
`encodeBatchColumnar` / `decodeBatch` (batch). All 16 data types, full
duplex. Use the facade unless you are building custom tooling; the
`encodeAnansiPacket` / `decodeAnansiPacket` variants add the transform
envelope (`FLAG_COMPRESSED`, `FLAG_ENCRYPTED`, `FLAG_HASH_PRESENT`).

## Validation

```ts
import { DocumentValidator, SchemaValidator } from "@asaidimu/anansi";

const validator = new DocumentValidator(manifest /* or schema */);
const issues: Issue[] = await validator.validate(doc);
const partial         = await validator.validatePartial(doc);
const loose           = await validator.validateLoose(doc);

const schemaIssues = await SchemaValidator.validate(schemaDef); // vs meta-schema
```

`Issue[]` mirrors the Go `common.Issue` shape (code, message, location), so
validation results cross the language boundary intact. For form libraries,
`StandardDocumentValidator` implements the **Standard Schema** interface —
drop it straight into TanStack Form, Conform, or any Standard-Schema-aware
tooling:

```ts
import { StandardDocumentValidator } from "@asaidimu/anansi";
const schema = new StandardDocumentValidator(manifest);
```

## Migrations without Go

The TS side runs the full migration lifecycle — versioning helpers plus a
streaming engine:

```ts
import {
  MigrationEngine, classifyChangeImpact,
  calculateNextBump, bumpVersion,
} from "@asaidimu/anansi";

const impact = classifyChangeImpact(changes);   // breaking vs safe
const bump   = calculateNextBump(version, impact);
const next   = bumpVersion(version, bump);

const engine = new MigrationEngine(currentSchema, migrations, history);
await engine.add({ changes, description, rollback, transform });
const dry: DryRunResult = await engine.dryRun(candidate);
const out: ReadableStream = await engine.migrate(docStream);
const back: ReadableStream = await engine.rollback(out);
await engine.rollbackToVersion(out, "1.2.0");
```

`migrate` / `rollback` operate on streams, so large datasets never
materialize in memory. Migrations carry checksums; concurrent runs fail
fast (`MigrationErrorCode.CONCURRENT_OPERATION`). `engine.data()` snapshots
`{ schema, history, migrations }` for inspection or persistence.

## Transforms

Compression (ZSTD), integrity (BLAKE3-128), and encryption (AES-256-GCM)
are async and browser-safe (WebCrypto, hash-wasm, fzstd backends) — the same
code runs in Node, Bun, and the browser. Enable per-codec via options (above)
or call `encodeAnansiPacket` / `decodeAnansiPacket` directly for manual
control.

## Cross-language guarantee

CI replays Go-generated golden packets (`packages/anansi/testdata/golden.json`)
through the TS codec byte-for-byte, plus parity suites for the linker,
transforms, validation, and migrations. Drift between the implementations
fails the build before a release can cut. See the
[wire format explainer](/explanations/wire-format) for the design and the
[Go ⇄ TS round trip example](/examples/encoding-roundtrip) for a working demo.

## API map

| Area | Entry points |
| --- | --- |
| Codec facade | `AnansiCodec.create`, `encode`, `decode`, `encodeBatch`, `decodeBatch`, `encodeColumnar` |
| Schema pipeline | `parseSchema`, `link`, `buildManifest`, `Compiler`, `DataTypes`, descriptor/slot helpers |
| Wire codecs | `encodeDocument`, `decodeDocument`, `encodeBatchRows`, `encodeBatchColumnar`, `decodeBatch` |
| Transforms | `encodeAnansiPacket`, `decodeAnansiPacket`, `FLAG_*` |
| Validation | `DocumentValidator`, `SchemaValidator`, `StandardDocumentValidator`, `Issue`, `PredicateMap` |
| Migrations | `MigrationEngine`, `classifyChangeImpact`, `calculateNextBump`, `bumpVersion`, `schemaChangeToPatch`, migration/patch types |
| Utilities | `deepMerge`, `sha256` |

## Related

- [Wire format](/explanations/wire-format) — packet design, transforms, versioning.
- [Go ⇄ TS round trip](/examples/encoding-roundtrip) — end-to-end demo.
- [Schema format](/reference/schema-format) — the JSON the codec consumes.
