// MigrationEngine — manages schema versioning, migrations, and rollback.
//
// Provides a stream-based API: callers pass data as ReadableStream, the engine
// pipes it through transforms, and returns the transformed stream.

import type {
  DataTransform,
  Migration,
  SchemaChange,
} from "./schema/migration.ts";
import { schemaChangeToPatch, bumpVersion, calculateNextBump } from "./schema/migration.ts";
import type { SchemaDefinition } from "./schema/generated.ts";
import { sha256 } from "./utils.ts";

// ── Error types ──────────────────────────────────────────────────────────────

export enum MigrationErrorCode {
  INVALID_SCHEMA = "INVALID_SCHEMA",
  INVALID_MIGRATION = "INVALID_MIGRATION",
  CHECKSUM_MISMATCH = "CHECKSUM_MISMATCH",
  TIMEOUT = "TIMEOUT",
  MEMORY_LIMIT = "MEMORY_LIMIT",
  CONCURRENT_OPERATION = "CONCURRENT_OPERATION",
  TRANSFORM_ERROR = "TRANSFORM_ERROR",
  VERSION_NOT_FOUND = "VERSION_NOT_FOUND",
  CIRCULAR_DEPENDENCY = "CIRCULAR_DEPENDENCY",
  STREAM_ERROR = "STREAM_ERROR",
  ROLLBACK_ERROR = "ROLLBACK_ERROR",
  MISSING_TRANSFORM = "MISSING_TRANSFORM",
}

export class MigrationError extends Error {
  constructor(
    message: string,
    public readonly code: MigrationErrorCode,
    public readonly migrationId?: string,
    public readonly cause?: Error,
  ) {
    super(message);
    this.name = "MigrationError";
  }
}

// �─ Minimal RFC 6902 JSON Patch applier ─────────────────────────────────────

function parsePointer(path: string): string[] {
  if (path === "" || path === "/") return [];
  return path
    .slice(1)
    .split("/")
    .map((s) => s.replace(/~1/g, "/").replace(/~0/g, "~"));
}

function navigate(obj: any, parts: string[]): { parent: any; key: string } {
  let parent = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    const part = parts[i];
    if (parent === null || typeof parent !== "object") {
      throw new Error(`anansi/engine: path "${part}" not found`);
    }
    parent = Array.isArray(parent)
      ? parent[parseInt(part, 10)]
      : parent[part];
  }
  return { parent, key: parts[parts.length - 1] };
}

function applyPatchOps(target: any, ops: Array<{ op: string; path: string; value?: any; from?: string }>): any {
  let result = JSON.parse(JSON.stringify(target));

  for (const op of ops) {
    const parts = parsePointer(op.path);

    switch (op.op) {
      case "add": {
        if (parts.length === 0) { result = op.value; break; }
        const { parent, key } = navigate(result, parts);
        if (Array.isArray(parent)) {
          const idx = key === "-" ? parent.length : parseInt(key, 10);
          parent.splice(idx, 0, op.value);
        } else {
          parent[key] = op.value;
        }
        break;
      }
      case "remove": {
        const { parent, key } = navigate(result, parts);
        if (Array.isArray(parent)) {
          parent.splice(parseInt(key, 10), 1);
        } else {
          delete parent[key];
        }
        break;
      }
      case "replace": {
        const { parent, key } = navigate(result, parts);
        parent[key] = op.value;
        break;
      }
      case "move": {
        const fromParts = parsePointer(op.from!);
        const fromNav = navigate(result, fromParts);
        const val = Array.isArray(fromNav.parent)
          ? fromNav.parent.splice(parseInt(fromNav.key, 10), 1)[0]
          : (() => { const v = fromNav.parent[fromNav.key]; delete fromNav.parent[fromNav.key]; return v; })();
        const toParts = parsePointer(op.path);
        const toNav = navigate(result, toParts);
        if (Array.isArray(toNav.parent)) {
          toNav.parent.splice(parseInt(toNav.key, 10), 0, val);
        } else {
          toNav.parent[toNav.key] = val;
        }
        break;
      }
      case "test": {
        const { parent, key } = navigate(result, parts);
        if (JSON.stringify(parent[key]) !== JSON.stringify(op.value)) {
          throw new Error(`anansi/engine: test failed at ${op.path}`);
        }
        break;
      }
    }
  }

  return result;
}

// ── Checksum ────────────────────────────────────────────────────────────────

async function migrationChecksum(m: Omit<Migration, "checksum">): Promise<string> {
  const payload = JSON.stringify({
    id: m.id,
    schemaVersion: m.schemaVersion,
    changes: m.changes,
    description: m.description,
    rollback: m.rollback,
    createdAt: m.createdAt,
  });
  return sha256(payload);
}

