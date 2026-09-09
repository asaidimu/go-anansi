// Transform frames (spec sections 4.1–4.3): compression, encryption,
// integrity hashing — full duplex, browser-compatible.
//
// Wire layout (identical to the Go implementation):
//
//	[flags: u8]                    bit2 = compressed, bit6 = encrypted,
//	                               bit7 = hash present
//	[schema_version: u8]
//	[digest: 16 bytes]             if bit7: BLAKE3(plaintext body)[0..16)
//	[nonce: 12 bytes]              if bit6
//	[payload...]                   AEAD(inner) if encrypted; otherwise
//	                               inner directly
//	inner = [plain_len: uvarint][zstd frame]   when compressed
//	inner = body                                otherwise
//
// Order of operations follows the spec: encode plaintext → compress →
// encrypt; decode decrypt → decompress → verify digest over plaintext.
//
// Backend matrix (browser + Bun + Node):
//   - BLAKE3-128:      hash-wasm (WASM)
//   - AES-256-GCM:     WebCrypto crypto.subtle (async everywhere)
//   - zstd decompress: fzstd (pure JS)
//   - zstd compress:   node:zlib when present (Bun/Node). Browsers have no
//                      stdlib zstd compressor — clients that cannot compress
//                      simply send plain packets; servers accept both.
//
// All transformed entry points are async because of WebCrypto/hash-wasm.

import { blake3 } from "hash-wasm";
import { decompress as fzstdDecompress } from "fzstd";
import { Reader, putUvarint } from "./varint.ts";
import {
  PACKET, fullVersionToHeader, headerFullVersion,
  encodeBatchColumnar,
  FLAG_COMPRESSED, FLAG_ENCRYPTED, FLAG_HASH_PRESENT,
} from "./packet.ts";
import type { EncodeKind } from "./packet.ts";

const packetTypeFromFlags = (flags: number): number => flags & 0x03;
import {
  encodeDenseInto, encodeSparseInto,
  decodeDenseInto, decodeSparseInto, decodeColumnarInto,
  encodeBatchColumnarBody,
  BATCH_FLAG_SPARSE, BATCH_FLAG_COLUMNAR,
} from "./packet.ts";
import type { ManifestField } from "../schema/link.ts";

export {
  FLAG_COMPRESSED, FLAG_ENCRYPTED, FLAG_HASH_PRESENT,
} from "./packet.ts";

const HASH_SIZE = 16;
const NONCE_SIZE = 12;
const MAX_DECOMPRESSED = 512 << 20; // spec 9.2.2 bomb guard

// ── backend resolution ──────────────────────────────────────────────────────

type ZlibLike = {
  zstdCompressSync?: (d: Uint8Array) => Uint8Array;
  zstdDecompressSync?: (d: Uint8Array, len?: number) => Uint8Array;
};

let zlibCache: ZlibLike | undefined;
let zlibResolved = false;
function nodeZlib(): ZlibLike | undefined {
  if (!zlibResolved) {
    zlibResolved = true;
    try {
      // Dynamic require keeps bundlers from hard-linking node:zlib in
      // browser builds.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      zlibCache = (globalThis as any).process?.getBuiltinModule?.("node:zlib")
        ?? undefined;
    } catch {
      zlibCache = undefined;
    }
  }
  return zlibCache;
}

function zstdCompress(body: Uint8Array): Uint8Array {
  const z = nodeZlib()?.zstdCompressSync?.(body);
  if (!z) {
    throw new Error(
      "anansi: zstd compression is unavailable in this runtime (no node:zlib). " +
      "Send plain packets, or provide a WASM compressor.",
    );
  }
  return z;
}

function zstdDecompress(src: Uint8Array, declaredLen: number): Uint8Array {
  const native = nodeZlib()?.zstdDecompressSync?.(src);
  if (native) return native;
  void declaredLen; // fzstd self-terminates; caller validates the length.
  return fzstdDecompress(src);
}

