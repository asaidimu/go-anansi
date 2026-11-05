# Validation: Struct-Level Serialization Feasibility Report

Target: `go-anansi-struct-serialization-feasibility.pdf`, branch
`investigation/struct-api` (`1c8d3ea`–`6c523be`), supplied as
`go-anansi-perf-work.bundle`.

This document records an independent validation of the report: every
quantitative and behavioral claim was checked against the code and by running
the branch's own suites and benchmarks. The validation branch is
`validate/struct-serialization-report`, based on the bundle tip `6c523be`;
the defects found during validation are fixed on it (see "Defects found").

## Method

- Branch fetched from the bundle, based on `main` at `ddcbd0a` (v8.10.3);
  `git merge-base` confirms `ddcbd0a` is the only shared ancestor.
- Full suite: `make test` (which sets `ANANSI_ENV=development` and
  `-tags sqlite_fts5`), plus `go vet -tags sqlite_fts5 ./...`.
- Benchmarks: `go test -bench . -benchmem -run '^$'` on
  `core/dto/anansi` and `core/schema/definition`, and the two MemStats churn
  probes.
- Differential testing beyond the committed suite: randomized
  marshal/unmarshal comparisons against the container path, malformed-number
  and escape-sequence probes, prepopulated decode targets, and
  materialization comparisons.
- Machine: Intel i5-8500 (6 cores), Go 1.27.0, Linux/amd64. The report's
  numbers come from a 2-core Xeon sandbox, so absolute ns/op and B/op are not
  directly comparable; ratios and allocation counts are.

## Claims verified