// ── Apply schema changes to a schema ────────────────────────────────────────

function applySchemaChanges(
  schema: SchemaDefinition,
  changes: SchemaChange[],
  version: string,
): SchemaDefinition {
  let result = { ...schema, version };
  for (const change of changes) {
    const patches = schemaChangeToPatch(change, result);
    result = applyPatchOps(result, patches);
    result.version = version;
  }
  return result;
}

// ── Types ───────────────────────────────────────────────────────────────────

export interface EngineState {
  schema: SchemaDefinition;
  migrations: Migration[];
  history: SchemaDefinition[];
}

export interface DryRunResult {
  newSchema: SchemaDefinition;
  dataPreview: ReadableStream<any>;
}

// ── Transform resolution ─────────────────────────────────────────────────────

async function resolveTransform(
  migration: Migration,
  direction: "forward" | "backward",
): Promise<any | null> {
  if (!migration.transform) {
    return null;
  }

  if (typeof migration.transform === "string") {
    if (
      migration.transform.startsWith("http://") ||
      migration.transform.startsWith("https://")
    ) {
      return resolveRemoteTransform(migration.transform, direction);
    }
    return resolveLocalTransform(migration.transform, direction);
  }

  const fn = migration.transform[direction];
  return fn;
}

async function resolveRemoteTransform(
  url: string,
  direction: "forward" | "backward",
): Promise<any> {
  try {
    const response = await fetch(url);
    if (!response.ok) {
      throw new MigrationError(
        `Failed to fetch transform module: ${url}`,
        MigrationErrorCode.TRANSFORM_ERROR,
      );
    }
    const moduleText = await response.text();

    if (typeof window !== "undefined") {
      const blob = new Blob([moduleText], {
        type: "application/javascript",
      });
      const blobUrl = URL.createObjectURL(blob);
      const mod = (await import(blobUrl)).default;
      return mod[direction];
    } else {
      const { runInNewContext } = await import("node:vm");
      const sandbox = { module: { exports: {} }, console };
      runInNewContext(moduleText, sandbox, url);
      return (sandbox.module.exports as DataTransform<any, any>)[direction];
    }
  } catch (error) {
    if (error instanceof MigrationError) throw error;
    throw new MigrationError(
      `Failed to load remote transform module: ${url}`,
      MigrationErrorCode.TRANSFORM_ERROR,
      undefined,
      error as Error,
    );
  }
}

async function resolveLocalTransform(
  path: string,
  direction: "forward" | "backward",
): Promise<any> {
  try {
    const mod = await import(path);
    const dataTransform: DataTransform<any, any> = mod.default;
    return dataTransform[direction];
  } catch (error) {
    if (error instanceof MigrationError) throw error;
    throw new MigrationError(
      `Failed to import local transform module: ${path}`,
      MigrationErrorCode.TRANSFORM_ERROR,
      undefined,
      error as Error,
    );
  }
}

// ── MigrationEngine ─────────────────────────────────────────────────────────

export class MigrationEngine {
  private currentSchema!: SchemaDefinition;
  private history: Array<SchemaDefinition> = [];
  private migrations: Migration<any>[] = [];
  private isProcessing = false;

  constructor(
    currentSchema: SchemaDefinition,
    migrations?: Array<Migration<any>>,
    history?: Array<SchemaDefinition>,
  ) {
    this.currentSchema = currentSchema;

    if (migrations) {
      this.migrations = migrations.sort((a, b) =>
        a.schemaVersion.localeCompare(b.schemaVersion),
      );
    }
    if (history) {
      this.history = history.sort((a, b) =>
        a.version.localeCompare(b.version),
      );
    }
  }

  // ── Public API ──────────────────────────────────────────────────────

  data(): EngineState {
    return {
      schema: this.currentSchema,
      history: [...this.history],
      migrations: [...this.migrations],
    };
  }

  async add(opts: {
    changes: SchemaChange[];
    description: string;
    rollback?: SchemaChange[];
    transform?: string | DataTransform<any, any>;
  }): Promise<void> {
    if (this.isProcessing) {
      throw new MigrationError(
        "Concurrent operation",
        MigrationErrorCode.CONCURRENT_OPERATION,
      );
    }
    if (!opts.changes?.length) {
      throw new MigrationError(
        "Migration must include changes",
        MigrationErrorCode.INVALID_MIGRATION,
      );
    }

    const newMigration: Migration<any> = {
      id: Date.now().toString(),
      schemaVersion: this.currentSchema.version,
      changes: opts.changes,
      description: opts.description,
      status: "pending",
      rollback: opts.rollback,
      transform: opts.transform as DataTransform<any, any>,
      createdAt: new Date().toISOString(),
      checksum: "",
    };

    newMigration.checksum = await migrationChecksum(newMigration);
    this.migrations.push(newMigration);
  }

