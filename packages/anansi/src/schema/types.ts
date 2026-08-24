// Schema definition model.
//
// The structural types are AUTO-GENERATED from the meta schema — see
// generated.ts (regenerate via `go run ./cmd/anansi codegen typescript`).
// This module layers small runtime helpers (the Literal wrapper mirroring
// Go's LiteralValue zero/null semantics, tolerant parse) on top.

import type { FieldDefinition, SchemaDefinition } from "./generated.ts";

export * from "./generated.ts";

/** FieldDefinition plus Go-supported field-level enum `values`. */
export type FieldDef = FieldDefinition & { values?: unknown[] };

/**
 * Parsed literal wrapper — mirrors Go LiteralValue zero/null semantics:
 * absent key → "zero", explicit JSON null → "null".
 */
export class Literal {
  constructor(
    public readonly kind: "zero" | "null" | "string" | "integer" | "float" | "boolean" | "object" | "array",
    public readonly value: unknown,
  ) {}
  isZero(): boolean { return this.kind === "zero"; }
  isNull(): boolean { return this.kind === "null"; }
  static fromJSON(v: unknown): Literal {
    if (v === undefined) return new Literal("zero", undefined);
    if (v === null) return new Literal("null", undefined);
    if (typeof v === "string") return new Literal("string", v);
    if (typeof v === "boolean") return new Literal("boolean", v);
    if (typeof v === "number") {
      return Number.isInteger(v) ? new Literal("integer", v) : new Literal("float", v);
    }
    if (Array.isArray(v)) return new Literal("array", v);
    if (typeof v === "object") return new Literal("object", v);
    throw new Error(`anansi: unsupported literal ${typeof v}`);
  }
}

export function parseSchema(
  data: string | SchemaDefinition | Record<string, unknown>,
): SchemaDefinition {
  const s = (
    typeof data === "string" ? JSON.parse(data) : data
  ) as unknown as SchemaDefinition;
  if (!s.name) throw new Error("anansi: schema requires a name");
  return s;
}
