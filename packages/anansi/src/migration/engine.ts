// MigrationEngine — manages schema versioning, migrations, and rollback.
//
// Provides a stream-based API: callers pass data as ReadableStream, the engine
// pipes it through transforms, and returns the transformed stream.

import type {
  DataTransform,
  Migration,
  SchemaChange,
} from "./migration.ts";
import { schemaChangeToPatch, bumpVersion, calculateNextBump } from "./migration.ts";
import { parseVersion } from "@asaidimu/utils-version";
import type { SchemaDefinition } from "../schema/generated.ts";
import { sha256 } from "../utils.ts";
import {
  AnansiCodes,
  anansiError,
  internalError,
  isMigrationError,
} from "../errors.ts";

// ── Minimal RFC 6902 JSON Patch applier ─────────────────────────────────────

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
      throw internalError(`anansi/engine: path "${part}" not found`);
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
          throw internalError(`anansi/engine: test failed at ${op.path}`);
        }
        break;
      }
    }
  }

  return result;
}

// ── Checksum ────────────────────────────────────────────────────────────────

/** JSON data with every object's keys sorted recursively, so structurally
 * equivalent payloads stringify identically regardless of insertion order. */
function canonicalJson(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalJson);
  if (value !== null && typeof value === "object") {
    const src = value as Record<string, unknown>;
    const out: Record<string, unknown> = {};
    for (const key of Object.keys(src).sort()) {
      if (src[key] === undefined) continue;
      out[key] = canonicalJson(src[key]);
    }
    return out;
  }
  return value;
}

export async function migrationChecksum(m: Omit<Migration, "checksum">): Promise<string> {
  const payload = JSON.stringify(
    canonicalJson({
      id: m.id,
      schemaVersion: m.schemaVersion,
      changes: m.changes,
      description: m.description,
      rollback: m.rollback,
      createdAt: m.createdAt,
    }),
  );
  return sha256(payload);
}

// ── Migration ID ordering ────────────────────────────────────────────────────

/** Order migration IDs without locale dependence: numeric when both IDs are
 * all digits (Date.now() timestamps from add()), code-unit comparison
 * otherwise (hand-written or external IDs like "m1"). Pure Number() ordering
 * would mis-sort mixed numeric/non-numeric sets (NaN), and localeCompare
 * collates "999" after "1000" and varies by locale. */
