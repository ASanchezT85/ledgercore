# Changelog

Notable changes to LedgerCore. Dates are ISO-8601.

The project is **pre-1.0**. Until `1.0.0`, a minor version may contain a
breaking change; each one is listed here explicitly.

## [Unreleased] — 0.1.0 candidate

The first open-source release. LedgerCore was previously a private codebase
backing a hosted commercial sandbox; this release ends that and makes the
project self-hostable, licensed and publishable.

### Added

- **Apache-2.0 licence** and `NOTICE`. Rationale in
  [`docs/oss-transition/LICENSE_DECISION.md`](docs/oss-transition/LICENSE_DECISION.md).
- `SECURITY.md`, `CONTRIBUTING.md`, `.env.example`, `.gitattributes`.
- **`examples/golden-scenarios.sh`** — 37 assertions covering every guarantee the
  README makes, including the concurrency races, runnable against a local stack.
- **Three property tests** using Go's built-in fuzzing, no new dependency:
  `FuzzParseFormatRoundTrip`, `FuzzParseUnitsNeverRounds`,
  `FuzzValidateBalancedIsOrderIndependent`.
- **`TestBalanceGuardIsSearchPathIndependent`**, a regression test for the fix
  below, verified to fail without it.
- Documentation, in English: `docs/architecture.md`, `docs/invariants.md`
  (13 invariants, each pointing at its test), `docs/concurrency.md`,
  `docs/api-design.md`, and `docs/oss-transition/` (state, audits, decisions,
  test strategy and the verification transcript).
- Every published container port is now overridable
  (`LEDGERCORE_GATEWAY_PORT`, `LEDGERCORE_POSTGRES_PORT`, …).

### Fixed

- **Double-entry guard depended on the caller's `search_path`** (migration
  `0009`). The six `SECURITY INVOKER` guard functions in the `ledger` schema
  resolved table and function names through the *caller's* `search_path`. From a
  session without `ledger` on its path — a `psql` session, a restore, a
  maintenance script — `COMMIT` failed with
  `function check_transaction_balance(uuid) does not exist` **instead of running
  the balance check**, and a caller able to create objects earlier on the path
  could have made the guard validate different tables. Direct SQL writes are
  precisely what migration `0003` exists to catch. `search_path` is now pinned on
  all seven functions and the internal call is schema-qualified.
- **Go module path pointed at a namespace owned by someone else.** It was
  `github.com/ledgercore/ledgercore/...`; that GitHub organisation exists and is
  not the author's, so `go get` would have resolved against a third party. Now
  `github.com/ASanchezT85/ledgercore/...`.
- **Unauthenticated signup and a 14-day tenant-deletion sweeper were always on.**
  Both are trial-service machinery: one lets anyone on the network create a
  tenant, the other hard-deletes tenants and their ledger data on a timer. They
  now require `LEDGERCORE_ENV=sandbox-public`. Also fixed the nil-pointer-in-an-
  interface trap that would have registered the endpoint anyway.
- **`docker compose up` failed on any host already using 4222, 5432 or 8080.**
  Ports are overridable and NATS is no longer published to the host.
- README quickstart referenced a compose path that does not exist
  (`infra/docker-compose.yml`) and the wrong clone URL.
- ADR-008 claimed balances are reconstructed from `postings.effective_at`. That
  column is on `transactions`, not `postings`.

### Changed

- **Retired the hosted SaaS model.** `ledgercore.sanchezavila.com` and
  `api.ledgercore.sanchezavila.com` are decommissioned; the VPS that served them
  is released. Nothing in the project depends on infrastructure the author owns.
  See [`docs/oss-transition/VPS_RETIREMENT.md`](docs/oss-transition/VPS_RETIREMENT.md).
- The retired domain is gone from the four OpenAPI documents, the console and
  both SDK READMEs. Both SDKs already defaulted to `http://localhost:8080`.
- All eight ADRs rewritten in English and anonymised. Project documentation is
  now English throughout; the console UI remains bilingual.
- Real payment-provider names in examples replaced with fictional ones.
- Repository description and topics set; no website points at the retired domain.

### Removed

- **Public blog and comment moderation** — 3 239 lines, its own `blog` schema,
  database role and migration. It was marketing for the discontinued commercial
  positioning and the only unauthenticated write path in the product.
- **Commercial pricing tiers** from the landing page.
- Pitch deck and PDFs, hosted-sandbox terms and privacy pages, internal security
  policies, VPS runbooks, brand guide, audit correspondence and phase reports —
  none of which belong in an open-source repository. Preserved outside it.
- The shared-VPS compose overlay, the Caddy snippet, the systemd unit and the
  host backup/ops scripts, all specific to the retired deployment.
- The `infra/postgres/policytest` module, whose only test covered the removed
  blog role.

### Security

- Full-history `gitleaks` scan **with the project allowlist disabled**: 63
  commits, 6.15 MB, **no findings**. Every credential-shaped string in the
  repository is a documented example or a test fixture. See
  [`docs/oss-transition/SECRET_AUDIT.md`](docs/oss-transition/SECRET_AUDIT.md).
- Every secret belonging to the retired deployment is treated as compromised and
  must never be reused. The repository's deploy key was revoked.

### Known limitations

Unchanged by this release and stated plainly in the README: no KMS, no HA, no
global rate limiting, OpenTelemetry wired but no-op, holds have no accounting
representation of their own, and GitHub Actions has never executed for this
repository.

---

## Before this

The project's earlier history was a private commercial codebase. It is not
described here as releases, and it is **not part of this repository**: it
contained a former employer's confidential material and was purged before
publication. It survives only in private mirrors held by the author.
