# VERIFICATION

Everything the README and the invariants document claim, executed and recorded.
Run on 2026-08-31 against a stack started from a **clean volume**:

```bash
docker compose -f infra/compose/docker-compose.yml down -v
docker compose -f infra/compose/docker-compose.yml up -d --build
```

Ports were remapped for this run (`LEDGERCORE_LEDGER_PORT=18081`, etc.) because
the host already had services on 5432, 4222 and 8080 — which is itself one of the
findings below. Requests use `X-Tenant-Id` because development mode has auth
disabled.

---

## 1 · The stack comes up from nothing

```
$ docker compose -f infra/compose/docker-compose.yml ps
gateway         Up
identity        Up
ledger-core     Up
nats            Up (healthy)
postgres        Up (healthy)
reconciliation  Up
webhooks        Up

$ curl -s -o /dev/null -w '%{http_code}' http://localhost:18081/healthz   # 200
  ... 18082 → 200   18083 → 200   18084 → 200
```

### Two reproducibility defects found and fixed here

**Fixed host ports.** The first `up` failed outright:

```
Error response from daemon: ... Bind for 127.0.0.1:4222 failed: port is already allocated
```

Every published port is now `${VAR:-default}`, and NATS is no longer published to
the host at all — nothing outside the compose network talks to it.

**A failed first `up` poisons the volume.** After that failure, every subsequent
start failed with:

```
goose run: ERROR: no schema has been selected to create in (SQLSTATE 3F000)
```

PostgreSQL had initialised partially — 2 of 5 roles created — and then skipped
its init script forever, because it only runs one on an *empty* data directory.
Nothing in the docs said so. The recovery (`down -v`) is now in the README's
Running locally section.

## 2 · The quickstart

```
POST /v1/ledgers                                    → 201
POST /v1/accounts  assets:cash        asset  DEBIT  → 201
POST /v1/accounts  liabilities:...    liability CREDIT → 201
POST /v1/accounts  revenue:fees       revenue CREDIT   → 201
```

An early attempt used `"type":"income"` and was rejected:

```json
{"error":{"code":"validation_failed",
          "message":"type must be one of asset|liability|equity|revenue|expense"}}
```

Recorded because it is the API behaving well: a stable code and a message that
names the allowed values.

**Balanced transaction** — USD 100.00 in, 97.00 to the customer, 3.00 to fees:

```json
{"id":"01a05895-e3fd-...","status":"posted","postings":[
  {"account_id":"...cash",   "direction":"DEBIT", "amount":{"asset":"USD","amount":"10000"}},
  {"account_id":"...wallet", "direction":"CREDIT","amount":{"asset":"USD","amount":"9700"}},
  {"account_id":"...fees",   "direction":"CREDIT","amount":{"asset":"USD","amount":"300"}}]}
```

**Balance:**

```json
{"data":[{"asset":"USD","exponent":2,"posted":"9700","pending":"0",
          "available":"9700","held":"0",
          "posted_debits":"0","posted_credits":"9700","version":1}]}
```

## 3 · Red team — every case that must fail

| # | Attempt | Result |
|---|---|---|
| B | Debit 10000 vs credit 5000 | `unbalanced_transaction` — *"asset USD has debits 10000 and credits 5000"* |
| C | Debit USD 10000 vs credit EUR 10000 | `unbalanced_transaction` — USD has debits and no credits. **No implicit FX.** |
| D | Same key, same payload | `200` + `X-Idempotent-Replay: true` |
| E | Same key, different payload | `idempotency_conflict` |
| F | `"1.005"` at exponent 2 | `validation_failed` — must be a string-encoded integer |
| G | Amount `-100` | `validation_failed` — *"posting amounts must be greater than zero"* |
| H | `99999999999999999999999` | `validation_failed` |
| L | `9223372036854775808` (int64 max + 1) | `validation_failed` |
| J | Reverse an already-reversed transaction, with a **distinct** key | `conflict` — *"transaction is already reversed"*. With no key it replays instead; see §10 |
| S | Hold 999.00 against 50.00 available | `insufficient_funds` — *"available balance 5000 USD is less than hold amount 99900 USD"* |
| U | Capture an already-captured hold | `conflict` |
| W | Release an already-captured hold | `conflict` |

**Reversal (I).** Produced a *new* posted transaction with
`reference: "reversal-of:01a05895-e3fd-..."` and opposite directions. Balance
afterwards:

```json
{"posted":"0","posted_debits":"9700","posted_credits":"9700","version":2}
```

Net zero, gross movement still visible. Nothing was erased.

## 4 · Enforcement below the application

Run as the PostgreSQL **superuser**, bypassing every line of Go:

```
UPDATE ledger.postings SET amount = 1 WHERE amount = 10000;
  ERROR:  ledger postings are append-only
  CONTEXT: PL/pgSQL function ledger.postings_append_only() line 10 at RAISE

DELETE FROM ledger.postings;
  ERROR:  ledger postings are append-only
```

**Direct unbalanced insert.** One posting, no matching credit, inserted straight
into the tables:

```
ERROR:  double-entry violation: transaction 77077f9b-... has 1 posting(s), at least 2 are required
```

Two postings that do not sum:

```
ERROR:  double-entry violation: transaction afe2408f-... is unbalanced in 1 asset(s) (debits <> credits)
```

### The defect this uncovered

On the **first** attempt, the same insert failed with the wrong error:

```
ERROR:  function check_transaction_balance(uuid) does not exist
CONTEXT: PL/pgSQL function ledger.transactions_balance_guard() line 3 at PERFORM
```

