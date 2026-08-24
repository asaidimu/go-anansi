// Packet engine: dense/sparse/batch encode+decode over a linked manifest
// (spec sections 3.1–3.3). Full duplex — every direction, every kind.

import { Reader, putUvarint, putVarint } from "./varint.ts";
import * as V from "./values.ts";
import { C } from "./values.ts";
import type { Buf } from "./values.ts";
import type { ManifestField } from "../schema/link.ts";

export const PACKET = { Dense: 0, Sparse: 1, Batch: 2, Stream: 3 } as const;
export const FLAG_COMPRESSED = 0x04;
export const FLAG_ENCRYPTED = 0x40;
export const FLAG_HASH_PRESENT = 0x80;
export const BATCH_FLAG_COLUMNAR = 0x01;
export const BATCH_FLAG_SPARSE = 0x02;

// ── state resolution ────────────────────────────────────────────────────────

type State = "notSet" | "null" | "value";

/** Resolve a dotted path against a document. Returns [found, value]. */
function lookup(doc: Record<string, unknown>, path: string): [boolean, unknown] {
  const segs = path.split(".");
  let cur: unknown = doc;
  for (let i = 0; i < segs.length; i++) {
    if (cur === null || typeof cur !== "object") return [false, undefined];
    const obj = cur as Record<string, unknown>;
    if (!(segs[i] in obj)) return [false, undefined];
    cur = obj[segs[i]!];
  }
  if (cur === undefined) return [false, undefined];
  return [true, cur];
}

function stateOf(doc: Record<string, unknown>, f: ManifestField): State {
  // Spec 2.7: THREE distinct states. A property present with `null` is an
  // explicit Null (dense code 01 / sparse null-bit) — distinct from Not Set
  // (property missing or undefined). Only undefined/missing collapses.
  const [ok, v] = lookup(doc, f.path);
  if (!ok || v === undefined) return "notSet";
  if (v === null) return "null";
  return "value";
}

function elementKey(child: ManifestField, mount: string): ManifestField {
  const strip = mount.length + 1;
  return { ...child, path: child.path.slice(strip) };
}

function elementFields(child: ManifestField[], mount: string): ManifestField[] {
  return child.map((c) => elementKey(c, mount));
}

// ── value dispatch ──────────────────────────────────────────────────────────

function writeValue(b: Buf, f: ManifestField, v: unknown): void {
  switch (f.t) {
    case "int": V.writeInt(b, v as number); break;
    case "float": V.writeFloat(b, v as number); break;
    case "string": V.writeString(b, v as string); break;
    case "bool": V.writeBoolSparse(b, v as boolean); break;
    case "bytes": V.writeBytesFromBase64(b, v as string); break;
    case "geometry": V.writeGeometry(b, v as number[][]); break;
    case "record": V.writeRecordBody(b, v as Record<string, unknown>); break;
    case "unknown": V.writeAny(b, v); break;
    case "array_int": V.writeArrayInt(b, v as number[]); break;
    case "array_float": V.writeArrayFloat(b, v as number[]); break;
    case "array_string": V.writeArrayString(b, v as string[]); break;
    case "array_bool": V.writeArrayBool(b, v as boolean[]); break;
    case "array_bytes": V.writeArrayBytesFromBase64(b, v as string[]); break;
    case "array_geometry": V.writeArrayGeometry(b, v as number[][][]); break;
    case "array_unknown": V.writeArrayUnknown(b, v as unknown[]); break;
    default:
      throw new Error(`anansi: cannot write value for type ${f.t} field ${f.path}`);
  }
}

function readValue(r: Reader, f: ManifestField): unknown {
  switch (f.t) {
    case "int": return r.varint();
    case "float": return V.readFloat(r);
    case "string": return V.readString(r);
    case "bool": return V.readBoolSparse(r);
    case "bytes": return V.readBytesAsBase64(r);
    case "geometry": return V.readGeometry(r);
    case "record": return V.readRecordBody(r);
    case "unknown": return V.readAny(r);
    case "array_int": return V.readArrayInt(r);
    case "array_float": return V.readArrayFloat(r);
    case "array_string": return V.readArrayString(r);
    case "array_bool": return V.readArrayBool(r);
    case "array_bytes": return V.readArrayBytesAsBase64(r);
    case "array_geometry": return V.readArrayGeometry(r);
    case "array_unknown": return V.readArrayUnknown(r);
    default:
      throw new Error(`anansi: cannot read value for type ${f.t} field ${f.path}`);
  }
}

// ── nested array-object sub-packets ─────────────────────────────────────────

