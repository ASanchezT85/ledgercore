# FINAL_REPORT — LedgerCore open-source transition

**Date:** 2026-08-31 · **Baseline:** `8f59fc6`, tagged `pre-oss-transition-20260831`

---

## Verdict

# READY WITH CONDITIONS

Every gate is green except one, and that one is a hard stop:

> **The git history still contains employer-identifiable material.** The working
> tree is clean; the history is not. `docs/blueprint.md` and the original ADRs
> remain readable in earlier commits, and they name a former employer alongside
> specific internal production defects and a six-figure liability.
>
> **Do not change the repository's visibility until the history is purged.**
> The script is ready, with a mirror backup beside it:
> `C:\laragon\www\ledgercore-private\PURGE-HISTORY.sh`

It is one command. Everything else in this report is done and verified.

---

## State before

A private monorepo backing a hosted commercial ledger-as-a-service: four Go
services, a Next.js console, OpenAPI contracts, a public sandbox on a shared VPS
at `ledgercore.sanchezavila.com`, a pitch deck, pricing tiers, and a marketing
blog with reader comments. Two external audits had been passed. The README
declared the licence "Propietaria"; there was no `LICENSE` file.

64 commits, clean working tree, no tags, no CI that had ever executed.

Full inventory: [`CURRENT_STATE.md`](CURRENT_STATE.md).

## What changed

### Blockers fixed

| # | Finding | Resolution |
|---|---|---|
| 1 | **Go module path pointed at a namespace owned by someone else.** `github.com/ledgercore` is a real GitHub organisation, created 2025-10-04, zero repositories, not the author's. Every `go get` would have resolved against a third party. | Rewritten to `github.com/ASanchezT85/ledgercore/...` across 76 files. Build, vet and the full suite pass. |
| 2 | **Employer-identifiable material in project documentation.** `docs/blueprint.md` §13 and four ADRs named a former employer alongside their production defects, schema shapes, an internal identifier and a six-figure liability. | Blueprint removed from the repository; all eight ADRs rewritten in English and anonymised. **Working tree clean — history purge still pending.** |
| 3 | **No `LICENSE`.** | Apache-2.0 applied, with `NOTICE`. Ownership verified first: every commit is the author's, no third-party code, all dependencies permissive. |

### Correctness defects found and fixed

**The double-entry guard resolved through the caller's `search_path`.**

Inserting an unbalanced transaction with raw SQL — the exact scenario migration
`0003` exists to catch — failed with
`function check_transaction_balance(uuid) does not exist` instead of running the
balance check. It failed closed, but for the wrong reason, and a caller able to
create objects earlier on the path could have made the guard validate different
tables.

An audit of every function showed the pattern: all four `SECURITY DEFINER`
functions pinned `search_path`; none of the six `SECURITY INVOKER` guard
functions did. Migration `0009` pins all seven and schema-qualifies the internal
call.

The existing test could not have caught this: it runs as the runtime role, whose
`search_path` already contains `ledger`.
`TestBalanceGuardIsSearchPathIndependent` forces the opposite condition and was
verified in both directions — it fails without the migration with exactly that
diagnostic, and passes with it.

**Unauthenticated signup and a 14-day tenant-deletion sweeper were always on.**

Both are trial machinery: one lets anyone on the network create a tenant, the
other hard-deletes tenants and their ledger data on a timer, propagating the
purge across services. In a self-hosted deployment that is an anonymous write
surface and scheduled data loss. Now gated behind `LEDGERCORE_ENV=sandbox-public`
— which already forces a strong admin token, real auth and a master key. The
nil-pointer-inside-an-interface trap that would have registered the endpoint
regardless was fixed at the same time.

**Two ways the quickstart lied.** `docker compose up` failed outright on any host
already using 4222, 5432 or 8080, and a failed first `up` left PostgreSQL with a
half-initialised data directory that skipped its init script forever, producing a
message that says nothing about the actual cause. Ports are now overridable, NATS
is no longer published to the host, and the recovery is documented.

**ADR-008 claimed balances reconstruct from `postings.effective_at`.** That
column is on `transactions`. Corrected.

### What was removed

- **The public blog and comment moderation** — 3 239 lines, its own schema,
  database role and migration, and the only unauthenticated write path in a
  product about money. The isolation around it was competent; deleting it removed
  the surface instead of continuing to defend it.
- **Commercial pricing tiers** (USD 400–2 500/month) from the landing page.
- **Pitch deck and PDFs, hosted-sandbox terms and privacy pages, internal
  security policies, VPS runbooks, brand guide, audit correspondence, phase
  reports, the shared-VPS compose overlay, the Caddy snippet, the systemd unit,
  host backup and ops scripts.** All preserved outside the repository.
