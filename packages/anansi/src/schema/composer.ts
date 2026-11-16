import type {
  SchemaDefinition,
  NestedSchemaDefinition,
  FieldDefinition,
  SchemaReference,
  InlineTypeDescriptor,
  IndexDefinition,
  Constraint,
  ConstraintUnion,
} from "./generated.ts";

type Descriptor = SchemaReference | InlineTypeDescriptor | SchemaReference[];

export interface ShapeMeta {
  name: string;
  version?: string;
  description?: string;
  metadata?: unknown;
}

export type ComposeMeta = ShapeMeta;

// ---------------------------------------------------------------------------
// Reachability
// ---------------------------------------------------------------------------

function refIds(r: Descriptor | undefined): string[] {
  if (!r) return [];
  const arr = Array.isArray(r) ? r : [r];
  return arr
    .filter((x): x is SchemaReference => "id" in x && typeof x.id === "string")
    .map((x) => x.id);
}

function reachableNested(
  roots: Iterable<string>,
  schemas: Record<string, NestedSchemaDefinition>,
): Set<string> {
  const seen = new Set<string>();
  const stack = [...roots];
  while (stack.length > 0) {
    const id = stack.pop()!;
    if (seen.has(id)) continue;
    seen.add(id);
    const n = schemas[id];
    if (!n) continue;
    for (const rid of refIds(n.schema as Descriptor | undefined)) stack.push(rid);
    if (n.fields) {
      for (const f of Object.values(n.fields)) {
        for (const rid of refIds(f.schema as Descriptor | undefined)) stack.push(rid);
      }
    }
  }
  return seen;
}

// ---------------------------------------------------------------------------
// Root-path extraction
// ---------------------------------------------------------------------------

function rootOf(path: string): string {
  const dot = path.indexOf(".");
  return dot === -1 ? path : path.slice(0, dot);
}

/** Every root field name referenced by any leaf rule inside a constraint. */
function constraintRoots(c: ConstraintUnion): Set<string> {
  const out = new Set<string>();
  const walk = (u: ConstraintUnion): void => {
    if ("rules" in u) {
      for (const r of u.rules) walk(r);
      return;
    }
    for (const p of u.fields ?? []) out.add(rootOf(p));
  };
  walk(c);
  return out;
}

// ---------------------------------------------------------------------------
// Atomicity enforcement
// ---------------------------------------------------------------------------

/**
 * Classify each top-level constraint against the survival predicate:
 *   - all referenced roots survive → keep
 *   - none survive                  → drop (whole scope gone)
 *   - some survive                  → throw
 */