function writeNestedPacket(
  b: Buf,
  childFull: ManifestField[],
  mount: string,
  elements: Record<string, unknown>[],
  ctx: FrameCtx,
): void {
  // Elements are self-contained documents keyed by MOUNT-RELATIVE names.
  const child = elementFields(childFull, mount);
  putUvarint(b, elements.length);
  for (const el of elements) {
    // Auto-select density for this element, emit its own 2-byte header.
    let present = 0;
    for (const f of child) if (stateOf(el, f) !== "notSet") present++;
    const density = child.length > 0 ? present / child.length : 1;
    const useSparse = !(child.length <= 64 || density > 0.25);
    // Element headers inherit the parent's non-transform, non-type bits —
    // notably the columnar bit inside columnar batches — mirroring Go's
    // encodePacket mask (which strips type/transforms but keeps bit 3).
    const pt = useSparse ? PACKET.Sparse : PACKET.Dense;
    const flags =
      ((ctx.flags & ~(0x03 | FLAG_COMPRESSED | FLAG_ENCRYPTED | FLAG_HASH_PRESENT)) |
        pt) >>> 0;

    const body: Buf = [];
    const childCtx: FrameCtx = { flags: (ctx.flags & ~0x03) | pt, version: ctx.version };
    if (useSparse) encodeSparseInto(body, child, el, childCtx);
    else encodeDenseInto(body, child, el, childCtx);

    const frame = [flags, ctx.version, ...body];
    putUvarint(b, frame.length);
    for (const x of frame) b.push(x);
  }
}

/** Encode one element/body against fields; header written by caller. */
function encodeBody(
  fields: ManifestField[],
  doc: Record<string, unknown>,
  _ev: [number, number],
): number[] {
  // Auto-select per spec 6.1 (recursive schemas don't exist at this level).
  let present = 0;
  for (const f of fields) {
    if (stateOf(doc, f) !== "notSet") present++;
  }
  const density = fields.length > 0 ? present / fields.length : 1;
  const dense = fields.length <= 64 || density > 0.25;

  const buf: Buf = [];
  if (dense) encodeDenseInto(buf, fields, doc);
  else encodeSparseInto(buf, fields, doc);
  return buf;
}

function readNestedPacket(
  r: Reader,
  child: ManifestField[],
  mount: string,
  out: unknown[],
  poolDoc?: () => Record<string, unknown>,
): void {
  const n = r.uvarint();
  for (let i = 0; i < n; i++) {
    const len = r.uvarint();
    const payload = r.take(len);
    const pr = new Reader(payload);
    const elFlags = pr.byte();
    void pr.byte(); // element version byte
    const el = poolDoc ? poolDoc() : {};
    const pt = elFlags & 0x03;
    if (pt === PACKET.Dense) decodeDenseInto(pr, child, el, mount);
    else if (pt === PACKET.Sparse) decodeSparseInto(pr, child, el, mount);
    else throw new Error(`anansi: unsupported nested packet type ${pt}`);
    out.push(el);
  }
}

// ── DENSE ───────────────────────────────────────────────────────────────────

/** Parent-header context threaded to nested sub-packet writers. */
export interface FrameCtx {
  flags: number;
  version: number;
}

const DEFAULT_CTX: FrameCtx = { flags: PACKET.Dense, version: 0 };

export function encodeDenseInto(buf: Buf, fields: ManifestField[], doc: Record<string, unknown>, ctx: FrameCtx = DEFAULT_CTX): void {
  // State map.
  const nBits = 2 * fields.length;
  const packed = new Uint8Array((nBits + 7) >> 3);
  fields.forEach((f, i) => {
    const code = stateOf(doc, f) === "notSet" ? 0 : stateOf(doc, f) === "null" ? 1 : 2;
    packed[i >> 2] |= code << ((i & 3) << 1);
  });
  for (const x of packed) buf.push(x);

  // Value blocks in DataType order; TypeBool bit-packed (spec 2.5).
  const byDt = groupByType(fields);
  for (let dt = 0; dt <= 15; dt++) {
    const fs = byDt[dt];
    if (!fs || fs.length === 0) continue;
    if (dt === C.Bool) {
      const values: boolean[] = [];
      for (const f of fs) {
        const st = stateOf(doc, f);
        if (st !== "value") continue;
        values.push(lookup(doc, f.path)[1] as boolean);
        void st;
      }
      V.writeBoolBits(buf, values);
      continue;
    }
    for (const f of fs) {
      if (stateOf(doc, f) !== "value") continue;
      if (f.t === "array_object") {
        writeNestedPacket(buf, f.child!, f.name, (lookup(doc, f.path)[1] ?? []) as Record<string, unknown>[], ctx);
        continue;
      }
      writeValue(buf, f, lookup(doc, f.path)[1]);
    }
  }
}

