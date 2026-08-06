# Trial ledger: container-validator attempt (reverted 2026-08-06)

Working tree was reset to HEAD (324e32b). This file records the bugs discovered
during the reverted trial so the work can be re-done from a clean baseline.

## Bugs discovered

1. **Composite-with-union flattening demands all variants' required fields.**
   `compositeMergedFields` (`core/schema/validator/container_complex.go`) merged
   EVERY union variant's required fields into the composite object. So
   `Constraint = [ConstraintMetadata, ConstraintUnion]` with union
   `[ConstraintRule, ConstraintGroup]` demands the `ConstraintGroup` fields
   `operator`/`rules` even for rule-style constraints `{"name","predicate"}`.
   Reproduced as `TestMetaSchema/Self-Validation` failing with
   `REQUIRED_FIELD_MISSING` at `constraints.<id>.operator` / `.rules`
   (58 issues container path, 279 map engine). The ORIGINAL map engine solves
   this in `buildCompositeNode`: object parts are inlined individually, union
   parts become a `UnionValidationNode` requiring at-least-one-variant-match.
   Correct container fix: a `valueComposite` check that ANDs object-part
   required fields and ORs union-part variants (do not merge variant fields).

2. **Map-engine group-violation codes differ from test expectation.**
   `TestValidator_Validate_Constraints` (PartialStrict "field1=0 (group passes,
   standalone fails)") expected `[NOT_POSITIVE]` but engine reported
   `[CONSTRAINT_GROUP_VIOLATION NOT_ZERO NOT_POSITIVE]`.

3. **Released-document reads are graceful, not panicking.** After `d.Release()`
   (`d.c = nil`, `d.pool = nil`), `resolveFieldMode` returns `keyErr` and
   `GetOr` returns the default value. A committed test
   (`TestModelCollection_ReleasesConvertedDocument`) asserted a panic; the
   correct behavior (per the refactored getters) is a graceful default. Fixed
   in the trial by updating the test; re-apply that test change when re-doing.

## State at reset

- Validator files lived at `core/schema/validator/` (moved from
  `core/schema/definition/`), built on `ValidationConfig.CompiledSchema` +
  `pool.FromMap` + `vd.Validate(doc)` (container path).
- Container parity probe (`core/schema/meta/zz_probe_test.go`) reached 32/32
  stress-case HITs.
- Registry `AddSchemaVersion` + `cmd/anansi` schema gen routed through
  `schema.ValidateSchema` (container path).
- Meta stress/self-validation tests routed through `DevelopmentSchemaContainer`.
