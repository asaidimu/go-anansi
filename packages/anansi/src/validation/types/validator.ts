/**
 * types.ts
 *
 * Core type definitions for the schema validator.
 * Mirrors the Go type system faithfully, with idiomatic TypeScript adaptations:
 *   - FieldSchemaReference is a class with discriminator methods (.isSingle / .isMultiple / .isInline / .isZero)
 *   - Constraint is a discriminated union tagged by `kind`
 *   - Predicate returns boolean | Promise<boolean> to support async (DB-backed) validation
 */

import type {
  FieldType,
  FieldDefinition,
  SchemaDefinition,
  NestedSchemaDefinition,
  SchemaReference,
  InlineTypeDescriptor,
  IndexDefinition,
  ConstraintRule as SchemaConstraintRule,
  ConstraintGroup as SchemaConstraintGroup,
  Constraint as SchemaConstraint,
} from "./schema-definition";

// Re-export the schema definition types so the rest of the validator
// imports from a single location.
export type {
  FieldType,
  FieldDefinition,
  SchemaDefinition,
  NestedSchemaDefinition,
  SchemaReference,
  InlineTypeDescriptor,
  IndexDefinition,
};

// ---------------------------------------------------------------------------
// Issue
// ---------------------------------------------------------------------------

/** A single validation failure produced by a node. */
export interface Issue {
  /** Machine-readable error code (e.g. "TYPE_MISMATCH", "REQUIRED_FIELD_MISSING"). */
  code: string;
  /** Human-readable description. */
  message: string;
  /** Dot-separated path to the offending value (e.g. "user.address.street"). */
  path: string;
}

// ---------------------------------------------------------------------------
// ValidationMode
// ---------------------------------------------------------------------------

/**
 * Controls how strictly the validator interprets missing / unexpected fields.
 *
 * - `strict`        — full validation; required, unexpected, type, and constraint issues are all reported.
 * - `partialStrict` — missing-required issues are suppressed; everything else is reported.
 * - `loose`         — missing-required *and* unexpected-field issues are both suppressed.
 */
export type ValidationMode = "strict" | "partialStrict" | "loose";

// ---------------------------------------------------------------------------
// ValidationConfig
// ---------------------------------------------------------------------------

export interface ValidationConfig {
  /** Maximum nesting depth before aborting recursive traversal. Default: 20. */
  maxDepth: number;
  /** Validation strictness mode. Default: "strict". */
  mode: ValidationMode;
}

export function defaultValidationConfig(): ValidationConfig {
  return { maxDepth: 20, mode: "strict" };
}

// ---------------------------------------------------------------------------
// Predicate system
// ---------------------------------------------------------------------------

/** Parameters passed to every predicate invocation. */
export interface PredicateParams {
  /** The original top-level document (never changes across recursive calls). */
  root: Record<string, unknown>;
  /**
   * The data being validated at the current scope.
   * For global constraints this is the full root; for recursive/nested constraints
   * this is the sub-document at the constraint's base path.
   */
  data: unknown;
  /** Resolved field keys (relative to `data`) that the predicate should inspect. */
  keys: string[];
  /** Arbitrary predicate-specific parameters declared in the schema. */
  parameters?: unknown;
}

/**
 * A predicate function.
 * Returns an array of Issues (empty = success).
 * May be synchronous or asynchronous.
 */
export type PredicateFn = (
  params: PredicateParams,
) => Issue[] | Promise<Issue[]>;

/** Map of predicate name → predicate function. */
export type PredicateMap = Record<string, PredicateFn>;

// ---------------------------------------------------------------------------
// FieldSchemaReference — class with discriminator methods
// ---------------------------------------------------------------------------

/** Raw union stored inside FieldSchemaReference. */
type RawFieldSchema =
  | { kind: "zero" }
  | { kind: "single"; value: SchemaReference }
  | { kind: "multiple"; value: SchemaReference[] }
  | { kind: "inline"; value: InlineTypeDescriptor };

/**
 * Discriminated wrapper around the three legal forms of a field's `schema` property:
 *
 * - **single**   — `{ id: "..." }` — named reference (object / array / set / record / enum)
 * - **multiple** — `[{ id: "..." }, ...]` — named references array (union / composite)
 * - **inline**   — `{ type: "string" }` or `{ type: "string", values: [...] }` — inline descriptor
 * - **zero**     — absent / undefined
 *
 * Use the factory methods to construct instances.
 */
