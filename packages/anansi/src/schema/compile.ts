// Compile: SchemaDefinition → resolved model (port of resolved_schema.go).
// Every field resolves to exactly one variant; recursion is detected; paths
// are threaded exactly like Go's basePath mechanism. Concrete
// (slotIdx,fieldIdx) coordinates are assigned by link.ts, which owns slot
// allocation — mirroring the Go split.

import { Literal } from "./types.ts";
import type {
  FieldDefinition,
  FieldType as MetaFieldType,
  InlineTypeDescriptor,
  SchemaDefinition,
  SchemaReference as SchemaRef,
} from "./generated.ts";

/** Canonical field-type union, straight from the meta schema. */
export type FieldType = MetaFieldType;

/** FieldDefinition plus Go-supported field-level enum `values`. */
export type FieldDef = FieldDefinition & { values?: unknown[] };

export interface ResolvedEnum {
  lookup: Map<string, unknown>;
  complex: unknown[];
  expectNumeric: boolean;
}

export interface ResolvedNested {
  id: string;
  name: string;
  effectiveType: FieldType;
  fields: ResolvedField[];
  isRecursive: boolean;
  values?: unknown[];
  enumDef?: ResolvedEnum;
}

export interface ResolvedObjectRef { schema: ResolvedNested }
export interface ResolvedContainer {
  itemSchema?: ResolvedNested;
  itemType?: FieldType;
  itemEnum?: ResolvedEnum;
  record: boolean;
}
export interface ResolvedUnion { variants: ResolvedNested[] }
export interface ResolvedComposite { objectParts: ResolvedNested[] }
export interface ResolvedRecursive { schemaId: string }

export type ResolvedKind =
  | { tag: "scalar" }
  | { tag: "enum"; enumDef: ResolvedEnum }
  | { tag: "object"; schema: ResolvedNested }
  | { tag: "container"; c: ResolvedContainer }
  | { tag: "union"; variants: ResolvedNested[] }
  | { tag: "composite"; parts: ResolvedNested[] }
  | { tag: "recursive"; schemaId: string };

export interface ResolvedField {
  id: string;
  name: string;
  path: string;
  type: FieldType;
  required: boolean;
  deprecated: boolean;
  unique: boolean;
  nullable: boolean;
  hasDefault: boolean;

  kind: ResolvedKind;
}

function isSingleRef(s: FieldDef["schema"]): s is SchemaRef {
  return !!s && !Array.isArray(s) && typeof (s as SchemaRef).id === "string";
}
function isMultiRef(s: FieldDef["schema"]): s is SchemaRef[] {
  return Array.isArray(s);
}
function isInline(s: FieldDef["schema"]): s is InlineTypeDescriptor {
  return !!s && !Array.isArray(s) && typeof (s as SchemaRef).id !== "string";
}
function joinPath(prefix: string, name: string): string {
  return prefix ? `${prefix}.${name}` : name;
}

export class Compiler {
  private readonly nested = new Map<string, ResolvedNested>();
  private readonly building = new Set<string>();

  constructor(public readonly source: SchemaDefinition) {}

  compile(): { root: ResolvedField[]; schemas: Map<string, ResolvedNested> } {
    for (const id of Object.keys(this.source.schemas ?? {})) {
      this.compileNested(id);
    }
    const root = this.compileFields(this.source.fields ?? {}, [], "");
    return { root, schemas: this.nested };
  }

  private compileNested(id: string): ResolvedNested | null {
    const cached = this.nested.get(id);
    if (cached) return cached;
    if (this.building.has(id)) return null; // cycle → recursive at caller

    const def = (this.source.schemas ?? {})[id];
    if (!def) throw new Error(`anansi: schema '${id}' not found`);

    this.building.add(id);
    try {
      const effectiveType: FieldType =
        def.values && def.values.length > 0 ? "enum" : (def.type ?? "unknown");
      const rns: ResolvedNested = {
        id,
        name: def.name,
        effectiveType,
        fields: [],
        isRecursive: false,
        values: def.values,
        enumDef:
          effectiveType === "enum" ? buildEnum(def.values ?? [], def.type) : undefined,
      };
      this.nested.set(id, rns);

      if (def.fields) {
        rns.fields = this.compileFields(def.fields, [id], "");
      }
      rns.isRecursive = rns.fields.some((f) => f.kind.tag === "recursive");
      return rns;
    } finally {
      this.building.delete(id);
    }
  }

  private compileFields(
    fields: Record<string, FieldDef>,
    scope: string[],
    pathPrefix: string,
  ): ResolvedField[] {
    const out: ResolvedField[] = [];
    for (const [fid, f] of Object.entries(fields)) {
      out.push(this.compileField(fid, f, scope, pathPrefix));
    }
    return out;
  }

