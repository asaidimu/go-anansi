// Schema builder: dependency collection, modifiers, validation shape, and
// the pick/omit/compose projections.

import { describe, it, expect } from "bun:test";
import { s, type InferBuilder } from "../../src/schema/builder.ts";
import { SchemaValidator } from "../../src/validation/schema-validator.ts";
import type { FieldDefinition, SchemaDefinition } from "../../src/schema/generated.ts";

function fields(schema: SchemaDefinition): FieldDefinition[] {
  return Object.values(schema.fields ?? {});
}

function names(schema: SchemaDefinition): string[] {
  return fields(schema).map((f) => f.name).sort();
}

describe("s builder", () => {
  it("emits named field records and collects nested dependencies", async () => {
    const address = s.nested("address").fields({
      street: s.string().required(),
      city: s.string().default("Nairobi"),
    });
    const schema = s.define("user").fields({
      id: s.string().required(),
      addresses: s.array(s.object(address)).required(),
    }).build();

    expect(names(schema)).toEqual(["addresses", "id"]);
    const addressField = fields(schema).find((f) => f.name === "addresses")!;
    const addressId = (addressField.schema as { id: string }).id;
    expect(schema.schemas?.[addressId]?.name).toBe("address");
    expect(Object.keys(schema.schemas ?? {})).toHaveLength(1);
    expect(await SchemaValidator.validate(schema)).toEqual([]);
  });

  it("collects every union and composite variant", async () => {
    const a = s.nested("a").fields({ a: s.string() });
    const b = s.nested("b").fields({ b: s.integer() });
    const schema = s.define("sums").fields({
      either: s.union([a, b]),
      both: s.composite([a, b]),
    }).build();

    expect(Object.keys(schema.schemas ?? {})).toHaveLength(2);
    for (const field of fields(schema)) {
      expect(Array.isArray(field.schema)).toBe(true);
      expect((field.schema as unknown[]).length).toBe(2);
    }
    expect(await SchemaValidator.validate(schema)).toEqual([]);
  });

  it("applies chainable modifiers", () => {
    const field = s.string()
      .required()
      .nullable()
      .default(null)
      .description("optional label")
      .deprecated()
      .unique()
      .metadata({ internal: true });
    const schema = s.define("mods").field("label", field).build();
    const out = fields(schema)[0]!;

    expect(out).toMatchObject({
      name: "label",
      type: "string",
      required: true,
      nullable: true,
      default: null,
      description: "optional label",
      deprecated: true,
      unique: true,
      metadata: { internal: true },
    });
  });

  it("rejects unterminated or structurally invalid inline elements", () => {
    const address = s.nested("address").fields({ street: s.string() });
    expect(() => s.union([address])).toThrow(/≥2 variants/);
    expect(() => s.composite([address])).toThrow(/≥2 variants/);
    expect(() => s.array(s.array(s.string()))).toThrow(/Rule 20/);
    expect(() => s.array(s.record(s.object(address)))).toThrow(/typed record/);
    expect(() => s.array(s.object(address).nullable())).not.toThrow();
  });
});

describe("s.pick / s.omit / s.compose", () => {
  const address = s.nested("address").fields({ street: s.string() });
  const base = s.define("base").fields({
    id: s.string().required(),
    address: s.object(address),
    note: s.string(),
  }).build();

  it("picks fields and carries only reachable nested schemas", () => {
    const picked = s.pick(base, { name: "picked" }, ["id", "address"]);
    expect(names(picked)).toEqual(["address", "id"]);
    expect(Object.keys(picked.schemas ?? {})).toHaveLength(1);
    expect(picked.name).toBe("picked");
  });

  it("omits fields and rejects unknown names", () => {
    const omitted = s.omit(base, { name: "omitted" }, ["note"]);
    expect(names(omitted)).toEqual(["address", "id"]);
    expect(() => s.pick(base, { name: "x" }, ["missing"])).toThrow(/does not exist/);
    expect(() => s.omit(base, { name: "x" }, ["missing"])).toThrow(/does not exist/);
  });

  it("composes disjoint schemas and fails fast on name conflicts", () => {
    const left = s.define("left").fields({ a: s.string() }).build();
    const right = s.define("right").fields({ b: s.string() }).build();
    expect(names(s.compose({ name: "joined" }, left, right))).toEqual(["a", "b"]);

    const clash = s.define("clash").fields({ a: s.integer() }).build();
    expect(() => s.compose({ name: "bad" }, left, clash)).toThrow(/declared by both/);
  });

  it("rejects composing the same schema twice", () => {
    expect(() => s.compose({ name: "twice" }, base, base)).toThrow(/declared by both/);
  });
});

// --- compile-time builder inference -----------------------------------------

const BUILDER = s.define("typed").fields({
  id: s.string().required(),
  age: s.integer().nullable(),
  tags: s.array(s.string()),
});

type BuilderDocument = InferBuilder<typeof BUILDER>;

const builderGood: BuilderDocument = { id: "x", age: null, tags: ["a"] };
// @ts-expect-error id is required
const builderMissingId: BuilderDocument = { age: 1 };
// @ts-expect-error age is number | null
const builderBadAge: BuilderDocument = { id: "x", age: "nope" };

describe("InferBuilder", () => {
  it("keeps required, optional, and nullable builder fields distinct", () => {
    expect(builderGood.id).toBe("x");
    expect(builderMissingId).toBeDefined();
    expect(builderBadAge).toBeDefined();
  });
});
