// @asaidimu/anansi — self-contained TypeScript implementation of the
// Anansi binary wire format: schema compile/link/addressing, packet codecs,
// transforms, validation, and migrations.

export * from "./schema/types.ts";
export { Compiler, buildEnum } from "./schema/compile.ts";
export type {
  ResolvedField, ResolvedNested, ResolvedEnum,
} from "./schema/compile.ts";
export type {
  ManifestField, LinkResult, LinkedField, LinkedSlot,
  FieldDescriptor, Slot,
} from "./schema/link.ts";
export {
  link, makeDescriptor, unpackDescriptor, internalDP,
  addressForSteps, userDataDP, buildManifest,
  FD_NO_CHILD, MAX_SCHEMA_SLOTS, MULTI_STEP_BASE,
} from "./schema/link.ts";
export { container as DataTypes } from "./schema/dt.ts";

export {
  encodeDocument, decodeDocument,
  encodeBatchRows, encodeBatchColumnar, decodeBatch,
} from "./wire/packet.ts";
export type { EncodeKind } from "./wire/packet.ts";

// Friendly facade — one object per schema, cached by the consumer.
export { AnansiCodec } from "./codec.ts";
export type { AnansiCodecOptions } from "./codec.ts";

// Migration types: schema changes, data transforms, migrations, version bumping.
export type {
  SchemaChange,
  Migration,
  DataTransform,
  TransformFunction,
  ChangeImpact,
  FieldPatch,
  ConstraintPatch,
  IndexPatch,
  Patch,
  PatchOp,
} from "./schema/migration.ts";
export {
  classifyChangeImpact,
  calculateNextBump,
  bumpVersion,
  schemaChangeToPatch,
} from "./schema/migration.ts";

// Migration engine — schema versioning, dry-run, rollback, checksums.
export { MigrationEngine, MigrationError, MigrationErrorCode } from "./engine.ts";
export type { EngineState, DryRunResult } from "./engine.ts";

// Utilities.
export { deepMerge, sha256 } from "./utils.ts";

// Validation (adopted validator): documents against schemas, and schemas
// against the meta-schema. Re-exported wholesale so additions to
// ./validation/index.ts become public automatically — do not cherry-pick
// here, a forgotten entry silently drops the export from the published
// bundle (previously lost StandardDocumentValidator).
export * from "./validation/index.ts";

// Transforms (compression / integrity / encryption) — async, browser-safe.
export {
  encodeAnansiPacket, decodeAnansiPacket,
  encodeAnansiBatchRows, encodeAnansiBatchColumnar, decodeAnansiBatch,
  FLAG_COMPRESSED, FLAG_ENCRYPTED, FLAG_HASH_PRESENT,
} from "./wire/transforms.ts";
export type { EncodeTransforms, DecodeTransforms } from "./wire/transforms.ts";