export function decodeDenseInto(
  r: Reader,
  fields: ManifestField[],
  out: Record<string, unknown>,
  mount?: string,
): void {
  const states: number[] = [];
  const nBits = 2 * fields.length;
  const packed = r.take((nBits + 7) >> 3);
  for (let i = 0; i < fields.length; i++) {
    states.push((packed[i >> 2] >> ((i & 3) << 1)) & 0x03);
  }

  const byDt = groupByType(fields);
  for (let dt = 0; dt <= 15; dt++) {
    const fs = byDt[dt];
    if (!fs) continue;
    if (dt === C.Bool) {
      const count = fs.filter((_, i) => states[fields.indexOf(fs[i]!)] === 2).length;
      const values = V.readBoolBits(r, count);
      let next = 0;
      fs.forEach((f) => {
        const gi = fields.indexOf(f);
        if (states[gi] !== 2) return;
        assign(out, f.path, mount, values[next++]);
      });
      continue;
    }
    for (const f of fs) {
      const gi = fields.indexOf(f);
      const st = states[gi];
      if (st === 0) continue;
      if (st === 1) { assign(out, f.path, mount, null); continue; }
      if (f.t === "array_object") {
        const els: Record<string, unknown>[] = [];
        readNestedPacket(r, f.child!, f.name, els);
        assign(out, f.path, mount, els);
        continue;
      }
      assign(out, f.path, mount, readValue(r, f));
    }
  }
}

// ── SPARSE ──────────────────────────────────────────────────────────────────

export function encodeSparseInto(buf: Buf, fields: ManifestField[], doc: Record<string, unknown>, ctx: FrameCtx = { flags: PACKET.Sparse, version: 0 }): void {
  const set = fields.filter((f) => stateOf(doc, f) !== "notSet");
  putUvarint(buf, set.length);
  for (const f of set) {
    const st = stateOf(doc, f);
    let dp = f.dp;
    if (st === "null") dp |= 1;
    else dp &= 0xfffffffe; // canonical (non-null) DataPoint
    putUvarint(buf, dp);
    if (st === "value") {
      if (f.t === "array_object") {
        writeNestedPacket(buf, f.child!, f.name, (lookup(doc, f.path)[1] ?? []) as Record<string, unknown>[], ctx);
      } else {
        writeValue(buf, f, lookup(doc, f.path)[1]);
      }
    }
  }
}

export function decodeSparseInto(
  r: Reader,
  fields: ManifestField[],
  out: Record<string, unknown>,
  mount?: string,
): void {
  const byDP = new Map<number, ManifestField>();
  for (const f of fields) byDP.set(f.dp & 0xfffffffe, f);

  const n = r.uvarint();
  for (let i = 0; i < n; i++) {
    const wireDP = r.uvarint();
    const isNull = (wireDP & 1) !== 0;
    const f = byDP.get(wireDP & 0xfffffffe);
    if (!f) throw new Error(`anansi: sparse packet references unknown data point ${wireDP}`);
    if (isNull) { assign(out, f.path, mount, null); continue; }
    if (f.t === "array_object") {
      const els: Record<string, unknown>[] = [];
      readNestedPacket(r, f.child!, f.name, els);
      assign(out, f.path, mount, els);
      continue;
    }
    assign(out, f.path, mount, readValue(r, f));
  }
}

// ── assignment into nested output ───────────────────────────────────────────

function assign(
  out: Record<string, unknown>,
  fullPath: string,
  mount: string | undefined,
  value: unknown,
): void {
  let path = fullPath;
  if (mount && path.startsWith(mount + ".")) {
    path = path.slice(mount.length + 1);
    // Element objects key by relative leaf names only (single segment or
    // deeper flattened paths inside the element schema).
    setPath(out, path.split("."), value);
    return;
  }
  setPath(out, path.split("."), value);
}

function setPath(obj: Record<string, unknown>, segs: string[], v: unknown): void {
  let cur = obj;
  for (let i = 0; i < segs.length - 1; i++) {
    const s = segs[i]!;
    if (typeof cur[s] !== "object" || cur[s] === null) cur[s] = {};
    cur = cur[s] as Record<string, unknown>;
  }
  cur[segs[segs.length - 1]!] = v;
}

function groupByType(fields: ManifestField[]): (ManifestField[] | undefined)[] {
  const g: (ManifestField[] | undefined)[] = [];
  for (const f of fields) {
    const dt = TYPE_TO_DT[f.t] ?? C.Unknown;
    (g[dt] ??= []).push(f);
  }
  return g;
}

