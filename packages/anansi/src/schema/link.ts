// Link: resolved model → flat slot/descriptor tables (port of link.go +
// compiled.go's descriptor layout and address.go's space).

import { container, TYPE_NAMES } from "./dt.ts";
import { Compiler, ResolvedField, ResolvedNested, FieldType, ResolvedEnum } from "./compile.ts";
import { SchemaDefinition } from "./types.ts";

// ── constants (must match Go exactly) ────────────────────────────────────────
export const FD_NO_CHILD = 0x3f;
export const MAX_FIELDS_PER_SCHEMA = 128;
export const MAX_SCHEMA_SLOTS = 63; // FdNoChild
export const ADDR_BITS = 27;
export const SINGLE_STEP_REGION = 1 << 14;
export const MULTI_STEP_BASE = SINGLE_STEP_REGION;

export type FieldKind = 0 | 1 | 2 | 3; // Simple, Object, ArrayField, Complex

export interface FieldDescriptor {
  raw: number; // uint32
  dt: number;
  kind: FieldKind;
  schemaIdx: number;
  fieldIdx: number;
  childSchemaIdx: number;
  terminal: boolean;
  required: boolean;
  hasDefault: boolean;
  deprecated: boolean;
  unique: boolean;
  nullable: boolean;
  recursive: boolean;
}

/** Internal DataPoint (side-table identity) — compiled.go:186 formula. */
export function internalDP(fd: FieldDescriptor): number {
  return ((fd.raw & 0xffffffe0) | (((fd.raw >>> 28) & 0xf) << 1)) >>> 0;
}

export function makeDescriptor(
  dt: number,
  kind: FieldKind,
  schemaIdx: number,
  fieldIdx: number,
  o: {
    required: boolean; hasDefault: boolean; deprecated: boolean; unique: boolean;
    terminal: boolean; nullable: boolean; recursive: boolean; child: number;
  },
): FieldDescriptor {
  let fd = 0;
  fd |= (dt & 0xf) << 28;
  fd |= (schemaIdx & 0x3f) << 22;
  fd |= (fieldIdx & 0x7f) << 15;
  fd |= (o.child & 0x3f) << 9;
  fd |= (kind & 0x3) << 7;
  if (o.required) fd |= 1 << 6;
  if (o.hasDefault) fd |= 1 << 5;
  if (o.deprecated) fd |= 1 << 4;
  if (o.unique) fd |= 1 << 3;
  if (o.terminal) fd |= 1 << 2;
  if (o.nullable) fd |= 1 << 1;
  if (o.recursive) fd |= 1 << 0;
  return unpackDescriptor(fd >>> 0);
}

export function unpackDescriptor(raw: number): FieldDescriptor {
  return {
    raw,
    dt: (raw >>> 28) & 0xf,
    schemaIdx: (raw >>> 22) & 0x3f,
    fieldIdx: (raw >>> 15) & 0x7f,
    childSchemaIdx: (raw >>> 9) & 0x3f,
    kind: ((raw >>> 7) & 0x3) as FieldKind,
    required: !!(raw & 1 << 6),
    hasDefault: !!(raw & 1 << 5),
    deprecated: !!(raw & 1 << 4),
    unique: !!(raw & 1 << 3),
    terminal: !!(raw & 1 << 2),
    nullable: !!(raw & 1 << 1),
    recursive: !!(raw & 1),
  };
}

export interface Slot {
  fieldStart: number;
  fieldCount: number;
  footprint: number;
}

export interface FieldMeta {
  name: string;
  path: string;
}

/** A linked field: everything the wire codec needs. */
export interface LinkedField {
  meta: FieldMeta;
  fd: FieldDescriptor;
  /** Canonical user-data DataPoint written by the Sparse format. */
  dp: number;
  /** Absolute descriptor index across all slots. */
  abs: number;
  /** For array_object fields: the linked child slot's fields. */
  child?: LinkedSlot;
}

export interface LinkedSlot {
  idx: number;
  slot: Slot;
  fields: LinkedField[];
}

function scalarDataType(ft: FieldType): number {
  switch (ft) {
    case "string": return container.TypeString;
    case "number": return container.TypeFloat;
    case "decimal": return container.TypeString; // canonical decimal string
    case "integer": return container.TypeInt;
    case "boolean": return container.TypeBool;
    case "bytes": return container.TypeBytes;
    default: return container.TypeUnknown;
  }
}

