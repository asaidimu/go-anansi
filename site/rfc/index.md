---
title: RFCs
description: "Proposals and design notes for go-anansi. None are formally accepted yet — read with caution. The status table tracks each one."
---

# RFCs

This section contains **proposals and design notes** for go-anansi. Most are
working drafts — none are formally accepted yet, and several describe
designs that have since been partially or fully superseded by the actual
implementation.

Read these to understand *why* a part of Anansi is shaped the way it is,
or to pick up context on a planned overhaul. **Do not treat anything here
as authoritative documentation** — for the current state, see
[Reference](/reference/schema-format).

## Status table

| RFC | Status | Notes |
| --- | --- | --- |
| [Query language grammar](/rfc/query-language) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Natural-language-like grammar mapped to JSON Query DSL. Not implemented. |
| [Full-text search](/rfc/search) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Pluggable full-text search layer in the query engine. |
| [Anansi encoding](/rfc/anansi-encoding) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Wire-format design. Partially landed — see [Wire format](/explanations/wire-format). |
| [Schema encoding](/rfc/schema-encoding) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Binary encoding for the schema IR. Not landed. |
| [BadgerDB interactor](/rfc/badgerdb-interactor) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Alternative backend to SQLite. Not landed. |
| [Query engine overhaul](/rfc/query-engine-overhaul) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Plans a rewrite of `core/query`. In progress. |
| [Monorepo decomposition](/rfc/monorepo-decomposition) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Explores splitting go-anansi into separately-versioned modules. |
| [Container-backed document refactor](/rfc/container-backed-document-refactor) | <span class="rfc-badge rfc-badge--draft">DRAFT</span> | Design notes for the document.Document refactor — **landed**, kept for history. |

## RFC process

Anansi doesn't have a formal RFC process yet. These documents live in the
repo as markdown files (historically under `todo/`) and are migrated here
for visibility. If you'd like to propose a new RFC, open a GitHub issue
first to gauge interest before writing the document.

## Status definitions

- <span class="rfc-badge rfc-badge--draft">DRAFT</span> — Working
  proposal. May be partially implemented, fully unimplemented, or
  superseded.
- <span class="rfc-badge rfc-badge--accepted">ACCEPTED</span> — Formally
  accepted; implementation in progress or complete.
- <span class="rfc-badge rfc-badge--rejected">REJECTED</span> — Considered
  and declined; kept for historical context.

> **TODO:** promote RFCs to ACCEPTED status when their designs are formally
> adopted, and migrate "landed" RFCs (like container-backed-document) into
> the appropriate Reference or Explanations page instead.
