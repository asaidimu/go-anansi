// SchemaUtil: hydrate / defaults / empty / pick / required, plus the
// memoized validator and its SCH-001 / SCH-006 failure modes.

import { describe, it, expect } from "bun:test";
import { SystemError, isKnownErrorCode } from "@asaidimu/utils-error";
import { SchemaUtil } from "../../src/schema/util.ts";
import { AnansiCodes } from "../../src/errors.ts";
import { DocumentValidator } from "../../src/validation/index.ts";
import type { SchemaDefinition } from "../../src/schema/generated.ts";

const SCHEMA: SchemaDefinition = {
  name: "util_fixture",
  version: "1.0.0",
  fields: {
    title: { name: "title", type: "string", required: true, default: "untitled" },
    subtitle: { name: "subtitle", type: "string", default: "sub" },
    views: { name: "views", type: "integer", default: 0 },
    published: { name: "published", type: "boolean", default: false },
    tags: { name: "tags", type: "array", default: ["x"] },
    active: { name: "active", type: "boolean" },
    score: { name: "score", type: "number" },
    payload: { name: "payload", type: "bytes" },
    meta: { name: "meta", type: "record" },
    blob: { name: "blob", type: "unknown" },
    geom: { name: "geom", type: "geometry" },
    status: {
      name: "status",
      type: "enum",
      schema: { type: "string", values: ["draft", "live"] },
    },
    author: { name: "author", type: "object", required: true, schema: { id: "author" } },
  },
  schemas: {
    author: {
      name: "author",
      fields: {
        name: { name: "name", type: "string", required: true, default: "anon" },
        age: { name: "age", type: "integer" },
      },
    },
  },
};

function caught(fn: () => unknown): unknown {
  try {
    fn();
  } catch (e) {
    return e;
  }
  throw new Error("expected a throw");
}

describe("SchemaUtil.hydrate", () => {
  const util = new SchemaUtil(SCHEMA);

  it("fills absent keys from defaults, including falsy ones", () => {
    expect(util.hydrate({})).toEqual({
      title: "untitled",
      subtitle: "sub",
      views: 0,
      published: false,
      tags: ["x"],
    });
  });

  it("preserves explicit null despite a default", () => {
    expect(util.hydrate({ subtitle: null }).subtitle).toBeNull();
  });

  it("recurses into nested objects after defaulting", () => {
    const out = util.hydrate({ author: { age: 30 } });
    expect(out.author).toEqual({ name: "anon", age: 30 });
  });

  it("preserves explicit null and extra keys inside nested objects", () => {
    const out = util.hydrate({ author: { name: null, nickname: "n" } });
    expect(out.author).toEqual({ name: null, nickname: "n" });
  });

  it("never mutates the input", () => {
    const input = { subtitle: null, author: { age: 30 } };
    const snapshot = structuredClone(input);
    const out = util.hydrate(input);
    expect(input).toEqual(snapshot);
    expect(out).not.toBe(input);
    expect(out.author).not.toBe(input.author);
  });

  it("passes extras through and tolerates non-object input", () => {
    expect(util.hydrate({ custom: 1 })).toMatchObject({ custom: 1, title: "untitled" });
    expect(util.hydrate(null)).toEqual(util.defaults());
    expect(util.hydrate(42)).toEqual(util.defaults());
  });

  it("static hydrate works without an instance and from JSON text", () => {
    expect(SchemaUtil.hydrate(SCHEMA, { subtitle: null }).subtitle).toBeNull();
    const fromText = SchemaUtil.hydrate(JSON.stringify(SCHEMA), {});
    expect(fromText.title).toBe("untitled");
    expect(fromText.author).toBeUndefined();
  });
});