  private compileField(
    fid: string,
    f: FieldDef,
    scope: string[],
    pathPrefix: string,
  ): ResolvedField {
    const type: FieldType = f.type ?? "unknown";
    const rf: ResolvedField = {
      id: fid,
      name: f.name,
      path: joinPath(pathPrefix, f.name),
      type,
      required: !!f.required,
      deprecated: !!f.deprecated,
      unique: !!f.unique,
      nullable: f.nullable !== false,
      hasDefault: !Literal.fromJSON(f.default).isZero(),
      kind: { tag: "scalar" },
    };

    const ref = f.schema;

    switch (type) {
      case "geometry":
        break; // terminal scalar-ish; classify maps to TypeGeometry

      case "record": {
        if (isSingleRef(ref)) {
          const target = this.resolve(ref.id);
          rf.kind = { tag: "container", c: { itemSchema: target ?? undefined, record: true } };
        } else {
          rf.kind = { tag: "container", c: { record: true } };
        }
        break;
      }

      case "array": {
        if (isMultiRef(ref)) {
          throw new Error(`anansi: ${rf.path}: array cannot reference multiple schemas`);
        }
        if (isSingleRef(ref)) {
          const target = this.require(ref.id, rf.path);
          rf.kind = { tag: "container", c: { itemSchema: target, record: false } };
        } else if (isInline(ref)) {
          const itemType = (ref.type ?? "unknown") as FieldType;
          const c: ResolvedContainer = { itemType, record: false };
          if ((ref.values?.length ?? 0) > 0) {
            c.itemEnum = buildEnum(ref.values!, itemType);
          }
          rf.kind = { tag: "container", c };
        } else {
          rf.kind = { tag: "container", c: { record: false } };
        }
        break;
      }

      case "object": {
        if (isSingleRef(ref)) {
          rf.kind = { tag: "object", schema: this.require(ref.id, rf.path) };
        } else if (isInline(ref)) {
          rf.kind = { tag: "object", schema: this.synthesizeInline(ref, fid, scope) };
        } else {
          // Bare object without shape: behaves like an open record value.
          rf.kind = { tag: "scalar" };
        }
        break;
      }

      case "union": {
        if (isMultiRef(ref)) {
          const variants: ResolvedNested[] = [];
          for (const r of ref) variants.push(this.require(r.id, rf.path));
          rf.kind = { tag: "union", variants };
        } else if (isSingleRef(ref)) {
          rf.kind = { tag: "object", schema: this.require(ref.id, rf.path) };
        } else {
          rf.kind = { tag: "scalar" };
        }
        break;
      }

      case "composite": {
        if (isMultiRef(ref)) {
          const parts: ResolvedNested[] = [];
          for (const r of ref) parts.push(this.require(r.id, rf.path));
          rf.kind = { tag: "composite", parts };
        } else {
          rf.kind = { tag: "scalar" };
        }
        break;
      }

      case "enum": {
        if ((f.values?.length ?? 0) > 0) {
          rf.kind = { tag: "enum", enumDef: buildEnum((f as FieldDef).values!, type) };
        } else if (isInline(ref) && (ref.values?.length ?? 0) > 0) {
          rf.kind = { tag: "enum", enumDef: buildEnum(ref.values!, (ref.type ?? "string") as FieldType) };
        } else if (isSingleRef(ref)) {
          const target = this.resolve(ref.id);
          rf.kind =
            target && target.enumDef
              ? { tag: "enum", enumDef: target.enumDef }
              : { tag: "scalar" };
        } else {
          rf.kind = { tag: "scalar" };
        }
        break;
      }

      default: {
        // Scalars / bytes / unknown — possibly referencing an enum schema.
        if (isSingleRef(ref)) {
          const target = this.resolve(ref.id);
          if (target === null) {
            rf.kind = { tag: "recursive", schemaId: ref.id };
          } else if (target.enumDef) {
            rf.kind = { tag: "enum", enumDef: target.enumDef };
          } else {
            rf.kind = { tag: "scalar" };
          }
        }
        break;
      }
    }

    return rf;
  }

  private resolve(id: string): ResolvedNested | null {
    if (this.nested.has(id)) return this.nested.get(id)!;
    if (this.building.has(id)) return null;
    return this.compileNested(id);
  }

  private require(id: string, at: string): ResolvedNested {
    const n = this.resolve(id);
    if (!n) throw new Error(`anansi: ${at}: unresolvable schema reference '${id}'`);
    return n;
  }

  private synthesizeInline(inline: InlineTypeDescriptor, fid: string, scope: string[]): ResolvedNested {
    const synthId = `inline:${[...scope, fid].join("/")}`;
    const existing = this.nested.get(synthId);
    if (existing) return existing;

    const rns: ResolvedNested = {
      id: synthId,
      name: synthId,
      effectiveType: (inline.type ?? "object") as FieldType,
      fields: [],
      isRecursive: false,
    };
    this.nested.set(synthId, rns);
    const fields = (inline as { fields?: Record<string, FieldDef> }).fields ?? {};
    rns.fields = this.compileFields(fields, [...scope, synthId], "");
    return rns;
  }
}

export function buildEnum(values: unknown[], t?: FieldType): ResolvedEnum {
  const lookup = new Map<string, unknown>();
  const complex: unknown[] = [];
  for (const v of values) {
    if (v === null || ["string", "number", "boolean"].includes(typeof v)) {
      lookup.set(JSON.stringify(v), v);
    } else {
      complex.push(v);
    }
  }
  return { lookup, complex, expectNumeric: t === "integer" || t === "number" };
}
