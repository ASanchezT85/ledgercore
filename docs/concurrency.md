# Concurrency

What happens when two requests arrive at the same time, and why the answer is
not "we hope they do not".

The general approach: **push the race to PostgreSQL and let it be settled by a
constraint or a row lock.** Application-level checks are for fast, descriptive
errors, never for correctness — a `SELECT` followed by an `INSERT` in Go has a
window between them, and money systems find that window.

---

## Isolation model

Transactions run at PostgreSQL's default **Read Committed**. That is a
deliberate choice, not an oversight: the invariants that must survive
concurrency are each protected by something stronger and more specific than an
isolation level.

| Race | Protected by |
|---|---|
| Duplicate request with the same idempotency key | Unique constraint on `(tenant_id, idempotency_key)` + payload fingerprint |
| Two holds against the same funds | `SELECT ... FOR UPDATE` on the balance row |
| Two reversals of one transaction | Conditional update on `transactions.status` |
| Unbalanced write from any path | Deferred constraint trigger at `COMMIT` |
| Event published without its state change | Same-transaction outbox insert |
| Two outbox pollers | `FOR UPDATE SKIP LOCKED` |

Serializable isolation would add retry-on-serialization-failure handling to
every write path in exchange for guarantees these mechanisms already give. If a
future invariant genuinely needs it, it should be adopted for that path, with
the retry loop written, not switched on globally and hoped for.

## Duplicate requests

Two identical `POST /v1/transactions` with the same key arriving simultaneously
race on the unique index. One inserts; the other gets a unique violation, reads
the stored fingerprint, finds it matches, and returns the original result with
`X-Idempotent-Replay: true`. If the fingerprint does not match, it is a
`409 idempotency_conflict`.

There is no window, because there is no check-then-insert: the constraint *is*
the check.

**Measured.** 20 identical requests fired in parallel against a clean stack:

```
      1 201      (created)
     19 200      (replay)
balance: posted 5000   —   not 100000
```

## Concurrent holds against the same balance

Creating a hold takes a row lock on `account_balances` for that
`(account_id, asset)` before reading availability:

```sql
SELECT posted_debits, posted_credits, held
FROM account_balances
WHERE account_id = $1 AND asset = $2
FOR UPDATE
```

Everything from the availability check to the `held` increment happens inside
that lock, so concurrent holds serialise on the account rather than all reading
the same stale availability.

**Measured.** An account funded with USD 100.00; 20 holds of USD 10.00 fired in
parallel — exactly ten fit:

```
     10 201                (accepted)
     10 422                (insufficient_funds)
balance: held 10000, available 0
```

Not one cent over-reserved.

> **Edge case, stated honestly.** If no `account_balances` row exists yet, `FOR
> UPDATE` has nothing to lock, so two first-ever holds on a brand-new account
> can both pass the lock. Both then compute an availability of zero and are both
> rejected, so no over-reservation is possible — but the protection in that
> instant comes from the account being empty, not from the lock.

## Concurrent reversals

Reversing sets `transactions.status` to `reversed` conditionally on it still
being `posted`. The first writer wins; every other one finds the row already
moved and gets `409 conflict`. The reversal transaction itself is written in the
same database transaction, so there is no state where a transaction is flagged
reversed without its compensating entries existing.

**Measured.** 10 reversals of one transaction fired in parallel, each with a
*different* idempotency key — so idempotency cannot be what saves it:

```
      1 201      (reversal created)
      9 409      (conflict: already reversed)
balance: posted 0, posted_debits 5000, posted_credits 5000
```

One reversal, and the gross history is still visible.

## Hold, capture and release

Capture and release both transition the hold's status conditionally, the same
way reversal does, so:

- capture then capture → the second is `conflict`;
- capture then release → `conflict`;
- release then capture → `conflict`.

Capture does not post accounting entries (see
[`architecture.md`](architecture.md#holds)), so there is no race between the
capture and a balance movement — the movement is a separate transaction the
caller posts, and linking it is what capture does.

**Measured** for the sequential double-capture and release-after-capture cases;
the *simultaneous* capture-and-release pair is **not** covered by an automated
test. See [`TEST_STRATEGY.md`](oss-transition/TEST_STRATEGY.md#known-gaps).

## The balance check under concurrency

The double-entry trigger is `DEFERRABLE INITIALLY DEFERRED`, so it evaluates at
`COMMIT`, when every posting of the transaction exists. That matters for
concurrency in a specific way: a transaction that inserts its postings across
several statements is never judged on a partial view of itself, and two
concurrent transactions never see each other's uncommitted postings, so neither
can be failed by the other's work in flight.

Since migration `0009`, the guard functions pin their own `search_path`, so the
check behaves identically no matter which session or role triggers it.

## The outbox poller

Pollers claim rows with:

```sql
SELECT ... FROM outbox WHERE published_at IS NULL
ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT n
```

`SKIP LOCKED` means several poller instances can run without coordinating: each
takes rows the others are not holding. There is no leader election and no
distributed lock.

A poller that crashes between publishing to NATS and stamping `published_at`
republishes on restart. That is why delivery is **at-least-once** and why every
consumer is idempotent — deterministic ids plus `ON CONFLICT DO NOTHING`.

## Webhook delivery

Deliveries are claimed with a `SECURITY DEFINER` function
(`webhooks.claim_due_deliveries`) that uses the same `FOR UPDATE SKIP LOCKED`
pattern, so multiple dispatcher instances do not double-send a claimed delivery.

Duplicates remain possible across the network boundary: a request that times out
after the receiver processed it is retried. Receivers must deduplicate on the
event id. LedgerCore does not promise exactly-once and will not.

## What is not protected

Stated plainly, because these are the places a reader should look first:

- **No global rate limiting.** Nothing in the services bounds how many
  concurrent requests a tenant can make. A single tenant can saturate the
  connection pool.
- **No queue fairness.** The outbox is FIFO by `created_at` across all tenants;
  one tenant emitting heavily delays everyone's events.
- **Simultaneous capture-and-release** of one hold is reasoned about but not
  covered by a test.
- **No chaos or crash-injection testing.** The outbox's crash-recovery behaviour
  follows from the design; nothing kills a poller mid-publish to prove it.
- **Connection-pool exhaustion** is not handled beyond pgx defaults.

## Reproducing the measurements

Every figure on this page came from the transcript in
[`oss-transition/VERIFICATION.md`](oss-transition/VERIFICATION.md), run against a
stack started with `docker compose -f infra/compose/docker-compose.yml up -d`.
They are shell loops firing `curl` in the background — crude, but they exercise
the real HTTP path, the real database and the real locks.