async function blake3Digest16(payload: Uint8Array): Promise<Uint8Array> {
  const hex = await blake3(payload, 128); // hex string, 32 chars
  const out = new Uint8Array(HASH_SIZE);
  for (let i = 0; i < HASH_SIZE; i++) {
    out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

async function aesGCM(key: Uint8Array): Promise<CryptoKey> {
  if (key.length !== 32) {
    throw new Error(`anansi: AES-256-GCM requires a 32-byte key, got ${key.length}`);
  }
  return crypto.subtle.importKey("raw", key as BufferSource, "AES-GCM", false, [
    "encrypt",
    "decrypt",
  ]);
}

// ── options ─────────────────────────────────────────────────────────────────

export interface EncodeTransforms {
  /** Compress the packet body (flags bit 2). */
  compression?: boolean;
  /** Embed BLAKE3-truncated digest over the plaintext body (bit 7). */
  integrity?: boolean;
  /** Seal with AES-256-GCM under this 32-byte key (bit 6). */
  encryptionKey?: Uint8Array;
}

export interface DecodeTransforms {
  decryptionKey?: Uint8Array;
}

// ── frame assembly / opening ────────────────────────────────────────────────

/**
 * Builds the final top-level packet from a plaintext body per the transform
 * options. `baseFlags` must already carry epoch + packet-type bits.
 */
export async function finishFrame(
  baseFlags: number,
  version: number,
  body: Uint8Array,
  cfg: EncodeTransforms,
): Promise<Uint8Array> {
  let flags = baseFlags;
  if (cfg.integrity) flags |= FLAG_HASH_PRESENT;
  if (cfg.compression) flags |= FLAG_COMPRESSED;
  if (cfg.encryptionKey) flags |= FLAG_ENCRYPTED;

  const head: number[] = [flags, version];
  if (cfg.integrity) {
    for (const b of await blake3Digest16(body)) head.push(b);
  }

  const payload: number[] = [];
  if (cfg.compression) {
    putUvarint(payload, body.length); // plain_len precedes the frame
    for (const b of zstdCompress(body)) payload.push(b);
  } else {
    for (const b of body) payload.push(b);
  }

  if (!cfg.encryptionKey) return new Uint8Array([...head, ...payload]);

  const cryptoKey = await aesGCM(cfg.encryptionKey);
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_SIZE));
  const sealed = new Uint8Array(
    await crypto.subtle.encrypt(
      { name: "AES-GCM", iv: nonce },
      cryptoKey,
      new Uint8Array(payload),
    ),
  );
  const out = new Uint8Array(head.length + NONCE_SIZE + sealed.length);
  out.set(Uint8Array.from(head), 0);
  out.set(nonce, head.length);
  out.set(sealed, head.length + NONCE_SIZE);
  return out;
}

/** Consumes digest/nonce/compression envelope; returns the plaintext body. */
export async function openFrame(
  r: Reader,
  flags: number,
  cfg: DecodeTransforms,
): Promise<Reader> {
  let stored: Uint8Array | undefined;
  if (flags & FLAG_HASH_PRESENT) {
    stored = r.take(HASH_SIZE);
  }

  let data = r.data.subarray(r.pos);

  if (flags & FLAG_ENCRYPTED) {
    if (!cfg.decryptionKey) {
      throw new Error("anansi: encrypted packet but no decryption key provided");
    }
    if (data.length < NONCE_SIZE) throw new Error("anansi: truncated nonce");
    const nonce = data.subarray(0, NONCE_SIZE);
    data = data.subarray(NONCE_SIZE);
    const cryptoKey = await aesGCM(cfg.decryptionKey);
    const plain = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: nonce as BufferSource },
      cryptoKey,
      data as BufferSource,
    );
    data = new Uint8Array(plain);
  }

  let plainLen = data.length;
  const rr = new Reader(data);
  if (flags & FLAG_COMPRESSED) {
    plainLen = rr.uvarint();
    if (plainLen > MAX_DECOMPRESSED) {
      throw new Error(`anansi: declared uncompressed size ${plainLen} exceeds limit`);
    }
    const inflated = zstdDecompress(data.subarray(rr.pos), plainLen);
    if (inflated.length !== plainLen) {
      throw new Error(`anansi: decompressed ${inflated.length} bytes, header declared ${plainLen}`);
    }
    data = inflated;
  }

  if (stored) {
    const want = await blake3Digest16(data);
    for (let i = 0; i < HASH_SIZE; i++) {
      if (stored[i] !== want[i]) {
        throw new Error("anansi: integrity check failed (packet digest mismatch)");
      }
    }
  }
  return new Reader(data);
}

export { packetTypeFromFlags };


// ── high-level duplex API ───────────────────────────────────────────────────

function selectDensity(fields: ManifestField[], doc: Record<string, unknown>): "dense" | "sparse" {
  let present = 0;
  for (const f of fields) {
    const [ok, v] = lookupExported(doc, f.path);
    if (ok && v !== null && v !== undefined) present++;
  }
  return fields.length <= 64 || fields.length === 0 || present / fields.length > 0.25
    ? "dense"
    : "sparse";
}

// lookup is module-private in packet.ts; tiny local copy.
function lookupExported(doc: Record<string, unknown>, path: string): [boolean, unknown] {
  let cur: unknown = doc;
  for (const seg of path.split(".")) {
    if (cur === null || typeof cur !== "object") return [false, undefined];
    const o = cur as Record<string, unknown>;
    if (!(seg in o)) return [false, undefined];
    cur = o[seg];
  }
  return [cur !== undefined, cur];
}

function documentBody(
  fields: ManifestField[],
  doc: Record<string, unknown>,
  kind: EncodeKind,
): Uint8Array {
  const useSparse =
    kind === "sparse" ||
    (kind === "auto" && selectDensity(fields, doc) === "sparse");
  const buf: number[] = [];
  if (useSparse) encodeSparseInto(buf, fields, doc);
  else encodeDenseInto(buf, fields, doc);
  return new Uint8Array(buf);
}

