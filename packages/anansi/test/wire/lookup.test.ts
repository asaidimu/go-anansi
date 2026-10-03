// Prototype-safety regressions: dotted-path lookup consults own properties
// only. With `in`, paths like "constructor" resolved against Object.prototype
// and phantom values leaked into encoded packets.

import { describe, it, expect } from "bun:test";
import { link, buildManifest } from "../../src/schema/link.ts";
import {
  encodeDocument,
  decodeDocument,
  encodeBatchRows,
  decodeBatch,
} from "../../src/wire/packet.ts";
import type { SchemaDefinition } from "../../src/schema/generated.ts";

const schema = {
  name: "t",
  version: "1.0.0",
  fields: {
    a: { name: "constructor", type: "string" },
    b: { name: "toString", type: "string" },
    c: { name: "title", type: "string" },
  },
} as SchemaDefinition;

const fields = buildManifest(link(schema));

describe("lookup uses own properties only", () => {
  it("absent prototype members encode as Not Set (dense)", () => {
    const wire = encodeDocument(fields, {} as never, 0, "dense");
    const back = decodeDocument(wire, fields);
    expect(back.doc).toEqual({});
    expect(Object.hasOwn(back.doc, "constructor")).toBe(false);
    expect(Object.hasOwn(back.doc, "toString")).toBe(false);
  });

  it("absent prototype members are omitted from sparse packets", () => {
    const wire = encodeDocument(fields, {} as never, 0, "sparse");
    const back = decodeDocument(wire, fields);
    expect(back.doc).toEqual({});
  });

  it("own properties with prototype names round-trip normally", () => {
    const doc = JSON.parse(
      '{"constructor":"mine","toString":"also mine","title":"t"}',
    ) as Record<string, unknown>;
    const wire = encodeDocument(fields, doc as never, 0, "dense");
    const back = decodeDocument(wire, fields);
    expect(back.doc).toEqual(doc);
  });

  it("prototype pollution does not mark fields as present", () => {
    (Object.prototype as Record<string, unknown>).polluted = "x";
    try {
      const polluted = link({
        name: "t",
        version: "1.0.0",
        fields: { p: { name: "polluted", type: "string" } },
      } as SchemaDefinition);
      const f = buildManifest(polluted);
      const wire = encodeDocument(f, {} as never, 0, "dense");
      const back = decodeDocument(wire, f);
      expect(back.doc).toEqual({});
    } finally {
      delete (Object.prototype as Record<string, unknown>).polluted;
    }
  });

  it("batch rows treat prototype members as absent", () => {
    const docs: Record<string, unknown>[] = [{}, { constructor: "own" }];
    const wire = encodeBatchRows(fields, docs as never, 0);
    const { docs: back } = decodeBatch(wire, fields);
    expect(back[0]).toEqual({});
    expect(back[1]).toEqual({ constructor: "own" });
  });
});