export function compareMigrationIds(a: string, b: string): number {
  const an = /^\d+$/.test(a) ? Number(a) : null;
  const bn = /^\d+$/.test(b) ? Number(b) : null;
  if (an !== null && bn !== null && an !== bn) return an < bn ? -1 : 1;
  if (a < b) return -1;
  if (a > b) return 1;
  return 0;
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

// String transforms (URLs or module paths) once fetched and executed remote or
// on-disk code; no code loading is supported anymore. Only inline
// { forward, backward } objects are accepted.
async function resolveTransform(
  migration: Migration,
  direction: "forward" | "backward",
): Promise<any | null> {
  if (!migration.transform) {
    return null;
  }

  if (typeof migration.transform === "string") {
    throw anansiError(
      AnansiCodes.MIG.TRANSFORM_ERROR,
      `String transforms are not supported: migration ${migration.id} must provide an inline { forward, backward } transform object`,
      { path: migration.id },
    );
  }

  return migration.transform[direction];
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
        parseVersion(a.schemaVersion).compare(parseVersion(b.schemaVersion)),
      );
    }
    if (history) {
      this.history = history.sort((a, b) =>
        parseVersion(a.version).compare(parseVersion(b.version)),
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
      throw anansiError(
        AnansiCodes.MIG.CONCURRENT_OPERATION,
        "Concurrent operation",
      );
    }
    if (!opts.changes?.length) {
      throw anansiError(
        AnansiCodes.MIG.INVALID_MIGRATION,
        "Migration must include changes",
      );
    }
    if (typeof opts.transform === "string") {
      throw anansiError(
        AnansiCodes.MIG.TRANSFORM_ERROR,
        "String transforms are not supported: transform must be an inline { forward, backward } object",
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
      throw anansiError(
        AnansiCodes.MIG.CONCURRENT_OPERATION,
        "Concurrent operation",
      );
    }

    try {
      this.isProcessing = true;
      let tempSchema: SchemaDefinition;
      let relevantMigrations: Migration<any>[];

      if (direction === "forward") {
        relevantMigrations = this.getRelevantMigrations(version);
        tempSchema = relevantMigrations.reduce(
          (acc, migration) => {
            return this.applySchemaChanges(acc, migration.changes, migration.id);
          },
          { ...this.currentSchema },
        );
      } else {
        // Mirror rollbackToVersion: resolve the same target version, walk back
        // the same number of history steps, and preview the same migration set.
        const target = this.rollbackTarget(version);
        relevantMigrations = this.getRollbackMigrations(target);
        const versionIndex = this.historyIndexOf(target);
        const historyCopy = [...this.history];
        let simulatedSchema = { ...this.currentSchema };
        for (let i = 0; i < this.history.length - versionIndex; i++) {
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
      if (isMigrationError(error)) throw error;
      throw anansiError(
        AnansiCodes.MIG.INVALID_SCHEMA,
        "Dry run failed",
        { cause: error },
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
      throw anansiError(
        AnansiCodes.MIG.CONCURRENT_OPERATION,
        "Concurrent operation",
      );
    }

    const pendingMigrations = await this.prepareMigration();
    const prevSchema = this.currentSchema;
    const prevHistoryLength = this.history.length;
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
    } catch (error) {
      // Restore pre-migration state so a failed migrate (e.g. an unresolvable
      // transform) leaves no half-transformed schema or stray history entry.
      this.currentSchema = prevSchema;
      this.history.length = prevHistoryLength;
      throw error;
    } finally {
      this.isProcessing = false;
    }
  }

  /**
   * Roll back the last applied migration.
   */
  async rollback(input: ReadableStream<any>): Promise<ReadableStream<any>> {
    if (this.isProcessing) {
      throw anansiError(
        AnansiCodes.MIG.CONCURRENT_OPERATION,
        "Concurrent operation",
      );
    }
    const lastApplied = this.migrations
      .filter((m) => m.status === "applied")
      .slice(-1)[0];

    if (!lastApplied) return input;

    return this.rollbackToVersion(this.rollbackTarget(), input);
  }

  /**
   * Roll back to a specific schema version.
   */
  async rollbackToVersion(
    targetVersion: string,
    input: ReadableStream<any>,
  ): Promise<ReadableStream<any>> {
    if (this.isProcessing) {
      throw anansiError(
        AnansiCodes.MIG.CONCURRENT_OPERATION,
        "Concurrent operation",
      );
    }

    const prevSchema = this.currentSchema;
    const prevHistory = [...this.history];
    try {
      this.isProcessing = true;
      const versionIndex = this.historyIndexOf(targetVersion);
      const migrations = this.getRollbackMigrations(targetVersion);
      const stepsBack = this.history.length - versionIndex;

      for (let i = 0; i < stepsBack; i++) {
        this.transformSchema("backward");
      }

      const transformedStream = await MigrationEngine.processMigrationList(
        input,
        "backward",
        migrations,
      );

      const target = parseVersion(targetVersion);
      this.migrations = this.migrations.map((m) => {
        if (parseVersion(m.schemaVersion).compare(target) >= 0 && m.status === "applied") {
          return { ...m, status: "rolled_back" as const };
        }
        return m;
      });

      return transformedStream;
    } catch (error) {
      // Restore pre-rollback state so a failed rollback leaves no popped
      // history entries or a half-reverted schema behind.
      this.currentSchema = prevSchema;
      this.history = prevHistory;
      throw error;
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
          throw anansiError(
            AnansiCodes.MIG.TRANSFORM_ERROR,
            `Failed to resolve transform for migration ${migration.id}`,
            { path: migration.id, cause: error },
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
                anansiError(
                  AnansiCodes.MIG.TRANSFORM_ERROR,
                  `Data transformation failed for migration ${migration.id}`,
                  { path: migration.id, cause: error },
                ),
              );
            }
          },
        }),
      );
    }, input);
  }

  // ── Private helpers ─────────────────────────────────────────────────

  /** Target version for a rollback with no explicit version: the version the
   * last applied migration rolled us back to, i.e. the newest history entry. */
  private rollbackTarget(version?: string): string {
    return (
      version ??
      this.history[this.history.length - 1]?.version ??
      this.currentSchema.version
    );
  }

  private historyIndexOf(targetVersion: string): number {
    const index = this.history.findIndex((s) => s.version === targetVersion);
    if (index === -1) {
      throw anansiError(
        AnansiCodes.MIG.VERSION_NOT_FOUND,
        `Version ${targetVersion} not found in history`,
      );
    }
    return index;
  }

  /** Migrations a rollback to targetVersion replays backwards: the applied
   * set recorded against exactly that version, newest id first. */
  private getRollbackMigrations(targetVersion: string): Migration<any>[] {
    return this.migrations
      .filter(
        (m) => m.schemaVersion === targetVersion && m.status === "applied",
      )
      .sort((a, b) => compareMigrationIds(b.id, a.id));
  }

  // @note #migration-id-sort issue resolved P2 : Migration IDs ordered with localeCompare, not numeric order
  // Fixed: both ordering sites now use compareMigrationIds (exported, unit-tested in test/engine.test.ts) — numeric when both IDs are all digits, code-unit comparison otherwise; no locale dependence and mixed/external IDs sort correctly.
  //
  // Migration IDs are Date.now().toString() (set in add()). Two sites used
  // localeCompare: this method's sort and rollbackToVersion's (now
  // getRollbackMigrations). localeCompare is locale-aware string
  // collation, which is correct only while every ID is a same-length
  // 13-digit timestamp. Mixed-length or externally-supplied IDs order
  // wrongly ("999" > "1000") and collation varies by locale. Fix by
  // sorting with a numeric comparator (e.g. Number(b.id) - Number(a.id))
  // when IDs are timestamps, or by issuing sort-safe IDs in add().
  // Note: schemaVersion sorting/filtering in this file has the same
  // lexicographic flaw and is being replaced by the core/common/version.go
  // port (src/version.ts) — that port does not cover migration-ID ordering;
  // compareMigrationIds handles it here.
  private getRelevantMigrations(version?: string): Migration<any>[] {
    const target = version ? parseVersion(version) : null;
    return [...this.migrations]
      .filter((m) => {
        const vs = target
          ? parseVersion(m.schemaVersion).compare(target) >= 0
          : true;
        return m.status === "pending" && vs;
      })
      .sort((a, b) => compareMigrationIds(a.id, b.id));
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
        throw anansiError(
          AnansiCodes.MIG.INVALID_SCHEMA,
          "Invalid schema change",
          { path: migrationId, cause: error },
        );
      }
    });

    return patches.reduce(
      (acc, patch) => {
        try {
          return applyPatchOps(acc, patch);
        } catch (error) {
          throw anansiError(
            AnansiCodes.MIG.INVALID_SCHEMA,
            "Failed to apply patch",
            { path: migrationId, cause: error },
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
          throw anansiError(
            AnansiCodes.MIG.CHECKSUM_MISMATCH,
            "Checksum mismatch",
            { path: migration.id },
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
        if (!schema) throw internalError("No previous version");
        this.currentSchema = schema;
        return;
      }

      const changes = this.migrations
        .filter((m) => m.status === "pending")
        .flatMap((m) => m.changes);

      if (!changes.length) return;

      // Apply first; only once the whole change set succeeded do we record the
      // pre-migration schema, so a failed apply leaves no stray history entry.
      const next = changes.reduce(
        (acc, change) => this.applySchemaChanges(acc, [change]),
        this.currentSchema,
      );
      this.history.push(structuredClone(this.currentSchema));
      this.currentSchema = next;
    } catch (error) {
      if (isMigrationError(error)) throw error;
      throw anansiError(
        AnansiCodes.MIG.INVALID_SCHEMA,
        "Schema transformation failed",
        { cause: error },
      );
    }
  }
}

export default MigrationEngine;