- The `infra/postgres/policytest` module, left empty once its only test — the
  blog role contract — was gone.

### What was added

- `LICENSE`, `NOTICE`, `SECURITY.md`, `CONTRIBUTING.md`, `.env.example`,
  `.gitattributes`, `CHANGELOG.md`.
- A README written in English: a quickstart verified command by command, a
  guarantees section where every line maps to a test, and a "does not guarantee"
  section as long as it.
- `docs/architecture.md`, `docs/invariants.md`, `docs/concurrency.md`,
  `docs/api-design.md`, and this `docs/oss-transition/` directory.
- **`examples/golden-scenarios.sh`** — 37 assertions, including the concurrency
  races, runnable in one command.
- **Three property tests** using Go's native fuzzing, no new dependency.

## Security findings

**Clean.** `gitleaks` over the full history **with the project allowlist
disabled** — 63 commits, 6.15 MB — reports **no findings**. No `.env` other than
`.env.example` has ever existed in a commit. Every credential-shaped string in
the repository is a documented example or a test fixture; each was traced
individually.

The 82 working-tree findings are all in git-ignored build artefacts
(`apps/console/.next/`) and one local development `.env` that has never been
committed.

Secrets belonging to the retired deployment are treated as compromised and must
never be reused. The repository's deploy key was revoked; it now has zero.

Detail: [`SECRET_AUDIT.md`](SECRET_AUDIT.md).

## IP findings

**No third-party code.** LedgerCore is greenfield — nothing copied from any prior
employer's system. Every dependency is MIT, BSD-3-Clause or Apache-2.0; no
copyleft or source-available component anywhere. Every commit is the author's, so
no contributor consent is needed to apply a licence.

The problems were of a different kind: internal knowledge about a named
employer's production systems, written into this repository's own documents.
Fixed in the tree; **pending in the history**.

Real payment-provider names used as illustrative values were replaced with
fictional ones — legally unremarkable, but they leaked an inference about the
employer's provider stack.

**No production data, no personal data.** Verified against the running
deployment, not assumed: the hosted database held one outbox row and one
page-view row. No tenants, no API keys, no ledger entries, no signups.

Detail: [`IP_AUDIT.md`](IP_AUDIT.md).

## Architecture

Unchanged by this transition, and sound. Four Go services over one PostgreSQL
instance, one schema each, NATS JetStream fed by a transactional outbox, a
Traefik gateway. Two rules hold it together: a service reads only its own schema,
and no service calls another on the write path.

[`../architecture.md`](../architecture.md)

## Invariants

Thirteen, documented with where each is enforced and the test that would fail if
it stopped holding — including the ones with no test, which say so.

The six that matter most were re-verified adversarially during this transition:
balanced (rejected a direct SQL insert), append-only (refused a superuser
`UPDATE` and `DELETE`), exact money (rejected excess decimals, negatives, and
`int64` max + 1), idempotent (20 parallel duplicates → one accounting effect),
reversible (gross totals grew, nothing erased), isolated (runtime role refused on
a sibling schema).

[`../invariants.md`](../invariants.md)

## Tests

```
23 packages ok, 0 failures     (unit + PostgreSQL integration + RLS + purge)
160 test functions, 3 fuzz targets
~29,000,000 fuzz executions across the three property targets, no failures
37/37 golden-scenario assertions against a live stack
4/4 role-separation assertions against a disposable PostgreSQL
```

The property test that found something found the *test's* claim was too strong,
not a defect: a posting set that is both negative and overflowing reports
whichever error validation meets first. Both orderings reject it, so the ledger
is safe; the assertion was narrowed to acceptance and the finding documented in
the test.

[`TEST_STRATEGY.md`](TEST_STRATEGY.md) · [`VERIFICATION.md`](VERIFICATION.md)

## Local setup

```bash
git clone https://github.com/ASanchezT85/ledgercore.git
cd ledgercore
docker compose -f infra/compose/docker-compose.yml up -d --build
bash examples/golden-scenarios.sh
```

Verified from a clean volume. No hosted service, no account, no key.

## SDK status

| SDK | Package | State |
|---|---|---|
| TypeScript | `@ledgercore/sdk@0.1.0` (npm) | Public, MIT, `v0.1.0`. Default base URL already `http://localhost:8080`. README fixed and pushed. |
| PHP | `ledgercore/sdk` (Packagist) | Same. |

Both namespaces are owned by the author. Neither SDK depends on any hosted
service, and the sources in the monorepo are byte-identical to the published
repositories (line endings aside) — checked, not assumed.

## Naming decision

