/**
 * @module schema-builder
 *
 * Zod-shaped builder for Anansi `SchemaDefinition`s.
 *
 * ## Features
 *
 * - **Chainable field modifiers** with typed returns (`.required()`,
 *   `.optional()`, `.nullable()`, `.default()`, `.unique()`, `.description()`).
 * - **Automatic dependency collection.** Passing a `NestedSchemaBuilder` to
 *   `s.object` / `s.array` / `s.union` / `s.composite` / `s.record` /
 *   `s.enumRef` records it as a dep; `.build()` flattens the whole transitive
 *   closure into `SchemaDefinition.schemas` keyed by UUID. Nested builders
 *   passed *through* another field builder (`s.array(s.object(x))`) are also
 *   collected — this is what fixes `SCHEMA_REFERENCE_NOT_FOUND`.
 * - **UUIDs generated internally** — callers never see one.
 * - **Phantom types** on every builder so `InferBuilder<typeof builder>`
 *   recovers the document type without needing `as const` literals.
 * - **Rule checks at definition time**, not runtime: enum cardinality,
 *   union/composite arity, inline-kind restrictions.
 *
 * ## What does NOT work
 *
 * `.build()` returns the widened `SchemaDefinition` runtime type. So
 * `InferSchema<typeof built>` gives `unknown`-heavy types — that path requires
 * `as const` literals. Use `InferBuilder<typeof builder>` (before `.build()`)
 * or `InferBuilder<typeof builtSchemaViaSatisfies>` (after, but only if the
 * container uses `satisfies`, not `:`).
 */

import type {
  FieldDefinition,
  FieldType,
  InlineTypeDescriptor,
  InlineTypeKind,
  NestedSchemaDefinition,
  SchemaDefinition,
  SchemaReference,
} from "./generated.ts";
import { v7 as uuid } from "uuid";
import { composeSchemas, omitSchema, pickSchema } from "./composer.ts";
import type { ComposeMeta, ShapeMeta } from "./composer.ts";

// ============================================================================
// Extract types
// ============================================================================

/** Extract the raw value type from a field builder. */
export type InferFieldValue<F> =
  F extends FieldBuilder<infer V, any, any> ? V : never;

/** Extract the value type honoring nullability. */
export type InferFieldDocValue<F> =
  F extends FieldBuilder<infer V, any, infer N>
    ? N extends true
      ? V | null
      : V
    : never;

type AnyField = FieldBuilder<any, any, any>;

/** Object type produced by a set of fields, honoring `required`. */
export type BuildDoc<Fs extends Record<string, AnyField>> = {
  [K in keyof Fs as Fs[K] extends FieldBuilder<any, true, any>
    ? K
    : never]: InferFieldDocValue<Fs[K]>;
} & {
  [K in keyof Fs as Fs[K] extends FieldBuilder<any, true, any>
    ? never
    : K]?: InferFieldDocValue<Fs[K]>;
};

/** Extract the document type from any point in the builder chain. */
export type InferBuilder<B> =
  B extends BuiltSchema<infer T>
    ? T
    : B extends TopLevelSchemaBuilder<infer T>
      ? T
      : B extends NestedSchemaBuilder<infer T>
        ? T
        : B extends FieldBuilder<infer V, any, any>
          ? V
          : never;

/** Compiled schema plus its phantom document type. */
export type BuiltSchema<
  T extends Record<string, unknown> = Record<string, unknown>,
> = SchemaDefinition & { readonly _inferred?: T };

/** Extracted value type from a nested builder. */
type NestedValue<N> =
  N extends NestedSchemaBuilder<infer T> ? T : Record<string, unknown>;

/** Union-to-intersection helper for `s.composite`. */
type UnionToIntersection<U> = (
  U extends unknown ? (k: U) => void : never
) extends (k: infer I) => void
  ? I
  : never;

/** Element value union for a tuple of named sources. */
type TupleValues<Ns extends readonly NamedSource[]> = {
  [K in keyof Ns]: Ns[K] extends NestedSchemaBuilder<infer T> ? T : unknown;
};

/** `Pick` with a guard so `InferBuilder` returning `unknown` doesn't blow up. */
type PickDoc<T, K extends string> = [T] extends [never]
  ? Record<string, unknown>
  : T extends Record<string, unknown>
    ? Pick<T, Extract<K, keyof T>>
    : Record<string, unknown>;

