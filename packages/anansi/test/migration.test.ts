import { describe, it, expect } from "bun:test";
import {
  schemaChangeToPatch,
  classifyChangeImpact,
  calculateNextBump,
  bumpVersion,
  type SchemaChange,
  type SchemaDefinition,
} from "../src/schema/migration.ts";

const baseSchema: SchemaDefinition = {
  name: "User",
  version: "1.0.0",
  fields: {
    id: { name: "id", type: "string", required: true },
    name: { name: "name", type: "string" },
  },
  indexes: {
    idx_name: { name: "idx_name", type: "normal", fields: ["name"] },
  },
  constraints: {
    name_len: { name: "name_len", predicate: " minLength", parameters: 1 },
  },
};

// ── schemaChangeToPatch ─────────────────────────────────────────────────────

describe("schemaChangeToPatch", () => {
  it("addField", () => {
    const patches = schemaChangeToPatch(
      { type: "addField", id: "email", definition: { name: "email", type: "string" } },
      baseSchema,
    );
    expect(patches).toEqual([
      { op: "add", path: "/fields/email", value: { name: "email", type: "string" } },
    ]);
  });

  it("removeField", () => {
    const patches = schemaChangeToPatch({ type: "removeField", id: "name" }, baseSchema);
    expect(patches).toEqual([{ op: "remove", path: "/fields/name" }]);
  });

  it("modifyField", () => {
    const patches = schemaChangeToPatch(
      { type: "modifyField", id: "name", changes: { required: true, description: "full name" } },
      baseSchema,
    );
    expect(patches).toEqual([
      { op: "replace", path: "/fields/name/required", value: true },
      { op: "replace", path: "/fields/name/description", value: "full name" },
    ]);
  });

  it("addIndex creates /indexes if missing", () => {
    const schema = { ...baseSchema, indexes: undefined };
    const patches = schemaChangeToPatch(
      { type: "addIndex", id: "idx_new", definition: { name: "idx_new", type: "normal", fields: ["id"] } },
      schema,
    );
    expect(patches[0]).toEqual({ op: "add", path: "/indexes", value: {} });
    expect(patches[1]).toEqual({
      op: "add",
      path: "/indexes/idx_new",
      value: { name: "idx_new", type: "normal", fields: ["id"] },
    });
  });

  it("removeIndex", () => {
    const patches = schemaChangeToPatch({ type: "removeIndex", id: "idx_name" }, baseSchema);
    expect(patches).toEqual([{ op: "remove", path: "/indexes/idx_name" }]);
  });

  it("addConstraint creates /constraints if missing", () => {
    const schema = { ...baseSchema, constraints: undefined };
    const patches = schemaChangeToPatch(
      { type: "addConstraint", id: "c1", constraint: { name: "c1", predicate: "required" } },
      schema,
    );
    expect(patches[0]).toEqual({ op: "add", path: "/constraints", value: {} });
    expect(patches[1]).toEqual({
      op: "add",
      path: "/constraints/c1",
      value: { name: "c1", predicate: "required" },
    });
  });

  it("removeConstraint", () => {
    const patches = schemaChangeToPatch({ type: "removeConstraint", id: "name_len" }, baseSchema);
    expect(patches).toEqual([{ op: "remove", path: "/constraints/name_len" }]);
  });

  it("modifyProperty", () => {
    const patches = schemaChangeToPatch(
      { type: "modifyProperty", id: "version", value: "2.0.0" },
      baseSchema,
    );
    expect(patches).toEqual([{ op: "replace", path: "/version", value: "2.0.0" }]);
  });

  it("modifySchema recurses into child changes", () => {
    const schemaWithSchemas: SchemaDefinition = {
      ...baseSchema,
      schemas: {
        Address: { name: "Address", fields: { city: { name: "city", type: "string" } } },
      },
    };
    const patches = schemaChangeToPatch(
      {
        type: "modifySchema",
        id: "Address",
        changes: [{ type: "addField", id: "zip", definition: { name: "zip", type: "string" } }],
      },
      schemaWithSchemas,
    );
    expect(patches).toEqual([
      { op: "add", path: "/fields/zip", value: { name: "zip", type: "string" } },
    ]);
  });
});

// ── classifyChangeImpact ────────────────────────────────────────────────────

