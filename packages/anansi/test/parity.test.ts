// Parity: the TypeScript compile+link pipeline must reproduce Go's linker
// internals field-for-field (golden.json → compileDump). This is the
// referee for the entire port — packet tests mean nothing without it.

import { describe, it, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { link, addressForSteps, internalDP } from "../src/schema/link.ts";
import type { LinkedSlot } from "../src/schema/link.ts";

interface DumpField {
  slot: number; idx: number; path: string; name: string;
  dt: number; kind: number; terminal: boolean;
  dp: number; descriptor: number; localOffset: number; address: number;
}
interface Dump {
  version: number;
  slots: { fieldStart: number; fieldCount: number; footprint: number }[];
  fields: DumpField[];
}

const GOLDEN = JSON.parse(
  readFileSync(join(import.meta.dir, "..", "testdata", "golden.json"), "utf8"),
) as {
  schema: Record<string, unknown>;
  compileDump: Dump;
};

describe("compile/link parity vs Go", () => {
  const dump = GOLDEN.compileDump;
  const linked = link(GOLDEN.schema as never);

  it("reproduces the slot table", () => {
    expect(linked.slots.length).toBe(dump.slots.length);
    dump.slots.forEach((s, i) => {
      expect([
        linked.slots[i].fieldStart,
        linked.slots[i].fieldCount,
        linked.slots[i].footprint,
      ]).toEqual([s.fieldStart, s.fieldCount, s.footprint]);
    });
  });

  it("reproduces every field bit-for-bit in Go's absolute order", () => {
    const flat: DumpField[] = [];
    emitSlot(linked, linked.root, [], flat);

    const diffs: string[] = [];
    const cmp = (i: number, got: DumpField | undefined, want: DumpField): void => {
      if (!got) { diffs.push(`#${i}: missing, want ${want.path}`); return; }
      const fields = ["slot","idx","name","path","dt","kind","terminal","dp","descriptor","localOffset"] as const;
      for (const k of fields) {
        if (JSON.stringify(got[k]) !== JSON.stringify(want[k])) {
          diffs.push(`#${i} ${want.path}: ${k} got=${String(got[k])} want=${String(want[k])}`);
        }
      }
      if (want.terminal && got.address !== want.address) {
        diffs.push(`#${i} ${want.path}: address got=${got.address} want=${want.address}`);
      }
    };
    dump.fields.forEach((w, i) => cmp(i, flat[i], w));
    if (flat.length !== dump.fields.length) {
      diffs.push(`field count: got ${flat.length} want ${dump.fields.length}`);
    }
    expect(diffs).toEqual([]);
  });
});

/**
 * Emits fields in Go's absolute descriptor order:
 * pass 1 — this slot's own fields (declaration order);
 * pass 2 — each non-terminal child's subtree appended fully, in the order
 *          its link was triggered during Go's second pass.
 */
function emitSlot(
  linked: ReturnType<typeof link>,
  ls: LinkedSlot,
  prefix: Array<[number, number]>,
  flat: DumpField[],
): void {
  for (let j = 0; j < ls.slot.fieldCount; j++) {
    const lf = ls.fields[j];
    const fd = lf.fd;
    // Go's dump resolves each field's RELATIVE path from the root slot;
    // mounted children therefore report 0 here (their true addresses live
    // in compileDump.probes).
    const address =
      fd.terminal && ls.idx === 0
        ? addressForSteps(linked.slots, linked.descriptors, linked.localOffsets, [
            [0, j],
          ])
        : 0;
    flat.push({
      slot: fd.schemaIdx,
      idx: fd.fieldIdx,
      path: lf.meta.path,
      name: lf.meta.name,
      dt: fd.dt,
      kind: fd.kind,
      terminal: fd.terminal,
      dp: lf.dp,
      descriptor: fd.raw,
      localOffset: linked.localOffsets[lf.abs] ?? 0,
      address,
    });
  }
  // Pass 2 mirrors Go's children slice ordering (declaration order of the
  // parent's non-terminal fields).
  for (let j = 0; j < ls.slot.fieldCount; j++) {
    const lf = ls.fields[j];
    if (!lf.child || lf.fd.terminal) continue;
    const steps: Array<[number, number]> = [...prefix, [ls.idx, j]];
    // Array-object elements address relative to their own mount; flattened
    // objects continue the chain.
    emitSlot(linked, lf.child, lf.fd.dt === 14 ? [] : steps, flat);
  }
}