/** Full-duplex single-document encode with transforms. */
export async function encodeAnansiPacket(
  fields: ManifestField[],
  doc: Record<string, unknown>,
  fullVersion = 0,
  opts: EncodeTransforms & { kind?: EncodeKind } = {},
): Promise<Uint8Array> {
  const hv = fullVersionToHeader(fullVersion);
  const kind = opts.kind ?? "auto";
  const useSparse =
    kind === "sparse" || (kind === "auto" && selectDensity(fields, doc) === "sparse");
  const baseFlags = hv.flags | (useSparse ? PACKET.Sparse : PACKET.Dense);
  const body = documentBody(fields, doc, useSparse ? "sparse" : "dense");
  return finishFrame(baseFlags, hv.version, body, opts);
}

/** Full-duplex single-document decode with transforms. */
export async function decodeAnansiPacket(
  data: Uint8Array,
  fields: ManifestField[],
  opts: DecodeTransforms = {},
): Promise<{ version: number; doc: Record<string, unknown> }> {
  const r = new Reader(data);
  const flags = r.byte();
  const version = r.byte();
  if ((flags & FLAG_ENCRYPTED) && !opts.decryptionKey) {
    throw new Error("anansi: encrypted packet but no decryption key provided");
  }
  const body = await openFrame(r, flags, opts);
  const out: Record<string, unknown> = {};
  const pt = packetTypeFromFlags(flags);
  if (pt === PACKET.Dense) decodeDenseInto(body, fields, out);
  else if (pt === PACKET.Sparse) decodeSparseInto(body, fields, out);
  else throw new Error(`anansi: use batch API for packet type ${pt}`);
  return { version: headerFullVersion(flags, version), doc: out };
}

function batchRowsBody(
  fields: ManifestField[],
  docs: Record<string, unknown>[],
): { body: Uint8Array; sparse: boolean } {
  let present = 0, possible = 0;
  for (const d of docs) {
    for (const f of fields) {
      const [ok, v] = lookupExported(d, f.path);
      if (ok && v !== null && v !== undefined) present++;
      possible++;
    }
  }
  const density = possible > 0 ? present / possible : 1;
  const sparse = fields.length > 64 && density <= 0.25;

  const buf: number[] = [];
  putUvarint(buf, docs.length);
  buf.push(sparse ? BATCH_FLAG_SPARSE : 0);
  for (const d of docs) {
    if (sparse) encodeSparseInto(buf, fields, d);
    else encodeDenseInto(buf, fields, d);
  }
  return { body: new Uint8Array(buf), sparse };
}

/** Batch row-oriented encode with transforms. */
export async function encodeAnansiBatchRows(
  fields: ManifestField[],
  docs: Record<string, unknown>[],
  fullVersion = 0,
  opts: EncodeTransforms = {},
): Promise<Uint8Array> {
  const hv = fullVersionToHeader(fullVersion);
  const { body, sparse } = batchRowsBody(fields, docs);
  const baseFlags = hv.flags | PACKET.Batch | (sparse ? BATCH_FLAG_SPARSE : 0);
  return finishFrame(baseFlags, hv.version, body, opts);
}

/**
 * Columnar batch encode. Transforms are supported only on the row paths —
 * columnar ENCODE parity is still pending in TS; pass empty options to emit
 * a plain packet (decoding transformed columnar packets IS supported).
 */
/** Columnar batch encode with transforms — full duplex. */
export async function encodeAnansiBatchColumnar(
  fields: ManifestField[],
  docs: Record<string, unknown>[],
  fullVersion = 0,
  opts: EncodeTransforms = {},
): Promise<Uint8Array> {
  const hv = fullVersionToHeader(fullVersion);
  const baseFlags = hv.flags | PACKET.Batch | 0x08;
    const body = new Uint8Array(
    encodeBatchColumnarBody(fields, docs, { flags: baseFlags, version: hv.version }),
  );
  return finishFrame(baseFlags, hv.version, body, opts);
}

/** Full-duplex batch decode (row dense/sparse and columnar) with transforms. */
export async function decodeAnansiBatch(
  data: Uint8Array,
  fields: ManifestField[],
  opts: DecodeTransforms = {},
): Promise<{ version: number; docs: Record<string, unknown>[] }> {
  const r = new Reader(data);
  const flags = r.byte();
  const version = r.byte();
  if (packetTypeFromFlags(flags) !== PACKET.Batch) {
    throw new Error("anansi: not a batch packet");
  }
  const body = await openFrame(r, flags, opts);
  const count = body.uvarint();
  const batchFlags = body.byte();

  const out: Record<string, unknown>[] = Array.from({ length: count }, () => ({}));
  if (batchFlags & BATCH_FLAG_COLUMNAR || flags & 0x08) {
    decodeColumnarInto(body, fields, out);
  } else {
    for (const d of out) {
      if (batchFlags & BATCH_FLAG_SPARSE) decodeSparseInto(body, fields, d);
      else decodeDenseInto(body, fields, d);
    }
  }
  return { version: headerFullVersion(flags, version), docs: out };
}
