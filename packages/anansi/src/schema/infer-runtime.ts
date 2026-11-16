/**
 * @module infer-runtime
 *
 * Runtime type-tree inference for Anansi schemas.
 *
 * Produces a structured `InferredTypeNode` describing the shape a conforming
 * value must have. No validation is performed — this is pure structural
 * inference, suitable for reflection, code generation, UI scaffolding, and
 * documentation.
 *
 * ## Design decisions
 *
 * - **Registry keying.** `SchemaReference.id` resolves against the record key
 *   under `root.schemas` (Rule 14). This is what `buildRegistry` produces.
 * - **Enum detection.** Any nested schema with a populated `values` array is an
 *   enum, regardless of `type`. (Rule 2 formal example uses `type: "string"`.)
 *   Field-level `type: "enum"` looks up the target's `values`, or the inline
 *   descriptor's `values`.
 * - **Composite vs. union.** Composite is AND, union is OR. (Rules 5, 6.)
 * - **Inline descriptors.** Only `type` and — for scalars — `values` are read.
 *   Structural kinds are never valid inline (Rule 20, Form 3); they degrade to
 *   `{ kind: "unknown" }` instead of throwing.
 * - **Cycles.** Terminating refs (cycle points, depth caps) are emitted as
 *   `{ kind: "ref", id }` with `resolved` absent (Rule 16). Non-terminating
 *   refs carry their `resolved` expansion.
 * - **Invalid schemas** degrade to `unknown` rather than throwing, so this is
 *   safe against partially-trusted input.
 */

import type {
  SchemaDefinition,
  NestedSchemaDefinition,
  FieldDefinition,
  InlineTypeDescriptor,
  FieldType,
} from "./generated.ts";

// ============================================================================
// Public types
// ============================================================================

export type SchemaLike = SchemaDefinition | NestedSchemaDefinition;

/** Map of `schemas` record keys → their definitions. */
export type Registry = Record<string, SchemaLike>;

/**
 * Discriminated node describing a value's shape.
 *
 * Primitive kinds are collapsed into a single variant for concision; the
 * remaining kinds each carry a structural payload.
 */
export type InferredTypeNode =
  | { kind: "string" }
  | { kind: "number" }
  | { kind: "integer" }
  | { kind: "decimal" }
  | { kind: "boolean" }
  | { kind: "bytes" }
  | { kind: "geometry" }
  | { kind: "unknown" }
  | { kind: "enum"; values: unknown[] }
  | { kind: "array"; element: InferredTypeNode }
  | { kind: "record"; value: InferredTypeNode }
  | { kind: "object"; fields: Record<string, InferredField> }
  | { kind: "union"; variants: InferredTypeNode[] }
  | { kind: "composite"; parts: InferredTypeNode[] }
  | {
      kind: "ref";
      id: string;
      /** Present unless the ref was cut short (cycle or depth cap). */
      resolved?: InferredTypeNode;
    };

/** One field in an inferred object shape. */
export interface InferredField {
  type: InferredTypeNode;
  /** `true` when `nullable: true` — the value may be `null`. */
  nullable: boolean;
  /** `true` when `required: true` — the key must be present. */
  required: boolean;
  description?: string;
  deprecated?: boolean;
  /** Carried through; relevant to data validation, not to inference. */
  unique?: boolean;
  /** Carried through; does not affect the inferred type. */
  default?: unknown;
  /** Opaque user data carried through unchanged. */
  metadata?: unknown;
}

export interface InferOptions {
  /**
   * Registry to resolve refs against. Built from the schema's own `schemas`
   * map when omitted. **Keyed by `schemas` record key, not `name`.**
   */
  registry?: Registry;
  /**
   * If the root schema is itself a nested schema that appears in the registry,
   * pass its key here to seed the cycle-detection set — this prevents one
   * unnecessary level of expansion of self-referencing roots.
   */
  rootId?: string;
  /** Maximum recursion depth. Default 64. */
  maxDepth?: number;
  /**
   * What to emit for refs that don't resolve.
   * - `"unknown"` (default): degrade to `{ kind: "unknown" }`
   * - `"ref"`: keep `{ kind: "ref", id }` without `resolved`
   */
  onUnresolvedRef?: "unknown" | "ref";
}

// ============================================================================
// Public API
// ============================================================================

/**
 * Build a registry keyed by the record key under `schemas` — matching what
 * `SchemaReference.id` resolves against. **Do not key by `name`.**
 */
export function buildRegistry(root: SchemaDefinition): Registry {
  const registry: Registry = {};
  for (const [key, def] of Object.entries(root.schemas ?? {})) {
    registry[key] = def;
  }
  return registry;
}

/**
 * Infer the structural type of a schema.
 *
 * @example
 * ```ts
 * const tree = inferType(orderSchema);
 * console.log(formatTree(tree));
 * ```
 */
