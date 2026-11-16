// Runtime schema inference: field naming, registry resolution, cycles, and
// formatting. Type-level inference lives alongside it so `tsc` checks both.

import { describe, it, expect } from "bun:test";
import {
  buildRegistry,
  formatTree,
  inferType,
} from "../../src/schema/infer-runtime.ts";
import type { InferSchema } from "../../src/schema/infer-schema.ts";
import type {
  NestedSchemaDefinition,
  SchemaDefinition,
} from "../../src/schema/generated.ts";

const ADDRESS = "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e2f";
const URN = "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e3f";

function schema(): SchemaDefinition {
  return {
    name: "user",
    version: "1.0.0",
    fields: {
      f1: { name: "id", type: "string", required: true },
      f2: { name: "address", type: "object", schema: { id: ADDRESS } },
      f3: {
        name: "status",
        type: "enum",
        schema: { type: "string", values: ["active", "disabled"] },
      },
    },
    schemas: {
      [ADDRESS]: {
        name: "address",
        fields: {
          a1: { name: "street", type: "string", required: true },
          a2: { name: "city", type: "string", nullable: true },
        },
      },
    },
  };
}

describe("inferType", () => {
  it("keys inferred fields by name, not by the UUID record key", () => {
    const tree = inferType(schema());
    if (tree.kind !== "object") throw new Error("expected object");
    expect(Object.keys(tree.fields).sort()).toEqual(["address", "id", "status"]);
    expect(formatTree(tree)).toContain("id: string;");
    expect(formatTree(tree)).not.toContain("f1");
  });

  it("expands refs and renders nullable/optional markers", () => {
    const tree = inferType(schema());
    if (tree.kind !== "object") throw new Error("expected object");
    const address = tree.fields.address!.type;
    if (address.kind !== "ref") throw new Error("expected ref");
    expect(address.resolved?.kind).toBe("object");
    expect(formatTree(address)).toContain("street: string;");
    expect(formatTree(address)).toContain("city?: string | null;");
  });

  it("uses an explicit registry instead of ignoring it", () => {
    const node: NestedSchemaDefinition = {
      name: "node",
      fields: {
        name: { name: "name", type: "string" },
        child: { name: "child", type: "object", schema: { id: URN } },
      },
    };
    const registry = {
      [URN]: { name: "urn", type: "string" } as NestedSchemaDefinition,
    };

    const tree = inferType(node, { registry });
    if (tree.kind !== "object") throw new Error("expected object");
    const child = tree.fields.child!.type;
    expect(child.kind).toBe("ref");
    if (child.kind !== "ref") throw new Error("expected ref");
    expect(child.resolved?.kind).toBe("string");
  });

  it("cuts cycles at the ref without an expansion", () => {
    const node: NestedSchemaDefinition = {
      name: "node",
      fields: {
        name: { name: "name", type: "string" },
        next: { name: "next", type: "object", schema: { id: URN } },
      },
    };
    const tree = inferType(node, {
      registry: buildRegistry({ name: "r", version: "1", schemas: { [URN]: node } }),
      rootId: URN,
    });
    if (tree.kind !== "object") throw new Error("expected object");
    const next = tree.fields.next!.type;
    expect(next).toEqual({ kind: "ref", id: URN });
  });

  it("reads inline enums and rejects unknown refs by policy", () => {
    const inline = inferType({
      name: "enum",
      type: "string",
      values: ["a", "b"],
    });
    expect(inline).toEqual({ kind: "enum", values: ["a", "b"] });

    const missing = inferType({
      name: "missing",
      fields: { f: { name: "f", type: "object", schema: { id: "ghost" } } },
    });
    if (missing.kind !== "object") throw new Error("expected object");
    expect(missing.fields.f!.type).toEqual({ kind: "unknown" });
  });
});

// --- compile-time inference -------------------------------------------------

const TYPED_SCHEMA = {
  name: "typed",
  version: "1.0.0",
  fields: {
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e40": {
      name: "id",
      type: "string",
      required: true,
    },
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e41": {
      name: "age",
      type: "integer",
      nullable: true,
    },
    "01934d8a-7c24-7b3e-9f12-7a8b9c0d1e42": {
      name: "role",
      type: "enum",
      schema: { type: "string", values: ["admin", "user"] },
    },
  },
} as const satisfies SchemaDefinition;

type TypedDocument = InferSchema<typeof TYPED_SCHEMA>;

const typedGood: TypedDocument = { id: "x", age: null, role: "admin" };
// @ts-expect-error age is number | null
const typedBadAge: TypedDocument = { id: "x", age: "nope" };
// @ts-expect-error id is required
const typedMissingId: TypedDocument = { age: 1 };
// @ts-expect-error role is the inline literal union
const typedBadRole: TypedDocument = { id: "x", role: "owner" };

describe("InferSchema", () => {
  it("derives document keys from FieldDefinition.name", () => {
    expect(Object.keys(typedGood).sort()).toEqual(["age", "id", "role"]);
    expect(typedBadAge).toBeDefined();
    expect(typedMissingId).toBeDefined();
    expect(typedBadRole).toBeDefined();
  });
});
