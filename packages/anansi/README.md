# @asaidimu/anansi

Self-contained TypeScript implementation of the **Anansi binary wire format** —
the schema-driven, high-performance serialization used by the Go-Anansi
persistence layer. Compile schemas, address fields, encode/decode packets, and
validate documents — all in TypeScript, byte-compatible with the Go
implementation.

> **Versioning**: released in tandem with the Go library. v8.x.y of this
> package is wire-compatible with go-anansi v8.x.y.

## Install

```sh
bun add @asaidimu/anansi   # or npm/pnpm/yarn
```

Runs everywhere: Bun, Node ≥ 18, and browsers (WebCrypto + WASM backends; no
Node built-ins in the codec paths).

## Quick start

```ts
import { AnansiCodec } from "@asaidimu/anansi";

// Compile once (per schema version / endpoint), cache the instance.
const codec = await AnansiCodec.create(schemaJSON, { fullVersion: 7 });

// Encode & decode documents — auto-selects Dense vs Sparse framing.
const wire = await codec.encode(order);
const { version, doc } = await codec.decode(wire);
```

The codec binds everything expensive: compiled tables, addressing, and your
transform/key choices. Instances are immutable — share one across requests,
or keep one per endpoint/collection at your discretion.

### Batches

```ts
const upload   = await codec.encodeBatch(orders);       // row-oriented
const results  = await codec.decodeBatch(serverBytes);  // row or columnar in
```

## Client-side integration

The package ships the **codec only** — transport is yours. A typical web app
wires it into `fetch` like this:

**1. Bootstrap: fetch the schema once, compile once, cache per version.**

```ts
// anansi-client.ts
import {
  AnansiCodec,
  DocumentValidator, metaSchemaPredicateMap,
} from "@asaidimu/anansi";

export class AnansiClient {
  private constructor(
    private baseUrl: string,
    private codec: AnansiCodec,
    private validator: DocumentValidator,
  ) {}

  /** Server exposes its schema JSON + active version (once per session). */
  static async connect(baseUrl: string): Promise<AnansiClient> {
    const res = await fetch(`${baseUrl}/schema`);
    const { schema, fullVersion } = await res.json();

    const codec = await AnansiCodec.create(schema, { fullVersion });
    const validator = await DocumentValidator.create(
      schema as never, metaSchemaPredicateMap,
    );
    return new AnansiClient(baseUrl, codec, validator);
  }

  /** Send one document; returns the server's decoded reply. */
  async send(path: string, doc: Record<string, unknown>) {
    // Optional but recommended: validate user input before encoding.
    const issues = await this.validator.validate(doc);
    if (issues.length) throw new Error(`invalid document: ${issues[0]!.code}`);

    const wire = await this.codec.encode(doc);

    const res = await fetch(this.baseUrl + path, {
      method: "POST",
      headers: {
        "Content-Type": "application/vnd.anansi.binary",
        "X-Anansi-Version": String(this.codec.fullVersion),
      },
      body: wire,
    });

    return this.codec.decode(new Uint8Array(await res.arrayBuffer()));
  }
}
```

**2. Use it like any API client.**

```ts
const client = await AnansiClient.connect("https://api.example.com");
const { doc } = await client.send("/orders", order);
console.log(doc.order_id);
```

**3. Conventions that make servers cooperate.**

