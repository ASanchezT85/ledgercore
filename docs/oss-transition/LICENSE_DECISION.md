# LICENSE_DECISION

**Date:** 2026-08-31 · **Decision: Apache-2.0** for the monorepo.
The two SDK repositories keep **MIT**, which they already carry.

## Prerequisite: does the author own the code?

A licence can only be applied by whoever holds the rights. Checked before
choosing one:

- **Every commit in the history is authored by the repository owner.** No
  outside contribution exists, so no contributor agreement or relicensing
  consent is needed from anyone.
- **No third-party code was found.** The IP audit confirmed the project is
  greenfield — no file, schema or algorithm copied from any prior employer's
  system. See [`IP_AUDIT.md`](IP_AUDIT.md).
- **Every dependency is permissive** — MIT, BSD-3-Clause or Apache-2.0. No GPL,
  AGPL, SSPL or BUSL anywhere in the graph, so nothing in the dependency tree
  constrains the outbound licence.

The prerequisite holds.

## Options considered

### Apache-2.0 — chosen

- Permissive: use, modify, redistribute, sell, keep changes private.
- **Includes an express patent grant** (§3) and terminates it for anyone who
  brings a patent claim against the project.
- Requires preserving the notice and stating changes — a trivial burden.
- The default for infrastructure software: Kubernetes, Terraform, most of the
  CNCF, and Formance, the closest comparable in this category.

### MIT — rejected for the monorepo

Shorter and more widely recognised, and already used for the SDKs. Rejected for
the core on one point: **it says nothing about patents.** The implied licence a
court might read into it is not a written grant.

That matters more here than for a typical library. This is financial
infrastructure, a field with a long history of business-method and
payment-processing patents. An adopter's legal review will ask what happens if
the author, or a future acquirer of the author's rights, asserts a patent
against a user of the code. Apache-2.0 answers that in writing; MIT leaves it to
argument.

### GPL-3.0 / AGPL-3.0 — rejected

AGPL is the obvious lever for a ledger: it would force anyone offering a hosted
version to publish their modifications. Rejected because it contradicts what
this project is for. The goal is that the code be read, run, learned from and
borrowed — including inside a proprietary system. AGPL makes a company's legal
review the first gate a reader has to pass, and most will not bother. Copyleft
protects a commercial position this project has deliberately given up.

### Source-available (BUSL, Elastic, Commons Clause) — rejected

These prohibit competing commercial use and are **not open source** under the
OSI definition. The project's positioning is explicitly open source, and calling
a source-available licence "open source" is a misrepresentation the project will
not make.

## Why the mismatch with the SDKs is fine

| Repository | Licence |
|---|---|
| `ledgercore` (core, contracts, console) | Apache-2.0 |
| `ledgercore-sdk-typescript` | MIT |
| `ledgercore-sdk-php` | MIT |

MIT is compatible with Apache-2.0 in both directions for these purposes, and the
combination is common: a permissive core with even-lighter-weight clients. The
SDKs are thin HTTP wrappers with a tiny patent surface, they are already
published under MIT at `v0.1.0`, and relicensing published packages to solve a
non-problem would be churn. Keeping MIT also lowers the friction for the
audience most likely to vendor an SDK into an existing application.

## What was applied

- `LICENSE` — the verbatim Apache-2.0 text, fetched from apache.org, with the
  appendix completed: `Copyright 2026 Alexander Sanchez Torrejano`.
- `NOTICE` — required by Apache-2.0 §4(d) for attribution. Deliberately minimal;
  it grows only if the project ever bundles third-party code that requires
  attribution.
- `README.md` states the licence and links here. The previous "Propietaria"
  claim is gone.
- `CONTRIBUTING.md` states that contributions are licensed under Apache-2.0.
  This is the inbound=outbound convention that Apache-2.0 §5 already implies; no
  separate CLA is used, because a CLA is bureaucracy a single-maintainer project
  does not need and a deterrent to drive-by contributions.

## Per-file headers

Not added. Apache-2.0 recommends them but does not require them; a `LICENSE` at
the root covers the work. Adding a boilerplate header to several hundred files
adds noise to every diff for no practical gain at this size. If the project ever
distributes compiled artefacts where the root file may be separated from the
source, revisit it.

## What the licence does not do

Stated because it is a common misreading: **Apache-2.0 is a copyright licence,
not a warranty and not an endorsement.** §7 and §8 disclaim warranty and
liability entirely. Nothing about this licence implies the software is fit for
handling real money — that question is answered by the README's
[Limitations](../../README.md#limitations), not by the licence file.
