import { describe, it, expect } from "bun:test";
import { MigrationEngine, MigrationError, MigrationErrorCode } from "../src/engine.ts";
import type { SchemaDefinition } from "../src/schema/generated.ts";

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

  // ── checksum validation ──────────────────────────────────────────────

  it("checksum is deterministic", async () => {
    const engine = new MigrationEngine(baseSchema);
    const changes = [{ type: "addField" as const, id: "x", definition: { name: "x", type: "string" as const } }];
    await engine.add({ changes, description: "test" });
    const m = engine.data().migrations[0];
    const { sha256 } = await import("../src/utils.ts");
    const payload = JSON.stringify({
      id: m.id,
      schemaVersion: m.schemaVersion,
      changes: m.changes,
      description: m.description,
      rollback: m.rollback,
      createdAt: m.createdAt,
    });
    expect(m.checksum).toBe(await sha256(payload));
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

  // ── MigrationError ───────────────────────────────────────────────────

  it("MigrationError has correct properties", () => {
    const err = new MigrationError("test", MigrationErrorCode.CHECKSUM_MISMATCH, "m1");
    expect(err.message).toBe("test");
    expect(err.code).toBe(MigrationErrorCode.CHECKSUM_MISMATCH);
    expect(err.migrationId).toBe("m1");
    expect(err.name).toBe("MigrationError");
  });

  it("MigrationErrorCode has all expected codes", () => {
    expect(MigrationErrorCode.INVALID_SCHEMA).toBe("INVALID_SCHEMA");
    expect(MigrationErrorCode.INVALID_MIGRATION).toBe("INVALID_MIGRATION");
    expect(MigrationErrorCode.CHECKSUM_MISMATCH).toBe("CHECKSUM_MISMATCH");
    expect(MigrationErrorCode.TIMEOUT).toBe("TIMEOUT");
    expect(MigrationErrorCode.MEMORY_LIMIT).toBe("MEMORY_LIMIT");
    expect(MigrationErrorCode.CONCURRENT_OPERATION).toBe("CONCURRENT_OPERATION");
    expect(MigrationErrorCode.TRANSFORM_ERROR).toBe("TRANSFORM_ERROR");
    expect(MigrationErrorCode.VERSION_NOT_FOUND).toBe("VERSION_NOT_FOUND");
    expect(MigrationErrorCode.CIRCULAR_DEPENDENCY).toBe("CIRCULAR_DEPENDENCY");
    expect(MigrationErrorCode.STREAM_ERROR).toBe("STREAM_ERROR");
    expect(MigrationErrorCode.ROLLBACK_ERROR).toBe("ROLLBACK_ERROR");
    expect(MigrationErrorCode.MISSING_TRANSFORM).toBe("MISSING_TRANSFORM");
  });
});