| Convention | Why |
|---|---|
| `Content-Type: application/vnd.anansi.binary` | The MIME type reserved in the spec (Appendix B); lets servers route binary vs JSON bodies |
| `X-Anansi-Version` header (or the packet's own embedded version) | Schema pinning — server decodes with exactly the schema you compiled |
| One packet per HTTP body | Self-delineating framing; no length prefixes or chunking needed |
| Plain packets from clients | Servers accept them unconditionally; compression/encryption are optional upgrades |

**4. What the browser gives you for free.**

- All codec paths are pure JS + `Uint8Array` — no Node built-ins.
- Transforms use WebCrypto (`AES-GCM`), WASM (`BLAKE3`, hash-wasm), and pure
  JS (`fzstd`) — every backend works in a tab or worker.
- Only *outgoing* zstd compression is unavailable in browsers (no stdlib
  compressor); clients simply send plain packets and still receive compressed
  responses.

**5. Validate before you send, after you receive.**

```ts
await client.validator.validatePartial(patchBody); // PATCH-shaped documents
```

The same `DocumentValidator` semantics run on both sides of the wire, so
client-side validation catches what the server would reject — before the
bytes leave the tab.

## Transforms (compression · integrity · encryption)

All async (WebCrypto/WASM), all composable, all browser-safe:

```ts
import { encodeAnansiPacket, decodeAnansiPacket } from "@asaidimu/anansi";

const sealed = await encodeAnansiPacket(fields, doc, 7, {
  compression: true,            // ZSTD (flags bit 2)
  integrity: true,              // BLAKE3[0..16) over plaintext (bit 7)
  encryptionKey: key32bytes,    // AES-256-GCM (bit 6)
});

const { doc } = await decodeAnansiPacket(sealed, fields, {
  decryptionKey: key32bytes,
});
```

Backend matrix:

| Transform | Browser | Bun / Node |
|---|---|---|
| zstd decompress | `fzstd` (pure JS) | `fzstd` |
| zstd compress | send plain packets¹ | `node:zlib` |
| BLAKE3-128 | `hash-wasm` | `hash-wasm` |
| AES-256-GCM | WebCrypto | WebCrypto |

¹ Browsers have no stdlib zstd compressor. Servers accept plain packets, so
this never blocks a client; a WASM compressor can be added later.

Order of operations follows the spec: compress → encrypt on encode;
decrypt → decompress → verify digest over plaintext on decode. Tampered
packets fail loudly (`integrity check failed`).

## Validation

Documents against schemas, and schemas against the meta-schema:

```ts
import {
  DocumentValidator, SchemaValidator, metaSchemaPredicateMap,
} from "@asaidimu/anansi";

const validator = await DocumentValidator.create(schemaJSON, metaSchemaPredicateMap);
validator.validate(doc);          // strict
validator.validatePartial(patch); // PATCH payloads: skips REQUIRED_FIELD_MISSING
validator.validateLoose(draft);   // also skips UNEXPECTED_FIELD

await SchemaValidator.validate(schemaJSON); // schema ↔ meta-schema conformance
```

Modes, issue codes, constraint scoping, and predicate semantics mirror the Go
implementation in `core/schema/definition/validator.go`.

## Semantics worth knowing

- **int64 → number**: integer fields surface as JS numbers; values beyond
  `Number.MAX_SAFE_INTEGER` throw rather than silently losing precision.
- **Three field states** (spec §2.7), mapping 1:1 onto JavaScript:
  | JS | Wire state | Dense | Sparse |
  |---|---|---|---|
  | `undefined` / key missing | Not Set | `00`, no bytes | omitted |
  | `null` | Null | `01`, no bytes | DataPoint with null-bit set, no bytes |
  | any other value | Has Value | `10` + encoded value | DataPoint + encoded value |

  Decoding reverses it: Not Set omits the key, Null restores `null`. Inside
  records/unknown payloads nulls are payload bytes, preserved verbatim.
  (Note: Go's *JSON→document* boundary treats null leaves as absence before
  encoding — that is a JSON-layer choice, not a wire-format one.)
- **Zero-copy strings** are the default on decode: values view one bulk-copied,
  container-owned backing buffer (one memmove per packet, zero per-string
  allocations). Use `WithCopyStrings` if decoded documents outlive their
  working set.
- **Bytes** fields are base64 strings in the document model.
- **Schema versioning**: packets carry a 10-bit `fullVersion`; keep one
  compiled schema per version and decode against it (explicit pinning, no
  silent structural tolerance).

## Conformance

This package is generated-and-tested against the Go reference in the same
repository:

- **Linker parity** — TS compile/link reproduces Go's descriptors, DataPoints,
  local offsets, footprints and addresses field-for-field.
- **Golden vectors** — Go emits real packets (dense/sparse/batch × transform
  combinations); CI replays them byte-for-byte here and re-encodes to identical
  bytes wherever the transform is deterministic.
- Any drift fails the monorepo's Test workflow before release.

## API surface

| Group | Exports |
|---|---|
| Schema | `parseSchema`, `Compiler`, `link`, `buildManifest`, types |
| Packets | `AnansiCodec` (`create`/`encode`/`decode`/`encodeBatch`/…) | recommended facade |
| `encodeDocument`, `decodeDocument`, `encodeBatchRows`, `encodeBatchColumnar`, `decodeBatch` | functional form |
| Transforms (async) | `encodeAnansiPacket`, `decodeAnansiPacket`, `encodeAnansiBatchRows`, `encodeAnansiBatchColumnar`, `decodeAnansiBatch` |
| Validation | `DocumentValidator`, `SchemaValidator`, `metaSchemaPredicateMap` |

## Development

```sh
bun install
bun test          # unit + golden conformance suites
bun run build     # tsdown → dist (esm/cjs/dts)
bunx tsc --noEmit # typecheck
```

Golden fixtures come from the Go side:
`GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors`.

Releases are cut by semantic-release on `main` after the Test workflow passes;
the npm version always matches the Go module tag.

## License

AGPL-3.0-or-later. See [`LICENSE.md`](../../LICENSE.md) at the repository root.

Need different terms? A **commercial (private) license** is available from the
copyright holder for use cases where the AGPLv3's network-copyleft doesn't fit
(embedded products, SaaS without source disclosure, etc.). Contact:
[github.com/asaidimu](https://github.com/asaidimu).