type OmitDoc<T, K extends string> = [T] extends [never]
  ? Record<string, unknown>
  : T extends Record<string, unknown>
    ? Omit<T, Extract<K, keyof T>>
    : Record<string, unknown>;

type AsRecord<T> =
  T extends Record<string, unknown> ? T : Record<string, unknown>;

type ComposeDocs<Ss extends readonly BuiltSchema<any>[]> = AsRecord<
  UnionToIntersection<InferBuilder<Ss[number]>>
>;

// ============================================================================
// Source unions
// ============================================================================

export type ScalarKind = "string" | "number" | "integer" | "decimal";

type InlineSource = NestedSchemaBuilder<any> | AnyField | SchemaReference;
type NamedSource = NestedSchemaBuilder<any> | SchemaReference;

type InlineValue<S> =
  S extends NestedSchemaBuilder<infer T>
    ? T
    : S extends FieldBuilder<infer V, any, any>
      ? V
      : S extends SchemaReference
        ? unknown
        : never;

// ============================================================================
// FieldBuilder
// ============================================================================

export class FieldBuilder<
  V = unknown,
  R extends boolean = false,
  N extends boolean = false,
> {
  declare readonly _value: V;
  declare readonly _required: R;
  declare readonly _nullable: N;

  private def: Omit<FieldDefinition, "name">;
  private deps: Set<NestedSchemaBuilder<any>>;

  constructor(
    type: FieldType,
    extras: Partial<Omit<FieldDefinition, "type" | "name">> = {},
    deps: Iterable<NestedSchemaBuilder<any>> = [],
  ) {
    this.def = { type, ...extras } as Omit<FieldDefinition, "name">;
    this.deps = new Set(deps);
  }

  // ----- modifiers (each returns `this` re-typed) -----

  required(v = true): FieldBuilder<V, true, N> {
    this.def.required = v;
    return this as unknown as FieldBuilder<V, true, N>;
  }

  optional(): FieldBuilder<V, false, N> {
    this.def.required = false;
    return this as unknown as FieldBuilder<V, false, N>;
  }

  nullable(v = true): FieldBuilder<V, R, true> {
    this.def.nullable = v;
    return this as unknown as FieldBuilder<V, R, true>;
  }

  default(v: null): FieldBuilder<V, R, true>;
  default(v: V): FieldBuilder<V, R, N>;
  default(v: V | null): FieldBuilder<V, R, boolean> {
    if (v === null) this.def.nullable = true;
    this.def.default = v;
    return this as unknown as FieldBuilder<V, R, boolean>;
  }

  description(text: string): this {
    this.def.description = text;
    return this;
  }
  deprecated(v = true): this {
    this.def.deprecated = v;
    return this;
  }
  unique(v = true): this {
    this.def.unique = v;
    return this;
  }
  metadata(value: unknown): this {
    this.def.metadata = value;
    return this;
  }

  // ----- internal -----

  /** @internal */
  _deps(): ReadonlySet<NestedSchemaBuilder<any>> {
    return this.deps;
  }

  /** @internal — emit a `FieldDefinition` and its dependency set. */
  _compile(name: string): {
    def: FieldDefinition;
    deps: ReadonlySet<NestedSchemaBuilder<any>>;
  } {
    return { def: { ...this.def, name }, deps: this.deps };
  }

  /**
   * @internal — project this field into an inline descriptor or a single ref
   * suitable for `array` element / `record` value use.
   *
   * Throws if the field's own shape cannot be inlined (Rule 20: structural
   * kinds are never valid inline).
   */
  _asInline(): SchemaReference | InlineTypeDescriptor {
    const d = this.def;
    switch (d.type) {
      case "string":
      case "number":
      case "integer":
      case "decimal":
      case "boolean":
      case "bytes":
      case "unknown":
        return { type: d.type as InlineTypeKind };
      case "record":
        if (d.schema !== undefined) {
          throw new Error(
            "schema-builder: a typed record cannot be inlined; pass a named ref or an untyped s.record()",
          );
        }
        return { type: "record" };
      case "enum":
        if (d.schema !== undefined) {
          if (Array.isArray(d.schema)) {
            throw new Error(
              "schema-builder: cannot inline an array-of-refs; pass a single ref or a scalar builder",
            );
          }
          return d.schema;
        }
        throw new Error(
          "schema-builder: enum field has no schema to inline; use s.enum() or s.enumRef()",
        );
      case "object":
        if (d.schema !== undefined && !Array.isArray(d.schema)) {
          return d.schema;
        }
        throw new Error(
          "schema-builder: object fields require a single named reference",
        );
      default:
        throw new Error(
          `schema-builder: cannot inline structural kind "${d.type}" (Rule 20)`,
        );
    }
  }
}