function enumDataType(e?: ResolvedEnum): number {
  return e?.expectNumeric ? container.TypeInt : container.TypeString;
}

function containerDataType(itemSchema: ResolvedNested | undefined, itemType: FieldType | undefined): number {
  if (itemSchema) return container.TypeArrayObject;
  switch (itemType) {
    case "string": return container.TypeArrayString;
    case "number": return container.TypeArrayFloat;
    case "decimal": return container.TypeArrayString;
    case "integer": return container.TypeArrayInt;
    case "boolean": return container.TypeArrayBool;
    case "bytes": return container.TypeArrayBytes;
    case "geometry": return container.TypeArrayGeometry;
    case "unknown": return container.TypeArrayUnknown;
    default: return container.TypeUnknown;
  }
}

/** classifyField port: returns [dataType, kind, terminal]. */
function classify(rf: ResolvedField): [number, FieldKind, boolean] {
  const k = rf.kind;
  if (rf.type === "geometry") return [container.TypeGeometry, 0, true];
  if (k.tag === "scalar") return [scalarDataType(rf.type), 0, true];
  if (k.tag === "enum") return [enumDataType(k.enumDef), 0, true];
  if (k.tag === "recursive") return [container.TypeUnknown, 1, true];
  if (k.tag === "object") return [container.TypeUnknown, 1, false];
  if (k.tag === "container") {
    const terminal = !k.c.itemSchema;
    if (k.c.record) return [container.TypeRecord, 1, terminal];
    return [containerDataType(k.c.itemSchema, k.c.itemType), 2, terminal];
  }
  if (k.tag === "union") return [container.TypeUnknown, 3, false];
  // composite collapses like an object
  return [container.TypeUnknown, 1, false];
}

export interface LinkResult {
  slots: Slot[];
  metas: FieldMeta[]; // parallel to descriptors
  descriptors: FieldDescriptor[];
  localOffsets: number[];
  fieldTypes: FieldType[];
  root: LinkedSlot;
  /** abs index of each root-level array_object field's linked child */
  childrenByPath: Map<string, LinkedSlot>;
}

class Linker {
  readonly slots: Slot[] = [];
  readonly descriptors: FieldDescriptor[] = [];
  readonly metas: FieldMeta[] = [];
  readonly localOffsets: number[] = [];
  readonly fieldTypes: FieldType[] = [];
  readonly linkedSlots: LinkedSlot[] = [];

  assignSlot(rns: ResolvedNested): number {
    if (this.slots.length >= MAX_SCHEMA_SLOTS) {
      throw new Error(`anansi: exceeds maximum of ${MAX_SCHEMA_SLOTS} nested schemas while assigning '${rns.name}'`);
    }
    const idx = this.slots.length;
    this.slots.push({ fieldStart: -1, fieldCount: -1, footprint: -1 });
    this.linkedSlots.push({ idx, slot: this.slots[idx], fields: [] });
    return idx;
  }

  childSlotForField(rf: ResolvedField, schemaIdx: number): { idx: number; kindTag: ResolvedField["kind"] } | null {
    const k = rf.kind;
    if (k.tag === "recursive") return { idx: schemaIdx, kindTag: k };
    if (k.tag === "object") return { idx: this.assignSlot(k.schema), kindTag: k };
    if (k.tag === "composite") {
      const idx = this.assignSlot({
        id: `composite:${rf.path}`, name: `composite:${rf.name}`,
        effectiveType: "object", fields: mergeParts(k.parts), isRecursive: false,
      });
      return { idx, kindTag: k };
    }
    if (k.tag === "container" && k.c.itemSchema) {
      return { idx: this.assignSlot(k.c.itemSchema), kindTag: k };
    }
    return null;
  }

