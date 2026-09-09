---
title: Evolution Framework
description: "The unifying layer: registry, migrations, codegen, persistence seams, and modeling tools as one system. Revised in retrospect against the built stack."
---

# Evolution: Integrating Schema Management and Code Generation

A schema defines a single dataset. But modern systems do not run on
datasets; they run on data models — structured compositions of schemas
that collectively underpin persistence, APIs, and event-driven
workflows. The Evolution Framework is the thesis's answer to that gap: a
unifying layer that treats schemas not as isolated definitions but as
interconnected components of one system, versioned together, migrated
together, and generated from together. What follows is that framework as
first conceived, with each component checked against what was actually
built.

## The registry, as built

The original framework gave the registry a grand remit: store and
version-control all schemas, manage dependencies and metadata. The
implementation is less grand and more useful. A lockfile pins every field
ID and schema version; the collection registry tracks one active version
per collection and routes reads and writes to it; schema versions
accumulate as first-class entries rather than folklore in migration
filenames. Dependencies between schemas — references, compositions —
resolve at compile time against the registry, so no consumer ever follows
a reference to discover what a type is. The registry turned out to be
less a catalog than a contract: the place where the system's memory of
itself lives. That is a smaller job than the thesis described and a more
important one.

## Evolution managed, not feared

Schema evolution management was specified as migration paths with
preserved compatibility, transformations coupled to migrations, and
structured data changes. The built form is the `migrate` lifecycle the
rest of the documentation describes in operational detail: edit the
schema, preview with `--dry-run`, generate a versioned migration,
squash the history when it grows long, roll back when it must. The
normalization side effect — field-name keys rewritten to UUIDv7, system
fields injected — means the author's working document and the
production document are different revisions of one artifact rather than
two artifacts pretending to agree.

Two things deserve emphasis because the original text hurried past them.
First, preview is load-bearing, not cosmetic: dry-run exists because
migrations touch data, and data does not forgive. Second, the same
lifecycle runs in TypeScript as a streaming engine — dry-run, checksums,
rollback to version — so the evolution story holds across the language
boundary, not just inside Go. Compatibility, it turns out, is less a
property of migrations than a discipline of previewing them.

## Codegen utilities, scoped

The framework promised Data Model SDKs in multiple languages, with
validation and transformation logic included. The honest retrospective is
narrower. Go receives the full surface — structs, enums, projections,
typed collections with CRUD, validation, and subscriptions. TypeScript
receives the wire format, validation against schemas and the
meta-schema, and the migration engine. The `faker` target generates data
for tests. What does not exist, and should not be claimed, is a
symmetric multi-language SDK program; what exists instead is code
generation aimed precisely at what each language needs and nothing it
does not. The lesson generalizes: generate exactly, not extensively.

## Persistence: a seam, not a framework

Here the retrospective requires the most candor, because the original
text promised the most: adapters mapping schemas to SQL, NoSQL, and
key-value stores behind a consistent interface. What was built is a
reference backend in embedded SQLite and a single seam —
`DatabaseInteractor` — against which other backends negotiate
capability, with the query engine partitioning work per backend rather
than assuming uniformity. A Postgres attachment exists one layer up, in
hestia. NoSQL and key-value adapters do not exist and are not on any
roadmap; nothing in the architecture forbids them, but nothing should
imply they are around the corner either.

This is the adjustment the thesis needed most and resisted longest. A
framework that abstracts every store serves every store badly; a seam
that admits one reference implementation well, and negotiates honestly
with the next, has proven sufficient for a kernel, an ERP, and a visual
studio. The ambition did not shrink. It focused.

## Modeling tools, under other names

The original framework closed with Data Studio and Data Model Studio —
visual design and composition of schemas and registries. No product by
those names was ever shipped. What shipped instead, inside hedwig's
interface, is a collections studio in everything but the name: a visual
composer for collections with fields, indexes, and composition panels, a
live preview, schema validation with draft autosave, record tables and
forms, dashboards, and a relationship graph over the domain. It manages
anansi collections against a production ERP rather than demonstrating
against examples, which is arguably the stronger credential. Names are
cheap; the capability is what the thesis actually required, and the
capability exists.

## The lifecycle, automated

Taken together, the automated lifecycle the framework envisioned is
recognizable in daily use, even where its vocabulary changed:

1. **Definition** — schemas authored as JSON (or derived from Go structs),
   registered with versions and dependencies indexed.
2. **Evolution** — diffs become versioned migrations with preview,
   squash, and rollback; transformations ride alongside.
3. **Generation** — models, DTOs, and codecs emitted per language, each
   no more and no less than that language needs.
4. **Persistence** — one reference store, one seam, capability
   negotiation per backend.
5. **Integration** — kernel, application, and visual studio composed from
   the same documents, top to bottom of the stack.

The framework, in retrospect, was never a product to be delivered. It
was a constraint to be obeyed: everything derives from the schema, and
the schema remembers everything. Where that constraint held, the system
cohered. Where it slipped, the bugs lived. That, as much as any
component, is the finding.
