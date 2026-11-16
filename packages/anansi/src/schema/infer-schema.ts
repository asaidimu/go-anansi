/**
 * @module infer-schema
 *
 * Compile-time schema inference for Anansi schemas.
 *
 * Given a `SchemaDefinition` (or `NestedSchemaDefinition`) declared with
 * `as const satisfies …`, derive the TypeScript type a conforming value must
 * have. No validation is performed.
 *
 * ## Design decisions (see `schema_spec.md`)
 *
 * - **Registry keying.** `SchemaReference.id` matches the **record key** under
 *   `schemas`, never the nested schema's `name`. (Rule 14, Part VI.)
 * - **Enum detection.** A nested schema is an enum iff `values` is populated,
 *   regardless of `type`. The Rule 2 example is `{type:"string", values:[…]}`,
 *   not `{type:"enum"}`. (Rules 2, 4.)
 * - **Composite = AND, Union = OR.** (Rules 5, 6.)
 * - **Inline descriptors** carry only `type` and — for scalars — `values`.
 *   Structural kinds (`array`, `object`, `enum`, `union`, `composite`,
 *   `geometry`) are never valid inline. (Rule 20, Form 3.)
 * - **Cycles** are permitted (Rule 16); recursion terminates naturally through
 *   the type alias for the referenced schema.
 * - **Malformed schemas degrade to `unknown`** rather than erroring, so this
 *   works on partially-trusted input.
 *
 * @example
 * ```ts
 * const schema = { … } as const satisfies SchemaDefinition;
 * type User = InferSchema<typeof schema>;
 * ```
 */

import type {
  SchemaDefinition,
  NestedSchemaDefinition,
  FieldDefinition,
  InlineTypeDescriptor,
} from "./generated.ts";

// ============================================================================
// Internal type helpers
// ============================================================================

/**
 * Turn a union into an intersection. Used to model `composite` fields
 * (Rule 5: "Data MUST match ALL referenced schemas").
 */
type UnionToIntersection<U> =
  (U extends unknown ? (k: U) => void : never) extends (k: infer I) => void ? I : never;

/**
 * TS mapping for each primitive `FieldType`. Note:
 * - `decimal` → `number | string` because TS has no arbitrary-precision decimal;
 *   a caller may prefer `string`-only or `BigInt` — swap here to change policy.
 * - `geometry` is `Array<Array<number>>` per Rule 10.
 * - `unknown` → `unknown` per Rule 17.
 */
type PrimitiveOf<T extends string> =
  T extends "string" ? string :
  T extends "number" ? number :
  T extends "integer" ? number :
  T extends "decimal" ? number | string :
  T extends "boolean" ? boolean :
  T extends "bytes" ? Uint8Array :
  T extends "geometry" ? number[][] :
  unknown;

/**
 * Inline descriptor (Rule 20, Form 3).
 * - If `values` is present it is an inline enum → union of literals.
 * - `record` is the only structural kind legal inline; its element type is
 *   always `unknown` because the descriptor cannot carry a nested schema.
 */
type InferInline<D extends InlineTypeDescriptor> =
  D extends { values: infer V extends readonly unknown[] } ? V[number] :
  D extends { type: "record" } ? Record<string, unknown> :
  D extends { type: infer T extends string } ? PrimitiveOf<T> :
  unknown;

/**
 * Dispatch over the three `schema` forms (Rule 20):
 * 1. `SchemaReference` — `{ id }`
 * 2. `SchemaReferenceArray` — `[{ id }, …]`
 * 3. `InlineTypeDescriptor` — `{ type }`
 *
 * Arrays distribute over their elements, yielding a union. That is exactly
 * what `union` needs; `composite` wraps the result in `UnionToIntersection`.
 */
type InferDescriptor<D, Idx> =
  D extends readonly (infer U)[] ? InferDescriptor<U, Idx> :
  D extends { id: infer Id extends string }
    ? Id extends keyof Idx
      ? InferNested<Idx[Id], Idx>
      : unknown
    : D extends InlineTypeDescriptor ? InferInline<D>
    : unknown;

/**
 * Enum field inference (Rule 4).
 * - Referenced schema: pull `values` from the target if present.
 * - Inline descriptor: pull `values` directly.
 * - Fallback: `string | number` (the two legal enum base types).
 */
type InferEnumField<S, Idx> =
  S extends { id: infer Id extends string }
    ? Id extends keyof Idx
      ? Idx[Id] extends { values: infer V extends readonly unknown[] }
        ? V[number]
        : string | number
      : string | number
    : S extends { values: infer V extends readonly unknown[] }
      ? V[number]
      : string | number;

