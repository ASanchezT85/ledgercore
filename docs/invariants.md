# Invariants

The properties LedgerCore is supposed to hold, where each one is enforced, and
the test that would fail if it stopped holding.

An invariant with no test is a wish. If you add one to this file, add its test
in the same change; if you find one here without a test, that is a bug in the
document.

Legend for **Enforced by**:

- **DB** — a constraint, trigger or policy in PostgreSQL. Holds even against a
  direct SQL write that never touched the application.
- **App** — Go domain or application code. Gives a fast, descriptive error, but
  only on the paths that go through it.
- **DB + App** — both, deliberately. The application answers quickly; the
  database is the backstop.

---

## I-1 · Debits equal credits, per asset

Every *firm* transaction (`posted` or `reversed`) has, for each asset,
`SUM(debits) = SUM(credits)`, and at least two postings. Drafts are exempt while
they are being assembled; the check runs when they become `posted`.

**Enforced by:** DB + App.
A `DEFERRABLE INITIALLY DEFERRED` constraint trigger evaluates it at `COMMIT`
(`0003_double_entry_constraint.sql`), so postings inserted across several
statements, or out of order, are all present when it runs.
`domain.ValidateBalanced` rejects it earlier with a 422.

**Tests:**
- `TestValidateBalanced` — domain rules.
- `FuzzValidateBalancedIsOrderIndependent` — acceptance does not depend on the
  order postings arrive in.
- `TestDoubleEntryConstraintRejectsUnbalancedDirectSQL` — a direct SQL insert
  bypassing the domain is refused at `COMMIT`.
- `TestBalanceGuardIsSearchPathIndependent` — the guard still runs the real
  check from a session that does not have `ledger` on its `search_path`.

> Balancing is **per asset**. A USD debit against a EUR credit does not balance
> and is rejected; LedgerCore never converts implicitly. See I-9.

## I-2 · Posted history is append-only

A row in `postings` can never be updated or deleted. A `transactions` row moves
only `draft → posted → reversed`, and a trigger validates which columns may
change on that move.

**Enforced by:** DB.
`postings_append_only()` and `transactions_guard()` raise on
`BEFORE UPDATE OR DELETE`. This holds for a superuser as well — verified by
hand during the transition audit, not only through the application.

The one sanctioned exception is the maintenance escape hatch
(`ledger.allow_maintenance`), restricted to the `ledgercore_maint` role, which
no running service can authenticate as.

**Tests:**
- `TestPostingsAreAppendOnly`
- `TestRuntimeRoleCannotBypassAppendOnly` — the runtime role cannot use the
  escape hatch either; only the sanctioned purge function can.
- `TestPurgeExpiredSandboxTenant` — the purge path works *and* is owned by
  `ledgercore_maint`.

## I-3 · Money is exact

Amounts are `int64` minor units plus an asset code. No float is used anywhere on
a money path. Parsing rejects rather than rounds: more fractional digits than
the asset's exponent allows is an error, and so is anything outside `int64`.

**Enforced by:** App (`libs/go/money`) + DB (`amount BIGINT`,
`CHECK (amount > 0)`).

**Tests:**
- `TestParseUnits`, `TestParseFormatRoundTrip`, `TestAdd`, `TestSub` — including
  the `0.1 + 0.2` class of case and explicit overflow checks.
- `FuzzParseFormatRoundTrip` — for any `int64` and any exponent, format then
  parse returns the identical number of minor units.
- `FuzzParseUnitsNeverRounds` — anything `ParseUnits` accepts survives a round
  trip, and it never accepts more fractional digits than the exponent allows.

## I-4 · Idempotency is deterministic

The same idempotency key with the same payload returns the original result and
sets `X-Idempotent-Replay: true`. The same key with a *different* payload is an
`idempotency_conflict`, never a second transaction.

**Enforced by:** DB.
A unique constraint over `(tenant_id, idempotency_key)` plus a stored
fingerprint of the semantic payload (`0004_idempotency_fingerprint.sql`). It is
a constraint, not a check-then-insert, so two concurrent identical requests
cannot both pass the check.