  linkFields(fields: ResolvedField[], schemaIdx: number): { count: number; slot: LinkedSlot } {
    const slotRef = this.linkedSlots[schemaIdx];
    const children: { rf: ResolvedField; fd: FieldDescriptor; childIdx: number }[] = [];

    for (let i = 0; i < fields.length; i++) {
      if (fields.length > MAX_FIELDS_PER_SCHEMA) {
        throw new Error(`anansi: slot ${schemaIdx} exceeds ${MAX_FIELDS_PER_SCHEMA} fields`);
      }
      const rf = fields[i];
      const [dt, kind, terminal] = classify(rf);
      const cs = this.childSlotForField(rf, schemaIdx);
      const childIdx = cs ? cs.idx : FD_NO_CHILD;
      const recursive = rf.kind.tag === "recursive";

      const fd = makeDescriptor(dt, kind, schemaIdx, i, {
        required: rf.required,
        hasDefault: rf.hasDefault,
        deprecated: rf.deprecated,
        unique: rf.unique,
        terminal,
        nullable: rf.nullable,
        recursive,
        child: childIdx,
      });

      const abs = this.descriptors.length;
      this.descriptors.push(fd);
      this.fieldTypes.push(rf.type);
      const lf: LinkedField = {
        meta: { name: rf.name, path: rf.path },
        fd,
        dp: 0, // finalized after offsets
        abs,
      };
      slotRef.fields.push(lf);
      this.metas.push(lf.meta);

      if (!terminal && childIdx !== FD_NO_CHILD) {
        children.push({ rf, fd, childIdx });
      }
    }

    // Pass 2: link non-terminal children.
    for (const c of children) {
      const k = c.rf.kind;
      if (k.tag === "object" || k.tag === "composite") {
        const src = k.tag === "object" ? k.schema.fields : mergeParts(k.parts);
        const childSlot = this.linkFields(src, c.childIdx);
        this.setSlot(c.childIdx, childSlot.count);
      } else if (k.tag === "container" && k.c.itemSchema) {
        const childSlot = this.linkFields(k.c.itemSchema.fields, c.childIdx);
        this.setSlot(c.childIdx, childSlot.count);
      }
      // Attach LinkedSlot reference onto the parent LinkedField.
      const parentField = slotRef.fields.find((f) => f.fd === c.fd)!;
      parentField.child = this.linkedSlots[c.childIdx];
    }

    return { count: fields.length, slot: slotRef };
  }

  private setSlot(idx: number, count: number) {
    const s = this.slots[idx];
    if (s.fieldStart === -1) s.fieldStart = this.descriptors.length - count;
    s.fieldCount = count;
  }
}

function mergeParts(parts: ResolvedNested[]): ResolvedField[] {
  const out: ResolvedField[] = [];
  for (const p of parts) out.push(...p.fields);
  return out;
}

/** Finalize footprints (bottom-up), LocalOffsets and per-field DataPoints. */
function finalize(l: Linker): void {
  // Footprints bottom-up (slots are indexed DFS: parent before child).
  for (let i = l.slots.length - 1; i >= 0; i--) {
    const s = l.slots[i];
    if (s.fieldCount < 0) { s.fieldStart = 0; s.fieldCount = 0; }
    let fp = 0;
    for (let j = 0; j < s.fieldCount; j++) {
      const abs = s.fieldStart + j;
      const fd = l.descriptors[abs];
      if (fd.terminal) fp += 1;
      else if (fd.childSchemaIdx !== FD_NO_CHILD) fp += l.slots[fd.childSchemaIdx].footprint;
    }
    s.footprint = fp;
  }

  // LocalOffsets: prefix sums within each slot.
  for (const s of l.slots) {
    let acc = 0;
    for (let j = 0; j < s.fieldCount; j++) {
      const abs = s.fieldStart + j;
      const fd = l.descriptors[abs];
      l.localOffsets[abs] = acc;
      if (fd.terminal) acc += 1;
      else if (fd.childSchemaIdx !== FD_NO_CHILD) acc += l.slots[fd.childSchemaIdx].footprint;
    }
  }
}

/**
 * Address computation (address.go computeAddress).
 * steps = [(schemaIdx,fieldIdx), ...] with the LAST step being the leaf.
 */
export function addressForSteps(
  slots: Slot[],
  descriptors: FieldDescriptor[],
  localOffsets: number[],
  steps: Array<[number, number]>,
): number {
  if (steps.length === 0) return 0;
  if (steps.length === 1) {
    const [si, fi] = steps[0];
    const abs = slots[si].fieldStart + fi;
    if (!descriptors[abs].terminal) return 0;
    return (abs + 1) >>> 0; // +1 keeps 0 reserved
  }
  let base = MULTI_STEP_BASE;
  for (let i = 0; i < steps.length; i++) {
    const [si, fi] = steps[i];
    const abs = slots[si].fieldStart + fi;
    const fd = descriptors[abs];
    base += localOffsets[abs];
    if (i === steps.length - 1) {
      if (!fd.terminal) return 0;
      return base >>> 0;
    }
    if (fd.terminal) return 0;
  }
  return base >>> 0;
}

