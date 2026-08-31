# CURRENT_STATE — LedgerCore as found

**Date:** 2026-08-31 · **Baseline:** the last commit before the transition began.

> The pre-transition history is **not** part of this repository. It contained a
> former employer's confidential material and was purged before publication; it
> survives only in private mirrors held by the author. Commit hashes from before
> the transition therefore do not resolve here, deliberately.

This document describes the system as it actually was when the open-source
transition started — not as it was designed or advertised.

---

## 1. Repositories

| Repository | Visibility (before) | Commits | Tags | Role |
|---|---|---|---|---|
| `ASanchezT85/ledgercore` | private | 64 | none | Monorepo: 4 Go services, shared lib, Next.js console, contracts, infra, docs |
| `ASanchezT85/ledgercore-sdk-php` | **public** | 2 | `v0.1.0` | PHP SDK, MIT |
| `ASanchezT85/ledgercore-sdk-typescript` | **public** | 2 | `v0.1.0` | TypeScript SDK, MIT |

Branches in the monorepo: `main` (default), `hardening/p0-auditoria` (merged
work, stale). Working tree was clean and in sync with `origin/main`.

Published packages, both owned by the author (verified against the registries):

- npm `@ledgercore/sdk@0.1.0` — maintainer `asanchezt85`
- Packagist `ledgercore/sdk` — maintainer `ASanchezT85`

## 2. Runtime and stack

- **Go 1.26** — four services, standard-library `net/http`, `pgx/v5`, `goose`
  migrations, `log/slog`.
  - `ledger-core` :8081 — ledgers, accounts, transactions, postings, holds, reports
  - `identity` :8082 — tenants, API keys, token exchange, JWKS
  - `reconciliation` :8083 — external source import and matching
  - `webhooks` :8084 — subscriptions and signed delivery
- **PostgreSQL 17** — one schema per service, RLS on business tables, a
  `NOSUPERUSER NOBYPASSRLS` runtime role per service, a separate migrator role
  and a `ledgercore_maint` role that owns the maintenance escape hatch.
- **NATS JetStream** — stream `LEDGERCORE`, transactional outbox + poller.
- **Traefik** :8080 — gateway.
- **Next.js console** :3000 — tenant dashboard, developer portal, marketing
  landing, blog, sandbox signup, comment moderation.
- Optional `otel-lgtm` profile for Grafana.

### Size

| Area | Lines |
|---|---|
| Go source (non-test) | 12 664 |
| Go tests | 7 707 |
| Console TypeScript | 11 074 |
| SDKs (PHP + TS, vendored deps included) | 7 013 |

40 Go test files, 160 `Test*` functions, 0 fuzz targets.

## 3. API surface

OpenAPI 3.1, one document per service, in `contracts/openapi/`:

| Document | Operations |
|---|---|
| `ledger.v1.yaml` | 20 |
| `reconciliation.v1.yaml` | 8 |
| `identity.v1.yaml` | 7 |
| `webhooks.v1.yaml` | 7 |

Plus 7 JSON Schema event contracts in `contracts/events/`.

Auth: API key (`lk_<env>_…`, stored as SHA-256 hash) exchanged for a 15-minute
Ed25519 JWT, validated by the other services against identity's JWKS.
`LEDGERCORE_AUTH_DISABLED=true` bypasses this for local development only.

## 4. Deployment as found

A shared 2 vCPU / 2 GB Namecheap VPS, also hosting the author's personal
portfolio, which owns TLS termination (Caddy) and the shared `web` Docker
network.

- 7 LedgerCore containers up (uptime ~2 weeks), 1 migration one-shot exited 0
- Volumes: `ledgercore_postgres_data` (64 MB), `ledgercore_go_cache` (776 MB)
- Code at `/opt/ledgercore` (14 MB), deployed by read-only deploy key
- `ledgercore-stack.service` (systemd, enabled) as a boot guard
- Two Caddy vhosts: `ledgercore.sanchezavila.com`, `api.ledgercore.sanchezavila.com`
- Both responded HTTP 200 at audit time
- No LedgerCore entry in `/etc/cron.d` — the backup cron in
  `scripts/backup/install-cron.sh` was never installed

**Database contents at audit time:** 1 row in `identity.outbox`, 1 row in
`blog.post_views`. No tenants, no API keys, no signups, no ledger data.
The hosted sandbox never accumulated real usage, and holds **no personal data**.

## 5. CI

`.github/workflows/ci.yml` exists (Go matrix, console build, OpenAPI lint,
Postgres integration matrix), but **GitHub Actions is disabled on the account by
billing** — every run recorded `startup_failure` with zero jobs. Evidence of a
green build was produced instead by `scripts/ci-local.sh`, which runs the same
gates against a clean `git archive` checkout.

Dependabot is configured (`.github/dependabot.yml`).

## 6. Findings that blocked publication

| # | Finding | Severity |
|---|---|---|
| 1 | Go module path was `github.com/ledgercore/ledgercore/...` — a GitHub organisation created 2025-10-04 that the author does **not** control | **Blocker** |
| 2 | `docs/blueprint.md` §13 and four ADRs described a named former employer's internal production defects, schema shapes, an internal identifier and a six-figure liability | **Blocker** |
| 3 | No `LICENSE` file; README declared "Propietaria" | **Blocker** |
| 4 | README quickstart pointed at a non-existent compose path (`infra/docker-compose.yml`) and at the wrong clone URL | High |
| 5 | Retired domain hard-coded as the first `servers:` entry in all four OpenAPI documents, in the console docs and in both SDK READMEs | High |
| 6 | Real payment-provider names (Thunes, dLocal) used as illustrative values in public specs and console demo data | Medium |
| 7 | Console carried SaaS-model surface — sandbox signup, pricing, marketing blog, comment moderation — tied to the retired hosted service | Medium |
| 8 | Repository docs were Spanish-only, which does not serve the intended audience | Medium |

## 7. What was already sound

Worth recording, because it shaped how much had to change:

- Full local stack in `infra/compose/docker-compose.yml` — Postgres, NATS,
  Traefik, the four services, a migration one-shot and profile-gated console and
  observability. **No dependency on the author's VPS to run the system.**
- Double-entry balance enforced by a **deferred constraint trigger** in
  Postgres, not by application code.
- Append-only enforced by `BEFORE UPDATE OR DELETE` triggers.
- RLS `FORCE`d with `WITH CHECK` across 17 tables, plus per-service isolation
  tests that run as the real runtime roles.
- Idempotency backed by a database fingerprint constraint.
- Webhook secrets encrypted at rest; HMAC signature with timestamp.
- The public blog post already anonymised its war stories correctly — the
  employer-identifiable material was confined to internal documents.
- A full-history `gitleaks` scan **with the repository allowlist disabled**
  found zero leaks across all 62 scanned commits.