**Tests:**
- `TestIdempotencyFingerprintConflict`
- `TestIdempotencyFingerprintSemanticFields` — the fingerprint covers the fields
  that change meaning and ignores the ones that do not.
- `TestReverseIdempotencyFingerprint`
- `TestCreateTransactionIdempotencyAndBalances`

## I-5 · A reversal is a new accounting event

Reversing does not modify the original transaction. It writes a new `posted`
transaction with the opposite directions, referencing the original
(`reversed_by`, `reference: reversal-of:<id>`).

Two behaviours that are easy to conflate, both verified:

- **Retrying the same reversal** — no explicit idempotency key — derives the key
  `reversal-of:<id>` and **replays**: `200` with `X-Idempotent-Replay: true`,
  returning the same reversal. Retrying after a timeout is therefore safe.
- **Asking for a second, distinct reversal** — a different explicit key — is a
  `409 conflict`.

**Enforced by:** App, on top of I-2 (the original *cannot* be edited even if the
application tried).

**Tests:** `TestReverse`, `TestReverseTransaction`, `TestDirectionOpposite`,
`TestReverseIdempotencyFingerprint`, and scenario E of
[`examples/golden-scenarios.sh`](../examples/golden-scenarios.sh), which asserts
both branches and that the gross `posted_debits` / `posted_credits` grow rather
than being rewritten.

## I-6 · A balance is derivable from postings

Every balance is an aggregation over postings. `posted`, `available`, `pending`
and `held` are reported separately, along with the gross `posted_debits` and
`posted_credits` behind the net figure. `account_balances` is maintained in the
**same transaction** as the postings that move it, so it is a cache that cannot
lag, and a verifier can recompute it from scratch and detect drift.

**Enforced by:** App, with a DB-side verifier
(`ledger.verify_account_balances`, `0006_balance_verifier.sql`).

**Tests:**
- `TestBalanceMath`, `TestRunningBalance`
- `TestVerifyBalancesDetectsDrift` — drift between the materialised balance and
  the postings is detected rather than trusted.
- `TestTrialBalanceAsOfReconstructsHistory` — the balance at a past date is
  reconstructed by aggregating postings over their transaction's `effective_at`.
- `TestStatementOpeningRunningClosing` — opening + movements = closing.

## I-7 · A tenant cannot reach another tenant's data

Row-Level Security is `FORCE`d, with a `USING` and a `WITH CHECK` clause, on
every business table. Services connect as `NOSUPERUSER NOBYPASSRLS` roles and
set `app.tenant_id` per transaction through `pgxutil.WithTenantTx`.

**Enforced by:** DB.

**Tests:** `TestRLSCrossTenantIsolation`, `TestTenantIsolation`, and a
`rls_isolation_test.go` in each of the four services. These run **as the real
runtime role**, not as a superuser, which is the only way the assertion means
anything.

## I-8 · Every service stays inside its own schema

`ledger`, `identity`, `recon` and `webhooks` are separate schemas. A service's
runtime role has privileges on its own schema only, and cross-schema access is
explicitly revoked.

**Enforced by:** DB grants and explicit cross-schema `REVOKE`s in
`infra/postgres/init/01-init.sql`.

**Test:** the `role-separation` CI job, which applies the real `01-init.sql` to a
clean PostgreSQL and then asserts, as the actual `ledgercore_ledger_rt` role,
that it:

1. is `NOSUPERUSER` and `NOBYPASSRLS`;
2. cannot run DDL even in its own schema, while DML still works;
3. **cannot `SELECT` from a sibling schema**;
4. cannot `SET ROLE ledgercore_maint`.

**Caveat, stated because it matters:** GitHub Actions has never executed for this
repository (billing), and `scripts/ci-local.sh` does not include this gate — it
deliberately avoids requiring `psql`. All four assertions were therefore run by
hand against a disposable PostgreSQL during the transition audit and passed; see
[`VERIFICATION.md`](oss-transition/VERIFICATION.md). Until CI actually runs, this
invariant is verified but not continuously enforced.

