// Regressions for spec 2.4 varint decoding: the uint32 contract of
// uvarint(), the 10-byte cap, and Go's 10th-byte guard.

import { describe, it, expect } from "bun:test";
import { Reader, putUvarint, putUvarintBig, putVarint } from "../../src/wire/varint.ts";

function readerOf(bytes: number[]): Reader {
  return new Reader(new Uint8Array(bytes));
}

describe("uvarint uint32 contract", () => {
  it("round-trips the full uint32 range", () => {
    for (const v of [0, 1, 127, 128, 0x7fffffff, 0xffffffff]) {
      const buf: number[] = [];
      putUvarint(buf, v);
      expect(readerOf(buf).uvarint()).toBe(v);
    }
  });

  it("rejects values beyond uint32 instead of silently truncating", () => {
    const buf: number[] = [];
    putUvarintBig(buf, 0x1_0000_0000n); // 2^32 — wraps to 0 under >>> 0
    expect(() => readerOf(buf).uvarint()).toThrow("varint overflow");
  });

  it("decodes non-canonical but small encodings (Go parity)", () => {
    // continuation bytes with zero payload encode 0 in 5 bytes
    expect(readerOf([0x80, 0x80, 0x80, 0x80, 0x00]).uvarint()).toBe(0);
  });
});

describe("over-long varints (spec 2.4.3 / security 9.2.1)", () => {
  it("rejects 10 continuation bytes (Go TestReadUvarintTooLong)", () => {
    expect(() => readerOf(Array(10).fill(0xff)).uvarint()).toThrow(
      "varint overflow",
    );
    expect(() => readerOf(Array(10).fill(0xff)).uvarintBig()).toThrow(
      "varint overflow",
    );
  });

  it("rejects a 10th byte above 1", () => {
    const bytes = [...Array(9).fill(0xff), 0x02];
    expect(() => readerOf(bytes).uvarintBig()).toThrow("varint overflow");
    expect(() => readerOf(bytes).uvarint()).toThrow("varint overflow");
  });

  it("accepts a 10th byte of 0 or 1 (top bit only)", () => {
    const topBit = [...Array(9).fill(0xff), 0x01];
    expect(readerOf(topBit).uvarintBig()).toBe(0xffffffffffffffffn);
    const zero = [...Array(9).fill(0x80), 0x00];
    expect(readerOf(zero).uvarintBig()).toBe(0n);
  });

  it("errors on a dangling continuation byte (Go TestReadUvarintBufferUnderflow)", () => {
    expect(() => readerOf([0x80]).uvarint()).toThrow("end of buffer");
  });
});

describe("signed varint (zigzag)", () => {
  it("round-trips safe integers through the bigint path", () => {
    for (const v of [0, -1, 1, -64, 64, -(2 ** 40), 2 ** 40]) {
      const buf: number[] = [];
      putVarint(buf, v);
      expect(readerOf(buf).varint()).toBe(v);
    }
  });
});
