/**
 * errors.ts
 *
 * Anansi's error vocabulary: registered `SystemError` codes (via
 * @asaidimu/utils-error — the TypeScript mirror of Go's `common.SystemError`),
 * plus the builders and predicates shared across the package.
 *
 * Codes are registered at module load so `new SystemError({ code })` always
 * resolves full metadata (category, httpStatus, logLevel, action) instead of
 * the generic unknown-code fallback. Registration is guarded so a re-evaluated
 * module (HMR, duplicated dependency) never throws.
 */
import {
  SystemError,
  isKnownErrorCode,
  registerErrorCode,
  type ErrorCodeMetadata,
  type SystemErrorParams,
} from "@asaidimu/utils-error";
import {
  VersionCodes,
  VERSION_CODE_METADATA,
} from "@asaidimu/utils-version";

export { SystemError };

/**
 * Registered error codes, grouped by subsystem. Key names double as the
 * registry `name` (e.g. `AnansiCodes.MIG.TRANSFORM_ERROR` → "MIG-007" with
 * `name: "TRANSFORM_ERROR"`).
 */
export const AnansiCodes = {
  /** Migration engine (engine.ts). */
  MIG: {
    INVALID_SCHEMA: "MIG-001",
    INVALID_MIGRATION: "MIG-002",
    CHECKSUM_MISMATCH: "MIG-003",
    TIMEOUT: "MIG-004",
    MEMORY_LIMIT: "MIG-005",
    CONCURRENT_OPERATION: "MIG-006",
    TRANSFORM_ERROR: "MIG-007",
    VERSION_NOT_FOUND: "MIG-008",
    CIRCULAR_DEPENDENCY: "MIG-009",
    STREAM_ERROR: "MIG-010",
    ROLLBACK_ERROR: "MIG-011",
    MISSING_TRANSFORM: "MIG-012",
  },
  /** Semantic versioning (@asaidimu/utils-version). */
  VER: VersionCodes,
  /** Schema parse/compile/link + SchemaUtil (schema/*). */
  SCH: {
    SCHEMA_NAME_MISSING: "SCH-001",
    SCHEMA_NOT_FOUND: "SCH-002",
    SCHEMA_REF_INVALID: "SCH-003",
    SCHEMA_LITERAL_UNSUPPORTED: "SCH-004",
    SCHEMA_SLOT_LIMIT: "SCH-005",
    VALIDATOR_BUILD_FAILED: "SCH-006",
  },
  /** Wire codecs, values, transforms (wire/*). */
  WIR: {
    VARINT_OVERFLOW: "WIR-001",
    END_OF_BUFFER: "WIR-002",
    UNSAFE_INTEGER: "WIR-003",
    UNSUPPORTED_VALUE: "WIR-004",
    UNKNOWN_VALUE_TAG: "WIR-005",
    SCHEMA_VERSION_EXCEEDED: "WIR-006",
    TRANSFORM_UNAVAILABLE: "WIR-007",
    NOT_BATCH_PACKET: "WIR-008",
    DIGEST_MISMATCH: "WIR-009",
    CRYPTO_KEY_INVALID: "WIR-010",
    SIZE_LIMIT_EXCEEDED: "WIR-011",
    UNKNOWN_DATA_POINT: "WIR-012",
  },
  /** Validation graph construction (validation/graph.ts). */
  GPH: {
    VALIDATION_GRAPH_CYCLE: "GPH-001",
  },
} as const;

type GroupName = keyof typeof AnansiCodes;
/** Any registered anansi code (union for autocomplete, widened with `string`). */
export type AnansiCode =
  | { [G in GroupName]: (typeof AnansiCodes)[G][keyof (typeof AnansiCodes)[G]] }[GroupName]
  | (string & {});