describe("SchemaUtil.defaults / empty / pick / required", () => {
  const util = new SchemaUtil(SCHEMA);

  it("defaults() applies every schema default", () => {
    expect(util.defaults()).toEqual({
      title: "untitled",
      subtitle: "sub",
      views: 0,
      published: false,
      tags: ["x"],
    });
  });

  it("empty() produces type-zero values, with defaults winning", () => {
    const out = util.empty();
    expect(out.title).toBe("untitled");
    expect(out.subtitle).toBe("sub");
    expect(out.views).toBe(0);
    expect(out.published).toBe(false);
    expect(out.tags).toEqual(["x"]);
    expect(out.active).toBe(false);
    expect(out.score).toBe(0);
    expect(out.payload).toBeInstanceOf(Uint8Array);
    expect((out.payload as Uint8Array).length).toBe(0);
    expect(out.meta).toEqual({});
    expect(out.blob).toBeNull();
    expect(out.geom).toBeNull();
    expect(out.status).toBe("draft");
    expect(out.author).toEqual({ name: "anon", age: 0 });
  });

  it("pick() subsets a document by top-level keys, omitting missing ones", () => {
    const doc = { title: "T", views: 1, extra: true };
    expect(util.pick(doc, ["title", "missing"])).toEqual({ title: "T" });
    expect(util.pick(doc, ["views", "extra"])).toEqual({ views: 1, extra: true });
    expect(util.pick(null, ["title"])).toEqual({});
  });

  it("required() returns recursive dotted paths", () => {
    expect(util.required()).toEqual(["title", "author", "author.name"]);
  });
});

describe("SchemaUtil validator", () => {
  const util = new SchemaUtil(SCHEMA);

  it("memoizes a single compiled instance for identical arguments", () => {
    const v1 = util.validator();
    const v2 = util.validator();
    expect(v1).toBe(v2);
    expect(v1).toBeInstanceOf(DocumentValidator);
    const v3 = util.validator({ custom: () => [] });
    expect(v3).not.toBe(v1);
  });

  it("validate() reports a clean document as valid", async () => {
    const issues = await util.validate({ title: "Hi", author: { name: "A" } });
    expect(issues).toEqual([]);
  });

  it("validate() flags missing required fields", async () => {
    const issues = await util.validate({});
    expect(issues.some((i) => i.code === "REQUIRED_FIELD_MISSING" && i.path === "title")).toBe(true);
    expect(issues.some((i) => i.code === "REQUIRED_FIELD_MISSING" && i.path === "author")).toBe(true);
  });

  it("validate() flags type mismatches", async () => {
    const issues = await util.validate({ title: 123, author: { name: "A" } });
    expect(issues.some((i) => i.code === "TYPE_MISMATCH" && i.path === "title")).toBe(true);
  });

  it("standardValidator() wraps the memoized validator as Standard Schema V1", async () => {
    const sv = util.standardValidator();
    const good = await sv["~standard"].validate({ title: "Hi", author: { name: "A" } });
    expect("value" in good).toBe(true);
    const bad = await sv["~standard"].validate({});
    expect("issues" in bad).toBe(true);
  });
});

describe("SchemaUtil failure modes", () => {
  it("constructor throws SCH-001 when the schema has no name", () => {
    const err = caught(() => new SchemaUtil({ version: "1.0.0" } as SchemaDefinition));
    expect(err).toBeInstanceOf(SystemError);
    expect((err as SystemError).code).toBe(AnansiCodes.SCH.SCHEMA_NAME_MISSING);
    expect(isKnownErrorCode((err as SystemError).code)).toBe(true);
  });

  it("constructor wraps malformed JSON as SCH-001 with a cause", () => {
    const err = caught(() => new SchemaUtil("{not json"));
    expect(err).toBeInstanceOf(SystemError);
    expect((err as SystemError).code).toBe(AnansiCodes.SCH.SCHEMA_NAME_MISSING);
    expect((err as SystemError).cause).toBeInstanceOf(SyntaxError);
  });

  it("validator() throws SCH-006 with a cause when the graph cannot build", () => {
    const dangling = new SchemaUtil({
      name: "dangling",
      version: "1.0.0",
      fields: { a: { name: "a", type: "object", schema: { id: "ghost" } } },
    } as SchemaDefinition);
    let err: unknown;
    try {
      dangling.validator();
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(SystemError);
    expect((err as SystemError).code).toBe(AnansiCodes.SCH.VALIDATOR_BUILD_FAILED);
    expect((err as SystemError).cause).toBeInstanceOf(SystemError);
  });
});
