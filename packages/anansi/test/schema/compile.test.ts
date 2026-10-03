// Compile-stage regressions: isInline must mirror Go's IsInline — an inline
// descriptor declares a type or values; a bare {} is not a descriptor.

import { describe, it, expect } from "bun:test";
import { Compiler } from "../../src/schema/compile.ts";
import type { SchemaDefinition } from "../../src/schema/generated.ts";

function compile(fields: SchemaDefinition["fields"]): ReturnType<Compiler["compile"]> {
  return new Compiler({
    name: "t",
    version: "1.0.0",
    fields,
    schemas: {},
  } as SchemaDefinition).compile();
}

describe("isInline guard (Go IsInline parity)", () => {
  it("treats a bare {} schema as no schema, not an inline descriptor", () => {
    const { root } = compile({
      obj: { name: "obj", type: "object", schema: {} as never },
      arr: { name: "arr", type: "array", schema: {} as never },
    });

    const obj = root.find((f) => f.name === "obj")!;
    expect(obj.kind).toEqual({ tag: "scalar" }); // bare object without shape

    const arr = root.find((f) => f.name === "arr")!;
    expect(arr.kind).toEqual({ tag: "container", c: { record: false } });
  });

  it("still resolves inline descriptors declaring a type", () => {
    const { root } = compile({
      arr: { name: "arr", type: "array", schema: { type: "string" } },
    });
    const arr = root.find((f) => f.name === "arr")!;
    expect(arr.kind).toEqual({
      tag: "container",
      c: { itemType: "string", record: false },
    });
  });

  it("still resolves inline descriptors declaring values", () => {
    const { root } = compile({
      e: { name: "e", type: "enum", schema: { values: ["a", "b"] } as never },
    });
    const e = root.find((f) => f.name === "e")!;
    expect(e.kind.tag).toBe("enum");
    if (e.kind.tag === "enum") {
      expect(e.kind.enumDef.lookup.has('"a"')).toBe(true);
      expect(e.kind.enumDef.lookup.has('"b"')).toBe(true);
    }
  });

  it("still resolves named single references", () => {
    const c = new Compiler({
      name: "t",
      version: "1.0.0",
      fields: {
        obj: { name: "obj", type: "object", schema: { id: "shape" } },
      },
      schemas: {
        shape: { name: "shape", type: "object", fields: {} },
      },
    } as SchemaDefinition);
    const { root } = c.compile();
    const obj = root.find((f) => f.name === "obj")!;
    expect(obj.kind.tag).toBe("object");
  });
});
