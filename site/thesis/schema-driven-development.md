---
title: Schema Driven Development
description: "The thesis: a single schema document as the source of truth for storage, validation, evolution, and code generation. Revised in retrospect against the built stack."
---

# Schema Driven Development

## Core concepts

Contemporary practice describes one model three times over: the language
type system says what a program holds, the database schema says what a
store holds, the API contract says what a wire carries. Three
descriptions, maintained separately, drifting separately — each precise
in its own idiom, none aware of the others. The premise of this work is
that the fragmentation is unnecessary. A single schema document can serve
as the source of truth, independent of any implementation technology,
with every layer derived rather than duplicated.

Such a document must satisfy several masters at once. It has to be
precise enough to generate storage, legible enough to serve as
documentation, stable enough to version, and neutral enough that no
single consumer owns it. What was written here optimistically — that one
document could drive validation, code generation, documentation, version
control, and change tracking together — has since become the ordinary
working description of the stack: one JSON schema per collection, read by
SQLite DDL, Go structs, the query engine, and the TypeScript codec. The
ambition survived; only its tense changed, from proposal to report.

## The evolution problem, answered

Of all the claims in the original thesis, the one about change was the
most exposed. Long-lived systems must adapt their structures while
preserving existing data and maintaining functionality, and migration
practice — manual scripts, careful orchestration, held breath — is where
confidence goes to die. The thesis demanded a systematic answer, and
named its parts plainly: version tracking and dependency management, data
transformation with validation, rollback capabilities, and deployments
that do not take the system down.

The answer, as built, rests on four load-bearing pieces, and it is worth
naming them because none of them is the glamorous part. Field IDs pin a
field's identity across renames, so a rename is never mistaken for a
delete plus an add. A lockfile pins the registry. Migrations are
versioned artifacts with dry-run preview, squashing for long histories,
and rollback — generated from schema diffs, never hand-written. And the
whole lifecycle runs identically in TypeScript, as a streaming engine, so
data in flight migrates under the same rules as data at rest. The
unglamorous machinery turned out to matter more than the migration format
itself. That was not predicted. It is the main lesson.

Honesty requires the boundary as well. What the machinery versions is
structure; what it cannot version is meaning. Altering what a field
*means*, as opposed to its shape, still calls for human judgment with
every migration. The thesis never claimed otherwise, and neither does the
implementation — a limit worth stating plainly, since limits honestly
declared are what separate a thesis from a sales pitch.

## One schema, many readers

Enterprise systems rarely hold a single model. They hold many, related
ones, and the original thesis asked, reasonably, for relationship
definitions between schemas, consistency across their boundaries,
efficient querying across them, and change management that spans
dependencies. Multiplicity, in other words, with the same guarantees as
singularity.

As built, the composition story runs through the document itself. Nested
schemas compose object shapes under globally unique field IDs; references
mount shared shapes without copying; unions and composites express
variant structure; indexes and constraints declare storage and validation
behavior alongside everything else. Cross-schema querying runs through
the hybrid engine — SQL pushdown where the backend allows, in-memory
residual where it doesn't. The composition proved sufficient for an
entire ERP's domain, built on this stack. And the boundary that held is
worth underlining: relationships are *declared* in the schema and
*executed* by the layers, never the reverse. The document describes; the
machinery obeys.

## Codegen with discipline

Code generation was always the bridge between the document and the
working system — typed structs, enums, projections, collection wrappers,
validation logic — and the original thesis already sensed the accompanying
dangers: build complexity, workflow friction, generated code
maintenance, version-control noise. Implementation added two corrections
of its own, both matters of discipline rather than technology.

First, the generated file is overwritten on every run, without exception.
Custom logic lives in purpose-named files; any workflow that edits output
is broken by design, and the generator says so in its own header.
Second, generation targets are not symmetric, and should not be. Go gets
the full surface, from structs through persistence wrappers; TypeScript
gets the wire format, validation, and migrations — a codec with
guarantees, not an SDK with pretensions. Generating less, exactly, beat
generating more, approximately.

The older anxieties resolved, in the end, into a single rule: the schema
is the only file anyone edits, and everything else is derived on demand.
When that rule holds, codegen disappears into the workflow and nobody
thinks about it again. When it doesn't, no tooling saves you. There is no
third case.

## Persistence without capture

Finally, the thesis asked for database-agnostic queries, pluggable
backends, transactions, and caching — while warning, rightly, that
abstraction layers have a habit of becoming their own complexity. The
implementation answered by scoping the ambition down to a seam rather
than a framework: one reference backend in embedded SQLite, one interface
for the rest, and a query engine that negotiates capability per backend
instead of pretending all backends are alike. Transactions, caching —
id-cache, artifact repos, pooled documents — and the event bus all sit
above the seam and never touch a driver.

Narrower than promised, then, and more honest than planned. It has proven
sufficient for everything built on it so far — a kernel, an ERP, a visual
studio — and sufficiency, in infrastructure, is the highest praise
available. The seam remains where a wider backend story can attach; it
simply declines to pretend that story has already been written.
