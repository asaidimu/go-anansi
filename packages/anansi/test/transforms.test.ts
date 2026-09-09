// Transform conformance: TS must decode every Go-produced transformed
// packet, and byte-match wherever the transform is deterministic
// (integrity-only). Encryption is nonce-random by design — decode parity +
// self round-trip only.

import { describe, it, expect } from "bun:test";
import { makeStripNulls, hexToBytes, bytesToHex } from "./helpers.ts";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { link, buildManifest } from "../src/schema/link.ts";
import {
  encodeAnansiPacket, decodeAnansiPacket,
  encodeAnansiBatchRows, decodeAnansiBatch,
  encodeAnansiBatchColumnar,
} from "../src/wire/transforms.ts";

interface GoldenCase {
  name: string;
  kind: string;
  packet: string;
  payload: unknown;
  transforms?: { compression?: boolean; integrity?: boolean; encryptionKey?: string };
}
interface Golden {
  schema: Record<string, unknown>;
  cases: GoldenCase[];
}

const GOLDEN = JSON.parse(
  readFileSync(join(import.meta.dir, "..", "testdata", "golden.json"), "utf8"),
) as Golden;

const fields = buildManifest(link(GOLDEN.schema as never));
const stripNulls = makeStripNulls(fields);


describe("transform conformance vs Go", () => {
  for (const c of GOLDEN.cases) {
    if (!c.transforms || c.kind.startsWith("batch")) continue;
    describe(c.name, () => {
      const tf = c.transforms!;
      const key = tf.encryptionKey ? hexToBytes(tf.encryptionKey) : undefined;

      it("decodes the Go packet", async () => {
        const { doc } = await decodeAnansiPacket(
          hexToBytes(c.packet),
          fields,
          key ? { decryptionKey: key } : {},
        );
        expect(doc).toEqual(stripNulls(c.payload) as Record<string, unknown>);
      });

      if (!tf.compression && !tf.encryptionKey) {
        // Deterministic digest → byte parity holds.
        it("re-encodes byte-identically", async () => {
          const wire = await encodeAnansiPacket(
            fields,
            stripNulls(c.payload) as Record<string, unknown>,
            0,
            { integrity: true },
          );
          const gotH = bytesToHex(wire);
          const wantH = bytesToHex(hexToBytes(c.packet));
          if (gotH !== wantH) {
            let i = 0;
            while (i < Math.min(gotH.length, wantH.length) && gotH[i] === wantH[i]) i++;
            throw new Error(
              `integrity re-encode mismatch @${i >> 1}: got=${gotH.slice(i - 4, i + 10)} want=${wantH.slice(i - 4, i + 10)} lens=${wire.length}/${hexToBytes(c.packet).length}`,
            );
          }
        });
      }
    });
  }

  it("flags carry the spec bits", async () => {
    const doc = { count: 1 };
    const wire = await encodeAnansiPacket(fields, doc, 0, {
      compression: true,
      integrity: true,
      encryptionKey: new Uint8Array(32).fill(7),
    });
    expect(wire[0] & 0x04).not.toBe(0);
    expect(wire[0] & 0x40).not.toBe(0);
    expect(wire[0] & 0x80).not.toBe(0);
  });
});

