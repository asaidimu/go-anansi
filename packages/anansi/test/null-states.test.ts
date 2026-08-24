// Spec §2.7 conformance: the THREE field states — Not Set, Null, Has Value —
// are distinct on the wire and preserved through TS encode→decode.

import { describe, it, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { link, buildManifest } from "../src/schema/link.ts";
import {
  encodeDocument, decodeDocument,
  encodeBatchRows, encodeBatchColumnar, decodeBatch,
} from "../src/wire/packet.ts";

const GOLDEN = JSON.parse(
  readFileSync(join(import.meta.dir, "..", "testdata", "golden.json"), "utf8"),
) as { schema: Record<string, unknown> };

const fields = buildManifest(link(GOLDEN.schema as never));

// title=string set, nickname? not in schema — use known fields:
// count(int), title(string), price(float)
const doc3states = {
  count: 42,        // Has Value
  title: null,      // Null (explicit)
  price: undefined, // Not Set
};
const docAfterDecode = { count: 42, title: null };

function hexToBytes(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  return out;
}

describe("three-state field semantics (spec 2.7)", () => {
  it("dense preserves all three states", () => {
    const wire = encodeDocument(fields, doc3states as never, 0, "dense");
    const back = decodeDocument(wire, fields);
    expect(back.doc).toEqual(docAfterDecode);

    // State map must carry 01 (Null) for title, not 00/10.
    const nFields = fields.length;
    const smBytes = Math.ceil((2 * nFields) / 8);
    const packed = wire.subarray(2, 2 + smBytes);
    const idxTitle = fields.findIndex((f) => f.path === "title");
    const code =
      (packed[idxTitle! >> 2] >> ((idxTitle! & 3) << 1)) & 0x03;
    expect(code).toBe(0b01);
  });

  it("sparse marks null via the DataPoint null bit", () => {
    const wire = encodeDocument(fields, doc3states as never, 0, "sparse");
    const r = { pos: 2 }; // skip flags+version
    void r;
    const body = wire.subarray(2);
    // body: [field_count][dp varint][value]...
    const fieldCount = body[0];
    expect(fieldCount).toBe(2); // title(null) + count(value); price absent

    // find title's entry: walk entries, match canonical dp
    const titleDp = fields.find((f) => f.path === "title")!.dp & 0xfffffffe;
    let pos = 1;
    for (let i = 0; i < fieldCount; i++) {
      let dp = 0, shift = 0;
      for (;;) {
        const b = body[pos++];
        dp |= (b! & 0x7f) << shift;
        if (!(b! & 0x80)) break;
        shift += 7;
      }
      const isNull = (dp & 1) === 1;
      const canonical = dp & 0xfffffffe;
      if (canonical === titleDp) {
        expect(isNull).toBe(true); // null bit SET for explicit null
        continue;
      }
      expect(isNull).toBe(false); // value-bearing fields keep null bit clear
      // skip value bytes by consuming until next entry/end via length rules —
      // only ints here, so one varint value byte follows.
      while (body[pos]! & 0x80) pos++;
      pos++;
    }
  });

  it("batch row keeps the distinction per record", () => {
    const docs = [
      { count: 1, title: "set" },   // Has Value
      { count: 2, title: null },    // Null
      { count: 3 },                 // Not Set
    ];
    const wire = encodeBatchRows(fields, docs as never, 0);
    const { docs: back } = decodeBatch(wire, fields);
    expect("title" in back[0]!).toBe(true);
    expect(back[0]!.title).toBe("set");
    expect(back[1]!.title).toBeNull();
    expect("title" in back[2]!).toBe(false);
  });

  it("columnar keeps the distinction per record", () => {
    const docs = [
      { count: 1, title: "set" },
      { count: 2, title: null },
      { count: 3 },
    ];
    const wire = encodeBatchColumnar(fields, docs as never, 0);
    const { docs: back } = decodeBatch(wire, fields);
    expect(back[0]!.title).toBe("set");
    expect(back[1]!.title).toBeNull();
    expect("title" in back[2]!).toBe(false);
  });
});
