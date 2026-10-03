import { describe, it, expect } from "bun:test";
import {
  SystemError,
  getErrorMetadata,
  isKnownErrorCode,
} from "@asaidimu/utils-error";
import {
  AnansiCodes,
  anansiError,
  internalError,
  isAnansiError,
  isMigrationError,
} from "../src/errors.ts";

const allGroups = Object.values(AnansiCodes);
const allCodes = allGroups.flatMap((g) => Object.values(g));

describe("anansi error codes", () => {
  it("registers every code in AnansiCodes", () => {
    expect(allCodes.length).toBeGreaterThan(0);
    for (const code of allCodes) {
      expect(isKnownErrorCode(code)).toBe(true);
    }
  });

  it("registers distinct codes", () => {
    expect(new Set(allCodes).size).toBe(allCodes.length);
  });

  it("resolves registry metadata for every code", () => {
    for (const [group, codes] of Object.entries(AnansiCodes)) {
      for (const [key, code] of Object.entries(codes)) {
        const meta = getErrorMetadata(code);
        expect(meta.code).toBe(code);
        expect(meta.name).toBe(key);
        expect(meta.category).toBeTruthy();
        expect(meta.description).not.toContain("Unknown error");
        expect(code.startsWith(`${group}-`)).toBe(true);
      }
    }
  });

  it("uses documented CATEGORY-XXX code format", () => {
    for (const code of allCodes) {
      expect(code).toMatch(/^(MIG|VER|SCH|WIR|GPH)-\d{3}$/);
    }
  });
});

describe("anansiError", () => {
  it("builds a SystemError with the requested code and message", () => {
    const err = anansiError(AnansiCodes.MIG.TRANSFORM_ERROR, "boom");
    expect(err).toBeInstanceOf(SystemError);
    expect(err.code).toBe("MIG-007");
    expect(err.message).toBe("boom");
    expect(err.codeMetadata.name).toBe("TRANSFORM_ERROR");
    expect(err.getHttpStatus()).toBe(500);
  });

  it("defaults the message to the registry description", () => {
    const err = anansiError(AnansiCodes.SCH.SCHEMA_NAME_MISSING);
    expect(err.message).toBe(getErrorMetadata("SCH-001").description);
  });

  it("forwards path/operation/cause", () => {
    const cause = new Error("root");
    const err = anansiError(AnansiCodes.MIG.INVALID_SCHEMA, "bad schema", {
      path: "migration-42",
      operation: "migrate",
      cause,
    });
    expect(err.path).toBe("migration-42");
    expect(err.operation).toBe("migrate");
    expect(err.cause).toBe(cause);
  });
});

describe("internalError", () => {
  it("uses the built-in SYS-001 code", () => {
    const err = internalError("invariant violated");
    expect(err).toBeInstanceOf(SystemError);
    expect(err.code).toBe("SYS-001");
    expect(err.message).toBe("invariant violated");
    expect(err.codeMetadata.name).toBe("INTERNAL_ERROR");
  });

  it("is not an anansi error (SYS-001 is built-in)", () => {
    expect(isAnansiError(internalError("x"))).toBe(false);
  });
});

describe("isAnansiError", () => {
  it("recognizes errors built with anansiError", () => {
    for (const code of allCodes) {
      expect(isAnansiError(anansiError(code, "x"))).toBe(true);
    }
  });

  it("rejects foreign SystemErrors, plain errors, and non-errors", () => {
    expect(isAnansiError(new SystemError({ code: "VAL-001" }))).toBe(false);
    expect(isAnansiError(new Error("plain"))).toBe(false);
    expect(isAnansiError({ code: "MIG-001", message: "x" })).toBe(false);
    expect(isAnansiError(null)).toBe(false);
    expect(isAnansiError(undefined)).toBe(false);
  });
});

describe("isMigrationError", () => {
  it("true only for MIG-* codes", () => {
    expect(isMigrationError(anansiError(AnansiCodes.MIG.CHECKSUM_MISMATCH))).toBe(
      true,
    );
    expect(isMigrationError(anansiError(AnansiCodes.WIR.DIGEST_MISMATCH))).toBe(
      false,
    );
    expect(isMigrationError(anansiError(AnansiCodes.VER.HISTORY_FULL))).toBe(
      false,
    );
    expect(isMigrationError(new Error("plain"))).toBe(false);
  });
});
