// Isomorphic utilities — zero external dependencies.

/** Type guard: plain object (not array, not null). */
function isObject(item: unknown): item is Record<string, unknown> {
  return item !== null && typeof item === "object" && !Array.isArray(item);
}

/**
 * Deep-merge `update` into `target`, returning a new object.
 * Arrays are replaced, not merged.
 */
export function deepMerge<T extends Record<string, unknown>>(
  target: T,
  update: Partial<T>,
): T {
  const out = { ...target };
  for (const key of Object.keys(update) as (keyof T)[]) {
    const val = update[key];
    if (isObject(val) && isObject(out[key])) {
      (out as Record<string, unknown>)[key as string] = deepMerge(
        out[key] as Record<string, unknown>,
        val as Record<string, unknown>,
      );
    } else if (val !== undefined) {
      (out as Record<string, unknown>)[key as string] = val;
    }
  }
  return out;
}

/**
 * Isomorphic SHA-256 hex digest.
 *
 * Uses `globalThis.crypto.subtle` when available (browsers, Node 20+,
 * Bun, Deno), falls back to `node:crypto` for older Node runtimes.
 */
export async function sha256(input: string): Promise<string> {
  // Web Crypto API (browsers, Node 20+, Bun, Deno)
  if (typeof globalThis.crypto?.subtle?.digest === "function") {
    const data = new TextEncoder().encode(input);
    const hash = await globalThis.crypto.subtle.digest("SHA-256", data);
    return Array.from(new Uint8Array(hash))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }

  // Node.js fallback
  const { createHash } = await import("node:crypto");
  return createHash("sha256").update(input).digest("hex");
}