describe("classifyChangeImpact", () => {
  it("removeField → major", () => {
    expect(classifyChangeImpact({ type: "removeField", id: "x" })).toBe("major");
  });

  it("removeIndex → major", () => {
    expect(classifyChangeImpact({ type: "removeIndex", id: "x" })).toBe("major");
  });

  it("removeConstraint → major", () => {
    expect(classifyChangeImpact({ type: "removeConstraint", id: "x" })).toBe("major");
  });

  it("addField → minor", () => {
    expect(classifyChangeImpact({ type: "addField", id: "x", definition: { name: "x", type: "string" } })).toBe("minor");
  });

  it("addIndex → minor", () => {
    expect(classifyChangeImpact({ type: "addIndex", id: "x", definition: { name: "x", type: "normal", fields: [] } })).toBe("minor");
  });

  it("addConstraint → minor", () => {
    expect(classifyChangeImpact({ type: "addConstraint", id: "x", constraint: { name: "x", predicate: "p" } })).toBe("minor");
  });

  it("modifyField required=true → major", () => {
    expect(classifyChangeImpact({ type: "modifyField", id: "x", changes: { required: true } })).toBe("major");
  });

  it("modifyField type change → major", () => {
    expect(classifyChangeImpact({ type: "modifyField", id: "x", changes: { type: "number" } })).toBe("major");
  });

  it("modifyField unique=true → major", () => {
    expect(classifyChangeImpact({ type: "modifyField", id: "x", changes: { unique: true } })).toBe("major");
  });

  it("modifyField deprecated → minor", () => {
    expect(classifyChangeImpact({ type: "modifyField", id: "x", changes: { deprecated: true } })).toBe("minor");
  });

  it("modifyField description → patch", () => {
    expect(classifyChangeImpact({ type: "modifyField", id: "x", changes: { description: "hi" } })).toBe("patch");
  });

  it("modifyIndex unique change → major", () => {
    expect(classifyChangeImpact({ type: "modifyIndex", id: "x", changes: { unique: true } })).toBe("major");
  });

  it("modifyIndex description → minor", () => {
    expect(classifyChangeImpact({ type: "modifyIndex", id: "x", changes: { description: "hi" } })).toBe("minor");
  });

  it("modifySchema returns worst child impact", () => {
    const change: SchemaChange = {
      type: "modifySchema",
      id: "nested",
      changes: [
        { type: "modifyField", id: "x", changes: { description: "ok" } }, // patch
        { type: "removeField", id: "y" }, // major
      ],
    };
    expect(classifyChangeImpact(change)).toBe("major");
  });
});

// ── calculateNextBump ───────────────────────────────────────────────────────

describe("calculateNextBump", () => {
  it("returns major if any change is major", () => {
    expect(
      calculateNextBump([
        { type: "addField", id: "a", definition: { name: "a", type: "string" } },
        { type: "removeField", id: "b" },
      ]),
    ).toBe("major");
  });

  it("returns minor if highest is minor", () => {
    expect(
      calculateNextBump([
        { type: "addField", id: "a", definition: { name: "a", type: "string" } },
        { type: "modifyField", id: "b", changes: { description: "x" } },
      ]),
    ).toBe("minor");
  });

  it("returns patch if all are patch", () => {
    expect(
      calculateNextBump([
        { type: "modifyField", id: "a", changes: { description: "x" } },
        { type: "modifyField", id: "b", changes: { default: 42 } },
      ]),
    ).toBe("patch");
  });

  it("empty changes → patch", () => {
    expect(calculateNextBump([])).toBe("patch");
  });
});

// ── bumpVersion ─────────────────────────────────────────────────────────────

describe("bumpVersion", () => {
  it("major", () => expect(bumpVersion("1.2.3", "major")).toBe("2.0.0"));
  it("minor", () => expect(bumpVersion("1.2.3", "minor")).toBe("1.3.0"));
  it("patch", () => expect(bumpVersion("1.2.3", "patch")).toBe("1.2.4"));
  it("handles v prefix", () => expect(bumpVersion("v1.2.3", "major")).toBe("2.0.0"));
  it("throws on invalid", () => {
    expect(() => bumpVersion("bad", "major")).toThrow("invalid version");
  });
});