export class FieldSchemaReference {
  private readonly raw: RawFieldSchema;

  private constructor(raw: RawFieldSchema) {
    this.raw = raw;
  }

  // ── Factories ─────────────────────────────────────────────────────────────

  static zero(): FieldSchemaReference {
    return new FieldSchemaReference({ kind: "zero" });
  }

  static fromSingle(ref: SchemaReference): FieldSchemaReference {
    return new FieldSchemaReference({ kind: "single", value: ref });
  }

  static fromMultiple(refs: SchemaReference[]): FieldSchemaReference {
    return new FieldSchemaReference({ kind: "multiple", value: refs });
  }

  static fromInline(descriptor: InlineTypeDescriptor): FieldSchemaReference {
    return new FieldSchemaReference({ kind: "inline", value: descriptor });
  }

  /**
   * Parse the raw `schema` property off a `FieldDefinition` into a typed
   * `FieldSchemaReference`.  Mirrors the Go deserialization logic.
   */
  static fromFieldSchema(
    schema:
      | SchemaReference
      | SchemaReference[]
      | InlineTypeDescriptor
      | undefined,
  ): FieldSchemaReference {
    if (schema === undefined || schema === null) {
      return FieldSchemaReference.zero();
    }

    // Array → multiple named references
    if (Array.isArray(schema)) {
      return FieldSchemaReference.fromMultiple(schema as SchemaReference[]);
    }

    const obj = schema as unknown as Record<string, unknown>;

    // Has `id` → named single reference
    if ("id" in obj && typeof obj["id"] === "string") {
      return FieldSchemaReference.fromSingle(schema as SchemaReference);
    }

    // Has `type` (no `id`) → inline descriptor
    if ("type" in obj) {
      return FieldSchemaReference.fromInline(schema as InlineTypeDescriptor);
    }

    return FieldSchemaReference.zero();
  }

  // ── Discriminators ────────────────────────────────────────────────────────

  isZero(): boolean {
    return this.raw.kind === "zero";
  }

  isSingle(): boolean {
    return this.raw.kind === "single";
  }

  isMultiple(): boolean {
    return this.raw.kind === "multiple";
  }

  isInline(): boolean {
    return this.raw.kind === "inline";
  }

  // ── Accessors (throw if wrong kind) ───────────────────────────────────────

  asSingle(): SchemaReference {
    if (this.raw.kind !== "single") {
      throw new Error(
        `FieldSchemaReference is '${this.raw.kind}', not 'single'`,
      );
    }
    return this.raw.value;
  }

  asMultiple(): SchemaReference[] {
    if (this.raw.kind !== "multiple") {
      throw new Error(
        `FieldSchemaReference is '${this.raw.kind}', not 'multiple'`,
      );
    }
    return this.raw.value;
  }

  asInline(): InlineTypeDescriptor {
    if (this.raw.kind !== "inline") {
      throw new Error(
        `FieldSchemaReference is '${this.raw.kind}', not 'inline'`,
      );
    }
    return this.raw.value;
  }
}

// ---------------------------------------------------------------------------
// Constraint discriminated union
// ---------------------------------------------------------------------------

/**
 * Internal constraint kind tag — used by the validator to discriminate between
 * rules and groups without relying on structural duck-typing.
 */
export type ConstraintKind = "rule" | "group";

/**
 * A leaf constraint: a named predicate with optional field paths and parameters.
 * Mirrors Go's `ConstraintRule`.
 */
export interface ValidatorConstraintRule {
  kind: "rule";
  name: string;
  description: string | undefined;
  predicate: string;
  parameters: unknown;
  fields: string[];
}

/**
 * A logical group of constraints combined with a `LogicalOperator`.
 * Mirrors Go's `ConstraintGroup`.
 */
export interface ValidatorConstraintGroup {
  kind: "group";
  name: string;
  description: string | undefined;
  operator: string; // LogicalOperatorEnum from schema-definition.ts
  rules: ValidatorConstraint[];
}

