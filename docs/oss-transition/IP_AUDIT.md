# IP_AUDIT

**Date:** 2026-08-31 · **Scope:** `ASanchezT85/ledgercore` and both SDK
repositories, current tree and full history

## Summary

| Classification | Count | Status |
|---|---|---|
| BLOCKER | 2 | Both resolved in the working tree; **one requires a history rewrite before publication** |
| NEEDS_REWRITE | 1 | Resolved |
| NEEDS_REMOVAL | 1 | Resolved |
| SAFE | rest | — |
| NEEDS_OWNER_CONFIRMATION | 0 | — |

**No third-party code was found in this repository.** LedgerCore is greenfield:
no file, schema, or algorithm was copied from any prior employer's system. Every
dependency is permissively licensed and compatible with Apache-2.0.

The problems found were of a different kind: **internal knowledge about a
previous employer's production system, written down in this repository's own
documents.** Not their code — their incidents, schema shapes, an internal
identifier and a specific financial figure, attributed to them by name.

---

## BLOCKER-1 · Named employer's internal defects in project documentation

**Where:** `docs/blueprint.md` §13 (a 15-row table), plus context paragraphs in
ADR-001, ADR-002, ADR-003, ADR-005 and ADR-008.

**What it contained.** A section titled "Lecciones de <employer> → decisiones de
diseño", naming the company and describing, for each row, a specific production
defect of theirs. Among them:

- a decimal-normalisation helper producing a ×1000 error, with a unit test that
  froze the bug;
- **"una deuda real de seis cifras con un proveedor"** — a six-figure liability
  to a named class of counterparty, missing from their ledger;
- an exact count of orders left uncovered by a replay;
- their money column type and the fact that currency was a foreign key into a
  monolith's catalogue;
- **`@ledger_allow_maintenance`** — a literal internal identifier from their
  codebase;
- the number of transaction types compiled as constants;
- which database engine they run, which cloud service they operate on, and the
  design of an internal audit tool.

**Why this is a blocker, not a nuisance.** Individually each is a war story;
together, attributed by name, they are a description of a specific company's
internal systems, defects and financial exposure, published by someone who
learned them from the inside. That is a confidentiality problem regardless of
whether an employment contract exists, and it is also a professional one: it is
the kind of document that makes a reader trust the author *less*.

It is worth recording that the project's **public-facing content already handled
this correctly**. The published blog post covering the same six-figure incident
says "a payment provider" and "our systems" — no company, no identifiers. The
anonymisation discipline existed; it had simply never been applied to the
internal documents, which nobody expected to publish.

**Resolution.**
- `docs/blueprint.md` removed from the repository entirely. It is a business and
  strategy document — pitch, roadmap, team, fundraising — that does not belong in
  an open-source repository on its own merits. Preserved outside the repo.
- All eight ADRs rewritten in English and anonymised. The engineering lessons are
  kept, because they are genuinely why the design is what it is; the attribution,
  the identifiers, the schema specifics and the financial figure are gone.
  ADR-005 now reads "observed in production systems the team has operated" and
  describes the failure mode, not the company.
- Verified: a case-insensitive search for the company's name and its product's
  name over the entire tree returns nothing. (The search terms are deliberately
  not written out here — reproducing them in a public document would defeat the
  exercise. They are the two names removed in commit `9a51d6a`.)

> ### Remaining action — history rewrite required
>
> The working tree is clean, but `docs/blueprint.md` and the original ADRs
> **remain in the git history** and would be readable by anyone the moment the
> repository becomes public. Removing a file in a later commit does not remove
> it from the repository.
>
> This is the one blocker still open. See
> [`FINAL_REPORT.md`](FINAL_REPORT.md) for the publication gate and the
> procedure.

## BLOCKER-2 · Go module path claimed a namespace owned by someone else

**Where:** every `go.mod` and every Go import — 76 files.

**What.** The module path was `github.com/ledgercore/ledgercore/...`.
`github.com/ledgercore` is a **real GitHub organisation, created 2025-10-04,
with zero public repositories, that the author does not control** (verified
against the GitHub API; the author's only organisation membership is
unrelated).