// ============================================================================
// NestedSchemaBuilder
// ============================================================================

export class NestedSchemaBuilder<T = Record<string, unknown>> {
  declare readonly _value: T;

  /** Stable identity — the key in the parent's `schemas` map. */
  readonly id: string;
  readonly name: string;
  readonly description?: string;

  private _mode: "schema" | "type" | "enum" = "schema";
  private _fields = new Map<string, AnyField>();
  private _values?: unknown[];
  private _scalarType?: FieldType;
  private _default?: unknown;

  constructor(name: string, description?: string) {
    this.id = uuid();
    this.name = name;
    this.description = description;
  }

  /** Schema mode — object shape. */
  fields<Fs extends Record<string, AnyField>>(
    spec: Fs,
  ): NestedSchemaBuilder<BuildDoc<Fs>> {
    for (const [k, v] of Object.entries(spec)) this._fields.set(k, v);
    this._mode = "schema";
    return this as unknown as NestedSchemaBuilder<BuildDoc<Fs>>;
  }

  /** Enum mode — literal union. */
  values<V extends string | number>(
    vals: readonly V[],
  ): NestedSchemaBuilder<V> {
    this._values = vals.slice();
    this._mode = "enum";
    return this as unknown as NestedSchemaBuilder<V>;
  }

  /** Type mode — scalar or collection. */
  scalar<S extends ScalarKind>(kind: S): NestedSchemaBuilder<PrimitiveOf<S>> {
    this._scalarType = kind;
    this._mode = "type";
    return this as unknown as NestedSchemaBuilder<PrimitiveOf<S>>;
  }

  /** Whole-object fallback used when a referencing field has no default. */
  default(v: unknown): this {
    this._default = v;
    return this;
  }

  /** Ref form for use as `field.schema`. */
  ref(): SchemaReference {
    return { id: this.id };
  }

  /** @internal */
  _build(): NestedSchemaDefinition {
    const out: NestedSchemaDefinition = {
      name: this.name,
      ...(this.description !== undefined && { description: this.description }),
    };

    if (this._mode === "schema") {
      const fields: Record<string, FieldDefinition> = {};
      for (const [name, fb] of this._fields) {
        fields[uuid()] = fb._compile(name).def;
      }
      out.fields = fields;
    } else if (this._mode === "enum") {
      out.type = (this._scalarType ?? "string") as FieldType;
      out.values = this._values ?? [];
    } else if (this._scalarType) {
      out.type = this._scalarType;
    }

    if (this._default !== undefined) out.default = this._default;
    return out;
  }

  /** @internal — transitive closure of nested dependencies, deduped by id. */
  _collect(
    out: Set<NestedSchemaBuilder<any>> = new Set(),
  ): Set<NestedSchemaBuilder<any>> {
    if (out.has(this)) return out;
    out.add(this);
    for (const fb of this._fields.values()) {
      for (const dep of fb._deps()) dep._collect(out);
    }
    return out;
  }
}

// ============================================================================
// TopLevelSchemaBuilder
// ============================================================================

export class TopLevelSchemaBuilder<
  T extends Record<string, unknown> = Record<string, unknown>,
