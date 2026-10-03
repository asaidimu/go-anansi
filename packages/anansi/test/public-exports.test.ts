import { describe, it, expect } from "bun:test";
import * as root from "../src/index.ts";
import * as validation from "../src/validation/index.ts";
import * as errors from "../src/errors.ts";
// Type-level pins: these fail `bun run typecheck` (tsc --noEmit) if the
// root barrel ever drops them again. They are erased at runtime.
import type {
  StandardDocumentValidator as RootStandardDocumentValidator,
  ValidatedSchemaDefinition as RootValidatedSchemaDefinition,
  SchemaUtil as RootSchemaUtil,
  AnansiCode as RootAnansiCode,
} from "../src/index.ts";

const _typePins: [
  RootStandardDocumentValidator | null,
  RootValidatedSchemaDefinition | null,
  RootSchemaUtil | null,
  RootAnansiCode | null,
] | null = null;
void _typePins;

describe("public barrel (src/index.ts)", () => {
  it("re-exports StandardDocumentValidator for downstream Standard Schema interop", () => {
    expect((root as any).StandardDocumentValidator).toBe(
      (validation as any).StandardDocumentValidator,
    );
    expect(typeof (root as any).StandardDocumentValidator).toBe("function");
  });

  it("keeps every validation barrel value export public (no silent drops)", () => {
    const missing = Object.keys(validation).filter((k) => !(k in root));
    expect(missing).toEqual([]);
  });

  it("re-exports the error vocabulary from src/errors.ts", () => {
    expect(root.AnansiCodes).toBe(errors.AnansiCodes);
    expect(root.anansiError).toBe(errors.anansiError);
    expect(root.isAnansiError).toBe(errors.isAnansiError);
    expect(root.SystemError).toBe(errors.SystemError);
  });

  it("re-exports SchemaUtil", async () => {
    const { SchemaUtil } = await import("../src/schema/util.ts");
    expect(root.SchemaUtil).toBe(SchemaUtil);
    expect(typeof root.SchemaUtil).toBe("function");
  });
});