Publishing under that path would have meant every `go get` resolving against a
third party's namespace — broken for users, and a claim on a name that is not
the author's to make.

**Resolution.** Rewritten to `github.com/ASanchezT85/ledgercore/...` across all
76 files. Build, vet and the full test suite pass afterwards.

## NEEDS_REWRITE · Real payment-provider names as illustrative values

**Where:** `contracts/openapi/ledger.v1.yaml`, the console's served copy, and
`apps/console/lib/mock-data.ts`.

**What.** Account paths and demo data used **Thunes** and **dLocal** as example
counterparties (`assets:provider:thunes:usd`, "dLocal (pay-in LatAm)").

Nominative use of a company name in an example is legally unremarkable. The
problem is inferential: those are the providers of the author's previous
employer, so publishing them as "the examples that came to mind" leaks a fact
about that employer's provider stack, and it implies a relationship with those
companies that does not exist.

**Resolution.** Replaced throughout with the fictional `AcmePay` and `NordPay`.
No real vendor name remains in any public artefact.

## NEEDS_REMOVAL · Commercial and private-infrastructure documents

**What.** Documents that are not open-source artefacts and, in several cases,
carry private detail:

| Removed | Why |
|---|---|
| `docs/LedgerCore-Pitch.pdf`, `-Print.pdf`, `docs/pitch-deck/` | Investor material for a discontinued commercial plan |
| `docs/pricing.md`, landing pricing tiers | Pricing for a retired service |
| `docs/legal/sandbox-terms.md`, `sandbox-privacy.md` | Terms for a hosted service that will not exist |
| `docs/legal/politicas-seguridad.md`, `runbook-incidentes.md` | Internal draft policies naming the founder's personal accounts |
| `docs/runbook-vps.md`, `runbook-vps-compartido.md` | Private infrastructure: host addresses, DNS, deployment procedure |
| `docs/brand/marca.md` | Internal brand guidance |
| `docs/entrega-para-auditoria.md`, `docs/evidencia-reauditoria.md`, `docs/reportes-fase-1-5.md` | Private audit correspondence and phase reports |
| `INTEGRATION-REPORT.md` | Internal delivery report |
| `infra/compose/docker-compose.vps-shared.yml`, `Caddyfile.ledgercore.snippet`, `ledgercore-stack.service`, `scripts/backup/*`, `scripts/ops/*` | Configuration specific to the retired private host |

All preserved outside the repository. The same history-rewrite caveat as
BLOCKER-1 applies to these paths.

## SAFE · Dependencies and licensing

Every direct dependency is permissive and compatible with Apache-2.0:

| Dependency | License |
|---|---|
| `github.com/jackc/pgx/v5` | MIT |
| `github.com/google/uuid` | BSD-3-Clause |
| `github.com/nats-io/nats.go` | Apache-2.0 |
| `github.com/pressly/goose/v3` | MIT |
| `github.com/golang-jwt/jwt/v5` | MIT |
| `postgres`, `nats`, `traefik`, `golang` (images) | PostgreSQL / Apache-2.0 / MIT / BSD-3 |

The console's dependency tree is Next.js and TypeScript tooling, all MIT or
Apache-2.0. `apps/console/public/vendor/scalar.standalone.js` is a vendored copy
of Scalar's API reference bundle — MIT, committed deliberately so the docs page
works with no external network access, consistent with the licence.

No GPL, AGPL, SSPL, BUSL or other copyleft or source-available component appears
anywhere in the dependency graph.

## SAFE · Authorship

Every commit in the history is authored by the repository owner. There is no
contributed code from a third party, so applying a licence requires no
contributor agreement or relicensing consent from anyone else.

## SAFE · Data

No production data, no personal data, no real customer identifiers. Verified
directly against the running deployment: the hosted database contained **one
outbox row and one page-view row** — no tenants, no API keys, no ledger entries,
no signups, no email addresses. Every fixture in the repository is synthetic and
readable as such.

## Verdict

Two blockers were found; both are fixed in the working tree.

**One blocker remains open for publication:** the employer-identifiable material
and the private documents are still present in the git history and must be
purged before the repository's visibility changes. The working tree is safe; the
history is not.