/** A constraint is either a single rule or a logical group of rules. */
export type ValidatorConstraint =
  | ValidatorConstraintRule
  | ValidatorConstraintGroup;

/**
 * Convert a raw `Constraint` from `schema-definition.ts` into the validator's
 * internal tagged union form.  Rules that lack a `predicate` field are treated
 * as groups.
 */
export function toValidatorConstraint(
  raw: SchemaConstraint,
): ValidatorConstraint {
  const r = raw as unknown as Record<string, unknown>;

  if ("predicate" in r && typeof r["predicate"] === "string") {
    const rule = raw as SchemaConstraintRule;
    return {
      kind: "rule" as const,
      name: rule.name,
      description: rule.description ?? undefined,
      predicate: rule.predicate,
      parameters: rule.parameters ?? undefined,
      fields: (rule.fields ?? []) as string[],
    };
  }

  const group = raw as SchemaConstraintGroup;
  return {
    kind: "group" as const,
    name: group.name,
    description: group.description ?? undefined,
    operator: group.operator as string,
    rules: (group.rules ?? []).map((r) =>
      toValidatorConstraint(r as SchemaConstraint),
    ),
  };
}

/** Keyed map of constraint ID → ValidatorConstraint. */
export type SchemaConstraintMap = Record<string, ValidatorConstraint>;

/**
 * Convert a raw `Record<string, Constraint>` (from SchemaDefinition) to the
 * validator's internal map form.
 */
export function toConstraintMap(
  raw: Record<string, SchemaConstraint> | undefined,
): SchemaConstraintMap {
  if (!raw) return {};
  const out: SchemaConstraintMap = {};
  for (const [id, c] of Object.entries(raw)) {
    out[id] = toValidatorConstraint(c);
  }
  return out;
}

// ---------------------------------------------------------------------------
// Constraint specificity & scope — mirrors Go constants exactly
// ---------------------------------------------------------------------------

export type ConstraintSpecificity = 1 | 2 | 3;
export const SpecificityNestedSchema: ConstraintSpecificity = 1;
export const SpecificitySchemaReference: ConstraintSpecificity = 2;
export const SpecificityTopLevel: ConstraintSpecificity = 3;

export type ConstraintScope = "global" | "recursive";

export interface EffectiveConstraint {
  constraint: ValidatorConstraint;
  specificity: ConstraintSpecificity;
  basePath: string;
  scope: ConstraintScope;
}

// ---------------------------------------------------------------------------
// NodeResult
// ---------------------------------------------------------------------------

export interface NodeResult {
  issues: Issue[];
  success: boolean;
  skipped: boolean;
}

export const SUCCESS: NodeResult = {
  issues: [],
  success: true,
  skipped: false,
};
export const SKIPPED: NodeResult = { issues: [], success: true, skipped: true };

export function failNode(issues: Issue[]): NodeResult {
  return { issues, success: false, skipped: false };
}

export function createIssue(
  code: string,
  message: string,
  path: string,
): Issue {
  return { code, message, path };
}

// ---------------------------------------------------------------------------
// ValidationContext — state threaded through a traversal pass
// ---------------------------------------------------------------------------

export interface ValidationContext {
  /** The original top-level document; never changes. */
  originalRoot: Record<string, unknown>;
  /** The current root being validated (may be a sub-document for recursive schemas). */
  rootData: unknown;
  /** Alias for rootData — kept for parity with Go. */
  data: unknown;
  /** The predicate function registry. */
  functionMap: PredicateMap;
  /** Maximum allowed nesting depth. */
  maxDepth: number;
  /** Validation strictness. */
  mode: ValidationMode;
  /**
   * Per-node success state. Index = node ID, value = success (true) | failure (false) | absent (undefined).
   * Used by the traversal loop to propagate skipping when a dependency failed.
   */
  visited: Map<number, boolean>;
  /** Accumulated issues for this traversal. */
  issues: Issue[];
}

export function makeValidationContext(
  document: Record<string, unknown>,
  fmap: PredicateMap,
  mode: ValidationMode,
  maxDepth: number,
): ValidationContext {
  return {
    originalRoot: document,
    rootData: document,
    data: document,
    functionMap: fmap,
    maxDepth,
    mode,
    visited: new Map(),
    issues: [],
  };
}