const CODE_METADATA: Record<string, Omit<ErrorCodeMetadata, "code">> = {
  // ── MIG ────────────────────────────────────────────────────────────────
  [AnansiCodes.MIG.INVALID_SCHEMA]: {
    name: "INVALID_SCHEMA",
    description: "Migration target schema failed validation",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.MIG.INVALID_MIGRATION]: {
    name: "INVALID_MIGRATION",
    description: "Migration definition is malformed (e.g. no changes)",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.MIG.CHECKSUM_MISMATCH]: {
    name: "CHECKSUM_MISMATCH",
    description: "Stored migration checksum does not match the recomputed one",
    category: "system",
    httpStatus: 500,
    logLevel: "error",
  },
  [AnansiCodes.MIG.TIMEOUT]: {
    name: "TIMEOUT",
    description: "Migration operation exceeded its time budget",
    category: "system",
    httpStatus: 408,
    logLevel: "warn",
  },
  [AnansiCodes.MIG.MEMORY_LIMIT]: {
    name: "MEMORY_LIMIT",
    description: "Migration operation exceeded its memory budget",
    category: "system",
    httpStatus: 507,
    logLevel: "error",
  },
  [AnansiCodes.MIG.CONCURRENT_OPERATION]: {
    name: "CONCURRENT_OPERATION",
    description: "Another migration operation is already in progress",
    category: "concurrency",
    httpStatus: 409,
    logLevel: "warn",
    action: "Retry once the in-flight migration finishes",
  },
  [AnansiCodes.MIG.TRANSFORM_ERROR]: {
    name: "TRANSFORM_ERROR",
    description: "A migration transform failed or is unsupported",
    category: "system",
    httpStatus: 500,
    logLevel: "error",
  },
  [AnansiCodes.MIG.VERSION_NOT_FOUND]: {
    name: "VERSION_NOT_FOUND",
    description: "Requested version not found in migration history",
    category: "business",
    httpStatus: 404,
    logLevel: "info",
  },
  [AnansiCodes.MIG.CIRCULAR_DEPENDENCY]: {
    name: "CIRCULAR_DEPENDENCY",
    description: "Migration dependency graph contains a cycle",
    category: "validation",
    httpStatus: 400,
    logLevel: "error",
  },
  [AnansiCodes.MIG.STREAM_ERROR]: {
    name: "STREAM_ERROR",
    description: "Error while streaming migration data",
    category: "network",
    httpStatus: 502,
    logLevel: "error",
  },
  [AnansiCodes.MIG.ROLLBACK_ERROR]: {
    name: "ROLLBACK_ERROR",
    description: "Rollback failed while restoring the previous state",
    category: "system",
    httpStatus: 500,
    logLevel: "error",
    action: "Inspect the wrapped cause; the history snapshot may need manual recovery",
  },
  [AnansiCodes.MIG.MISSING_TRANSFORM]: {
    name: "MISSING_TRANSFORM",
    description: "A transform required by a migration change is missing",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },

  // ── VER ────────────────────────────────────────────────────────────────
  // Owned and registered by @asaidimu/utils-version (module load order:
  // this import registers the codes before the guarded loop below runs).
  ...VERSION_CODE_METADATA,

  // ── SCH ────────────────────────────────────────────────────────────────
  [AnansiCodes.SCH.SCHEMA_NAME_MISSING]: {
    name: "SCHEMA_NAME_MISSING",
    description: "Schema definition requires a name",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.SCH.SCHEMA_NOT_FOUND]: {
    name: "SCHEMA_NOT_FOUND",
    description: "Referenced schema not found",
    category: "validation",
    httpStatus: 404,
    logLevel: "warn",
  },
  [AnansiCodes.SCH.SCHEMA_REF_INVALID]: {
    name: "SCHEMA_REF_INVALID",
    description: "Schema reference is unresolvable or ambiguous",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.SCH.SCHEMA_LITERAL_UNSUPPORTED]: {
    name: "SCHEMA_LITERAL_UNSUPPORTED",
    description: "Unsupported literal type in schema",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.SCH.SCHEMA_SLOT_LIMIT]: {
    name: "SCHEMA_SLOT_LIMIT",
    description: "Schema exceeds link slot or per-schema field limits",
    category: "validation",
    httpStatus: 413,
    logLevel: "error",
  },
  [AnansiCodes.SCH.VALIDATOR_BUILD_FAILED]: {
    name: "VALIDATOR_BUILD_FAILED",
    description: "Validation graph could not be built from the schema",
    category: "system",
    httpStatus: 500,
    logLevel: "error",
    action: "Validate the schema against the meta-schema; check the wrapped cause for the failing reference",
  },

  // ── WIR ────────────────────────────────────────────────────────────────
  [AnansiCodes.WIR.VARINT_OVERFLOW]: {
    name: "VARINT_OVERFLOW",
    description: "Varint exceeds the maximum representable range",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.WIR.END_OF_BUFFER]: {
    name: "END_OF_BUFFER",
    description: "Input ended before the declared structure was fully read",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.WIR.UNSAFE_INTEGER]: {
    name: "UNSAFE_INTEGER",
    description: "Integer is not a safe JS integer",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.WIR.UNSUPPORTED_VALUE]: {
    name: "UNSUPPORTED_VALUE",
    description: "Value type cannot be encoded or decoded on the wire",
    category: "validation",
    httpStatus: 400,
    logLevel: "error",
  },
  [AnansiCodes.WIR.UNKNOWN_VALUE_TAG]: {
    name: "UNKNOWN_VALUE_TAG",
    description: "Unknown any-value tag byte",
    category: "validation",
    httpStatus: 400,
    logLevel: "error",
  },
  [AnansiCodes.WIR.SCHEMA_VERSION_EXCEEDED]: {
    name: "SCHEMA_VERSION_EXCEEDED",
    description: "Declared schema version exceeds the supported maximum",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.WIR.TRANSFORM_UNAVAILABLE]: {
    name: "TRANSFORM_UNAVAILABLE",
    description: "Requested transform support is not enabled in this build",
    category: "system",
    httpStatus: 501,
    logLevel: "error",
    action: "Rebuild with the required transform support or avoid packets that need it",
  },
  [AnansiCodes.WIR.NOT_BATCH_PACKET]: {
    name: "NOT_BATCH_PACKET",
    description: "Operation requires a batch packet",
    category: "validation",
    httpStatus: 400,
    logLevel: "warn",
  },
  [AnansiCodes.WIR.DIGEST_MISMATCH]: {
    name: "DIGEST_MISMATCH",
    description: "Packet integrity digest does not match the payload",
    category: "system",
    httpStatus: 500,
    logLevel: "error",
    action: "Discard the packet; it was truncated or tampered with in transit",
  },
  [AnansiCodes.WIR.CRYPTO_KEY_INVALID]: {
    name: "CRYPTO_KEY_INVALID",
    description: "Encryption key size is invalid or a required key/nonce is missing",
    category: "validation",
    httpStatus: 400,
    logLevel: "error",
  },
  [AnansiCodes.WIR.SIZE_LIMIT_EXCEEDED]: {
    name: "SIZE_LIMIT_EXCEEDED",
    description: "Declared or actual payload size exceeds the configured limit",
    category: "validation",
    httpStatus: 413,
    logLevel: "warn",
  },
  [AnansiCodes.WIR.UNKNOWN_DATA_POINT]: {
    name: "UNKNOWN_DATA_POINT",
    description: "Packet references an unknown data point address",
    category: "validation",
    httpStatus: 400,
    logLevel: "error",
  },

  // ── GPH ────────────────────────────────────────────────────────────────
  [AnansiCodes.GPH.VALIDATION_GRAPH_CYCLE]: {
    name: "VALIDATION_GRAPH_CYCLE",
    description: "Validation graph contains a dependency cycle",
    category: "validation",
    httpStatus: 400,
    logLevel: "error",
  },
};

