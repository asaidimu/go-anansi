import { describe, it, expect } from "bun:test";
import { SystemError, isKnownErrorCode } from "@asaidimu/utils-error";
import {
  MigrationEngine,
  compareMigrationIds,
  migrationChecksum,
} from "../../src/migration/engine.ts";
import { AnansiCodes } from "../../src/errors.ts";
import type { SchemaDefinition } from "../../src/schema/generated.ts";

const baseSchema: SchemaDefinition = {
  name: "User",
  version: "1.0.0",
  fields: {
    id: { name: "id", type: "string", required: true },
    name: { name: "name", type: "string" },
  },
};

const fieldKeys = (s: SchemaDefinition) => Object.keys(s.fields ?? {});

/** Create a ReadableStream from an array of chunks. */
function streamFrom<T>(chunks: T[]): ReadableStream<T> {
  return new ReadableStream({
    start(ctrl) {
      for (const chunk of chunks) ctrl.enqueue(chunk);
      ctrl.close();
    },
  });
}

/** Collect all chunks from a ReadableStream. */
async function collect<T>(stream: ReadableStream<T>): Promise<T[]> {
  const reader = stream.getReader();
  const out: T[] = [];
  let done = false;
  while (!done) {
    const result = await reader.read();
    if (result.done) { done = true; break; }
    out.push(result.value);
  }
  return out;
}

// ── Constructor & data() ────────────────────────────────────────────────────