> {
  declare readonly _value: T;

  readonly name: string;
  readonly description?: string;
  private _version = "1.0.0";
  private _fields = new Map<string, AnyField>();

  constructor(name: string, description?: string) {
    this.name = name;
    this.description = description;
  }

  version(v: string): this {
    this._version = v;
    return this;
  }

  fields<Fs extends Record<string, AnyField>>(
    spec: Fs,
  ): TopLevelSchemaBuilder<BuildDoc<Fs>> {
    for (const [k, v] of Object.entries(spec)) this._fields.set(k, v);
    return this as unknown as TopLevelSchemaBuilder<BuildDoc<Fs>>;
  }

  field<K extends string, F extends AnyField>(
    key: K,
    fb: F,
  ): TopLevelSchemaBuilder<T & BuildDoc<Record<K, F>>> {
    this._fields.set(key, fb);
    return this as unknown as TopLevelSchemaBuilder<T & BuildDoc<Record<K, F>>>;
  }

  /** Flatten and emit a `SchemaDefinition`. Transitive nested schemas land in `schemas`. */
  build(): BuiltSchema<T> {
    const fields: Record<string, FieldDefinition> = {};
    const nested = new Set<NestedSchemaBuilder<any>>();

    for (const [name, fb] of this._fields) {
      const { def, deps } = fb._compile(name);
      fields[uuid()] = def;
      for (const d of deps) d._collect(nested);
    }

    const schemas: Record<string, NestedSchemaDefinition> = {};
    for (const n of nested) schemas[n.id] = n._build();

    const out: BuiltSchema<T> = {
      name: this.name,
      version: this._version,
      ...(this.description !== undefined && { description: this.description }),
      fields,
      ...(Object.keys(schemas).length > 0 && { schemas }),
    };
    return out;
  }
}

// ============================================================================
// Internal source projection
// ============================================================================

function toInline(src: InlineSource): SchemaReference | InlineTypeDescriptor {
  if (src instanceof FieldBuilder) return src._asInline();
  if (src instanceof NestedSchemaBuilder) return src.ref();
  return src;
}

function toRef(src: NamedSource): SchemaReference {
  return src instanceof NestedSchemaBuilder ? src.ref() : src;
}

/**
 * Collect `NestedSchemaBuilder` deps reachable from `src`.
 *
 * For a `FieldBuilder`, recurse through its own `deps` — this is what fixes
 * the case `s.array(s.object(nested))` where the nested builder is hidden one
 * level deep inside the field builder.
 */
function toDeps(
  src: InlineSource | readonly InlineSource[],
): NestedSchemaBuilder<any>[] {
  const arr = Array.isArray(src) ? src : [src];
  const out: NestedSchemaBuilder<any>[] = [];
  const visit = (x: InlineSource): void => {
    if (x instanceof NestedSchemaBuilder) out.push(x);
    else if (x instanceof FieldBuilder) for (const d of x._deps()) out.push(d);
  };
  for (const x of arr) visit(x);
  return out;
}

// ============================================================================
// Primitive mapping (mirror of infer-schema's `PrimitiveOf`)
// ============================================================================

type PrimitiveOf<T extends string> = T extends "string"
  ? string
  : T extends "number"
    ? number
    : T extends "integer"
      ? number
      : T extends "decimal"
        ? number | string
        : never;

// ============================================================================
// Factory surface
// ============================================================================