/**
 * Per-type field inference. Order matters — each branch is
 * mutually exclusive with the others except the `record` pair.
 */
type InferFieldType<F extends FieldDefinition, Idx> =
  // union: distribute over descriptor array
  F extends { type: "union", schema: infer S } ? InferDescriptor<S, Idx> :
  // composite: AND of descriptor array
  F extends { type: "composite", schema: infer S } ? UnionToIntersection<InferDescriptor<S, Idx>> :
  // array: element descriptor, then wrap
  F extends { type: "array", schema: infer S } ? InferDescriptor<S, Idx>[] :
  // record (typed)
  F extends { type: "record", schema: infer S } ? Record<string, InferDescriptor<S, Idx>> :
  // record (untyped)
  F extends { type: "record" } ? Record<string, unknown> :
  // object: single ref, resolve to its schema-mode shape
  F extends { type: "object", schema: infer S } ? InferDescriptor<S, Idx> :
  // enum: pull `values` from ref or inline
  F extends { type: "enum", schema: infer S } ? InferEnumField<S, Idx> :
  // primitive
  F extends { type: infer T extends string } ? PrimitiveOf<T> :
  unknown;

/** Wrap in `| null` when `nullable: true`. */
type InferField<F extends FieldDefinition, Idx> =
  F extends { nullable: true } ? InferFieldType<F, Idx> | null : InferFieldType<F, Idx>;

/** The document key a field contributes — its `name`, never the map key. */
type FieldName<F> = F extends { name: infer N extends string } ? N : never;

/** Object shape that respects `required` (Rule 8: optionality from `required`). */
type BuildObject<F extends Record<string, FieldDefinition>, Idx> = {
  [K in keyof F as F[K] extends { required: true }
    ? FieldName<F[K]>
    : never]-?: InferField<F[K], Idx>;
} & {
  [K in keyof F as F[K] extends { required: true }
    ? never
    : FieldName<F[K]>]?: InferField<F[K], Idx>;
};

/**
 * Type-mode nested schemas (Rule 2, type mode).
 *
 * A type-mode nested schema is not an object; it describes a value used by
 * other fields. Its `schema` (if any) points at the inner shape.
 */
type InferTypeMode<T extends string, S, Idx> =
  T extends "array"
    ? S extends { schema: infer Sch } ? InferDescriptor<Sch, Idx>[] : unknown[] :
  T extends "record"
    ? S extends { schema: infer Sch } ? Record<string, InferDescriptor<Sch, Idx>> : Record<string, unknown> :
  T extends "union"
    ? S extends { schema: infer Sch } ? InferDescriptor<Sch, Idx> : unknown :
  T extends "composite"
    ? S extends { schema: infer Sch } ? UnionToIntersection<InferDescriptor<Sch, Idx>> : unknown :
  T extends "enum"
    ? S extends { values: infer V extends readonly unknown[] } ? V[number] : string | number :
  PrimitiveOf<T>;

/**
 * Dispatch over the two modes (Rule 2):
 * 1. **Schema mode** — `fields` populated → object shape.
 * 2. **Enum mode** — `values` populated → literal union.
 * 3. **Type mode** — `type` populated → per-type shape.
 */
type InferNested<S, Idx> =
  S extends { fields: infer F extends Record<string, FieldDefinition> } ? BuildObject<F, Idx> :
  S extends { values: infer V extends readonly unknown[] } ? V[number] :
  S extends { type: infer T extends string } ? InferTypeMode<T, S, Idx> :
  Record<string, unknown>;

/**
 * The nested-schema registry, keyed by record key under `schemas` — this is
 * what `SchemaReference.id` actually resolves against (Rule 14). Never keyed
 * by `name`.
 */
type IndexOf<S> =
  S extends { schemas: infer M extends Record<string, unknown> } ? M : {};

// ============================================================================
// Public
// ============================================================================

/**
 * Derive the TypeScript type a conforming value must have.
 *
 * @typeParam S A schema declared with `as const satisfies SchemaDefinition`
 *              (or `… NestedSchemaDefinition`) so that `type`, `required`,
 *              `nullable`, `id`, and `values` survive as literals.
 *
 * @example
 * ```ts
 * type User = InferSchema<typeof userSchema>;
 * // {
 * //   id: string;
 * //   email: string;
 * //   age?: number | null;
 * //   role: "admin" | "user";
 * // }
 * ```
 */
export type InferSchema<S extends SchemaDefinition | NestedSchemaDefinition> =
  InferNested<S, IndexOf<S>>;