/**
 * utils.ts
 *
 * Pure helper functions used throughout the validator.
 * Mirrors the Go utilities in validator.go and the `utils` package.
 */

import type { ValidationContext } from "./types";

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

/**
 * Joins `basePath` and `fieldName` with a dot separator.
 * If `basePath` is empty, returns `fieldName` directly (no leading dot).
 */
export function buildPath(basePath: string, fieldName: string): string {
  if (basePath === "") return fieldName;
  return `${basePath}.${fieldName}`;
}

/**
 * Builds both the dot-separated path string and the pre-split parts array
 * in one pass, avoiding redundant work in hot loops.
 */
export function buildPathAndParts(
  basePath: string,
  baseParts: string[],
  fieldName: string,
): [path: string, parts: string[]] {
  if (basePath === "") {
    return [fieldName, [fieldName]];
  }
  return [`${basePath}.${fieldName}`, [...baseParts, fieldName]];
}

/**
 * Splits a dot-separated path into its component parts.
 * Returns an empty array for an empty path (matches Go behaviour).
 */
export function splitPath(path: string): string[] {
  if (path === "") return [];
  return path.split(".");
}

/**
 * Returns the "nesting depth" of a path, counting both dot-separated
 * segments and array-index brackets (`[`).  Mirrors Go's `getPathDepth`.
 */
export function getPathDepth(pathParts: string[]): number {
  let depth = pathParts.length;
  for (const part of pathParts) {
    for (const ch of part) {
      if (ch === "[") depth++;
    }
  }
  return depth;
}

/**
 * Converts relative field name paths to absolute paths by prepending `basePath`.
 * Returns a parallel pair of [absolutePaths, absolutePathParts].
 */
export function resolveConstraintFieldPaths(
  basePath: string,
  baseParts: string[],
  fieldNames: string[],
): [paths: string[], parts: string[][]] {
  const paths: string[] = [];
  const parts: string[][] = [];

  for (const fieldName of fieldNames) {
    const fieldParts = splitPath(fieldName);

    if (basePath === "") {
      paths.push(fieldName);
      parts.push(fieldParts);
    } else {
      paths.push(buildPath(basePath, fieldName));
      parts.push([...baseParts, ...fieldParts]);
    }
  }

  return [paths, parts];
}

// ---------------------------------------------------------------------------
// Data traversal
// ---------------------------------------------------------------------------

/**
 * Retrieves the value at `pathParts` from `root`, returning `[value, true]`
 * on success and `[undefined, false]` when the path does not exist.
 *
 * Supports both dot-notation segments and array-index notation (`field[0]`).
 * Mirrors Go's `utils.GetValueByParts`.
 */
export function getValueByParts(
  root: unknown,
  pathParts: string[],
): [value: unknown, exists: boolean] {
  if (pathParts.length === 0) {
    return [root, true];
  }

  let current: unknown = root;

  for (const part of pathParts) {
    if (current === null || current === undefined) {
      return [undefined, false];
    }

    // Handle array-index notation: "field[0]" or "[0]"
    const bracketIdx = part.indexOf("[");

    if (bracketIdx === -1) {
      // Plain key
      if (typeof current !== "object" || Array.isArray(current)) {
        return [undefined, false];
      }
      const map = current as Record<string, unknown>;
      if (!(part in map)) {
        return [undefined, false];
      }
      current = map[part];
    } else {
      // Key + one or more bracket indices: "tags[0]" or "[0]"
      const key = part.slice(0, bracketIdx);

      if (key !== "") {
        if (typeof current !== "object" || Array.isArray(current)) {
          return [undefined, false];
        }
        const map = current as Record<string, unknown>;
        if (!(key in map)) {
          return [undefined, false];
        }
        current = map[key];
      }

      // Extract all bracket indices
      const bracketPart = part.slice(bracketIdx);
      const indexMatches = bracketPart.matchAll(/\[(\d+)\]/g);
      for (const match of indexMatches) {
        const rawIdx = match[1];
        if (rawIdx === undefined) return [undefined, false];
        const idx = parseInt(rawIdx, 10);
        if (!Array.isArray(current) || idx >= (current as unknown[]).length) {
          return [undefined, false];
        }
        current = (current as unknown[])[idx];
      }
    }
  }

  return [current, true];
}