export function inferType(schema: SchemaLike, opts: InferOptions = {}): InferredTypeNode {
  const registry =
    opts.registry ??
    ((schema as SchemaDefinition).schemas
      ? buildRegistry(schema as SchemaDefinition)
      : {});

  const seen = new Set<string>();
  if (opts.rootId) seen.add(opts.rootId);

  return inferNode(schema, {
    registry,
    maxDepth: opts.maxDepth ?? 64,
    onUnresolvedRef: opts.onUnresolvedRef ?? "unknown",
    depth: 0,
    seen,
  });
}

// ============================================================================
// Internal inference
// ============================================================================

interface Ctx {
  registry: Registry;
  maxDepth: number;
  onUnresolvedRef: "unknown" | "ref";
  depth: number;
  seen: Set<string>;
}

/**
 * Top-level dispatch over the three nested-schema modes (Rule 2):
 * enum → schema mode → type mode.
 */
function inferNode(schema: SchemaLike, ctx: Ctx): InferredTypeNode {
  const nested = schema as NestedSchemaDefinition;

  // Enum mode — `values` present implies enum, whatever `type` says.
  if (nested.values && nested.values.length > 0) {
    return { kind: "enum", values: nested.values };
  }

  // Schema mode — `fields` present.
  if (schema.fields) {
    const fields: Record<string, InferredField> = {};
    for (const def of Object.values(schema.fields)) {
      fields[def.name] = fieldFrom(def, ctx);
    }
    return { kind: "object", fields };
  }

  // Type mode.
  return typeModeNode((nested.type ?? "object") as FieldType, nested, ctx);
}

/**
 * Type-mode nested schemas. `object` is never valid here (Rule 2) and
 * degrades to `unknown`.
 */
function typeModeNode(
  t: FieldType,
  nested: NestedSchemaDefinition,
  ctx: Ctx,
): InferredTypeNode {
  const d = nested.schema;
  switch (t) {
    case "array":
      return { kind: "array", element: descriptorNode(d, ctx) };
    case "record":
      return { kind: "record", value: descriptorNode(d, ctx) };
    case "union":
      return { kind: "union", variants: descriptorNodes(d, ctx) };
    case "composite":
      return { kind: "composite", parts: descriptorNodes(d, ctx) };
    case "enum":
      // Defensive: valid enum detection went through the `values` check above.
      return { kind: "enum", values: nested.values ?? [] };
    case "string":
    case "number":
    case "integer":
    case "decimal":
    case "boolean":
    case "bytes":
    case "geometry":
    case "unknown":
      return { kind: t };
    case "object":
      // Rule 2: object is never a valid type-mode type.
      return { kind: "unknown" };
    default:
      return { kind: "unknown" };
  }
}

/** Construct an `InferredField` from a `FieldDefinition`. */
function fieldFrom(def: FieldDefinition, ctx: Ctx): InferredField {
  return {
    type: inferFieldType(def, ctx),
    nullable: def.nullable ?? false,
    required: def.required ?? false,
    description: def.description,
    deprecated: def.deprecated,
    unique: def.unique,
    default: def.default,
    metadata: def.metadata,
  };
}

/** Field-type dispatch (Rules 3–9, 20). */
function inferFieldType(def: FieldDefinition, ctx: Ctx): InferredTypeNode {
  const d = def.schema;

  switch (def.type) {
    case "array":
      return { kind: "array", element: descriptorNode(d, ctx) };

    case "record":
      // Rule 9: schema optional; absent → untyped map.
      return {
        kind: "record",
        value: d ? descriptorNode(d, ctx) : { kind: "unknown" },
      };

    case "union":
      return { kind: "union", variants: descriptorNodes(d, ctx) };

    case "composite":
      return { kind: "composite", parts: descriptorNodes(d, ctx) };

    case "object":
      // Rule 8: single named ref required; missing → unknown (defensive).
      return d ? descriptorNode(d, ctx) : { kind: "unknown" };

    case "enum":
      return enumNodeFromField(d, ctx);

    // Primitives — Rule 3 prohibits a schema; if present we ignore it.
    case "string":
    case "number":
    case "integer":
    case "decimal":
    case "boolean":
    case "bytes":
    case "geometry":
    case "unknown":
      return { kind: def.type };

    default:
      return { kind: "unknown" };
  }
}

/**
 * Enum inference for `type: "enum"` fields (Rule 4).
 * - Ref → target's `values`.
 * - Inline descriptor → its `values`.
 * - Fallback → `unknown` (spec-invalid).
 */
function enumNodeFromField(
  d: NestedSchemaDefinition["schema"],
  ctx: Ctx,
): InferredTypeNode {
  if (!d || Array.isArray(d)) return { kind: "unknown" };

  if ("id" in d) {
    const target = ctx.registry[d.id];
    if (target) {
      const nested = target as NestedSchemaDefinition;
      if (nested.values) return { kind: "enum", values: nested.values };
    }
    return { kind: "enum", values: [] };
  }

  // Inline descriptor form (Rule 20, Form 3).
  return { kind: "enum", values: d.values ?? [] };
}

