# TODO List for Go-Anansi Production Readiness

This document outlines the immediate actionable tasks derived from the `ROADMAP.md` to achieve production readiness by July 15, 2025.

## High Priority

*   **Fix `tests/unit/persistence` Critical Failures:**
    *   Address `TestPersistence_Collection` Panic (unexpected mock call).
    *   Resolve `TestCollectionBase_Read` Data Structure Mismatches (QueryResult.Data type).
    *   Correct `TestCollectionBase_Update` Optimistic Locking Return (count mismatch).
*   **Fix `tests/unit/ephemeral` Core Logic:**
    *   Refine `SelectDocuments` Filter Logic (Contains/NotContains operators).
    *   Debug `UpdateDocuments` and `DeleteDocuments` Logic (incorrect counts/updates).

## Medium Priority

*   **Standardize Error Messages:**
    *   Harmonize error messages in `persistence` and `schema` modules.
*   **Comprehensive Code Review:**
    *   Review `core/persistence` (collection, executor, persistence, registry).
    *   Review `core/ephemeral` (interactor.go).
    *   Review `core/query` and `core/schema`.
    *   Review `sqlite` module.

## Low Priority / Verification

*   Re-run All Tests after fixes.
*   Performance Benchmarking (if applicable).
*   Update Documentation (`README.md`, `docs/`, `CHANGELOG.md`).
