// Schema composer: reachability pruning, atomicity of indexes/constraints,
// and compose conflict handling.

import { describe, it, expect } from "bun:test";
import {
  composeSchemas,
  omitSchema,
  pickSchema,
} from "../../src/schema/composer.ts";
import type { SchemaDefinition } from "../../src/schema/generated.ts";

function base(): SchemaDefinition {
  return {
    name: "base",
    version: "1.0.0",
    fields: {
      a: { name: "a", type: "string" },
      b: { name: "b", type: "string" },
      c: { name: "c", type: "object", schema: { id: "obj" } },
    },
    schemas: {
      obj: {
        name: "obj",
        fields: { d: { name: "d", type: "string" } },
      },
      unused: {
        name: "unused",
        fields: { e: { name: "e", type: "string" } },
      },
    },
    indexes: {
      ab: { name: "idx_a_b", type: "normal", fields: ["a", "b"] },
      c: { name: "idx_c", type: "normal", fields: ["c"] },
      cd: { name: "idx_c_d", type: "normal", fields: ["c.d"] },
    },
    constraints: {
      ab: { name: "need_ab", predicate: "p", fields: ["a", "b"] },
      c: { name: "need_c", predicate: "p", fields: ["c"] },
    },
  };
}

function names(schema: SchemaDefinition): string[] {
  return Object.values(schema.fields ?? {}).map((f) => f.name).sort();
}

describe("pickSchema / omitSchema", () => {
  it("prunes unreachable nested schemas", () => {
    const picked = pickSchema(base(), { name: "picked" }, ["a", "b"]);
    expect(names(picked)).toEqual(["a", "b"]);
    expect(picked.schemas).toBeUndefined();
    expect(Object.keys(picked.indexes ?? {})).toEqual(["ab"]);
    expect(Object.keys(picked.constraints ?? {})).toEqual(["ab"]);
  });

  it("keeps nested schemas reachable through a surviving field", () => {
    const picked = pickSchema(base(), { name: "picked" }, ["c"]);
    expect(Object.keys(picked.schemas ?? {})).toEqual(["obj"]);
    expect(Object.keys(picked.indexes ?? {}).sort()).toEqual(["c", "cd"]);
  });

  it("refuses partial survival of an atomic index or constraint", () => {
    expect(() => pickSchema(base(), { name: "bad" }, ["a"])).toThrow(
      /index "idx_a_b" is atomic/,
    );
    const noIndexes = base();
    delete noIndexes.indexes;
    expect(() => pickSchema(noIndexes, { name: "bad" }, ["a"])).toThrow(
      /constraint "need_ab" is atomic/,
    );
  });

  it("omit is pick's complement and validates every dropped name", () => {
    const omitted = omitSchema(base(), { name: "omitted" }, ["a", "b"]);
    expect(names(omitted)).toEqual(["c"]);
    expect(() => omitSchema(base(), { name: "bad" }, ["nope"])).toThrow(
      /does not exist/,
    );
  });
});

describe("composeSchemas", () => {
  function source(name: string, field: string, type: "string" | "integer"): SchemaDefinition {
    return {
      name,
      version: "1.0.0",
      fields: { [`${name}_${field}`]: { name: field, type } },
    };
  }

  it("merges fields, nested schemas, indexes, and constraints", () => {
    const left = source("left", "a", "string");
    left.schemas = { s1: { name: "s1", fields: { x: { name: "x", type: "string" } } } };
    left.indexes = { i: { name: "i_a", type: "normal", fields: ["a"] } };
    const right = source("right", "b", "integer");
    right.schemas = { s2: { name: "s2", fields: { y: { name: "y", type: "integer" } } } };

    const out = composeSchemas({ name: "joined" }, [left, right]);
    expect(names(out)).toEqual(["a", "b"]);
    expect(Object.keys(out.schemas ?? {}).sort()).toEqual(["s1", "s2"]);
    expect(Object.keys(out.indexes ?? {})).toEqual(["i"]);
  });

  it("fails fast on duplicate field names with no resolution mode", () => {
    const left = source("left", "a", "string");
    const right = source("right", "a", "integer");

    expect(() => composeSchemas({ name: "x" }, [left, right])).toThrow(
      /declared by both/,
    );
  });

  it("treats the same source listed twice as a conflict", () => {
    const left = source("left", "a", "string");
    expect(() => composeSchemas({ name: "x" }, [left, left])).toThrow(
      /declared by both/,
    );
  });

  it("fails fast on duplicate ids in any merged map", () => {
    const left = source("left", "a", "string");
    const duplicateField = source("right", "b", "integer");
    duplicateField.fields = { left_a: { name: "b", type: "integer" } };
    expect(() => composeSchemas({ name: "x" }, [left, duplicateField])).toThrow(
      /field id "left_a" is declared more than once/,
    );

    const duplicateSchema = source("right", "b", "integer");
    const nested = {
      name: "s",
      fields: { x: { name: "x", type: "string" as const } },
    };
    left.schemas = { s: nested };
    duplicateSchema.schemas = { s: nested };
    expect(() => composeSchemas({ name: "x" }, [left, duplicateSchema])).toThrow(
      /nested schema id "s" is declared more than once/,
    );

    const duplicateIndex = source("right", "b", "integer");
    const idx = { name: "idx", type: "normal" as const, fields: ["b"] };
    left.indexes = { idx };
    duplicateIndex.indexes = { idx };
    expect(() => composeSchemas({ name: "x" }, [left, duplicateIndex])).toThrow(
      /index id "idx" is declared more than once/,
    );

    const duplicateConstraint = source("right", "b", "integer");
    const constraint = { name: "c", predicate: "p", fields: ["b"] };
    left.constraints = { c: constraint };
    duplicateConstraint.constraints = { c: constraint };
    expect(() => composeSchemas({ name: "x" }, [left, duplicateConstraint])).toThrow(
      /constraint id "c" is declared more than once/,
    );
  });
});