/**
 * Convenience wrapper that reads from `ctx.rootData`.
 */
export function getNodeValue(
  ctx: ValidationContext,
  pathParts: string[],
): [value: unknown, exists: boolean] {
  return getValueByParts(ctx.rootData, pathParts);
}

// ---------------------------------------------------------------------------
// Type guards
// ---------------------------------------------------------------------------

/**
 * Returns the value cast as `Record<string, unknown>` if it is a plain
 * (non-array) object, or `null` otherwise.
 * Mirrors Go's `utils.GetMapStringAny`.
 */
export function getMapStringAny(
  value: unknown,
): Record<string, unknown> | null {
  if (
    value !== null &&
    value !== undefined &&
    typeof value === "object" &&
    !Array.isArray(value)
  ) {
    return value as Record<string, unknown>;
  }
  return null;
}

// ---------------------------------------------------------------------------
// Deep equality & comparability
// ---------------------------------------------------------------------------

/**
 * Returns `true` if `v` can be stored as a Map key and compared by value
 * identity — i.e. it is a primitive (string, number, boolean, null, undefined,
 * bigint, symbol) rather than a reference type.
 * Mirrors Go's `isSafeComparable`.
 */
export function isSafeComparable(v: unknown): boolean {
  if (v === null || v === undefined) return true;
  const t = typeof v;
  return (
    t === "string" ||
    t === "number" ||
    t === "boolean" ||
    t === "bigint" ||
    t === "symbol"
  );
}

/**
 * Recursive structural equality, with optional numeric-equivalence mode.
 *
 * `numericEquivalent = true` means that `1` (integer-like) and `1.0`
 * (float-like) compare as equal, mirroring Go's cross-numeric-type comparison.
 * In standard JSON, all numbers are already `number`, so this is mainly needed
 * for enum value matching where values came from heterogeneous sources.
 */
export function deepEqual(
  a: unknown,
  b: unknown,
  numericEquivalent = false,
): boolean {
  if (a === b) return true;
  if (a === null || b === null) return a === b;
  if (a === undefined || b === undefined) return a === b;

  const ta = typeof a;
  const tb = typeof b;

  // Primitive fast paths
  if (ta === "string" && tb === "string") return a === b;
  if (ta === "boolean" && tb === "boolean") return a === b;

  // Numeric
  if (ta === "number" && tb === "number") {
    if (numericEquivalent) {
      // Allow int-like vs float-like equivalence (e.g. 1 === 1.0)
      return (a as number) === (b as number);
    }
    return a === b;
  }

  // Cross-numeric when numericEquivalent is set
  if (numericEquivalent && ta === "number" && tb === "number") {
    return (a as number) === (b as number);
  }

  if (ta !== "object" || tb !== "object") return false;

  // Arrays
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
      if (!deepEqual(a[i], b[i], numericEquivalent)) return false;
    }
    return true;
  }

  if (Array.isArray(a) !== Array.isArray(b)) return false;

  // Plain objects
  const oa = a as Record<string, unknown>;
  const ob = b as Record<string, unknown>;
  const keysA = Object.keys(oa);
  const keysB = Object.keys(ob);
  if (keysA.length !== keysB.length) return false;
  for (const k of keysA) {
    if (!(k in ob)) return false;
    if (!deepEqual(oa[k], ob[k], numericEquivalent)) return false;
  }
  return true;
}

// ---------------------------------------------------------------------------
// FieldType numeric helpers (JSON-safe)
// ---------------------------------------------------------------------------

/**
 * Returns true if `value` is a valid `integer` in the JSON sense:
 * a `number` with no fractional component.
 */
export function isInteger(value: unknown): boolean {
  return typeof value === "number" && Number.isInteger(value);
}

/**
 * Returns true if `value` is a valid `decimal` in the JSON sense:
 * a `number` *with* a fractional component (i.e. not an integer).
 */
export function isDecimal(value: unknown): boolean {
  return typeof value === "number" && !Number.isInteger(value);
}

/**
 * Returns true if `value` is any `number` (covers both integer and decimal).
 */
export function isNumber(value: unknown): boolean {
  return typeof value === "number";
}