const anansiCodeSet: ReadonlySet<string> = new Set(Object.keys(CODE_METADATA));

for (const [code, meta] of Object.entries(CODE_METADATA)) {
  if (!isKnownErrorCode(code)) {
    registerErrorCode({ ...meta, code });
  }
}

/**
 * Build a `SystemError` carrying an anansi code. `message` overrides the
 * registry description; `extra` forwards `path` / `operation` / `cause` /
 * `issues` / `severity` to `SystemErrorParams`.
 */
export function anansiError(
  code: AnansiCode,
  message?: string,
  extra?: Omit<SystemErrorParams, "code" | "message">,
): SystemError {
  return new SystemError({ code, message, ...extra });
}

/**
 * Invariant violation: a state that should be unreachable from valid input.
 * Uses the built-in `SYS-001 INTERNAL_ERROR` code rather than an anansi code.
 */
export function internalError(
  message: string,
  extra?: Omit<SystemErrorParams, "code" | "message">,
): SystemError {
  return anansiError("SYS-001", message, extra);
}

/** True for a `SystemError` carrying one of this package's registered codes. */
export function isAnansiError(error: unknown): error is SystemError {
  return error instanceof SystemError && anansiCodeSet.has(error.code);
}

/** True for a `SystemError` carrying a migration (MIG-*) code. */
export function isMigrationError(error: unknown): error is SystemError {
  return error instanceof SystemError && error.code.startsWith("MIG-");
}