export const s = {
  // ----- primitives -----
  string: (): FieldBuilder<string> => new FieldBuilder("string"),
  number: (): FieldBuilder<number> => new FieldBuilder("number"),
  integer: (): FieldBuilder<number> => new FieldBuilder("integer"),
  decimal: (): FieldBuilder<number | string> => new FieldBuilder("decimal"),
  boolean: (): FieldBuilder<boolean> => new FieldBuilder("boolean"),
  bytes: (): FieldBuilder<Uint8Array> => new FieldBuilder("bytes"),
  geometry: (): FieldBuilder<number[][]> => new FieldBuilder("geometry"),
  unknown: (): FieldBuilder<unknown> => new FieldBuilder("unknown"),

  // ----- enums (Rule 4) -----

  /** Inline enum: `{ type: "enum", schema: { type: base, values } }`. */
  enum: <V extends string | number>(
    vals: readonly V[],
    base: ScalarKind = "string",
  ): FieldBuilder<V> =>
    new FieldBuilder("enum", {
      schema: { type: base, values: vals.slice() },
    }),

  /** Enum via named ref — target must carry `values` (Rule 2/4). */
  enumRef: <V extends string | number>(
    src: NestedSchemaBuilder<V>,
  ): FieldBuilder<V> => new FieldBuilder("enum", { schema: src.ref() }, [src]),

  // ----- collections -----

  /** Rule 7: exactly one element source. Accepts a `NestedSchemaBuilder`, a scalar `FieldBuilder`, or a raw `SchemaReference`. */
  array: <E extends InlineSource>(element: E): FieldBuilder<InlineValue<E>[]> =>
    new FieldBuilder("array", { schema: toInline(element) }, toDeps(element)),

  /** Rule 9: zero or one value source. No arg → `Record<string, unknown>`. */
  record: <E extends InlineSource = SchemaReference>(
    value?: E,
  ): FieldBuilder<
    [E] extends [SchemaReference]
      ? Record<string, unknown>
      : Record<string, InlineValue<E>>
  > =>
    new FieldBuilder(
      "record",
      value !== undefined ? { schema: toInline(value) } : {},
      value !== undefined ? toDeps(value) : [],
    ) as unknown as FieldBuilder<
      [E] extends [SchemaReference]
        ? Record<string, unknown>
        : Record<string, InlineValue<E>>
    >,

  /** Rule 8 / 20: exactly one named ref. Inline forbidden. */
  object: <N extends NestedSchemaBuilder<any>>(
    src: N,
  ): FieldBuilder<NestedValue<N>> =>
    new FieldBuilder("object", { schema: src.ref() }, [
      src,
    ]) as unknown as FieldBuilder<NestedValue<N>>,

  // ----- sum / intersection (Rules 5, 6) -----

  /** Rule 6: ≥2 named refs. */
  union: <Ns extends readonly NamedSource[]>(
    variants: Ns,
  ): FieldBuilder<TupleValues<Ns>[number]> => {
    if (variants.length < 2) {
      throw new Error("schema-builder: union requires ≥2 variants (Rule 6)");
    }
    return new FieldBuilder(
      "union",
      { schema: variants.map(toRef) },
      toDeps(variants),
    ) as unknown as FieldBuilder<TupleValues<Ns>[number]>;
  },

  /** Rule 5: ≥2 named refs, intersected. */
  composite: <Ns extends readonly NamedSource[]>(
    variants: Ns,
  ): FieldBuilder<UnionToIntersection<TupleValues<Ns>[number]>> => {
    if (variants.length < 2) {
      throw new Error(
        "schema-builder: composite requires ≥2 variants (Rule 5)",
      );
    }
    return new FieldBuilder(
      "composite",
      { schema: variants.map(toRef) },
      toDeps(variants),
    ) as unknown as FieldBuilder<UnionToIntersection<TupleValues<Ns>[number]>>;
  },

  // ----- named sub-schemas -----
  nested: (name: string, description?: string) =>
    new NestedSchemaBuilder(name, description),

  // ----- top-level -----
  define: (name: string, description?: string) =>
    new TopLevelSchemaBuilder<{}>(name, description),
  /**
   * New schema containing only the named top-level fields of `original`.
   * Nested schemas reachable from the survivors are carried over; unreachable
   * ones, and indexes/constraints whose root field was dropped, are removed.
   */
  pick<B extends BuiltSchema<any>, const K extends string>(
    original: B,
    meta: ShapeMeta,
    fields: readonly K[],
  ): BuiltSchema<PickDoc<InferBuilder<B>, K>> {
    return pickSchema(original, meta, fields) as BuiltSchema<
      PickDoc<InferBuilder<B>, K>
    >;
  },

  /** Complement of `s.pick`. */
  omit<B extends BuiltSchema<any>, const K extends string>(
    original: B,
    meta: ShapeMeta,
    fields: readonly K[],
  ): BuiltSchema<OmitDoc<InferBuilder<B>, K>> {
    return omitSchema(original, meta, fields) as BuiltSchema<
      OmitDoc<InferBuilder<B>, K>
    >;
  },

  /**
   * New schema whose top-level field set is the union of the sources'.
   * Any duplicate field name, field id, nested schema id, index id, or
   * constraint id is a hard error — compose never resolves conflicts.
   */
  compose<const Ss extends readonly BuiltSchema<any>[]>(
    meta: ComposeMeta,
    ...sources: Ss
  ): BuiltSchema<ComposeDocs<Ss>> {
    return composeSchemas(meta, sources) as BuiltSchema<ComposeDocs<Ss>>;
  },
};
