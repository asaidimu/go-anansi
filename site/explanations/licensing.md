---
title: "Licensing"
description: "AGPLv3-or-later with a commercial license available. The CLA grants the maintainer dual-licensing rights. Read this before contributing or embedding in a closed-source product."
---

# Licensing

Go-Anansi is licensed under the **GNU Affero General Public License v3**
(AGPLv3-or-later). The published npm package `@asaidimu/anansi` shares this
license.

## What AGPLv3 means

AGPLv3 is a strong copyleft license. The "A" matters: **network use is
distribution.** If you run a modified Anansi as part of a network-accessible
service, you must offer the source code — including your modifications — to
users of that service.

For most open-source projects (internal tools, OSS SaaS, anything where
source disclosure is acceptable), AGPLv3 is fine. For closed-source SaaS,
embedded products, or proprietary distribution where source disclosure is
not acceptable, AGPLv3 doesn't fit.

This is a deliberate choice. The maintainer wants Anansi to be open and
usable for open-source work, while preserving a commercial licensing track
for closed-source use.

## Commercial license

If the AGPLv3's network-copyleft terms do not fit your use case — embedded
products, SaaS without source disclosure, proprietary distribution — a
**commercial license** is available from the copyright holder. See the
[LICENSE.md](https://github.com/asaidimu/go-anansi/blob/main/LICENSE.md)
file's commercial licensing section.

The commercial license is a separate agreement between you and the copyright
holder. It subsumes all contributions made under the CLA (see below) so the
maintainer can offer a single, unified commercial license to licensees.

## The CLA and dual licensing

All contributions are made under the
[CLA](https://github.com/asaidimu/go-anansi/blob/main/CLA.md), which grants
the maintainer the right to distribute contributions under **both** licensing
tracks (AGPLv3 and commercial). This is what enables the project to stay
dual-licensed:

- Open-source users get AGPLv3.
- Commercial licensees get a separate, more permissive license from the
  copyright holder.
- Contributions don't fragment the license — the CLA ensures the maintainer
  can relicense all contributions uniformly.

Without a CLA, every contributor would retain copyright on their changes,
and the maintainer couldn't offer a commercial license that subsumes those
contributions. Each contributor would need to be tracked down for
permission, which is impractical for a project that wants to accept community
contributions.

## How to accept the CLA

Acceptance is recorded via the **PR checklist** — there's no separate signed
document to mail in. When you open a PR, the checklist includes a "I have
read and accept the CLA" item. Checking it constitutes acceptance.

If you're contributing on behalf of an employer, confirm they're OK with the
CLA terms before checking the box. The CLA is between you and the project;
your employer's policies are your responsibility.

## Decision matrix

| Your use case | License path |
| --- | --- |
| Internal tool, source is fine | AGPLv3, no further action |
| Open-source SaaS, source disclosure OK | AGPLv3, no further action |
| Closed-source SaaS | Commercial license required |
| Embedded product, proprietary distribution | Commercial license required |
| Public API serving Anansi-modified code | AGPLv3 applies, source disclosure required |
| Contributing a PR | Accept the CLA via the PR checklist |

## Read the full text

- [LICENSE.md](https://github.com/asaidimu/go-anansi/blob/main/LICENSE.md) —
  the full AGPLv3 + commercial licensing section.
- [CLA.md](https://github.com/asaidimu/go-anansi/blob/main/CLA.md) — the
  Contributor License Agreement.

## Related

- [CLA](/contribute/cla) — the contribution-side explainer.
- [Getting started](/contribute/getting-started) — the contribution process.