describe("transform round-trips (self)", () => {
  const base = GOLDEN.cases.find((c) => c.name === "dense_full")!;
  const doc = stripNulls(base.payload) as Record<string, unknown>;
  const key = new Uint8Array(32).fill(0x42);

  const combos = [
    ["plain", {}],
    ["comp", { compression: true }],
    ["hash", { integrity: true }],
    ["comp_hash", { compression: true, integrity: true }],
    ["enc_comp_hash", { encryptionKey: key, compression: true, integrity: true }],
  ] as const;

  for (const [name, opts] of combos) {
    it(`single ${name}`, async () => {
      const wire = await encodeAnansiPacket(fields, doc, 300, opts as never);
      const back = await decodeAnansiPacket(
        wire,
        fields,
        "encryptionKey" in opts ? { decryptionKey: key } : {},
      );
      expect(back.version).toBe(300);
      expect(back.doc).toEqual(doc);
    });
  }

  it("batch rows comp+hash", async () => {
    const docs = [doc, { count: 7 }, {}];
    const wire = await encodeAnansiBatchRows(fields, docs, 0, {
      compression: true,
      integrity: true,
    });
    const back = await decodeAnansiBatch(wire, fields);
    expect(back.docs).toEqual(docs);
  });

  it("wrong key is rejected", async () => {
    const wire = await encodeAnansiPacket(fields, doc, 0, {
      encryptionKey: key,
    });
    const bad = new Uint8Array(32).fill(0x99);
    await expect(
      decodeAnansiPacket(wire, fields, { decryptionKey: bad }),
    ).rejects.toThrow();
  });

  it("tampered body fails the integrity check", async () => {
    const wire = await encodeAnansiPacket(fields, doc, 0, { integrity: true });
    const tampered = new Uint8Array(wire);
    tampered[tampered.length - 1] ^= 0xff;
    await expect(decodeAnansiPacket(tampered, fields)).rejects.toThrow(
      /integrity/i,
    );
  });

  it("encrypted without a key throws", async () => {
    const wire = await encodeAnansiPacket(fields, doc, 0, {
      encryptionKey: key,
    });
    await expect(decodeAnansiPacket(wire, fields)).rejects.toThrow(
      /no decryption key/i,
    );
  });
});

describe("transformed columnar (full duplex)", () => {
  const base = GOLDEN.cases.find((c) => c.name === "dense_full")!;
  const docs = [
    stripNulls(base.payload) as Record<string, unknown>,
    { count: 7, items: [{ sku: "Z9" }] },
    {},
  ];
  const key = new Uint8Array(32).fill(0x42);

  it("columnar comp+hash round-trip", async () => {
    const wire = await encodeAnansiBatchColumnar(fields, docs, 7, {
      compression: true,
      integrity: true,
    });
    expect(wire[0] & 0x08).not.toBe(0); // columnar bit in header
    const back = await decodeAnansiBatch(wire, fields);
    expect(back.version).toBe(7);
    expect(back.docs).toEqual(docs);
  });

  it("columnar enc+comp+hash round-trip", async () => {
    const wire = await encodeAnansiBatchColumnar(fields, docs, 300, {
      encryptionKey: key,
      compression: true,
      integrity: true,
    });
    expect(wire[0] & 0x40).not.toBe(0);
    const back = await decodeAnansiBatch(wire, fields, {
      decryptionKey: key,
    });
    expect(back.version).toBe(300);
    expect(back.docs).toEqual(docs);
  });

  it("decodes the Go transformed-columnar vector", async () => {
    const c = GOLDEN.cases.find(
      (x) => x.name === "batch_columnar_enc_comp_hash",
    )!;
    if (!c.transforms?.encryptionKey) {
      throw new Error("vector must carry the encryption key");
    }
    const gk = hexToBytes(c.transforms.encryptionKey);
    const back = await decodeAnansiBatch(hexToBytes(c.packet), fields, {
      decryptionKey: gk,
    });
    // Element objects key by relative names; normalize expected likewise.
    const want = stripNulls(c.payload) as Record<string, unknown>[];
    expect(back.docs.length).toBe(want.length);
    for (let i = 0; i < want.length; i++) {
      for (const [k, v] of Object.entries(want[i])) {
        if (v === null) continue;
        if (k === "items") {
          const items = back.docs[i]!.items as Record<string, unknown>[];
          const wantItems = (v as { sku?: string; qty?: number }[]).map((e) => ({
            sku: e.sku ?? null,
            qty: e.qty ?? null,
          }));
          const gotItems = items.map((e) => ({
            sku: e.sku ?? null,
            qty: e.qty ?? null,
          }));
          expect(gotItems).toEqual(wantItems.filter((x) => x.sku !== null || x.qty !== null));
          continue;
        }
        expect((back.docs[i] as Record<string, unknown>)[k]).toEqual(v);
      }
    }
  });
});