**Keep `LedgerCore`.** The name is generic and collides with at least one direct
competitor on Packagist, and the GitHub organisation `ledgercore` belongs to a
third party. But the namespaces that gate distribution — the npm scope and the
Packagist vendor — are already owned *and published under*, the GitHub
organisation is unnecessary once the module path is correct, and renaming would
burn two published package versions to win a search race this project is not
entering.

[`NAMING_REVIEW.md`](NAMING_REVIEW.md)

## License

**Apache-2.0** for the core; the SDKs keep MIT. Chosen over MIT for the express
patent grant, which a legal review of financial infrastructure will ask about;
over AGPL because copyleft protects a commercial position this project has
deliberately given up; over source-available because calling that "open source"
would be a misrepresentation.

[`LICENSE_DECISION.md`](LICENSE_DECISION.md)

## VPS and domain retirement

**Done.** The stack, its volumes, its images, its code, its systemd unit and its
two Caddy vhosts are gone. Disk went from 52% to 45%; containers from 10 to 3.
The portfolio sharing that host was verified up at every step and never went
down.

Nothing was lost: the database held no tenant data, a final dump was taken
anyway, and the repository reproduces the whole stack with one command.

**One manual step remains:** delete the two DNS A records
(`ledgercore` and `api.ledgercore`) in the Namecheap panel, which is outside this
session's reach. They now resolve to a host that serves nothing for them.

[`VPS_RETIREMENT.md`](VPS_RETIREMENT.md)

## Remaining risks

Ordered by how much they should worry a reader.

1. **The git history is not yet purged.** The one blocker. Until then the
   repository must stay private.
2. **CI has never executed for this repository.** GitHub Actions is disabled on
   the account for billing; every run recorded `startup_failure` with zero jobs.
   The workflow is written and every job was verified by hand, but no commit here
   has been validated automatically. This is the largest gap in the assurance
   story, and the README says so.
3. **No independent security review.** Two external audits covered the earlier
   commercial codebase; nothing has reviewed it as an open-source artefact.
4. **Key management is not production-grade.** Signing keys and webhook secrets
   are encrypted at rest with a master key from the environment. A managed KMS is
   the right answer and is not implemented.
5. **No HA, no backups, no global rate limiting.** One PostgreSQL, one of each
   service, no failover.
6. **Observability is thin.** Structured logs and request ids are real;
   OpenTelemetry is wired through the standard variables and is a no-op unless
   configured.
7. **Concurrency is asserted by a shell script**, which needs a live stack and
   runs on demand. It should also live in the Go suite.
8. **No SDK end-to-end test.** Both SDKs are tested against a fake transport, so
   a drifted contract would pass.
9. **The SDK sources are duplicated** between the monorepo and the published
   repositories, kept in step by discipline rather than tooling. No drift today —
   checked.
10. **The console is Spanish-first.** Documentation and code are English; the UI
    has both locales with Spanish as default. Cosmetic, but inconsistent with the
    project's positioning.

## Publication readiness

| Gate | Status |
|---|---|
| **Security** — secret audit clean | Full history, allowlist disabled, 0 findings |
| **Security** — credentials rotated/revoked | Deploy key revoked; retired secrets dead with the host |
| **Security** — no real data | Verified against the live database: none |
| **Security** — `SECURITY.md` | Present, with a private reporting channel and an explicit scope |
| **IP** — audit clean in the tree | Yes |
| **IP** — audit clean in the history | **NO — the blocker** |
| **IP** — licence decision | Apache-2.0, ownership verified first |
| **Correctness** — invariants documented | 13, each mapped to its test |
| **Correctness** — tests | 23 packages green, 3 fuzz targets, 37 scenario assertions |
| **Correctness** — concurrency | Three races measured and asserted |
| **Correctness** — money | Property-tested; excess decimals, negatives and overflow rejected |
| **Reproducibility** — clone, run, migrate, test | Verified from a clean volume |
| **Documentation** — README, architecture, invariants, limitations, contributing | Present |
| **Packages** — SDK URLs correct, no dead-SaaS dependency | Fixed and pushed |
| **Release** — version and changelog | `CHANGELOG.md` written; tag pending the history purge |

## What to do next

1. **Run `PURGE-HISTORY.sh`**, verify its two assertions, force-push.
2. Re-run `make test-go` and `examples/golden-scenarios.sh` after the rewrite.
3. Flip the repository to public.
4. Tag `v0.1.0` and publish release notes from `CHANGELOG.md`.
5. Enable GitHub secret scanning and push protection — free for public
   repositories.
6. Delete the two DNS A records at Namecheap.
7. Get CI running. Risk 2 outranks everything else on this list once the project
   is public.
