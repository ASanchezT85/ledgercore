# NAMING_REVIEW

**Date:** 2026-08-31 · **Question:** keep the name *LedgerCore*, or rename before
going public?

## Recommendation

**Keep `LedgerCore`.** Rename only if the project later acquires commercial
ambitions that make trademark and search position matter. The name is generic
and contested, and this review does not pretend otherwise — but the two
namespaces that actually gate distribution are already owned by the author, and
the one that is not is also not needed.

---

## What was checked

### GitHub

| Namespace | Status |
|---|---|
| `github.com/ASanchezT85/ledgercore` | **Owned.** The canonical repository URL. |
| `github.com/ledgercore` (organisation) | **Taken by a third party.** Created 2025-10-04, zero public repositories, no profile, no contact. Squatted or abandoned. |

The organisation being unavailable was, until this transition, a live defect
rather than a naming inconvenience: the Go module path pointed at it. See
[`IP_AUDIT.md`](IP_AUDIT.md#blocker-2--go-module-path-claimed-a-namespace-owned-by-someone-else).
With the module path corrected to the user namespace, the organisation is no
longer needed for anything. Go module paths under a user account are ordinary
and resolve correctly.

Searching GitHub for `ledgercore` returns several unrelated repositories. None
has meaningful traction, but the name does not identify this project uniquely.

### Package registries

| Registry | Name | Status |
|---|---|---|
| npm | `@ledgercore/sdk` | **Owned and published** at `0.1.0`, maintainer `asanchezt85`. The `@ledgercore` **scope is held**. |
| npm | `ledgercore` (unscoped) | Unregistered. |
| Packagist | `ledgercore/sdk` | **Owned and published**, maintainer `ASanchezT85`. The `ledgercore` **vendor namespace is held**. |
| Packagist | `mohamedhekal/ledgercore` | **Direct collision.** "Double-entry ledger engine for Laravel ERP/fintech: balanced journals, immutable postings, balances, and period locks." Zero downloads. |
| Packagist | `othmanhaba/ledger-core` | Near collision. "Generic double-entry ledger core package for Laravel." 87 downloads. |

The collision on Packagist is the sharpest finding: another package with the
same name, in the same category, describing almost the same feature set. It sits
under a different vendor namespace, so there is no technical conflict — a
consumer types `composer require ledgercore/sdk` and gets this project — but
someone searching "ledgercore double entry" will find both.

### Discoverability and SEO

"LedgerCore" is a compound of two of the most generic words in the domain. It
competes with `ledger-core`, `LedgerCore`, `Ledger Core`, and the SEO gravity of
Ledger SAS (the hardware wallet company), whose name dominates the term
"ledger" in any search that touches finance and software.

Realistically, this project will not win an organic search for its own name. A
reader will arrive via a direct link — a portfolio, a profile, a shared URL —
not via a query.

### Cost of renaming now

| Item | Cost |
|---|---|
| Two published packages | Deprecate `@ledgercore/sdk` and `ledgercore/sdk`, republish under the new name, keep the old ones pointing at the new. Recoverable, but permanently visible. |
| Two public SDK repositories | Rename; GitHub redirects, so tolerable. |
| Go module path | Another 76-file rewrite. Cheap — just done. |
| Identifiers throughout | `LEDGERCORE_*` environment variables, the `LedgerCore\` PHP namespace, `X-LedgerCore-Signature`, the `LEDGERCORE` NATS stream, database role prefixes, the console. Mechanical but wide. |
| Documentation and diagrams | Everything written during this transition. |
| An existing published article | Already public under the current name. |

Perhaps a day of work, plus two package deprecations that stay on the public
record.

## The decision

Weighing it:

**For keeping it.** The names that gate distribution — the npm scope and the
Packagist vendor — are already owned *and already published under*. That is the
scarce resource, and it is secured. The GitHub organisation is unobtainable but
unnecessary now that the module path is correct. The name is accurate,
pronounceable, and says what the thing is in one word. And per the project's own
priority order, discoverability ranks below correctness, safety, reproducibility
and documentation; a rename buys position in a race this project is not
entering.

**Against.** The name is generic, contested by at least one direct competitor in
the same category, and unwinnable in search.

The deciding consideration is what this project is *for*. It is a reference
implementation and an engineering portfolio piece, evaluated by people who
follow a link and read the code. For that audience the name is a label, not a
customer acquisition channel. Spending a day renaming, and burning two published
package versions, to improve a metric that does not affect the goal is exactly
the kind of trade this project should not make.

**Decision: keep `LedgerCore`.** Documented here so it reads as a decision and
not an oversight.

### If it is revisited later

Trigger conditions: the project acquires commercial intent, or the Packagist
collision starts causing real confusion.

Requirements for a replacement: not a `ledger`+noun compound; a free GitHub
organisation, npm scope and Packagist vendor; no collision inside fintech or
blockchain; and pronounceable in English and Spanish.

The migration would be: register the namespaces first, rename the repositories
(GitHub redirects), rewrite the module path, publish new packages, deprecate the
old ones with a pointer, and keep the old names reserved.

## Actions taken from this review

1. Module path corrected to `github.com/ASanchezT85/ledgercore/...` — this was a
   correctness blocker, independent of naming.
2. Every reference to the retired `ledgercore.sanchezavila.com` domain removed.
   The canonical identity of the project is now the GitHub repository.
3. Both published packages keep their names; their READMEs no longer point at a
   dead host.
