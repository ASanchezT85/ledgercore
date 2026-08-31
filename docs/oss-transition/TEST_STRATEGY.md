# TEST_STRATEGY

What is tested, at which level, and — the more useful half — what is not.

## Inventory

| Kind | Where | Count |
|---|---|---|
| Unit | `*_test.go` across all modules | 160 `Test*` functions in 39 files |
| Property / fuzz | `libs/go/money`, `ledger-core/internal/domain` | 3 `Fuzz*` targets |
| Integration (needs PostgreSQL) | `*/internal/adapters/postgres/` | 15 files, env-gated |
| Tenant isolation (RLS) | `rls_isolation_test.go` × 4 services | 4 files |
| API / handler | `internal/adapters/http*/` | included above |
| Role separation | `role-separation` CI job (psql over the real `01-init.sql`) | 4 assertions |
| Contract | `contracts` CI job: OpenAPI lint + drift against the console's copies | 2 checks |
| SDK | `sdks/php/tests` (4 files), `sdks/typescript/test` (3 files) | — |
| Secret scanning | `secret-scan` CI job (gitleaks) | — |

Integration tests skip cleanly when `LEDGERCORE_TEST_ADMIN_URL` is unset, so
`make test-go` works on a bare checkout with no database.

## Principles

**An invariant without a test is a wish.** Every entry in
[`invariants.md`](../invariants.md) names the test that would fail if it stopped
holding. Where none exists, the document says so instead of implying coverage.

**Tests run as the real role.** The integration suites do not connect as a
superuser. Each service's `TestMain` provisions the actual role model — migrator,
maint, and a `NOSUPERUSER NOBYPASSRLS` runtime role — runs migrations as the
migrator, then runs the suite as the runtime role. An RLS test executed as
superuser proves nothing, because superusers bypass RLS.

**Prefer the database as the assertion.** The strongest tests here bypass the
application entirely and write SQL directly, because that is the threat model:
`TestDoubleEntryConstraintRejectsUnbalancedDirectSQL` inserts an unbalanced
transaction with raw SQL and asserts the `COMMIT` fails.

**A test that cannot fail is not a test.** The deleted blog isolation test
carried a comment recording that an earlier draft had `REVOKE`d the privileges it
then asserted were absent — staying green even after a deliberate `GRANT`. The
rule that came out of it: a test must never set up the posture it is checking.

## Levels

### Unit

Pure domain logic with no I/O: `ValidateBalanced`, `Reverse`, balance
arithmetic, the money package, webhook signature construction and verification,
the CSV matcher.

### Property / fuzz

Go's built-in fuzzing, no dependency. Three targets, chosen where a table test
structurally cannot reach:

| Target | Property |
|---|---|
| `FuzzParseFormatRoundTrip` | For any `int64` and any exponent, format-then-parse returns the identical minor units. Money cannot change value by being displayed. |
| `FuzzParseUnitsNeverRounds` | Anything `ParseUnits` accepts survives a round trip, and it never accepts more fractional digits than the exponent allows. |
| `FuzzValidateBalancedIsOrderIndependent` | Whether a set of postings is *accepted* does not depend on the order it arrives in. |

Seeds run as ordinary tests. Extended fuzzing is manual:

```bash
go test ./money/ -run '^$' -fuzz FuzzParseFormatRoundTrip -fuzztime 60s
```

Each was fuzzed for 40–45 seconds during the transition — roughly 29 million
executions in total, no failures after the third target's property was corrected.
That third target **did** find a real order-dependence: a posting set that is
both negative and overflowing reports whichever error the loop reaches first.
Both orderings reject it, so the ledger is safe; the assertion was too strong,
and the finding is documented in the test itself.

### Integration

Against a real PostgreSQL, exercising migrations, triggers, constraints and RLS.
Covers the deferred balance trigger, append-only enforcement, the idempotency
fingerprint, the maintenance-role purge, the balance verifier, trial balance and
statement reconstruction, and hold lifecycles.

### Concurrency

Automated in [`examples/golden-scenarios.sh`](../../examples/golden-scenarios.sh),
which fires parallel `curl` at a running stack and asserts the outcome. It is a
shell script rather than a Go test, so it needs a live stack and is not part of
`make test-go`. The races it covers:

| Race | Result |
|---|---|
| 20 identical idempotent requests | 1 created, 19 replayed; one accounting effect |
| 20 holds against funds for 10 | 10 accepted, 10 `insufficient_funds`; nothing over-reserved |
| 10 concurrent reversals, distinct keys | 1 created, 9 `conflict`; history intact |
| Reversal retried with no key | replays the same reversal (200 + `X-Idempotent-Replay`) |