/** Single descriptor → node. Arrays distribute as unions. */
function descriptorNode(
  d: NestedSchemaDefinition["schema"],
  ctx: Ctx,
): InferredTypeNode {
  if (!d) return { kind: "unknown" };
  if (Array.isArray(d)) {
    return d.length === 1
      ? descriptorNode(d[0], ctx)
      : { kind: "union", variants: d.map((x) => descriptorNode(x, ctx)) };
  }
  if ("id" in d) return resolveRef(d.id, ctx);
  return inlineNode(d);
}

/** Descriptor array → array of nodes. Used by union/composite. */
function descriptorNodes(
  d: NestedSchemaDefinition["schema"],
  ctx: Ctx,
): InferredTypeNode[] {
  if (!d) return [];
  const arr = Array.isArray(d) ? d : [d];
  return arr.map((x) => descriptorNode(x, ctx));
}

/**
 * Inline descriptor → node (Rule 20, Form 3).
 *
 * Only `type` and `values` are read. `values` on a scalar implies an inline
 * enum; `record` is the only structural kind legal inline. Anything else is
 * spec-invalid and degrades to `unknown`.
 */
function inlineNode(d: InlineTypeDescriptor): InferredTypeNode {
  if (d.values && d.values.length > 0) {
    return { kind: "enum", values: d.values };
  }
  switch (d.type) {
    case "string":
    case "number":
    case "integer":
    case "decimal":
    case "boolean":
    case "bytes":
    case "unknown":
      return { kind: d.type };
    case "record":
      // Inline records carry no element schema.
      return { kind: "record", value: { kind: "unknown" } };
    default:
      // Structural kinds are never valid inline.
      return { kind: "unknown" };
  }
}

/**
 * Resolve `{ id }` against the registry. Cycles and depth caps produce a
 * `{ kind: "ref", id }` node **without** `resolved`; successful resolutions
 * carry the expanded shape.
 */
function resolveRef(id: string, ctx: Ctx): InferredTypeNode {
  if (ctx.depth >= ctx.maxDepth || ctx.seen.has(id)) {
    return { kind: "ref", id };
  }
  const target = ctx.registry[id];
  if (!target) {
    return ctx.onUnresolvedRef === "unknown" ? { kind: "unknown" } : { kind: "ref", id };
  }
  const child: Ctx = {
    ...ctx,
    depth: ctx.depth + 1,
    seen: new Set(ctx.seen).add(id),
  };
  return { kind: "ref", id, resolved: inferNode(target, child) };
}

// ============================================================================
// Formatting
// ============================================================================

/**
 * Format a runtime tree as readable pseudo-TypeScript. Intended for debugging
 * and documentation, not for use as source — comments, references, and shared
 * type hoisting are not emitted.
 *
 * - Enums render as literal unions.
 * - Array elements that are unions/composites are parenthesized.
 * - Cycle points render as `Ref<id>`.
 */
export function formatTree(node: InferredTypeNode, depth = 0): string {
  const pad = "  ".repeat(depth);
  const childPad = "  ".repeat(depth + 1);

  switch (node.kind) {
    case "string":
    case "number":
    case "integer":
    case "decimal":
    case "boolean":
    case "bytes":
    case "geometry":
    case "unknown":
      return node.kind;

    case "enum":
      return node.values.map((v) => JSON.stringify(v)).join(" | ");

    case "array": {
      const el = formatTree(node.element, depth);
      return needsParens(node.element) ? `(${el})[]` : `${el}[]`;
    }

    case "record":
      return `Record<string, ${formatTree(node.value, depth)}>`;

    case "union":
      return node.variants
        .map((v) => {
          const s = formatTree(v, depth);
          return v.kind === "composite" ? `(${s})` : s;
        })
        .join(" | ");

    case "composite":
      return node.parts
        .map((p) => {
          const s = formatTree(p, depth);
          return p.kind === "union" ? `(${s})` : s;
        })
        .join(" & ");

    case "object": {
      const entries = Object.entries(node.fields);
      if (entries.length === 0) return "{}";
      const lines = entries.map(([key, f]) => {
        const opt = f.required ? "" : "?";
        const nul = f.nullable ? " | null" : "";
        return `${childPad}${key}${opt}: ${formatTree(f.type, depth + 1)}${nul};`;
      });
      return `{\n${lines.join("\n")}\n${pad}}`;
    }

    case "ref":
      return node.resolved ? formatTree(node.resolved, depth) : `Ref<${node.id}>`;
  }
}

/** True when the node binds looser than `[]` and needs parens to nest. */
function needsParens(n: InferredTypeNode): boolean {
  return n.kind === "union" || n.kind === "composite";
}