| Claim | Result |
| --- | --- |
| Refactor and follow-up commits exist with the stated hashes (`1c8d3ea`, `8d33b4a`, `6aecbdb`, `2ae5064`, `320d3bb`, `6c523be`) | Verified |
| `core/schema/binding` exists below the document layer and imports neither `core/document` nor `core/data` | Verified |
| `a*` facades are document-free (`anansi.Marshal/Unmarshal`, `anansijson.Marshal/Unmarshal/MarshalAppend`) | Verified |
| Output is deterministic and carries no `_id_`/`_metadata_` | Verified (`TestDeterministicOutput`, `TestJSONOutputShape`, `TestFastDeterministic`) |
| Pool-poisoning/alias safety on the binary path (`WithCopyStrings`) | Verified (`TestUnmarshalReleaseSafety`, 7 reuse cycles) |
| Full suite green: 41 packages, zero failures, under the Makefile invocation | Verified before and after the fixes on this branch |
| `go vet` clean | Verified |
| Compiled-plan marshal/unmarshal beats `encoding/json` by ~2x+, with allocation parity | Verified: 2.0x small / 2.1x large marshal, 2.1x / 2.4x unmarshal, 1 and 3 allocs (stdlib 1 and 10); bytes 64/384 and 48/368 |
| `MarshalAppend` steady state is allocation-free and byte-identical to `Marshal` | Verified: 0 B/op, 0 allocs/op; churn probe 0.00 MB / 0 mallocs / 0 GC cycles |
| Binary wire is 169 vs 361 bytes for the Order fixture (53% smaller) | Verified exactly |
| Churn table: marshal garbage 0.38 KB/op and 11 GC cycles/100k, unmarshal 17.62 vs 25.57 MB per 50k, 3 vs 10 allocs | Reproduced within noise: 36.67 MB, 11 cycles, 17.64 vs 25.57 MB, 3.01 vs 10.00 allocs |
| `ContainerValidator` fast path: 161 ns, 0 B, 0 allocs; map pipeline 2 886/720/8 and 4 939/2 376/43; materialize 1 920/1 656/35 | Allocation and byte counts reproduced exactly; on this machine 229 ns vs 8 177 ns graph-only and 17 915 ns end-to-end, i.e. 36x and 78x faster (the report's 18x/30x is conservative) |
| Validator churn table: 226.67 MB/43.01/68, 68.68 MB/8.00/21, native exact zeroes | Reproduced: 226.90 MB/43.01/72, 68.74 MB/8.00/22, native exact zeroes |
| Error-prefix change from `document:` to `binding:` is not asserted by any repository test | Verified |
| DTO JSON shrinks versus document JSON | Verified for the Order fixture: 361 vs 572 bytes (36.9% smaller; report says 30%) |
| Persistence registry rejection at `registry.go:537` without the dev env | Verified |
| Constructor usage table dispositions (zero production call sites for the struct-to-map family) | Spot-checked: the struct-to-map constructors have no production call sites (one comment reference each), matching the report; exact test-call counts differ slightly from the table |
| `MarshalAppend` and validator fast-path alloc tests use `testing.AllocsPerRun(...) == 0` | Verified |

## Discrepancies and limits (not code defects)

- **Container-path benchmark rows cannot be reproduced.** The committed
  benchmarks measure stdlib, the compiled plan (`anansijson` now means plan
  path), the binary facade, and the document flow, but there is no benchmark
  that exercises `anansijson`'s legacy container path. Table 2's
  `anansijson` rows and Table 3's "container path" column therefore cannot be
  re-measured from the branch; the plan-vs-stdlib and plan-vs-document ratios
  can.
- **Absolute numbers are machine-specific.** Ratios and allocation counts
  match closely; absolute ns/op differ by roughly 0.7–3x in either direction.
  The report's 30x/18x validator speedups read as 78x/36x here.
- **"JSON payloads shrink by 30 percent"** is 36.9% on the Order fixture.
- **Stale statements in the report.** Section 4 says `core/schema/binding`
  imports `core/data` (it no longer does after `8d33b4a`), section 2 still
  names `data.ExtractDTOSchemaDirect` (moved to `core/schema`), and section 7
  says "40 packages" where the rest of the report (correctly) says 41.
- **"vet and gofmt clean"** holds for vet and for the new code; `gofmt -l`
  flags several touched files (`core/document/address.go`,
  `core/container/collection.go`, …), mostly pre-existing comment/alignment
  formatting plus the refactor's shims.
- **Constructor counts** are approximate: e.g. the test-call count for
  `NewDocumentFromStruct` is 8 calls, not 10, and `NewDocumentPool` has 8
  test references, not 6. The dispositions are unaffected.

## Defects found by validation (fixed on this branch)

1. **Escaped object keys failed to decode** (`core/dto/anansijson`). The
   key scanner rewound to the first byte *after* the opening quote before
   calling `parseString`, so every escaped key produced
   `anansijson: expected string`. This broke round-tripping a struct whose
   `anansi` tag contains `&`, `<`, `>`, `"`, `\` or a control character —
   the marshaller escapes those keys itself — and rejected legal JSON such as
   `{"\u0069d":1}`. Fixed; regression tests cover round-trip and hand-written
   escapes.
2. **Decoding appended to existing slices instead of replacing them.**
   `Unmarshal` into a reused target (e.g. a loop variable) appended the
   decoded elements after the old ones, unlike the container path which binds
   a fresh slice. Fixed by resetting the slice length before fill; regression
   test pins parity with the container path.
3. **Number grammar was laxer than the reference codec.** The fast path
   accepted `1.` and `1.e5` (and produced values) where the container path
   rejects them. `numberLit` now enforces a digit after `.` and in the
   exponent, and rejects leading zeros, matching the reference parser.
   Added malformed-number parity cases.
4. **Unpaired UTF-16 surrogates produced invalid UTF-8.** The fast path
   encoded lone surrogates (and over-long surrogate pairs) as raw surrogate
   code points; the container path emits U+FFFD and consumes a following
   `\uXXXX` even when it is not a valid low surrogate. The decoder now uses
   `utf16`/`utf8` with the reference parser's exact logic. Added parity
   cases.
5. **Slice growth copied pointers without write barriers.** `growSlice`
   moved the initialized prefix with a `[]byte` `copy`, which bypasses the
   GC's write barriers for pointer-bearing elements (strings, nested
   slices/structs). Growth now uses `reflect.Copy`, which shades the copied
   pointers. Allocation counts are unchanged (large unmarshal remains 3
   allocs / 368 B).
6. **`definition.Materialize` diverged from the document layer's map.**
   An explicitly-null array-of-object field materialized as `nil` here but as
   `[]` through `binding.MaterializeSlot` (the implementation behind
   `Document.ToMap`), so the validator's fallback lane could see data a
   document user's map would not. Materialize now mirrors `ToMap`; a
   regression test compares the two directly.

## Residual limitations

- **Null vs empty array-of-object in the native validator.** A directly
  built container with an explicitly-null, non-nullable array-of-object field
  makes the native lane emit
  `TYPE_MISMATCH` ("Field is not nullable"), while `DocumentValidator` on the
  document layer's map sees `[]` and reports nothing — the map representation
  cannot carry the distinction. The native behaviour is arguably the correct
  one, but it is not pinned by the differential suite. No semantic change was
  made.
- **Partial mutation on decode error.** The fast path writes fields as it
  parses, so a failed `Unmarshal` may leave the target partially updated;
  the container path leaves it untouched (it binds only after a successful
  decode). Error-presence parity is still pinned by the differential suite.
- The report's recommendation to plan-compile the container serializer and
  its other follow-ups remain unimplemented, as stated.

## Conclusion

The report's central claims — the decoupling refactor, document-free
serialization, deterministic output, the compiled-plan speedups, zero-garbage
`MarshalAppend`, and the container-native validator's cost profile — are
corroborated by the branch. Independent differential probing found six real
defects that the committed suite missed, including one (escaped keys) that
broke round-tripping for a class of valid structs and one (slice append) that
silently duplicated data on reused decode targets. They are fixed here with
regression tests, and the full suite still reports 41 packages green.
