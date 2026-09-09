// Schema mutation, migration, and transform types.
//
// These extend the core schema types (generated.ts) with the mutation and
// migration vocabulary needed by the MigrationEngine and related tooling.

import type {
  FieldDefinition,
  IndexDefinition,
  SchemaDefinition,
  Constraint,
} from "./generated.ts";

export type { SchemaDefinition } from "./generated.ts";

// ── Partial-update (patch) types ────────────────────────────────────────────

/**
 * Partial update semantics:
 * - `undefined` → no change
 * - `null` → clear/remove the property
 * - `value` → set to the provided value
 */
export type Patch<T, NonNullable extends keyof T = never> = {
  [K in keyof T]?: K extends NonNullable ? T[K] : T[K] | null;
};

export type FieldPatch = Patch<FieldDefinition, "name" | "type">;
export type ConstraintPatch = Patch<Constraint, "name">;
export type IndexPatch = Patch<IndexDefinition, "name">;

// ── Schema changes ──────────────────────────────────────────────────────────

export type SchemaChange =
  | { type: "modifyProperty"; id: keyof SchemaDefinition | "version"; value: unknown }
  | { type: "addField"; id: string; definition: FieldDefinition }
  | { type: "removeField"; id: string }
  | { type: "modifyField"; id: string; changes: FieldPatch }
  | { type: "addIndex"; id: string; definition: IndexDefinition }
  | { type: "removeIndex"; id: string }
  | { type: "modifyIndex"; id: string; changes: IndexPatch }
  | { type: "addConstraint"; id: string; constraint: Constraint }
  | { type: "removeConstraint"; id: string }
  | { type: "modifyConstraint"; id: string; changes: ConstraintPatch }
  | { type: "addSchema"; id: string; definition: NonNullable<SchemaDefinition["schemas"]>[string] }
  | { type: "removeSchema"; id: string }
  | { type: "modifySchema"; id: string; changes: SchemaChange[] };

// ── Data transforms ─────────────────────────────────────────────────────────

export type TransformFunction<Initial, Next> = (
  ctx: unknown,
  data: Initial,
) => Next | Promise<Next>;

export interface DataTransform<Initial = unknown, Next = unknown> {
  forward: TransformFunction<Initial, Next>;
  backward: TransformFunction<Next, Initial>;
}

// ── Migrations ──────────────────────────────────────────────────────────────

export interface Migration<Initial = unknown, Next = unknown> {
  id: string;
  schemaVersion: string;
  changes: SchemaChange[];
  description: string;
  status?: "pending" | "applied" | "rolled_back";
  rollback?: SchemaChange[];
  transform?: string | DataTransform<Initial, Next>;
  createdAt: string;
  dependencies?: string[];
  checksum: string;
}

// ── Schema change → JSON Patch (RFC 6902) ──────────────────────────────────

export interface PatchOp {
  op: "add" | "remove" | "replace" | "move" | "copy" | "test";
  path: string;
  value?: unknown;
  from?: string;
}

/**
 * Convert a `SchemaChange` into one or more RFC 6902 JSON Patch operations
 * that can be applied to a `SchemaDefinition` object.
 */