const TYPE_TO_DT: Record<string, number> = Object.fromEntries(
  Object.entries({
    unknown: C.Unknown, int: C.Int, float: C.Float, string: C.String,
    bool: C.Bool, bytes: C.Bytes, geometry: C.Geometry, record: C.Record,
    array_unknown: C.ArrayUnknown, array_int: C.ArrayInt,
    array_float: C.ArrayFloat, array_string: C.ArrayString,
    array_bool: C.ArrayBool, array_bytes: C.ArrayBytes,
    array_object: C.ArrayObject, array_geometry: C.ArrayGeometry,
  }),
);

// ── top-level packets ───────────────────────────────────────────────────────

export interface PacketHeader {
  flags: number;
  version: number;
}
export function fullVersionToHeader(v: number): PacketHeader {
  if (v < 0 || v > 1023) throw new Error(`anansi: schema version ${v} exceeds maximum of 1023`);
  return { flags: (v >> 8) << 4, version: v & 0xff };
}
export function headerFullVersion(flags: number, version: number): number {
  return (((flags >> 4) & 0x03) << 8) | version;
}

export type EncodeKind = "auto" | "dense" | "sparse";

export function encodeDocument(
  fields: ManifestField[],
  doc: Record<string, unknown>,
  fullVersion = 0,
  kind: EncodeKind = "auto",
): Uint8Array {
  const hv = fullVersionToHeader(fullVersion);
  let present = 0;
  for (const f of fields) if (stateOf(doc, f) !== "notSet") present++;
  const density = fields.length ? present / fields.length : 1;
  const useSparse =
    kind === "sparse" || (kind === "auto" && !(fields.length <= 64 || density > 0.25));

  const flags = hv.flags | (useSparse ? PACKET.Sparse : PACKET.Dense);
  const head = [flags, hv.version];
  const ctx: FrameCtx = { flags, version: hv.version };
  const body: Buf = [];
  if (useSparse) encodeSparseInto(body, fields, doc, ctx);
  else encodeDenseInto(body, fields, doc, ctx);
  return new Uint8Array([...head, ...body]);
}

export function decodeDocument(
  data: Uint8Array,
  fields: ManifestField[],
): { version: number; doc: Record<string, unknown> } {
  const r = new Reader(data);
  const flags = r.byte();
  const version = r.byte();
  if (((flags >> 4) & 3) === PACKET.Stream) throw new Error("anansi: stream packets unsupported");
  if (flags & (0x04 | 0x40 | 0x80)) {
    throw new Error("anansi: compressed/encrypted/hashed packets require transform support not enabled in this build");
  }
  const pt = flags & 0x03;
  const out: Record<string, unknown> = {};
  if (pt === PACKET.Dense) decodeDenseInto(r, fields, out);
  else if (pt === PACKET.Sparse) decodeSparseInto(r, fields, out);
  else throw new Error(`anansi: use batch API for packet type ${pt}`);
  return { version: headerFullVersion(flags, version), doc: out };
}

// ── BATCH ───────────────────────────────────────────────────────────────────

export function encodeBatchRows(
  fields: ManifestField[],
  docs: Record<string, unknown>[],
  fullVersion = 0,
): Uint8Array {
  const hv = fullVersionToHeader(fullVersion);
  let present = 0, possible = 0;
  for (const d of docs) {
    for (const f of fields) if (stateOf(d, f) !== "notSet") present++;
    possible += fields.length;
  }
  const density = possible ? present / possible : 1;
  const useSparse = fields.length > 64 && density <= 0.25;

  const head = [hv.flags | PACKET.Batch, hv.version];
  const buf: Buf = [];
  putUvarint(buf, docs.length);
  buf.push(useSparse ? BATCH_FLAG_SPARSE : 0);
  const rowCtx: FrameCtx = { flags: hv.flags | PACKET.Batch, version: hv.version };
  for (const d of docs) {
    if (useSparse) encodeSparseInto(buf, fields, d, rowCtx);
    else encodeDenseInto(buf, fields, d, rowCtx);
  }
  return new Uint8Array([...head, ...buf]);
}

export function encodeBatchColumnar(
  fields: ManifestField[],
  docs: Record<string, unknown>[],
  fullVersion = 0,
): Uint8Array {
  const hv = fullVersionToHeader(fullVersion);
  const head = [hv.flags | PACKET.Batch | 0x08, hv.version];
  const body = encodeBatchColumnarBody(fields, docs, { flags: head[0], version: hv.version });
  return new Uint8Array([...head, ...body]);
}

