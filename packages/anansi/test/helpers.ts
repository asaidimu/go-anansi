// Shared golden-test helpers.

import type { ManifestField } from "../src/schema/link.ts";

/**
 * Strip null SCHEMA-FIELD leaves: the wire model treats null as absence
 * (spec 2.7). Opaque subtrees (record / unknown fields) keep their nulls —
 * those are payload bytes, not field states.
 */
export function makeStripNulls(
  fields: ManifestField[],
): (v: unknown, path?: string) => unknown {
  const opaquePaths = fields
    .filter((f) => f.t === "record" || f.t === "unknown")
    .map((f) => f.path);
  const isOpaque = (path: string) =>
    opaquePaths.some((o) => path === o || path.startsWith(o + "."));
  const walk = (v: unknown, path: string): unknown => {
    if (isOpaque(path)) return v;
    if (Array.isArray(v)) return v.map((x) => walk(x, path));
    if (v && typeof v === "object") {
      const out: Record<string, unknown> = {};
      for (const [k, x] of Object.entries(v)) {
        if (x === null) continue;
        out[k] = walk(x, path ? `${path}.${k}` : k);
      }
      return out;
    }
    return v;
  };
  return (v) => walk(v, "");
}

export function hexToBytes(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

export function bytesToHex(b: Uint8Array): string {
  return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
}
