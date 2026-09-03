---
title: Thesis
description: "The academic thesis behind the whole effort — problem, theory, and unifying framework — revised in retrospect against what was built."
---

# Thesis

This section is the theory behind the stack, preserved as theory. Written
before implementation for the original anansi system, revised here in
retrospect — same ambitions, same premises, checked against what go-anansi,
hestia, and hedwig actually became. Start with the [editorial](/thesis/editorial)
for the verdicts, then read the chapters in order.

| Chapter | Question it answers |
| --- | --- |
| [Editorial: the plan vs reality](/thesis/editorial) | What survived, what transformed, what is still open |
| [Enterprise systems evolution](/thesis/enterprise-systems-evolution) | Why do enterprise models drift, and why don't existing tools stop it? |
| [Schema driven development](/thesis/schema-driven-development) | Why should one document drive storage, validation, evolution, and codegen? |
| [Evolution framework](/thesis/evolution-framework) | How do registry, migrations, codegen, persistence seams, and modeling tools compose into one system? |

The old system's API specifications (`interfaces/*`, `reference/*`,
`persistence*`, `registry`, `migrations`, `schema-definition`) are
deliberately not carried here — they specify an architecture this stack
superseded, and their subject matter now lives, corrected, in the
[reference](/reference/schema-format). What remains is the part with no
other home: the argument for why any of this exists.