  /**
   * Dry-run: simulate migration without modifying internal state.
   * Returns the projected schema and a data preview stream.
   */
  async dryRun(
    input: ReadableStream<any>,
    direction: "forward" | "backward",
    version?: string,
  ): Promise<DryRunResult> {
    if (this.isProcessing) {
      throw new MigrationError(
        "Concurrent operation",
        MigrationErrorCode.CONCURRENT_OPERATION,
      );
    }

    try {
      this.isProcessing = true;
      let simulatedSchema = { ...this.currentSchema };
      const relevantMigrations = this.getRelevantMigrations(direction, version);

      let tempSchema: SchemaDefinition;
      if (direction === "forward") {
        tempSchema = relevantMigrations.reduce((acc, migration) => {
          return this.applySchemaChanges(acc, migration.changes, migration.id);
        }, simulatedSchema);
      } else {
        // Walk backwards through history
        const historyCopy = [...this.history];
        for (let i = 0; i < relevantMigrations.length; i++) {
          const prev = historyCopy.pop();
          if (prev) simulatedSchema = { ...prev };
        }
        tempSchema = simulatedSchema;
      }

      const previewStream = await MigrationEngine.processMigrationList(
        input,
        direction,
        relevantMigrations,
      );

      return { newSchema: tempSchema, dataPreview: previewStream };
    } catch (error) {
      if (error instanceof MigrationError) throw error;
      throw new MigrationError(
        "Dry run failed",
        MigrationErrorCode.INVALID_SCHEMA,
        undefined,
        error as Error,
      );
    } finally {
      this.isProcessing = false;
    }
  }

  /**
   * Prepare pending migrations: validate checksums, return the list.
   */
  async prepareMigration(): Promise<Array<Migration<any>>> {
    const pendingMigrations = this.migrations.filter(
      (m) => m.status === "pending",
    );
    await this.validateMigrations(pendingMigrations);
    return pendingMigrations;
  }

  /**
   * Apply pending migrations. Takes an input data stream and returns
   * a transformed stream with all migration transforms applied.
   */
  async migrate(input: ReadableStream<any>): Promise<ReadableStream<any>> {
    if (this.isProcessing) {
      throw new MigrationError(
        "Concurrent operation",
        MigrationErrorCode.CONCURRENT_OPERATION,
      );
    }

    const pendingMigrations = await this.prepareMigration();
    try {
      this.isProcessing = true;
      this.transformSchema("forward");
      const result = await MigrationEngine.processMigrationList(
        input,
        "forward",
        pendingMigrations,
      );

      this.markMigrationsApplied(pendingMigrations);
      return result;
    } finally {
      this.isProcessing = false;
    }
  }

  /**
   * Roll back the last applied migration.
   */
  async rollback(input: ReadableStream<any>): Promise<ReadableStream<any>> {
    if (this.isProcessing) {
      throw new MigrationError(
        "Concurrent operation",
        MigrationErrorCode.CONCURRENT_OPERATION,
      );
    }
    const lastApplied = this.migrations
      .filter((m) => m.status === "applied")
      .slice(-1)[0];

    if (!lastApplied) return input;

    return this.rollbackToVersion(
      this.history[this.history.length - 1]?.version ||
        this.currentSchema.version,
      input,
    );
  }

  /**
   * Roll back to a specific schema version.
   */
  async rollbackToVersion(
    targetVersion: string,
    input: ReadableStream<any>,
  ): Promise<ReadableStream<any>> {
    if (this.isProcessing) {
      throw new MigrationError(
        "Concurrent operation",
        MigrationErrorCode.CONCURRENT_OPERATION,
      );
    }

    try {
      const versionIndex = this.history.findIndex(
        (s) => s.version === targetVersion,
      );

      if (versionIndex === -1) {
        throw new MigrationError(
          `Version ${targetVersion} not found in history`,
          MigrationErrorCode.VERSION_NOT_FOUND,
        );
      }

      const migrations = this.migrations
        .filter(
          (m) => m.schemaVersion === targetVersion && m.status === "applied",
        )
        .sort((a, b) => b.id.localeCompare(a.id));

      const stepsBack = this.history.length - versionIndex;

      if (stepsBack < 0) return input;

      for (let i = 0; i < stepsBack; i++) {
        this.transformSchema("backward");
      }

      const transformedStream = await MigrationEngine.processMigrationList(
        input,
        "backward",
        migrations,
      );

      this.migrations = this.migrations.map((m) => {
        if (m.schemaVersion >= targetVersion && m.status === "applied") {
          return { ...m, status: "rolled_back" as const };
        }
        return m;
      });

      return transformedStream;
    } finally {
      this.isProcessing = false;
    }
  }