## I-9 · Asset boundaries are respected

A posting carries an asset code; an amount without one is impossible to
construct. Balancing is per asset, so a transaction mixing currencies without a
matching pair on each side is rejected as unbalanced. There is no implicit FX
anywhere.

**Enforced by:** DB + App.

**Tests:** `TestValidateBalanced` (mixed-asset cases),
`FuzzValidateBalancedIsOrderIndependent` (its seeds mix assets),
`TestAssetExponent`.

## I-10 · A hold reduces availability without touching posted balance

A hold is a **reservation**, not an accounting entry. Creating one lowers
`available` and leaves `posted` unchanged. A hold larger than `available` is
`insufficient_funds`. Capturing links the hold to a separately posted
transaction — capture does not create accounting entries of its own. Capturing
or releasing twice is a `conflict`.

**Enforced by:** App.

**Tests:** `TestHoldLifecycle`, and `0008_hold_request_hash.sql` extends I-4 to
hold requests.

> Because holds are a reservation layer, they have no representation in the
> trial balance. That is a deliberate limitation, stated in the README, not an
> oversight.

## I-11 · Webhook delivery is at-least-once, and signatures are verifiable

Deliveries retry with backoff and are marked dead after a maximum number of
attempts. Each carries an HMAC signature over a timestamped payload; verification
rejects a tampered body, a tampered timestamp, the wrong secret and a stale
timestamp. Rotation keeps the previous secret valid for a grace window.

**Enforced by:** App.

**Tests:** `TestSignKnownVectors`, `TestVerifyRejectsTamperedBody`,
`TestVerifyRejectsTamperedTimestamp`, `TestVerifyRejectsWrongSecret`,
`TestVerifyRejectsStaleTimestamp`, `TestProcessSchedulesBackoffOnFailure`,
`TestProcessMarksDeadAfterMaxAttempts`, `TestSafeClientRejectsLoopback`,
`TestSafeControlBlocksMetadata` (SSRF).

**Not guaranteed:** exactly-once delivery, and ordering between events.
Consumers must be idempotent.

## I-12 · An event exists only if its transaction committed

Events are written to an `outbox` table in the same database transaction as the
state change. A poller (`FOR UPDATE SKIP LOCKED`) publishes them to NATS
afterwards. If the transaction rolls back, the event never existed; if the
publish fails, it is retried from the table.

**Enforced by:** DB (same-transaction insert) + App (poller).

**Tests:** the outbox is exercised through the service integration tests. There
is no test that kills the poller mid-publish — see
[`TEST_STRATEGY.md`](oss-transition/TEST_STRATEGY.md#known-gaps).

## I-13 · Credentials are never stored in the clear

API key secrets are stored as SHA-256 hashes with a prefix, and shown once. JWT
signing keys and webhook subscription secrets are encrypted at rest with a
master key supplied by the environment; the service refuses to start in a
hardened deployment without one.

**Enforced by:** App.

**Tests:** `TestCreateAPIKeyStoresOnlyHash`,
`TestEnsureSigningKeyEncryptsAtRest`,
`TestEnsureSigningKeyEncryptedWithoutMasterKeyFails`,
`TestEnsureSigningKeyWrongMasterKeyFails`, `TestTokenIssuanceWithEncryptedKey`,
and the webhooks `secret_crypt_test.go`.

**Not guaranteed:** the master key itself is an environment variable, not a
managed KMS. See the README's Limitations.

---

## What is deliberately *not* an invariant

- **Exactly-once webhook delivery.** At-least-once, always.
- **Event ordering.** NATS consumers are idempotent; they do not assume order.
- **A stable error code when an input is wrong in several ways at once.**
  Validation returns on the first problem it meets, so a payload that is both
  negative and overflowing may report either. It is always *rejected* — that
  part is the invariant, and it is what the property test asserts.
- **Holds appearing in the trial balance.** They are a reservation layer.
- **Cross-currency balancing.** There is no FX. See I-9.
