import { describe, it, expect } from "bun:test";
import { MigrationEngine } from "../src/engine.ts";
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
    const m = await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });
    expect(m.status).toBe("pending");
    expect(m.checksum).toMatch(/^[a-f0-9]{64}$/);
    expect(m.changes).toHaveLength(1);
    expect(engine.data().migrations).toHaveLength(1);
  });

  it("add() rejects empty changes", async () => {
    const engine = new MigrationEngine(baseSchema);
    await expect(engine.add({ changes: [], description: "empty" })).rejects.toThrow("must include changes");
  });

  // ── dryRun() ─────────────────────────────────────────────────────────

  it("dryRun forward simulates schema changes", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [
        { type: "addField", id: "email", definition: { name: "email", type: "string", required: true } },
        { type: "removeField", id: "name" },
      ],
      description: "replace name with email",
    });

    const dry = await engine.dryRun("forward");
    expect(dry.schema.version).toBe("2.0.0");
    expect(fieldKeys(dry.schema)).toEqual(["id", "email"]);

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

    const dry = await engine.dryRun("backward");
    expect(dry.schema.version).toBe("1.0.0");
    expect(fieldKeys(dry.schema)).toEqual(["id", "name"]);
  });

  // ── migrate() ────────────────────────────────────────────────────────

  it("migrate() applies pending migrations", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });

    const stream = await engine.migrate();
    expect(stream).toBeInstanceOf(ReadableStream);

    const state = engine.data();
    expect(state.schema.version).toBe("1.1.0");
    expect(fieldKeys(state.schema)).toEqual(["id", "name", "email"]);
    expect(state.migrations[0].status).toBe("applied");
    expect(state.history).toHaveLength(1);
    expect(state.history[0].version).toBe("1.0.0");
  });

  it("migrate() with transform callback", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });

    const chunks: unknown[] = [];
    const stream = await engine.migrate(async (m) => {
      return new ReadableStream({
        start(ctrl) {
          ctrl.enqueue({ migrated: m.id, version: m.schemaVersion });
          ctrl.close();
        },
      });
    });

    const reader = stream.getReader();
    let done = false;
    while (!done) {
      const result = await reader.read();
      if (result.done) { done = true; break; }
      chunks.push(result.value);
    }

    expect(chunks).toHaveLength(1);
    expect((chunks[0] as any).migrated).toBeTruthy();
  });

  it("migrate() is no-op when no pending migrations", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.migrate();
    expect(engine.data().schema.version).toBe("1.0.0");
  });

  // ── rollback() ───────────────────────────────────────────────────────

  it("rollback() reverts last applied migration", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });
    await engine.migrate();

    await engine.rollback();
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

    await engine.rollback("1.0.0");
    expect(engine.data().schema.version).toBe("1.0.0");
    expect(engine.data().migrations.filter((m) => m.status === "rolled_back")).toHaveLength(2);
  });

  // ── checksum validation ──────────────────────────────────────────────

  it("checksum is deterministic", async () => {
    const engine = new MigrationEngine(baseSchema);
    const changes = [{ type: "addField" as const, id: "x", definition: { name: "x", type: "string" as const } }];
    const m = await engine.add({ changes, description: "test" });
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

    (engine as any).processing = true;
    expect(() => engine.dryRun("forward")).toThrow("operation in progress");
    (engine as any).processing = false;
  });

  // ── version bumping through migrate ──────────────────────────────────

  it("migrate() bumps major version for breaking changes", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "removeField", id: "name" }],
      description: "remove name",
    });
    await engine.migrate();
    expect(engine.data().schema.version).toBe("2.0.0");
  });

  it("migrate() bumps minor version for additions", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "addField", id: "email", definition: { name: "email", type: "string" } }],
      description: "add email",
    });
    await engine.migrate();
    expect(engine.data().schema.version).toBe("1.1.0");
  });

  it("migrate() bumps patch for non-breaking modifications", async () => {
    const engine = new MigrationEngine(baseSchema);
    await engine.add({
      changes: [{ type: "modifyField", id: "name", changes: { description: "full name" } }],
      description: "add description",
    });
    await engine.migrate();
    expect(engine.data().schema.version).toBe("1.0.1");
  });
});