/** Canonical Sparse DataPoint for an addressed leaf. */
export function userDataDP(dt: number, addr: number): number {
  return ((addr << 5) | (dt << 1)) >>> 0;
}

export function link(source: SchemaDefinition): LinkResult {
  const compiler = new Compiler(source);
  const { root: rootFields, schemas } = compiler.compile();
  void schemas;

  const l = new Linker();
  // Root slot.
  l.slots.push({ fieldStart: -1, fieldCount: -1, footprint: -1 });
  l.linkedSlots.push({ idx: 0, slot: l.slots[0], fields: [] });
  const rootLinked = l.linkFields(rootFields, 0);
  l.slots[0].fieldStart = 0;
  l.slots[0].fieldCount = rootLinked.count;

  finalize(l);

  // Second pass: attach multi-step dps for flattened leaves. Root-level
  // single-step dps were set in finalize(); children (flattened objects and
  // array elements) need their full step chains.
  assignChildDPs(l, l.linkedSlots[0], []);

  return {
    slots: l.slots,
    metas: l.metas,
    descriptors: l.descriptors,
    localOffsets: l.localOffsets,
    fieldTypes: l.fieldTypes,
    root: l.linkedSlots[0],
    childrenByPath: collectChildren(l.linkedSlots[0]),
  };
}

function collectChildren(root: LinkedSlot): Map<string, LinkedSlot> {
  const m = new Map<string, LinkedSlot>();
  for (const f of root.fields) {
    if (f.child) m.set(f.meta.path, f.child);
  }
  return m;
}

/**
 * Recurse assigning canonical Sparse DataPoints.
 *
 * Mirrors Go's computeLeafKey: every terminal resolves its user-data
 * address over the accumulated mount steps (the walk always carries at
 * least the field's own step, so Address() takes the single- or multi-step
 * branch); TypeArrayObject fields key by their INTERNAL DataPoint.
 */
function assignChildDPs(l: Linker, ls: LinkedSlot, prefix: Array<[number, number]>): void {
  for (let j = 0; j < ls.slot.fieldCount; j++) {
    const abs = ls.slot.fieldStart + j;
    const fd = l.descriptors[abs];
    const lf = ls.fields[j];
    const steps: Array<[number, number]> = [...prefix, [ls.idx, j]];

    // computeLeafKey semantics: resolve the user-data address; when it is
    // 0 (non-terminal, or genuinely unaddressable) fall back to the
    // descriptor's INTERNAL DataPoint.
    const addr = addressForSteps(l.slots, l.descriptors, l.localOffsets, steps);
    lf.dp = addr === 0 ? internalDP(fd) : userDataDP(fd.dt, addr);

    if (!fd.terminal && lf.child) {
      assignChildDPs(l, lf.child, steps);
    }
  }
}

// ── MANIFEST ─────────────────────────────────────────────────────────────────

export interface ManifestField {
  /** Field's declared name (relative to its own slot). */
  name: string;
  /** Full dotted mount path from the document root ("address.street"). */
  path: string;
  /** container.DataType name tag. */
  t: string;
  /** Canonical Sparse DataPoint. */
  dp: number;
  /** TypeArrayObject element fields (full mount paths). */
  child?: ManifestField[];
}

/** Build the language-neutral wire manifest from linked tables. */
export function buildManifest(l: LinkResult): ManifestField[] {
  const emit = (ls: LinkedSlot, prefix: Array<[number, number]>, dot: string): ManifestField[] => {
    const out: ManifestField[] = [];
    for (let j = 0; j < ls.slot.fieldCount; j++) {
      const lf = ls.fields[j];
      const fd = lf.fd;
      const steps: Array<[number, number]> = [...prefix, [ls.idx, j]];
      const full = dot ? `${dot}.${lf.meta.name}` : lf.meta.name;
      const entry: ManifestField = {
        name: lf.meta.name,
        path: full,
        t: TYPE_NAMES[fd.dt],
        dp: lf.dp,
      };
      if (!fd.terminal && lf.child) {
        if (fd.dt === container.TypeArrayObject) {
          entry.child = emit(lf.child, steps, full);
          out.push(entry);
        } else {
          out.push(...emit(lf.child, steps, full));
        }
        continue;
      }
      out.push(entry);
    }
    return out;
  };
  return emit(l.root, [], "");
}
