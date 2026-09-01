// MigrationEngine — manages schema versioning, migrations, and rollback.
//
// This is a schema-only engine: it tracks version state and applies structural
// changes. Data transformation is the caller's responsibility — the engine
// provides hooks (`onMigrate`, `onRollback`) so callers can wire up their own
// ETL pipeline.

import type {
  DataTransform,
  Migration,
  SchemaChange,
} from "./schema/migration.ts";
import { schemaChangeToPatch, bumpVersion, calculateNextBump } from "./schema/migration.ts";
import type { SchemaDefinition } from "./schema/generated.ts";
import { sha256, deepMerge } from "./utils.ts";

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
      throw new Error(`anansi/engine: path "${part}" not found`);
    }
    parent = Array.isArray(parent)
      ? parent[parseInt(part, 10)]
      : parent[part];
  }
  return { parent, key: parts[parts.length - 1] };
}

function applyPatchOps(target: any, ops: Array<{ op: string; path: string; value?: any; from?: string }>): any {
  // Deep-clone via JSON round-trip (schema objects are JSON-safe)
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
    result.version = version; // ensure version stays in sync
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
  schema: SchemaDefinition;
  migrations: Migration[];
}

// ── MigrationEngine ─────────────────────────────────────────────────────────

export class MigrationEngine {
  private currentSchema: SchemaDefinition;
  private migrations: Migration[];
  private history: SchemaDefinition[];
  private processing = false;

  constructor(
    schema: SchemaDefinition,
    migrations?: Migration[],
    history?: SchemaDefinition[],
  ) {
    this.currentSchema = schema;
    this.migrations = migrations
      ? [...migrations].sort((a, b) => a.schemaVersion.localeCompare(b.schemaVersion))
      : [];
    this.history = history
      ? [...history].sort((a, b) => a.version.localeCompare(b.version))
      : [];
  }

  // ── Public API ──────────────────────────────────────────────────────

  /** Current engine state (read-only snapshot). */
  data(): EngineState {
    return {
      schema: this.currentSchema,
      migrations: [...this.migrations],
      history: [...this.history],
    };
  }

  /** Add a pending migration. */
  async add(opts: {
    changes: SchemaChange[];
    description: string;
    rollback?: SchemaChange[];
    transform?: string | DataTransform;
  }): Promise<Migration> {
    this.assertIdle();
    if (!opts.changes.length) {
      throw new Error("anansi/engine: migration must include changes");
    }

    const migration: Omit<Migration, "checksum"> = {
      id: Date.now().toString(),
      schemaVersion: this.currentSchema.version,
      changes: opts.changes,
      description: opts.description,
      status: "pending",
      rollback: opts.rollback,
      transform: opts.transform,
      createdAt: new Date().toISOString(),
    };

    const checksum = await migrationChecksum(migration);
    const full: Migration = { ...migration, checksum };
    this.migrations.push(full);
    return full;
  }

  /**
   * Dry-run: simulate migration (forward or backward) without modifying
   * internal state. Returns the projected schema and list of migrations
   * that would be applied.
   */
  async dryRun(
    direction: "forward" | "backward",
    version?: string,
  ): Promise<DryRunResult> {
    this.assertIdle();
    const relevant = this.getRelevant(direction, version);

    let simulated = { ...this.currentSchema };

    if (direction === "forward") {
      for (const m of relevant) {
        const bump = calculateNextBump(m.changes, simulated);
        const nextVersion = bumpVersion(simulated.version, bump);
        simulated = applySchemaChanges(simulated, m.changes, nextVersion);
      }
    } else {
      // Walk backwards through history
      const historyCopy = [...this.history];
      for (let i = 0; i < relevant.length; i++) {
        const prev = historyCopy.pop();
        if (prev) simulated = { ...prev };
      }
    }

    return { schema: simulated, migrations: relevant };
  }

  /**
   * Apply pending forward migrations.
   *
   * `transform` — optional async function called for each migration.  Receive
   * the migration and a `ReadableStream` of data chunks; return a transformed
   * stream.  If omitted, only schema state is updated (no data).
   */
  async migrate(
    transform?: (
      migration: Migration,
      data: ReadableStream<unknown>,
    ) => Promise<ReadableStream<unknown>>,
  ): Promise<ReadableStream<unknown>> {
    this.assertIdle();
    this.processing = true;

    try {
      const pending = this.migrations.filter((m) => m.status === "pending");
      this.validateMigrations(pending);

      // Apply schema changes
      this.history.push(structuredClone(this.currentSchema));
      for (const m of pending) {
        const bump = calculateNextBump(m.changes, this.currentSchema);
        const nextVersion = bumpVersion(this.currentSchema.version, bump);
        this.currentSchema = applySchemaChanges(this.currentSchema, m.changes, nextVersion);
      }

      // Build a synthetic data stream (empty by default — caller fills it)
      let stream = new ReadableStream<unknown>({
        start(ctrl) { ctrl.close(); },
      });

      // Apply transforms in order
      if (transform) {
        for (const m of pending) {
          stream = await transform(m, stream);
        }
      }

      // Mark applied
      for (const m of pending) m.status = "applied";

      return stream;
    } finally {
      this.processing = false;
    }
  }

  /**
   * Roll back the last applied migration, or all migrations back to
   * `version`.
   */
  async rollback(
    version?: string,
    transform?: (
      migration: Migration,
      data: ReadableStream<unknown>,
    ) => Promise<ReadableStream<unknown>>,
  ): Promise<ReadableStream<unknown>> {
    this.assertIdle();
    this.processing = true;

    try {
      const relevant = this.getRelevant("backward", version);
      if (!relevant.length) {
        return new ReadableStream({ start(c) { c.close(); } });
      }

      // Walk history backwards
      for (const m of relevant) {
        const prev = this.history.pop();
        if (prev) this.currentSchema = prev;
        m.status = "rolled_back";
      }

      let stream = new ReadableStream<unknown>({
        start(ctrl) { ctrl.close(); },
      });

      if (transform) {
        for (const m of relevant) {
          stream = await transform(m, stream);
        }
      }

      return stream;
    } finally {
      this.processing = false;
    }
  }

  // ── Private helpers ─────────────────────────────────────────────────

  private assertIdle() {
    if (this.processing) {
      throw new Error("anansi/engine: operation in progress");
    }
  }

  private getRelevant(direction: "forward" | "backward", version?: string): Migration[] {
    const status = direction === "forward" ? "pending" : "applied";
    return this.migrations
      .filter((m) => {
        if (m.status !== status) return false;
        if (version) {
          // For forward: include migrations up to `version`.
          // For backward: include migrations at or after `version`.
          const cmp = m.schemaVersion.localeCompare(version);
          return direction === "forward" ? cmp <= 0 : cmp >= 0;
        }
        return true;
      })
      .sort((a, b) =>
        direction === "forward"
          ? a.id.localeCompare(b.id)
          : b.id.localeCompare(a.id),
      );
  }

  private validateMigrations(migrations: Migration[]) {
    for (const m of migrations) {
      if (!m.changes.length) {
        throw new Error(`anansi/engine: migration ${m.id} has no changes`);
      }
    }
  }
}
