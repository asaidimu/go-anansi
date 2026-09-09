// Smoke tests for the adopted validation module: schemas against the
// meta-schema, and documents against a schema.

import { describe, it, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  DocumentValidator,
  SchemaValidator,
  metaSchemaPredicateMap,
} from "../src/validation/index.ts";

const GOLDEN = JSON.parse(
  readFileSync(join(import.meta.dir, "..", "testdata", "golden.json"), "utf8"),
);

describe("validation module", () => {
  it("validates the golden schema against the meta-schema", async () => {
    const issues = await SchemaValidator.validate(GOLDEN.schema as never);
    expect(issues).toEqual([]);
  });

  it("flags missing required fields in documents", async () => {
    const schema = {
      name: "req_test",
      version: "1.0.0",
      fields: {
        id: { name: "id", type: "string", required: true },
        n: { name: "n", type: "integer" },
      },
    };
    const v = await DocumentValidator.create(schema as never, metaSchemaPredicateMap);

    const ok = await v.validate({ id: "x", n: 1 });
    expect(ok).toEqual([]);

    const bad = await v.validate({ n: 1 });
    const codes = bad.map((i) => i.code ?? (i as unknown as { code?: string }).code);
    expect(bad.length).toBeGreaterThan(0);
    expect(codes).toContain("REQUIRED_FIELD_MISSING");
  });
});

describe("nullable semantics (drift fix)", () => {
  const build = (fields: Record<string, unknown>) =>
    DocumentValidator.create(
      { name: "null_t", version: "1.0.0", fields } as never,
      metaSchemaPredicateMap,
    );

  it("nullable:true accepts explicit null and satisfies required", async () => {
    const v = await build({
      nickname: { name: "nickname", type: "string", required: true, nullable: true },
    });
    expect(await v.validate({ nickname: null })).toEqual([]);
    expect(await v.validate({})).not.toEqual([]); // absent ≠ null
    expect((await v.validate({}))[0]?.code).toBe("REQUIRED_FIELD_MISSING");
  });

  it("nullable defaults to true: null is accepted without explicit nullable flag", async () => {
    const v = await build({
      nickname: { name: "nickname", type: "string" }, // no nullable flag → default true
    });
    const issues = await v.validate({ nickname: null });
    expect(issues).toEqual([]);
    expect(await v.validate({ nickname: "x" })).toEqual([]);
  });

  it("explicit nullable:false rejects null as TYPE_MISMATCH", async () => {
    const v = await build({
      nickname: { name: "nickname", type: "string", nullable: false },
    });
    const issues = await v.validate({ nickname: null });
    expect(issues.length).toBeGreaterThan(0);
    expect(issues[0]!.code).toBe("TYPE_MISMATCH");
    // but the real string still passes
    expect(await v.validate({ nickname: "x" })).toEqual([]);
  });

  it("nullable applies to nested object leaves via dotted paths", async () => {
    const v = await DocumentValidator.create(
      {
        name: "nested_null",
        version: "1.0.0",
        fields: {
          address: {
            name: "address",
            type: "object",
            schema: { id: "addr" },
          },
        },
        schemas: {
          addr: {
            name: "addr",
            fields: {
              zip: { name: "zip", type: "integer", nullable: true },
            },
          },
        },
      } as never,
      metaSchemaPredicateMap,
    );
    expect(await v.validate({ address: { zip: null } })).toEqual([]);
    const issues = await v.validate({ address: { zip: "abc" } });
    expect(issues.length).toBeGreaterThan(0);
  });
});

describe("index predicates resolve paths, not ids (drift fix)", () => {
  // Fields are keyed by UUID-ish IDs in root.fields; references use NAMES.
  const schema = {
    name: "idx_paths",
    version: "1.0.0",
    fields: {
      f_geom: {
        name: "shape",
        type: "geometry",
      },
      f_title: { name: "title", type: "string" },
      f_loc: {
        name: "location",
        type: "object",
        schema: { id: "loc" },
      },
    },
    schemas: {
      loc: {
        name: "loc",
        fields: {
          z: { name: "zip", type: "integer" },
        },
      },
    },
    indexes: {
      i1: {
        name: "i1",
        type: "spatial",
        fields: ["shape"], // by NAME — must resolve despite key 'f_geom'
      },
      i2: {
        name: "i2",
        type: "normal",
        condition: { field: "location.zip", operator: "eq", value: "oops" },
      },
    },
  };

  it("spatial index resolves geometry field by name", async () => {
    const issues = await SchemaValidator.validate(schema as never);
    const spatial = issues.filter(
      (i) => i.code === "SPATIAL_INDEX_NON_GEOMETRY",
    );
    expect(spatial).toEqual([]);
  });

  it("index conditions resolve nested dotted paths", async () => {
    const issues = await SchemaValidator.validate(schema as never);
    const cond = issues.filter(
      (i) => i.code === "INDEX_CONDITION_VALUE_TYPE_MISMATCH",
    );
    // value "oops" vs integer zip at location.zip → must be caught via path
    expect(cond.length).toBeGreaterThan(0);
  });

  it("still flags spatial indexes on non-geometry fields", async () => {
    const bad = {
      ...schema,
      indexes: { i1: { name: "i1", type: "spatial", fields: ["title"] } },
    };
    const issues = await SchemaValidator.validate(bad as never);
    expect(issues.some((i) => i.code === "SPATIAL_INDEX_NON_GEOMETRY")).toBe(
      true,
    );
  });
});