The guard resolved its own function through the **caller's** `search_path`. From
a psql session it therefore never ran the balance check at all. It failed closed
here, but for the wrong reason — and a caller able to create objects earlier on
the path could have made the guard validate a different pair of tables.

An audit of every function confirmed the pattern: all four `SECURITY DEFINER`
functions pinned `search_path`; **none of the six `SECURITY INVOKER` guard
functions did.**

Migration `0009` pins it on all seven and schema-qualifies the call.
`TestBalanceGuardIsSearchPathIndependent` was written to hold it, and verified in
both directions — with the migration removed it fails with exactly the
diagnostic above, and passes with it restored.

## 5 · Concurrency

Parallel `curl` against the running stack.

**20 identical requests, one idempotency key:**

```
      1 201      (created)
     19 200      (replay)
balance: posted 5000     ← not 100000
```

**20 holds of USD 10.00 against an account holding USD 100.00:**

```
     10 201      (accepted)
     10 422      (insufficient_funds)
balance: held 10000, available 0
```

Exactly ten fit; not one cent over-reserved.

**10 reversals of one transaction, each with a different idempotency key** — so
idempotency cannot be what saves it:

```
      1 201      (reversal created)
      9 409      (conflict: already reversed)
balance: posted 0, posted_debits 5000, posted_credits 5000
```

## 6 · Database role separation

The `role-separation` CI job has never executed (Actions is disabled on the
account), so all four of its assertions were run by hand against a **disposable**
PostgreSQL 17, applying the real `infra/postgres/init/01-init.sql`:

```
01-init.sql applied, no errors
1) runtime roles are NOSUPERUSER + NOBYPASSRLS ......... t
2) runtime role CANNOT run DDL in its own schema ...... OK (DML still works)
3) runtime role CANNOT read a sibling schema .......... OK
4) runtime role CANNOT SET ROLE ledgercore_maint ...... OK
```

This also confirms the init script still works after the `blog` role and schema
were removed from it.

## 7 · Test suite

```
go build ./... + go vet ./...   — all 5 modules, clean
go test ./...  — 23 packages ok, 0 failures
                 (includes the PostgreSQL integration, RLS and purge suites)
```

Windows Application Control intermittently blocks Go test binaries executing from
`%TEMP%` (`fork/exec ...: An Application Control policy has blocked this file`).
This looks like a failure and is not one; the run above sets `GOTMPDIR` inside the
repository and retries such blocks.

**Fuzzing**, roughly 29 million executions total:

```
FuzzParseFormatRoundTrip          40s   15,654,062 execs   PASS
FuzzParseUnitsNeverRounds         40s   13,704,083 execs   PASS
FuzzValidateBalancedIsOrderIndependent  45s  6,983,796 execs  PASS
```

The third target initially **failed**, and correctly:

```
order changed the verdict:
  [DEBIT USD 9223372036854775807, CREDIT USD 9223372036854775807,
   CREDIT USD -73, CREDIT USD 32]
  gives "non-positive", permutation [0 1 3 2] gives "overflow"
```

Both orderings reject the transaction; only *which* error is reported differs,
because validation returns on the first problem it meets. No money is at risk, so
the production code was left alone and the test's claim was narrowed to
acceptance — which is the property that actually protects the ledger.

## 8 · Secrets

```
gitleaks git --redact .              (no project allowlist, default rules)
  63 commits scanned, 6.15 MB
  no leaks found

gitleaks dir .                       (working tree, ignored files included)
  82 findings — 81 in apps/console/.next/ (build output, git-ignored)
                 1 in infra/compose/.env  (local dev file, git-ignored,
                                           never present in any commit)
```

Full analysis in [`SECRET_AUDIT.md`](SECRET_AUDIT.md).

## 9 · Deployment being retired

```
ledgercore.sanchezavila.com          200
api.ledgercore.sanchezavila.com/healthz  200

SELECT schemaname, relname, n_live_tup FROM pg_stat_user_tables WHERE n_live_tup > 0;
 identity | outbox     | 1
 blog     | post_views | 1
```

**No tenants, no API keys, no ledger data, no signups, no personal data.** The
hosted sandbox never accumulated real usage, which is what makes its retirement a
clean deletion rather than a migration. See
[`VPS_RETIREMENT.md`](VPS_RETIREMENT.md).

## 10 · All of the above, as a runnable script

Everything in sections 2, 3 and 5 is automated in
[`../../examples/golden-scenarios.sh`](../../examples/golden-scenarios.sh):

```
$ bash examples/golden-scenarios.sh
Setup                      3 assertions
A · Deposit                4
B · Invalid transactions   6
C · Idempotency            4
D · Holds                  9
E · Reversal               6
F · Concurrency            4
G · Trial balance          1
Result   passed: 37   failed: 0
```

Writing it surfaced one more thing worth recording. An early draft asserted that
reversing twice is always a `conflict`. It is not, and the API is right:

```
reverse #1 (no key)                 -> 201, no replay header
reverse #2 (no key)                 -> 200 + X-Idempotent-Replay: true
reverse #3 (no key)                 -> 200 + X-Idempotent-Replay: true
reverse    (distinct explicit key)  -> 409 conflict
```

Without a key the endpoint derives `reversal-of:<id>`, so a retry replays the
first reversal instead of failing — which is what you want from a client that
timed out. A *different* key means "reverse it again", and that is refused. The
script now asserts both branches, and the README and `invariants.md` were
corrected to describe this rather than the simpler thing that was written first.

## 11 · Console

```
rm -rf .next && npx tsc --noEmit     → clean
npx next build                        → success, 0 errors
```

No `/blog`, no `/moderation` route remains. The earlier typecheck errors were all
stale `.next/` generated types, not source problems.