Transcript in [`VERIFICATION.md`](VERIFICATION.md). Automating these is the
highest-value gap below.

### Contract

`contracts/openapi/*.yaml` is the source of truth. CI lints it and checks it has
not drifted from the copies the console serves. The SDKs are generated from it
and have their own unit tests with a fake transport.

## CI

Seven jobs in `.github/workflows/ci.yml`: `go` (build/vet/test matrix),
`pg-integration` (per-service PostgreSQL matrix), `role-separation`, `web`,
`compose-validate`, `secret-scan`, `contracts`.

> **GitHub Actions is disabled on this repository, and no commit has ever been
> validated by automated CI.**
>
> The history: while the repository was private, every run recorded
> `startup_failure` with zero jobs. Publishing it was expected to fix that —
> Actions is free and unmetered on public repositories — and a run did trigger
> on the first public push. It failed the same way: fourteen jobs, every one
> dead in four seconds with zero steps executed. That is the signature of the
> runner never starting, i.e. an account-level block, not a cost and not a
> defect in the workflow.
>
> Leaving it that way would put a permanently red badge on a project whose tests
> pass, which misinforms a reader more than silence does. So the failed runs were
> deleted and Actions was switched off for this repository.
>
> **The workflow file stays**, because it is accurate and because it works in a
> fork: Actions is free on public repositories, so anyone who forks this and
> pushes gets the full seven-job pipeline on their own account.
>
> This remains the single largest gap in the project's assurance story, and the
> README says so.

`scripts/ci-local.sh` is the substitute: it runs the same gates against a clean
`git archive` checkout, so the result is evidence about the tree as it would be
cloned, not about a developer's working directory. It deliberately requires no
`psql` and no goose CLI — each service's `TestMain` provisions everything through
`pgx`. It does **not** cover the `role-separation` gate, for the same reason.

## Known gaps

Listed so nobody has to discover them by being surprised.

1. **CI does not run.** See above. "Green" means "green on a machine", not
   "green on every commit". `scripts/ci-local.sh` is the substitute and runs the
   same gates from a clean checkout, but nothing runs it automatically.
   Dependabot is configured and, with Actions off, equally inert.
2. **Concurrency is covered by a shell script, not by the Go suite.**
   `examples/golden-scenarios.sh` asserts the races, but it needs a running
   stack and nothing runs it automatically. They should also exist as Go tests
   firing goroutines at a real database.
3. **Simultaneous capture-and-release of one hold** is reasoned about in
   [`concurrency.md`](../concurrency.md) but not tested. Sequential double
   capture and release-after-capture are covered.
4. **No crash-injection.** The outbox's at-least-once behaviour follows from
   same-transaction insert plus `SKIP LOCKED`, but nothing kills a poller between
   publishing and stamping to prove recovery.
5. **`role-separation` is CI-only.** Not reachable from `make test-go` or
   `ci-local.sh`. A contributor cannot easily run it locally.
6. **No load or performance testing.** No throughput figure is published, and
   none should be quoted.
7. **No SDK end-to-end test.** Both SDKs are tested against a fake transport, so
   nothing verifies a real request against a running LedgerCore. A drifted
   contract would pass.
8. **Console coverage is thin.** Typecheck and build only; no component or
   end-to-end tests.
9. **No mutation testing**, so the assertion strength of 160 tests is unmeasured.

## What to do next, in order

1. Get CI running. Everything else is worth less while gap 1 stands.
2. Automate the three concurrency races (gap 2) — they protect the invariants
   most likely to break under a refactor.
3. Add one SDK end-to-end test per language against a compose-started stack
   (gap 7).
4. Make `role-separation` runnable locally (gap 5).

## Running the tests

```bash
make test-go                       # vet + unit, no database
LEDGERCORE_TEST_ADMIN_URL='postgres://postgres:postgres@localhost:5432/ledgercore' make test-go
scripts/ci-local.sh                # full gate from a clean git archive checkout
go test ./money/ -run '^$' -fuzz FuzzParseFormatRoundTrip -fuzztime 60s
```

On Windows, some Application Control policies block test binaries running from
`%TEMP%`; set `GOTMPDIR` to a directory inside the repository. The symptom is
`fork/exec ...: An Application Control policy has blocked this file`, which looks
like a test failure but is not one.
