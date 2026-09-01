import { describe, it, expect } from "bun:test";
import { deepMerge, sha256 } from "../src/utils.ts";

describe("deepMerge", () => {
  it("merges flat objects", () => {
    expect(deepMerge({ a: 1, b: 2 }, { b: 3 })).toEqual({ a: 1, b: 3 });
  });

  it("merges nested objects recursively", () => {
    const result = deepMerge(
      { a: { b: 1, c: 2 } } as Record<string, unknown>,
      { a: { c: 3, d: 4 } } as Record<string, unknown>,
    );
    expect(result).toEqual({ a: { b: 1, c: 3, d: 4 } });
  });

  it("does not mutate originals", () => {
    const target = { a: { b: 1 } } as Record<string, unknown>;
    const update = { a: { c: 2 } } as Record<string, unknown>;
    deepMerge(target, update);
    expect(target).toEqual({ a: { b: 1 } });
  });

  it("replaces arrays (not merging)", () => {
    expect(deepMerge({ a: [1, 2] }, { a: [3] })).toEqual({ a: [3] });
  });

  it("undefined values in update are ignored", () => {
    expect(deepMerge({ a: 1 }, { a: undefined })).toEqual({ a: 1 });
  });
});

describe("sha256", () => {
  it("returns a 64-char hex string", async () => {
    const hash = await sha256("hello world");
    expect(hash).toMatch(/^[a-f0-9]{64}$/);
  });

  it("is deterministic", async () => {
    const a = await sha256("test");
    const b = await sha256("test");
    expect(a).toBe(b);
  });

  it("different inputs produce different hashes", async () => {
    const a = await sha256("foo");
    const b = await sha256("bar");
    expect(a).not.toBe(b);
  });

  it("known hash for 'hello'", async () => {
    const hash = await sha256("hello");
    expect(hash).toBe("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824");
  });
});
