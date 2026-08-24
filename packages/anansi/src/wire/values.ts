// Per-DataType value codecs (spec 2.5) over the TS linked model.

import { putUvarint, putVarint, Reader, putUvarintBig } from "./varint.ts";

export const C = {
  Unknown: 0, Int: 1, Float: 2, String: 3, Bool: 4, Bytes: 5,
  Geometry: 6, Record: 7, ArrayUnknown: 8, ArrayInt: 9, ArrayFloat: 10,
  ArrayString: 11, ArrayBool: 12, ArrayBytes: 13, ArrayObject: 14,
  ArrayGeometry: 15,
} as const;

const enc = new TextEncoder();
const dec = new TextDecoder();

// Portable base64 (browser + Bun + Node). Chunked to avoid arg-limit issues
// on large payloads.
function bytesToBase64(bytes: Uint8Array): string {
  let bin = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(bin);
}
function base64ToBytes(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export type Buf = number[];

export function checkInt(v: number): void {
  if (!Number.isSafeInteger(v)) {
    throw new Error(`anansi: integer ${v} is not a safe JS integer`);
  }
}

// ── scalars ─────────────────────────────────────────────────────────────────
export function writeInt(b: Buf, v: number): void { putVarint(b, v); }
export function readInt(r: Reader): number { return r.varint(); }

export function writeFloat(b: Buf, v: number): void {
  const dv = new DataView(new ArrayBuffer(8));
  dv.setFloat64(0, v, true);
  for (let i = 0; i < 8; i++) b.push(dv.getUint8(i));
}
export function readFloat(r: Reader): number {
  const raw = r.take(8);
  const dv = new DataView(raw.buffer, raw.byteOffset, 8);
  return dv.getFloat64(0, true);
}

export function writeString(b: Buf, s: string): void {
  const bytes = enc.encode(s);
  putUvarint(b, bytes.length);
  for (const x of bytes) b.push(x);
}
export function readString(r: Reader): string {
  const n = r.uvarint();
  return dec.decode(r.take(n));
}

export function writeBoolSparse(b: Buf, v: boolean): void { b.push(v ? 1 : 0); }
export function readBoolSparse(r: Reader): boolean { return r.byte() !== 0; }

export function writeBoolBits(b: Buf, values: boolean[]): void {
  const packed = new Uint8Array((values.length + 7) >> 3);
  values.forEach((v, i) => { if (v) packed[i >> 3] |= 1 << (i & 7); });
  for (const x of packed) b.push(x);
}
export function readBoolBits(r: Reader, n: number): boolean[] {
  const packed = r.take((n + 7) >> 3);
  const out: boolean[] = [];
  for (let i = 0; i < n; i++) out.push((packed[i >> 3] & (1 << (i & 7))) !== 0);
  return out;
}

/** Bytes fields travel as base64 strings in the document model. */
export function writeBytesFromBase64(b: Buf, s: string): void {
  const raw = base64ToBytes(s);
  putUvarint(b, raw.length);
  for (const x of raw) b.push(x);
}
export function readBytesAsBase64(r: Reader): string {
  const n = r.uvarint();
  return bytesToBase64(r.take(n));
}

export function writeGeometry(b: Buf, g: number[][]): void {
  putUvarint(b, g.length);
  for (const ring of g) {
    putUvarint(b, ring.length / 2);
    for (let i = 0; i < ring.length; i += 2) {
      writeFloat(b, ring[i]!);
      writeFloat(b, ring[i + 1]!);
    }
  }
}
export function readGeometry(r: Reader): number[][] {
  const rings = r.uvarint();
  const out: number[][] = [];
  for (let i = 0; i < rings; i++) {
    const pts = r.uvarint();
    const ring: number[] = [];
    for (let p = 0; p < pts; p++) {
      ring.push(readFloat(r), readFloat(r));
    }
    out.push(ring);
  }
  return out;
}

// ── any / record (sorted-key maps + tagged unions) ──────────────────────────
export const TAG = {
  Null: 0, Int: C.Int, Float: C.Float, String: C.String, Bool: C.Bool,
  Bytes: C.Bytes, Geometry: C.Geometry, Record: C.Record,
  ArrUnknown: C.ArrayUnknown, ArrInt: C.ArrayInt, ArrFloat: C.ArrayFloat,
  ArrString: C.ArrayString, ArrBool: C.ArrayBool, ArrBytes: C.ArrayBytes,
} as const;

function isRecordArray(v: unknown[]): v is number[][][] {
  return v.length > 0 && Array.isArray(v[0]) && Array.isArray(v[0][0]);
}

export function writeAny(b: Buf, v: unknown): void {
  if (v === null || v === undefined) { b.push(TAG.Null); return; }
  switch (typeof v) {
    case "boolean": b.push(TAG.Bool); writeBoolSparse(b, v); return;
    case "string": b.push(TAG.String); writeString(b, v); return;
    case "number": {
      if (Number.isSafeInteger(v)) { b.push(TAG.Int); writeInt(b, v); }
      else { b.push(TAG.Float); writeFloat(b, v); }
      return;
    }
    case "object": {
      if (Array.isArray(v)) {
        if (isRecordArray(v)) { b.push(TAG.Geometry); writeGeometry(b, v as unknown as number[][]); return; }
        if (v.every((x) => typeof x === "string")) {
          // ambiguous with array<string>: prefer typed string array
          b.push(TAG.ArrString);
          putUvarint(b, v.length);
          for (const s of v as string[]) writeString(b, s);
          return;
        }
        if (v.every((x) => Number.isSafeInteger(x))) {
          b.push(TAG.ArrInt);
          putUvarint(b, v.length);
          for (const x of v as number[]) writeInt(b, x);
          return;
        }
        if (v.every((x) => typeof x === "number")) {
          b.push(TAG.ArrFloat);
          putUvarint(b, v.length);
          for (const x of v as number[]) writeFloat(b, x);
          return;
        }
        if (v.every((x) => typeof x === "boolean")) {
          b.push(TAG.ArrBool);
          const bools = v as boolean[];
          putUvarint(b, bools.length);
          writeBoolBits(b, bools);
          return;
        }
        b.push(TAG.ArrUnknown);
        putUvarint(b, v.length);
        for (const e of v) writeAny(b, e);
        return;
      }
      // plain object → record
      b.push(TAG.Record);
      writeRecordBody(b, v as Record<string, unknown>);
      return;
    }
    default:
      throw new Error(`anansi: unsupported any value ${typeof v}`);
  }
}

export function readAny(r: Reader): unknown {
  const tag = r.byte();
  switch (tag) {
    case TAG.Null: return null;
    case TAG.Bool: return readBoolSparse(r);
    case TAG.String: return readString(r);
    case TAG.Int: return readInt(r);
    case TAG.Float: return readFloat(r);
    case TAG.Bytes: return readBytesAsBase64(r);
    case TAG.Geometry: return readGeometry(r);
    case TAG.Record: return readRecordBody(r);
    case TAG.ArrInt: {
      const n = r.uvarint();
      return Array.from({ length: n }, () => readInt(r));
    }
    case TAG.ArrFloat: {
      const n = r.uvarint();
      return Array.from({ length: n }, () => readFloat(r));
    }
    case TAG.ArrString: {
      const n = r.uvarint();
      return Array.from({ length: n }, () => readString(r));
    }
    case TAG.ArrBool: {
      const n = r.uvarint();
      return readBoolBits(r, n);
    }
    case TAG.ArrUnknown: {
      const n = r.uvarint();
      return Array.from({ length: n }, () => readAny(r));
    }
    default:
      throw new Error(`anansi: unknown any tag ${tag}`);
  }
}

export function writeRecordBody(b: Buf, m: Record<string, unknown>): void {
  const keys = Object.keys(m).sort();
  putUvarint(b, keys.length);
  for (const k of keys) {
    writeString(b, k);
    writeAny(b, m[k]);
  }
}

export function readRecordBody(r: Reader): Record<string, unknown> {
  const n = r.uvarint();
  const out: Record<string, unknown> = {};
  for (let i = 0; i < n; i++) {
    const k = readString(r);
    out[k] = readAny(r);
  }
  return out;
}

// ── typed arrays ────────────────────────────────────────────────────────────
export function writeArrayInt(b: Buf, v: number[]): void {
  putUvarint(b, v.length);
  for (const x of v) writeInt(b, x);
}
export function readArrayInt(r: Reader): number[] {
  const n = r.uvarint();
  return Array.from({ length: n }, () => readInt(r));
}
export function writeArrayFloat(b: Buf, v: number[]): void {
  putUvarint(b, v.length);
  for (const x of v) writeFloat(b, x);
}
export function readArrayFloat(r: Reader): number[] {
  const n = r.uvarint();
  return Array.from({ length: n }, () => readFloat(r));
}
export function writeArrayString(b: Buf, v: string[]): void {
  putUvarint(b, v.length);
  for (const s of v) writeString(b, s);
}
export function readArrayString(r: Reader): string[] {
  const n = r.uvarint();
  return Array.from({ length: n }, () => readString(r));
}
export function writeArrayBool(b: Buf, v: boolean[]): void {
  putUvarint(b, v.length);
  writeBoolBits(b, v);
}
export function readArrayBool(r: Reader): boolean[] {
  const n = r.uvarint();
  return readBoolBits(r, n);
}
export function writeArrayBytesFromBase64(b: Buf, v: string[]): void {
  putUvarint(b, v.length);
  for (const s of v) writeBytesFromBase64(b, s);
}
export function readArrayBytesAsBase64(r: Reader): string[] {
  const n = r.uvarint();
  return Array.from({ length: n }, () => readBytesAsBase64(r));
}
export function writeArrayGeometry(b: Buf, v: number[][][]): void {
  putUvarint(b, v.length);
  for (const g of v) writeGeometry(b, g);
}
export function readArrayGeometry(r: Reader): number[][][] {
  const n = r.uvarint();
  return Array.from({ length: n }, () => readGeometry(r));
}
export function writeArrayUnknown(b: Buf, v: unknown[]): void {
  putUvarint(b, v.length);
  for (const e of v) writeAny(b, e);
}
export function readArrayUnknown(r: Reader): unknown[] {
  const n = r.uvarint();
  return Array.from({ length: n }, () => readAny(r));
}

export { putUvarintBig };