/** Columnar body WITHOUT the 2-byte packet header (for transform framing). */
export function encodeBatchColumnarBody(
  fields: ManifestField[],
  docs: Record<string, unknown>[],
  ctx: FrameCtx,
): Buf {
  const buf: Buf = [];
  putUvarint(buf, docs.length);
  buf.push(BATCH_FLAG_COLUMNAR);

  const byDt = groupByType(fields);
  for (let dt = 0; dt <= 15; dt++) {
    const fs = byDt[dt];
    if (!fs || fs.length === 0) continue;

    // State column, field-major.
    const nBits = 2 * fs.length * docs.length;
    const col = new Uint8Array((nBits + 7) >> 3);
    let bit = 0;
    for (const f of fs) {
      for (const d of docs) {
        const st = stateOf(d, f);
        const code = st === "notSet" ? 0 : st === "null" ? 1 : 2;
        col[bit >> 3] |= code << (bit & 7);
        bit += 2;
      }
    }
    for (const x of col) buf.push(x);

    if (dt === C.Bool) {
      for (const f of fs) {
        const vals: boolean[] = [];
        for (const d of docs) {
          if (stateOf(d, f) !== "value") continue;
          vals.push(lookup(d, f.path)[1] as boolean);
        }
        V.writeBoolBits(buf, vals);
      }
      continue;
    }
    for (const f of fs) {
      for (const d of docs) {
        if (stateOf(d, f) !== "value") continue;
        if (f.t === "array_object") {
          writeNestedPacket(buf, f.child!, f.name, (lookup(d, f.path)[1] ?? []) as Record<string, unknown>[], ctx);
          continue;
        }
        writeValue(buf, f, lookup(d, f.path)[1]);
      }
    }
  }
  return buf;
}

export function decodeBatch(
  data: Uint8Array,
  fields: ManifestField[],
): { version: number; docs: Record<string, unknown>[] } {
  const r = new Reader(data);
  const flags = r.byte();
  const version = r.byte();
  if ((flags & 0x03) !== PACKET.Batch) throw new Error("anansi: not a batch packet");
  if (flags & (0x04 | 0x40 | 0x80)) {
    throw new Error("anansi: transformed batch packets unsupported in this build");
  }
  const count = r.uvarint();
  const batchFlags = r.byte();

  const docs: Record<string, unknown>[] = Array.from({ length: count }, () => ({}));

  if (batchFlags & BATCH_FLAG_COLUMNAR || flags & 0x08) {
    decodeColumnarInto(r, fields, docs);
  } else {
    const sparse = (batchFlags & BATCH_FLAG_SPARSE) !== 0;
    for (const d of docs) {
      if (sparse) decodeSparseInto(r, fields, d);
      else decodeDenseInto(r, fields, d);
    }
  }
  return { version: headerFullVersion(flags, version), docs };
}

export function decodeColumnarInto(
  r: Reader,
  fields: ManifestField[],
  docs: Record<string, unknown>[],
): void {
  const byDt = groupByType(fields);
  for (let dt = 0; dt <= 15; dt++) {
    const fs = byDt[dt];
    if (!fs || fs.length === 0) continue;

    const nBits = 2 * fs.length * docs.length;
    const col = r.take((nBits + 7) >> 3);
    const states: number[][] = [];
    let b = 0;
    for (let fi = 0; fi < fs.length; fi++) {
      const row: number[] = [];
      for (let ri = 0; ri < docs.length; ri++) {
        row.push((col[b >> 3] >> (b & 7)) & 0x03);
        b += 2;
      }
      states.push(row);
    }

    if (dt === C.Bool) {
      fs.forEach((f, fi) => {
        const nVals = states[fi]!.filter((s) => s === 2).length;
        const values = V.readBoolBits(r, nVals);
        let next = 0;
        states[fi]!.forEach((st, ri) => {
          if (st === 0) return;
          if (st === 1) { assign(docs[ri]!, f.path, undefined, null); return; }
          assign(docs[ri]!, f.path, undefined, values[next++]);
        });
      });
      continue;
    }

    fs.forEach((f, fi) => {
      states[fi]!.forEach((st, ri) => {
        if (st === 0) return;
        if (st === 1) { assign(docs[ri]!, f.path, undefined, null); return; }
        if (f.t === "array_object") {
          const els: Record<string, unknown>[] = [];
          readNestedPacket(r, f.child!, f.name, els);
          assign(docs[ri]!, f.path, undefined, els);
          return;
        }
        assign(docs[ri]!, f.path, undefined, readValue(r, f));
      });
    });
  }
}