  /**
   * Process a list of migrations on a data stream, applying transforms
   * in the specified direction (forward or backward).
   */
  static async processMigrationList(
    input: ReadableStream<any>,
    direction: "forward" | "backward",
    migrations: Migration<any>[],
  ): Promise<ReadableStream<any>> {
    const transformEntries = await Promise.all(
      migrations.map(async (migration) => {
        try {
          const transform = await resolveTransform(migration, direction);
          return { migration, transform };
        } catch (error) {
          throw new MigrationError(
            `Failed to resolve transform for migration ${migration.id}`,
            MigrationErrorCode.TRANSFORM_ERROR,
            migration.id,
            error as Error,
          );
        }
      }),
    );

    const validTransformEntries = transformEntries.filter(
      (
        entry,
      ): entry is {
        migration: Migration<any>;
        transform: any;
      } => Boolean(entry.transform),
    );

    return validTransformEntries.reduce((stream, { migration, transform }) => {
      return stream.pipeThrough(
        new TransformStream({
          async transform(chunk, controller) {
            try {
              const result = await transform(chunk);
              controller.enqueue(result);
            } catch (error) {
              controller.error(
                new MigrationError(
                  `Data transformation failed for migration ${migration.id}`,
                  MigrationErrorCode.TRANSFORM_ERROR,
                  migration.id,
                  error as Error,
                ),
              );
            }
          },
        }),
      );
    }, input);
  }

  // ── Private helpers ─────────────────────────────────────────────────

  private getRelevantMigrations(
    direction: "forward" | "backward",
    version?: string,
  ): Migration<any>[] {
    return [...this.migrations]
      .filter((m) => {
        const dir = direction === "forward" ? "pending" : "applied";
        const vs = version
          ? m.schemaVersion.localeCompare(version) >= 0
          : true;
        return m.status === dir && vs;
      })
      .sort((a, b) =>
        direction === "forward"
          ? a.id.localeCompare(b.id)
          : b.id.localeCompare(a.id),
      );
  }

  private applySchemaChanges(
    schema: SchemaDefinition,
    changes: SchemaChange[],
    migrationId?: string,
  ): SchemaDefinition {
    const bump = calculateNextBump(changes, schema);
    const version = bumpVersion(schema.version, bump);
    const patches = changes.map((change) => {
      try {
        return schemaChangeToPatch(change, schema);
      } catch (error) {
        throw new MigrationError(
          "Invalid schema change",
          MigrationErrorCode.INVALID_SCHEMA,
          migrationId,
          error as Error,
        );
      }
    });

    return patches.reduce(
      (acc, patch) => {
        try {
          return applyPatchOps(acc, patch);
        } catch (error) {
          throw new MigrationError(
            "Failed to apply patch",
            MigrationErrorCode.INVALID_SCHEMA,
            migrationId,
            error as Error,
          );
        }
      },
      { ...schema, version },
    );
  }

  private async validateMigrations(
    migrations: Migration<any>[],
  ): Promise<void> {
    await Promise.all(
      migrations.map(async (migration) => {
        const currentChecksum = await migrationChecksum(migration);
        if (migration.checksum !== currentChecksum) {
          throw new MigrationError(
            "Checksum mismatch",
            MigrationErrorCode.CHECKSUM_MISMATCH,
            migration.id,
          );
        }
      }),
    );
  }

  private markMigrationsApplied(migrations: Migration<any>[]) {
    this.migrations = this.migrations.map((m) =>
      migrations.some((mm) => mm.id === m.id) ? { ...m, status: "applied" } : m,
    );
  }

  private transformSchema(direction: "forward" | "backward") {
    try {
      if (direction === "backward") {
        const schema = this.history.pop();
        if (!schema) throw new Error("No previous version");
        this.currentSchema = schema;
        return;
      }

      const changes = this.migrations
        .filter((m) => m.status === "pending")
        .flatMap((m) => m.changes);

      if (!changes.length) return;

      this.history.push(structuredClone(this.currentSchema));
      this.currentSchema = changes.reduce(
        (acc, change) => this.applySchemaChanges(acc, [change]),
        this.currentSchema,
      );
    } catch (error) {
      if (error instanceof MigrationError) throw error;
      throw new MigrationError(
        "Schema transformation failed",
        MigrationErrorCode.INVALID_SCHEMA,
        undefined,
        error as Error,
      );
    }
  }
}

export default MigrationEngine;
