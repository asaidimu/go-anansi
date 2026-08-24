// Step 3 — TS CLIENT: decode the Go server's response — a Dense single and
// a COLUMNAR batch (the server's efficient result-set format) — and verify
// the values survive the round trip.
//
// Run:  bun run examples/encoding/ts-client/decode-response.ts

import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  parseSchema, link, buildManifest,
  decodeDocument, decodeBatch,
} from "../../../packages/anansi/src/index.ts";
import { schemaJSON, expectedResponse } from "../shared.ts";

function fromHex(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  return out;
}

const resp = JSON.parse(
  readFileSync(join(import.meta.dir, "..", "out", "server-response.json"), "utf8"),
);

const fields = buildManifest(link(parseSchema(schemaJSON)));

console.log("client ← server  (TS codec decoding Go-encoded packets)");

function canon(v: unknown): string {
  return JSON.stringify(v, (_k, x: unknown) =>
    x && typeof x === "object" && !Array.isArray(x)
      ? Object.fromEntries(Object.entries(x as object).sort(([a], [b]) => (a < b ? -1 : 1)))
      : x,
  );
}

const single = decodeDocument(fromHex(resp.single), fields);
if (canon(single.doc) !== canon(expectedResponse[0])) {
  throw new Error(`single mismatch:\n got ${JSON.stringify(single.doc)}`);
}
console.log(`  dense   : v${single.version} ✓ ${JSON.stringify(single.doc)}`);

const batch = decodeBatch(fromHex(resp.columnar), fields);
if (canon(batch.docs) !== canon(expectedResponse)) {
  throw new Error(`columnar mismatch:\n got ${JSON.stringify(batch.docs)}`);
}
console.log(`  columnar: v${batch.version} ✓ ${batch.docs.length} records decoded`);
console.log("\nfull duplex verified: TS ⇄ Go on identical bytes.");