export function schemaChangeToPatch(
  change: SchemaChange,
  schema: SchemaDefinition,
): PatchOp[] {
  const patches: PatchOp[] = [];

  switch (change.type) {
    case "modifyProperty":
      patches.push({ op: "replace", path: `/${change.id}`, value: change.value });
      break;

    case "addField":
      patches.push({ op: "add", path: `/fields/${change.id}`, value: change.definition });
      break;

    case "removeField":
      patches.push({ op: "remove", path: `/fields/${change.id}` });
      break;

    case "modifyField":
      for (const [key, value] of Object.entries(change.changes)) {
        patches.push({ op: "replace", path: `/fields/${change.id}/${key}`, value });
      }
      break;

    case "addIndex":
      if (!schema.indexes) {
        patches.push({ op: "add", path: "/indexes", value: {} });
      }
      patches.push({ op: "add", path: `/indexes/${change.id}`, value: change.definition });
      break;

    case "removeIndex":
      patches.push({ op: "remove", path: `/indexes/${change.id}` });
      break;

    case "modifyIndex":
      for (const [key, value] of Object.entries(change.changes)) {
        patches.push({ op: "replace", path: `/indexes/${change.id}/${key}`, value });
      }
      break;

    case "addConstraint":
      if (!schema.constraints) {
        patches.push({ op: "add", path: "/constraints", value: {} });
      }
      patches.push({ op: "add", path: `/constraints/${change.id}`, value: change.constraint });
      break;

    case "removeConstraint":
      patches.push({ op: "remove", path: `/constraints/${change.id}` });
      break;

    case "modifyConstraint":
      for (const [key, value] of Object.entries(change.changes)) {
        patches.push({ op: "replace", path: `/constraints/${change.id}/${key}`, value });
      }
      break;

    case "addSchema":
      if (!schema.schemas) {
        patches.push({ op: "add", path: "/schemas", value: {} });
      }
      patches.push({ op: "add", path: `/schemas/${change.id}`, value: change.definition });
      break;

    case "removeSchema":
      patches.push({ op: "remove", path: `/schemas/${change.id}` });
      break;

    case "modifySchema":
      for (const child of change.changes) {
        patches.push(...schemaChangeToPatch(child, schema));
      }
      break;
  }

  return patches;
}

// ── Change-impact helpers ───────────────────────────────────────────────────

export type ChangeImpact = "major" | "minor" | "patch";

/**
 * Classify the semver impact of a single schema change.
 *
 * - `major`  — field/index/constraint removal, breaking field changes (type
 *              change, required added, unique added, nested schema changed)
 * - `minor`  — field/index/constraint addition, deprecation
 * - `patch`  — non-breaking field/constraint modifications
 */
export function classifyChangeImpact(
  change: SchemaChange,
  currentSchema?: SchemaDefinition,
): ChangeImpact {
  switch (change.type) {
    case "removeField":
    case "removeIndex":
    case "removeConstraint":
      return "major";

    case "modifyField": {
      const c = change.changes;
      if (c.required === true) return "major";
      if (c.type !== undefined) return "major";
      if (c.schema !== undefined) return "major";
      if (c.unique === true) return "major";
      if (c.deprecated) return "minor";
      return "patch";
    }

    case "modifyIndex": {
      const c = change.changes;
      if (c.unique !== undefined || c.fields !== undefined) return "major";
      return "minor";
    }

    case "modifyConstraint": {
      const old = currentSchema?.constraints?.[change.id];
      if (old && "predicate" in change.changes && change.changes.predicate !== undefined) return "major";
      if (old && "parameters" in change.changes && change.changes.parameters !== undefined) return "major";
      return "minor";
    }

    case "addField":
    case "addIndex":
    case "addConstraint":
    case "addSchema":
    case "deprecateField" as unknown:
      return "minor";

    case "modifySchema": {
      let worst: ChangeImpact = "patch";
      for (const child of change.changes) {
        const impact = classifyChangeImpact(child, currentSchema);
        if (impact === "major") return "major";
        if (impact === "minor") worst = "minor";
      }
      return worst;
    }

    default:
      return "patch";
  }
}

/**
 * Calculate the next semver version from a set of schema changes.
 *
 * Returns `"major"`, `"minor"`, or `"patch"` as the bump kind.
 * The caller applies it to the current version string.
 */
export function calculateNextBump(
  changes: SchemaChange[],
  currentSchema?: SchemaDefinition,
): ChangeImpact {
  let highest: ChangeImpact = "patch";
  for (const change of changes) {
    const impact = classifyChangeImpact(change, currentSchema);
    if (impact === "major") return "major";
    if (impact === "minor") highest = "minor";
  }
  return highest;
}

/**
 * Bump a semver string by the given kind.
 */
export function bumpVersion(
  current: string,
  kind: ChangeImpact,
): string {
  const match = current.replace(/^v/, "").match(/^(\d+)\.(\d+)\.(\d+)/);
  if (!match) throw new Error(`anansi: invalid version format "${current}"`);
  let [, maj, min, pat] = match.map(Number);
  switch (kind) {
    case "major": maj++; min = 0; pat = 0; break;
    case "minor": min++; pat = 0; break;
    case "patch": pat++; break;
  }
  return `${maj}.${min}.${pat}`;
}
