// Engine-ordering regression — the old lexicographic compares failed
// ("10.0.0" < "9.0.0"). The version unit tests (parse, bitfield roundtrip,
// comparison, sorting, JSON, bumping, Versionable) moved to
// @asaidimu/utils-version (src/version/version.test.ts).

import { describe, it, expect } from "bun:test";
import { MigrationEngine } from "../../src/migration/engine.ts";
import type { Migration } from "../../src/migration/migration.ts";
import type { SchemaDefinition } from "../../src/schema/generated.ts";

describe("engine ordering (regression: lexicographic vs numeric)", () => {
  const schema: SchemaDefinition = { name: "t", version: "1.0.0" };
  const migration = (id: string, schemaVersion: string): Migration<any> => ({
    id,
    schemaVersion,
    changes: [],
    description: "",
    status: "pending",
    createdAt: "",
    checksum: "",
  });

  it("orders 9.0.0 before 10.0.0 (localeCompare gets this backwards)", () => {
    const engine = new MigrationEngine(
      schema,
      [migration("m10", "10.0.0"), migration("m9", "9.0.0")],
      [
        { name: "t", version: "10.0.0" },
        { name: "t", version: "9.0.0" },
      ],
    );
    const state = engine.data();
    expect(state.migrations.map((m) => m.schemaVersion)).toEqual([
      "9.0.0",
      "10.0.0",
    ]);
    expect(state.history.map((h) => h.version)).toEqual([
      "9.0.0",
      "10.0.0",
    ]);
  });
});