describe("MigrationEngine", () => {
  it("initialises with schema", () => {
    const engine = new MigrationEngine(baseSchema);
    const { schema, migrations, history } = engine.data();
    expect(schema.name).toBe("User");
    expect(schema.version).toBe("1.0.0");
    expect(migrations).toEqual([]);
    expect(history).toEqual([]);
  });

  it("initialises with existing migrations and history", () => {
    const prev = { ...baseSchema, version: "0.9.0" };
    const engine = new MigrationEngine(baseSchema, [{ id: "m1" } as any], [prev]);
    expect(engine.data().migrations).toHaveLength(1);
    expect(engine.data().history).toHaveLength(1);
  });

  // ── add() ────────────────────────────────────────────────────────────

  it("add() creates a pending migration with checksum", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });
    const m = engine.data().migrations[0];
    expect(m.status).toBe("pending");
    expect(m.checksum).toMatch(/^[a-f0-9]{64}$/);
    expect(m.changes).toHaveLength(1);
  });

  it("add() rejects empty changes", async () => {
    const engine = new MigrationEngine(baseSchema);
    await expect(engine.add({ changes: [], description: "empty" })).rejects.toThrow("must include changes");
  });

  it("add() rejects string transforms", async () => {
    const engine = new MigrationEngine(baseSchema);
    await expect(
      engine.add({
        changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
        description: "remote transform",
        transform: "https://evil.example/transform.js",
      }),
    ).rejects.toMatchObject({ code: AnansiCodes.MIG.TRANSFORM_ERROR });
    expect(engine.data().migrations).toHaveLength(0);
  });

  // ── dryRun() ─────────────────────────────────────────────────────────

  it("dryRun forward simulates schema and returns data preview", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [
        { type: "addField", id: "email", definition: { name: "email", type: "string", required: true } },
        { type: "removeField", id: "name" },
      ],
      description: "replace name with email",
    });

    const input = streamFrom([{ id: "1", name: "Alice" }]);
    const dry = await engine.dryRun(input, "forward");
    expect(dry.newSchema.version).toBe("2.0.0");
    expect(fieldKeys(dry.newSchema)).toEqual(["id", "email"]);

    // data preview stream should be readable
    const preview = await collect(dry.dataPreview);
    expect(preview).toHaveLength(1);

    // Original schema unchanged
    expect(engine.data().schema.version).toBe("1.0.0");
    expect(fieldKeys(engine.data().schema)).toEqual(["id", "name"]);
  });

  it("dryRun backward simulates rollback", async () => {
    const engine = new MigrationEngine(
      { ...baseSchema, version: "2.0.0", fields: { id: baseSchema.fields!.id, email: { name: "email", type: "string" } } },
      [{ id: "m1", schemaVersion: "1.0.0", changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }], status: "applied", description: "add email", checksum: "", createdAt: "" }],
      [{ ...baseSchema }],
    );

    const input = streamFrom([{ id: "1", email: "alice@test.com" }]);
    const dry = await engine.dryRun(input, "backward");
    expect(dry.newSchema.version).toBe("1.0.0");
    expect(fieldKeys(dry.newSchema)).toEqual(["id", "name"]);
  });

  it("dryRun backward throws VERSION_NOT_FOUND like rollbackToVersion", async () => {
    const engine = new MigrationEngine(baseSchema, [], [{ ...baseSchema }]);
    await expect(
      engine.dryRun(streamFrom([{ id: "1" }]), "backward", "9.9.9"),
    ).rejects.toMatchObject({ code: AnansiCodes.MIG.VERSION_NOT_FOUND });
  });

  it("dryRun does not mutate schema, history, or migrations", async () => {
    const v1 = { ...baseSchema };
    const v2: SchemaDefinition = { ...baseSchema, version: "2.0.0", fields: { id: baseSchema.fields!.id, email: { name: "email", type: "string" } } };
    const engine = new MigrationEngine(
      v2,
      [{ id: "m1", schemaVersion: "1.0.0", changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }], status: "applied", description: "", checksum: "", createdAt: "" }],
      [v1],
    );
    const before = engine.data();

    await engine.dryRun(streamFrom([{ id: "1" }]), "forward");
    await engine.dryRun(streamFrom([{ id: "1", email: "alice@test.com" }]), "backward");

    const after = engine.data();
    expect(after.schema).toEqual(before.schema);
    expect(after.history).toEqual(before.history);
    expect(after.migrations).toEqual(before.migrations);
  });

  // ── migrate() ────────────────────────────────────────────────────────

  it("migrate() applies pending migrations and returns transformed stream", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });

    const input = streamFrom([{ id: "1", name: "Alice" }]);
    const stream = await engine.migrate(input);
    expect(stream).toBeInstanceOf(ReadableStream);

    const state = engine.data();
    expect(state.schema.version).toBe("1.1.0");
    expect(fieldKeys(state.schema)).toEqual(["id", "name", "email"]);
    expect(state.migrations[0].status).toBe("applied");
    expect(state.history).toHaveLength(1);
    expect(state.history[0].version).toBe("1.0.0");
  });

  it("migrate() is no-op when no pending migrations", async () => {
    const engine = new MigrationEngine(baseSchema);
    const input = streamFrom([{ id: "1" }]);
    const stream = await engine.migrate(input);
    const chunks = await collect(stream);
    expect(chunks).toHaveLength(1);
    expect(engine.data().schema.version).toBe("1.0.0");
  });

  it("migrate() with transform applies data transforms via pipeThrough", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
      transform: {
        forward: (data: any) => ({ ...data, email: `${data.name.toLowerCase()}@test.com` }),
        backward: (data: any) => { const { email, ...rest } = data; return rest; },
      },
    });

    const input = streamFrom([{ id: "1", name: "Alice" }, { id: "2", name: "Bob" }]);
    const stream = await engine.migrate(input);
    const chunks = await collect(stream);

    expect(chunks).toHaveLength(2);
    expect(chunks[0]).toEqual({ id: "1", name: "Alice", email: "alice@test.com" });
    expect(chunks[1]).toEqual({ id: "2", name: "Bob", email: "bob@test.com" });
  });

  it("migrate() chains multiple transforms", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
      transform: {
        forward: (data: any) => ({ ...data, email: `${data.name.toLowerCase()}@test.com` }),
        backward: (data: any) => { const { email, ...rest } = data; return rest; },
      },
    });
    await engine.add({
      changes: [{ type: "addField", id: "age", definition: { name: "age", type: "number" } }],
      description: "add age",
      transform: {
        forward: (data: any) => ({ ...data, age: 0 }),
        backward: (data: any) => { const { age, ...rest } = data; return rest; },
      },
    });

    const input = streamFrom([{ id: "1", name: "Alice" }]);
    const stream = await engine.migrate(input);
    const chunks = await collect(stream);

    expect(chunks).toHaveLength(1);
    expect(chunks[0]).toEqual({ id: "1", name: "Alice", email: "alice@test.com", age: 0 });
  });

  it("migrate() rejects string transforms and leaves state untouched", async () => {
    const changes = [{ type: "addField" as const, id: "email", definition: { name: "email", type: "string" as const } }];
    const migration = {
      id: "m1",
      schemaVersion: "1.0.0",
      changes,
      description: "remote transform",
      status: "pending" as const,
      transform: "https://evil.example/transform.js",
      createdAt: "2026-01-01T00:00:00.000Z",
      checksum: "",
    };
    migration.checksum = await migrationChecksum(migration);
    const engine = new MigrationEngine(baseSchema, [migration]);

    const err = await engine
      .migrate(streamFrom([{ id: "1", name: "Alice" }]))
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(SystemError);
    expect(err).toMatchObject({
      code: AnansiCodes.MIG.TRANSFORM_ERROR,
      name: "SystemError",
      path: migration.id,
    });

    const state = engine.data();
    expect(state.schema.version).toBe("1.0.0");
    expect(state.history).toHaveLength(0);
    expect(state.migrations[0].status).toBe("pending");
  });

  it("migrate() with a failing schema change leaves history empty", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "modifyField", id: "missing", changes: { description: "x" } }],
      description: "broken change",
    });

    const err = await engine
      .migrate(streamFrom([{ id: "1" }]))
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(SystemError);
    expect((err as SystemError).name).toBe("SystemError");
    expect((err as SystemError).code).toBe(AnansiCodes.MIG.INVALID_SCHEMA);
    expect(isKnownErrorCode((err as SystemError).code)).toBe(true);

    const state = engine.data();
    expect(state.schema.version).toBe("1.0.0");
    expect(state.history).toHaveLength(0);
    expect(state.migrations[0].status).toBe("pending");
  });

  // ── rollback() ───────────────────────────────────────────────────────

  it("rollback() reverts last applied migration with stream", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
      transform: {
        forward: (data: any) => ({ ...data, email: `${data.name.toLowerCase()}@test.com` }),
        backward: (data: any) => { const { email, ...rest } = data; return rest; },
      },
    });

    const input = streamFrom([{ id: "1", name: "Alice" }]);
    await engine.migrate(input);

    const rollbackInput = streamFrom([{ id: "1", name: "Alice", email: "" }]);
    const rolledBack = await engine.rollback(rollbackInput);
    const chunks = await collect(rolledBack);

    const state = engine.data();
    expect(state.schema.version).toBe("1.0.0");
    expect(fieldKeys(state.schema)).toEqual(["id", "name"]);
    expect(state.migrations[0].status).toBe("rolled_back");
  });

  it("rollback() to specific version", async () => {
    const v1 = { ...baseSchema } satisfies SchemaDefinition;
    const v2: SchemaDefinition = { ...baseSchema, version: "2.0.0", fields: { id: baseSchema.fields!.id, email: { name: "email", type: "string" } } };
    const v3: SchemaDefinition = { ...v2, version: "3.0.0", fields: { ...v2.fields, age: { name: "age", type: "number" } } };

    const engine = new MigrationEngine(v3, [
      { id: "m1", schemaVersion: "1.0.0", changes: [{ type: "addField", id: "x", definition: { name: "x", type: "string" } }], status: "applied", description: "", checksum: "", createdAt: "" },
      { id: "m2", schemaVersion: "2.0.0", changes: [{ type: "addField", id: "y", definition: { name: "y", type: "string" } }], status: "applied", description: "", checksum: "", createdAt: "" },
    ], [v1, v2]);

    const input = streamFrom([{ id: "1" }]);
    await engine.rollbackToVersion("1.0.0", input);
    expect(engine.data().schema.version).toBe("1.0.0");
    expect(engine.data().migrations.filter((m) => m.status === "rolled_back")).toHaveLength(2);
  });

  it("rollback() returns input unchanged when no applied migrations", async () => {
    const engine = new MigrationEngine(baseSchema);
    const input = streamFrom([{ id: "1" }]);
    const result = await engine.rollback(input);
    const chunks = await collect(result);
    expect(chunks).toHaveLength(1);
  });

  it("rollbackToVersion replays backward transforms newest-id-first numerically", async () => {
    const v1 = { ...baseSchema };
    const v2: SchemaDefinition = { ...baseSchema, version: "2.0.0", fields: { id: baseSchema.fields!.id, email: { name: "email", type: "string" } } };
    const engine = new MigrationEngine(
      v2,
      [
        { id: "999", schemaVersion: "1.0.0", changes: [], status: "applied", description: "", checksum: "", createdAt: "",
          transform: { forward: (d: any) => d, backward: (d: any) => ({ ...d, log: `${d.log ?? ""}999,` }) } },
        { id: "1000", schemaVersion: "1.0.0", changes: [], status: "applied", description: "", checksum: "", createdAt: "",
          transform: { forward: (d: any) => d, backward: (d: any) => ({ ...d, log: `${d.log ?? ""}1000,` }) } },
      ],
      [v1],
    );

    const out = await collect(
      await engine.rollbackToVersion("1.0.0", streamFrom([{ id: "1", log: "" }])),
    );
    // Locale string order would put "999" first; numeric ids put "1000" first.
    expect(out[0].log).toBe("1000,999,");
  });

  // ── checksum validation ──────────────────────────────────────────────

  it("checksum is deterministic", async () => {
    const engine = new MigrationEngine(baseSchema);
    const changes = [{ type: "addField" as const, id: "x", definition: { name: "x", type: "string" as const } }];
    await engine.add({ changes, description: "test" });
    const m = engine.data().migrations[0];
    expect(m.checksum).toMatch(/^[a-f0-9]{64}$/);
    expect(m.checksum).toBe(await migrationChecksum(m));
    // Recomputing over the same content yields the same value.
    expect(await migrationChecksum(m)).toBe(await migrationChecksum({ ...m }));
  });

  it("checksum is independent of key insertion order", async () => {
    const a = {
      id: "m1",
      schemaVersion: "1.0.0",
      changes: [{ type: "addField" as const, id: "x", definition: { name: "x", type: "string" as const } }],
      description: "d",
      rollback: [{ type: "removeField" as const, id: "x" }],
      createdAt: "2026-01-01T00:00:00.000Z",
    };
    const b = {
      createdAt: "2026-01-01T00:00:00.000Z",
      rollback: [{ id: "x", type: "removeField" as const }],
      description: "d",
      changes: [{ definition: { type: "string" as const, name: "x" }, id: "x", type: "addField" as const }],
      schemaVersion: "1.0.0",
      id: "m1",
    };
    expect(await migrationChecksum(a)).toBe(await migrationChecksum(b));
  });

  // ── migration id ordering ────────────────────────────────────────────

  it("compareMigrationIds orders all-digit ids numerically", () => {
    expect(compareMigrationIds("999", "1000")).toBeLessThan(0);
    expect(compareMigrationIds("1000", "999")).toBeGreaterThan(0);
    expect(compareMigrationIds("1000", "1000")).toBe(0);
  });

  it("compareMigrationIds falls back to code-unit order for non-numeric ids", () => {
    expect(compareMigrationIds("m1", "m2")).toBeLessThan(0);
    expect(compareMigrationIds("m2", "m1")).toBeGreaterThan(0);
    expect(compareMigrationIds("1000", "m1")).toBeLessThan(0);
    expect(compareMigrationIds("m1", "1000")).toBeGreaterThan(0);
  });

  // ── concurrent guard ─────────────────────────────────────────────────

  it("rejects operations while processing", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });

    (engine as any).isProcessing = true;
    const input = streamFrom([{ id: "1" }]);
    expect(() => engine.dryRun(input, "forward")).toThrow("Concurrent operation");
    (engine as any).isProcessing = false;
  });

  // ── version bumping through migrate ──────────────────────────────────

  it("migrate() bumps major version for breaking changes", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "removeField", id: "name" }],
      description: "remove name",
    });
    const input = streamFrom([{ id: "1" }]);
    await engine.migrate(input);
    expect(engine.data().schema.version).toBe("2.0.0");
  });

  it("migrate() bumps minor version for additions", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });
    const input = streamFrom([{ id: "1" }]);
    await engine.migrate(input);
    expect(engine.data().schema.version).toBe("1.1.0");
  });

  it("migrate() bumps patch for non-breaking modifications", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "modifyField", id: "name", changes: { description: "full name" } }],
      description: "add description",
    });
    const input = streamFrom([{ id: "1" }]);
    await engine.migrate(input);
    expect(engine.data().schema.version).toBe("1.0.1");
  });

  // ── migration errors ─────────────────────────────────────────────────

  it("MIG code registry covers all expected codes", () => {
    const mig: Record<string, string> = AnansiCodes.MIG;
    expect(mig.INVALID_SCHEMA).toBe("MIG-001");
    expect(mig.INVALID_MIGRATION).toBe("MIG-002");
    expect(mig.CHECKSUM_MISMATCH).toBe("MIG-003");
    expect(mig.TIMEOUT).toBe("MIG-004");
    expect(mig.MEMORY_LIMIT).toBe("MIG-005");
    expect(mig.CONCURRENT_OPERATION).toBe("MIG-006");
    expect(mig.TRANSFORM_ERROR).toBe("MIG-007");
    expect(mig.VERSION_NOT_FOUND).toBe("MIG-008");
    expect(mig.CIRCULAR_DEPENDENCY).toBe("MIG-009");
    expect(mig.STREAM_ERROR).toBe("MIG-010");
    expect(mig.ROLLBACK_ERROR).toBe("MIG-011");
    expect(mig.MISSING_TRANSFORM).toBe("MIG-012");
    for (const code of Object.values(mig)) {
      expect(isKnownErrorCode(code)).toBe(true);
    }
  });
});
