// Step 1 — TS CLIENT: encode an order as Dense, Sparse and a row-Batch
// request packet, then write them for the Go "server" to consume.
//
// Run:  bun run examples/encoding/ts-client/main.ts

import { writeFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import {
  parseSchema, link, buildManifest,
  encodeDocument, encodeBatchRows,
} from "../../../packages/anansi/src/index.ts";
import { schemaJSON, order } from "../shared.ts";

const fields = buildManifest(link(parseSchema(schemaJSON)));

function toHex(b: Uint8Array): string {
  return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
}

const packets = {
  dense: toHex(encodeDocument(fields, order, 7, "dense")),
  sparse: toHex(encodeDocument(fields, order, 7, "sparse")),
  batch: toHex(encodeBatchRows(fields, [order, order], 7)), // e.g. bulk import
};

mkdirSync(join(import.meta.dir, "..", "out"), { recursive: true });
writeFileSync(
  join(import.meta.dir, "..", "out", "client-request.json"),
  JSON.stringify({ fullVersion: 7, ...packets }, null, 2),
);

console.log("client → server");
for (const k of ["dense", "sparse", "batch"] as const) {
  console.log(`  ${k.padEnd(6)} ${packets[k].length / 2} bytes`);
}
console.log("wrote out/client-request.json");
