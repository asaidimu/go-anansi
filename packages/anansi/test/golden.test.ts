// Golden conformance: TS must decode every Go-produced packet into the
// exact source payload, and re-encode to byte-identical packets.

import { describe, it, expect } from "bun:test";
import { makeStripNulls, hexToBytes, bytesToHex } from "./helpers.ts";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { link, buildManifest } from "../src/schema/link.ts";
import {
  encodeDocument, decodeDocument, encodeBatchRows, encodeBatchColumnar, decodeBatch,
} from "../src/wire/packet.ts";

interface GoldenCase {
  name: string;
  kind: string;
  packet: string; // hex
  payload: unknown;
  transforms?: {
    compression?: boolean;
    integrity?: boolean;
    encryptionKey?: string;
  };
}
interface Golden {
  schema: Record<string, unknown>;
  cases: GoldenCase[];
}

const GOLDEN = JSON.parse(
  readFileSync(join(import.meta.dir, "..", "testdata", "golden.json"), "utf8"),
) as Golden;

const linked = link(GOLDEN.schema as never);
const fields = buildManifest(linked);
const stripNulls = makeStripNulls(fields);



describe("golden packet conformance", () => {
  for (const c of GOLDEN.cases) {
    if (c.transforms) continue; // covered by transforms.test.ts (async paths)
    describe(c.name, () => {
      const packet = hexToBytes(c.packet);
      const isBatch = c.kind.startsWith("batch");

      if (!isBatch) {
        it("decodes to the source payload", () => {
          const { doc } = decodeDocument(packet, fields);
          expect(doc).toEqual(stripNulls(c.payload) as Record<string, unknown>);
        });
        it("re-encodes byte-identically", () => {
          const kind = c.kind === "dense" ? "dense" : "sparse";
          const wire = encodeDocument(fields, stripNulls(c.payload) as Record<string, unknown>, 0, kind);
          expect(bytesToHex(wire)).toBe(bytesToHex(packet));
        });
      } else {
        const payloads = stripNulls(c.payload) as Record<string, unknown>[];
        it("decodes records to the source payloads", () => {
          const { docs } = decodeBatch(packet, fields);
          expect(docs).toEqual(payloads);
        });
        it("re-encodes byte-identically", () => {
          const wire =
            c.kind === "batch_row"
              ? encodeBatchRows(fields, payloads, 0)
              : encodeBatchColumnar(fields, payloads, 0);
          expect(bytesToHex(wire)).toBe(bytesToHex(packet));
        });
      }
    });
  }
});
