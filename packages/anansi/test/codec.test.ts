// The friendly facade: one codec per schema, cached by the consumer.

import { describe, it, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { AnansiCodec } from "../src/codec.ts";
import { link, buildManifest } from "../src/schema/link.ts";
import { makeStripNulls, hexToBytes } from "./helpers.ts";

const GOLDEN = JSON.parse(
  readFileSync(join(import.meta.dir, "..", "testdata", "golden.json"), "utf8"),
) as {
  schema: Record<string, unknown>;
  cases: { name: string; kind: string; packet: string; payload: unknown }[];
};

const linked = link(GOLDEN.schema as never);
const fields = buildManifest(linked);
const stripNulls = makeStripNulls(fields);

describe("AnansiCodec facade", () => {
  const schema = GOLDEN.schema;
  const full = stripNulls(
    GOLDEN.cases.find((c) => c.name === "dense_full")!.payload,
  ) as Record<string, unknown>;

  it("create → encode → decode round-trip", async () => {
    const codec = await AnansiCodec.create(schema);
    const wire = await codec.encode(full);
    const back = await codec.decode(wire);
    expect(back.version).toBe(0);
    expect(back.doc).toEqual(full);
  });

  it("decodes Go-produced plain packets", async () => {
    const codec = await AnansiCodec.create(schema);
    const c = GOLDEN.cases.find((x) => x.name === "sparse_full")!;
    const { doc } = await codec.decode(hexToBytes(c.packet));
    expect(doc).toEqual(stripNulls(c.payload) as Record<string, unknown>);
  });

  it("honours fullVersion and kind options", async () => {
    const codec = await AnansiCodec.create(schema, { fullVersion: 300, kind: "sparse" });
    const wire = await codec.encode({ count: 5 });
    expect(wire[0]).toBe((1 << 4) | 0x01); // epoch 1 + Sparse
    expect(wire[1]).toBe(300 & 0xff);      // version byte
    const { version, doc } = await codec.decode(wire);
    expect(version).toBe(300);
    expect(doc).toEqual({ count: 5 });
  });

  it("batch encode/decode", async () => {
    const codec = await AnansiCodec.create(schema);
    const docs = [full, { count: 7 }, {}];
    const wire = await codec.encodeBatch(docs);
    const { docs: back } = await codec.decodeBatch(wire);
    expect(back).toEqual(docs);
  });

  it("transformed round-trip (comp + hash + AES)", async () => {
    const key = new Uint8Array(32).fill(0x42);
    const codec = await AnansiCodec.create(schema, {
      encryptionKey: key,
      decryptionKey: key,
      compression: true,
      integrity: true,
    });
    const wire = await codec.encode(full);
    expect(wire[0] & 0x04).not.toBe(0); // compressed
    expect(wire[0] & 0x40).not.toBe(0); // encrypted
    expect(wire[0] & 0x80).not.toBe(0); // hashed
    const { doc } = await codec.decode(wire);
    expect(doc).toEqual(full);
  });

  it("instances are independent — options do not leak between codecs", async () => {
    const key = new Uint8Array(32).fill(0x42);
    const sealed = await AnansiCodec.create(schema, {
      encryptionKey: key,
      decryptionKey: key,
    });
    const plain = await AnansiCodec.create(schema);

    // sealed encodes encrypted frames…
    const w1 = await sealed.encode({ title: "x" });
    expect(w1[0] & 0x40).not.toBe(0);
    // …while the sibling stays plain and rejects encrypted bytes without a key.
    const w2 = await plain.encode({ title: "x" });
    expect(w2[0] & 0x40).toBe(0);
    await expect(plain.decode(w1)).rejects.toThrow(/decryption key/i);
  });

  it("columnar via facade (plain)", async () => {
    const codec = await AnansiCodec.create(schema);
    const docs = [full, { count: 7 }, {}];
    const wire = await codec.encodeColumnar(docs);
    const { docs: back } = await codec.decodeBatch(wire);
    expect(back).toEqual(docs);
  });
});