function partitionConstraints(
  record: Record<string, Constraint> | undefined,
  survives: (root: string) => boolean,
  op: string,
): Record<string, Constraint> | undefined {
  if (!record) return undefined;
  const out: Record<string, Constraint> = {};

  for (const [id, c] of Object.entries(record)) {
    const roots = constraintRoots(c as ConstraintUnion);
    if (roots.size === 0) {
      out[id] = c; // schema-wide constraint, no field refs — always kept
      continue;
    }
    const keptRoots = [...roots].filter(survives);
    if (keptRoots.length === 0) continue; // drop — scope entirely gone
    if (keptRoots.length < roots.size) {
      const lost = [...roots].filter((r) => !survives(r));
      throw new Error(
        `${op}: constraint "${c.name}" is atomic over fields ` +
          `[${[...roots].join(", ")}], but this ${op} retains only ` +
          `[${keptRoots.join(", ")}] (loses [${lost.join(", ")}]). ` +
          `Pick all of the constraint's fields or none of them.`,
      );
    }
    out[id] = c;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/**
 * Indexes are atomic over their `fields` list. Single-field indexes are
 * naturally atomic; multi-field ones fail fast on partial survival.
 */
function partitionIndexes(
  record: Record<string, IndexDefinition> | undefined,
  survives: (root: string) => boolean,
  op: string,
): Record<string, IndexDefinition> | undefined {
  if (!record) return undefined;
  const out: Record<string, IndexDefinition> = {};

  for (const [id, item] of Object.entries(record)) {
    const roots = new Set(item.fields.map(rootOf));
    const keptRoots = [...roots].filter(survives);
    if (keptRoots.length === 0) continue;
    if (keptRoots.length < roots.size) {
      const lost = [...roots].filter((r) => !survives(r));
      throw new Error(
        `${op}: index "${item.name}" is atomic over fields ` +
          `[${[...roots].join(", ")}], but this ${op} retains only ` +
          `[${keptRoots.join(", ")}] (loses [${lost.join(", ")}]). ` +
          `Pick all of the index's fields or none of them.`,
      );
    }
    out[id] = item;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

// ---------------------------------------------------------------------------
// pick / omit / compose
// ---------------------------------------------------------------------------

export function pickSchema(
  schema: SchemaDefinition,
  meta: ShapeMeta,
  names: readonly string[],
): SchemaDefinition {
  const keep = new Set(names);
  const available = new Set(
    Object.values(schema.fields ?? {}).map((f) => f.name),
  );
  for (const name of keep) {
    if (!available.has(name)) {
      throw new Error(
        `pick: field "${name}" does not exist in schema "${schema.name}"`,
      );
    }
  }

  const fields: Record<string, FieldDefinition> = {};
  for (const [id, f] of Object.entries(schema.fields ?? {})) {
    if (keep.has(f.name)) fields[id] = f;
  }

  const roots: string[] = [];
  for (const f of Object.values(fields)) {
    roots.push(...refIds(f.schema as Descriptor | undefined));
  }
  const nestedIds = reachableNested(roots, schema.schemas ?? {});
  const schemas: Record<string, NestedSchemaDefinition> = {};
  for (const id of nestedIds) {
    const n = schema.schemas?.[id];
    if (n) schemas[id] = n;
  }

  const survives = (root: string): boolean => keep.has(root);

  // Atomicity: throw *before* constructing the output.
  const indexes = partitionIndexes(schema.indexes, survives, "pick");
  const constraints = partitionConstraints(schema.constraints, survives, "pick");

  const version = meta.version ?? schema.version;
  const description = meta.description ?? schema.description;
  const metadata = meta.metadata ?? schema.metadata;

  return {
    name: meta.name,
    version,
    ...(description !== undefined && { description }),
    ...(metadata !== undefined && { metadata }),
    fields,
    ...(Object.keys(schemas).length > 0 && { schemas }),
    ...(indexes && Object.keys(indexes).length > 0 && { indexes }),
    ...(constraints && Object.keys(constraints).length > 0 && { constraints }),
  };
}

export function omitSchema(
  schema: SchemaDefinition,
  meta: ShapeMeta,
  names: readonly string[],
): SchemaDefinition {
  const drop = new Set(names);
  const all = Object.values(schema.fields ?? {}).map((f) => f.name);
  for (const name of drop) {
    if (!all.includes(name)) {
      throw new Error(
        `omit: field "${name}" does not exist in schema "${schema.name}"`,
      );
    }
  }
  const keep = all.filter((name) => !drop.has(name));
  return pickSchema(schema, meta, keep);
}

export function composeSchemas(
  meta: ComposeMeta,
  sources: readonly SchemaDefinition[],
): SchemaDefinition {
  const fields: Record<string, FieldDefinition> = {};
  const schemas: Record<string, NestedSchemaDefinition> = {};
  const indexes: Record<string, IndexDefinition> = {};
  const constraints: Record<string, Constraint> = {};

  // name → { uuid, sourceName } of the field that owns that name.
  const owner = new Map<string, { uuid: string; source: string }>();

  const addAll = <T>(
    target: Record<string, T>,
    entries: Record<string, T> | undefined,
    kind: string,
    source: string,
  ): void => {
    for (const [id, value] of Object.entries(entries ?? {})) {
      if (Object.prototype.hasOwnProperty.call(target, id)) {
        throw new Error(
          `compose: ${kind} id "${id}" is declared more than once (source "${source}")`,
        );
      }
      target[id] = value;
    }
  };

  for (const src of sources) {
    for (const [id, f] of Object.entries(src.fields ?? {})) {
      const prior = owner.get(f.name);
      if (prior !== undefined) {
        throw new Error(
          `compose: field "${f.name}" declared by both "${prior.source}" ` +
            `and "${src.name}"`,
        );
      }
      if (Object.prototype.hasOwnProperty.call(fields, id)) {
        throw new Error(
          `compose: field id "${id}" is declared more than once (source "${src.name}")`,
        );
      }
      fields[id] = f;
      owner.set(f.name, { uuid: id, source: src.name });
    }
    addAll(schemas, src.schemas, "nested schema", src.name);
    addAll(indexes, src.indexes, "index", src.name);
    addAll(constraints, src.constraints, "constraint", src.name);
  }

  return {
    name: meta.name,
    version: meta.version ?? "1.0.0",
    ...(meta.description !== undefined && { description: meta.description }),
    ...(meta.metadata !== undefined && { metadata: meta.metadata }),
    fields,
    ...(Object.keys(schemas).length > 0 && { schemas }),
    ...(Object.keys(indexes).length > 0 && { indexes }),
    ...(Object.keys(constraints).length > 0 && { constraints }),
  };
}
