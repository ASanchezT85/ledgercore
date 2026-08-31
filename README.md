# LedgerCore

**Open-source double-entry ledger service for building auditable financial systems.**

Post balanced transactions, place holds on funds, reverse mistakes without
rewriting history, and derive every balance from the postings that produced it.
The accounting invariants are enforced by PostgreSQL, not by application code.

> **Status: early-stage, pre-1.0, reference implementation.**
> LedgerCore is not a regulated financial product and has not been through
> independent review. Read [Limitations](#limitations) before considering it for
> anything that moves real money.

---

## Quickstart

Requires Docker with Compose v2. Nothing else.

```bash
git clone https://github.com/ASanchezT85/ledgercore.git
cd ledgercore
docker compose -f infra/compose/docker-compose.yml up -d --build
```

That brings up PostgreSQL, NATS, a Traefik gateway and the four Go services, and
runs the migrations. Ports are overridable — see [Running locally](#running-locally).

Development mode has authentication disabled, so the tenant comes from a header:

```bash
API=http://localhost:8081
T='X-Tenant-Id: 11111111-1111-1111-1111-111111111111'
```

**Create a ledger and three accounts**

```bash
LEDGER=$(curl -s -X POST $API/v1/ledgers -H "$T" -H 'Content-Type: application/json' \
  -d '{"name":"Quickstart"}' | jq -r .id)

mkacct() { curl -s -X POST $API/v1/accounts -H "$T" -H 'Content-Type: application/json' \
  -d "{\"ledger_id\":\"$LEDGER\",\"name\":\"$1\",\"type\":\"$2\",\"normal_balance\":\"$3\"}" | jq -r .id; }

CASH=$(mkacct assets:cash asset DEBIT)
WALLET=$(mkacct liabilities:customer:wallet liability CREDIT)
FEES=$(mkacct revenue:fees revenue CREDIT)
```

**Post a balanced transaction** — a USD 100.00 deposit, 97.00 to the customer and
3.00 to fee revenue. Amounts are string-encoded integers in minor units.

```bash
curl -s -X POST $API/v1/transactions -H "$T" -H 'Content-Type: application/json' -d "{
  \"ledger_id\": \"$LEDGER\",
  \"idempotency_key\": \"dep-0001\",
  \"description\": \"Customer USD deposit\",
  \"postings\": [
    {\"account_id\":\"$CASH\",  \"direction\":\"DEBIT\",  \"amount\":{\"asset\":\"USD\",\"amount\":\"10000\"}},
    {\"account_id\":\"$WALLET\",\"direction\":\"CREDIT\", \"amount\":{\"asset\":\"USD\",\"amount\":\"9700\"}},
    {\"account_id\":\"$FEES\",  \"direction\":\"CREDIT\", \"amount\":{\"asset\":\"USD\",\"amount\":\"300\"}}
  ]
}"
```

**Check a balance**

```bash
curl -s $API/v1/accounts/$WALLET/balances -H "$T"
```

```json
{"data":[{"asset":"USD","exponent":2,"posted":"9700","pending":"0",
          "available":"9700","held":"0",
          "posted_debits":"0","posted_credits":"9700","version":1}]}
```

**Try an invalid transaction** — 100.00 debited against 50.00 credited:

```json
{"error":{"code":"unbalanced_transaction",
          "message":"debits do not equal credits: asset USD has debits 10000 and credits 5000"}}
```

**Reverse the deposit** — a new compensating transaction, not an edit:

```bash
curl -s -X POST $API/v1/transactions/$TXID/reverse -H "$T" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: rev-0001' -d '{}'
```

The balance returns to `"posted":"0"` — but `posted_debits` and `posted_credits`
both still read `"9700"`. Nothing was erased.

Every command above was run against a clean `docker compose up` while this
README was written. The full transcript, including the cases that must fail, is
in [`docs/oss-transition/VERIFICATION.md`](docs/oss-transition/VERIFICATION.md).

---

## What LedgerCore guarantees

Each of these is enforced in code and covered by a test.

### Balanced

Every posted transaction balances, per asset. The check is a **deferred
constraint trigger in PostgreSQL** that runs at `COMMIT`, so a direct SQL
`INSERT` bypassing the application is rejected too.

### Immutable history

`postings` are append-only: `UPDATE` and `DELETE` raise an exception in the
database — including for a superuser. A `transactions` row may only move
`draft → posted → reversed`.

### Exact money

Amounts are `BIGINT` in minor units plus an asset code and exponent. No floats
anywhere on the money path. More decimals than the asset allows, negative
amounts, and values outside `int64` are rejected rather than rounded.

### Idempotent writes

An idempotency key is required on every mutation. The same key with the same
payload returns the original result with `X-Idempotent-Replay: true`; the same
key with a different payload is an `idempotency_conflict`. Uniqueness is a
database constraint over a payload fingerprint, not a check-then-insert.

### Reversible corrections

A reversal creates a new posted transaction referencing the original. Reversing
twice is a `conflict`. The original postings are never touched.

### Auditable balances

A balance is an aggregation of postings. `posted`, `available`, `pending` and
`held` are reported separately, and `posted_debits` / `posted_credits` expose the
gross movement behind the net figure.

### Tenant isolation in the database

Every business table has Row-Level Security `FORCE`d with a `WITH CHECK` clause,
so one tenant cannot read or write another tenant's rows. Each service connects
as its own `NOSUPERUSER NOBYPASSRLS` role, and the isolation tests run as those
real roles rather than as a superuser.

Schema separation between services is enforced by explicit grants and cross-schema
`REVOKE`s in [`infra/postgres/init/01-init.sql`](infra/postgres/init/01-init.sql).
That part is **not** covered by an automated test — see
[`TEST_STRATEGY.md`](docs/oss-transition/TEST_STRATEGY.md#known-gaps).

## What LedgerCore does not guarantee

- **No compliance of any kind** — no KYC, AML, sanctions screening, tax handling
  or regulatory reporting. Not PCI, not SOC 2, not audited.
- **Not a payment processor.** It records money movement; it does not move money
  and has no PSP connectivity.
- **No FX.** A transaction balances *per asset*. A USD debit against a EUR credit
  is rejected as unbalanced, not silently converted. Cross-currency flows must be
  modelled explicitly, with your own rate and a gain/loss account.
- **No universal reconciliation.** The reconciliation service matches imported
  external records against internal postings. That is one narrow kind of
  reconciliation — not settlement, not provider reconciliation.
- **Webhook delivery is at-least-once**, never exactly-once. Consumers must be
  idempotent.
- **Key management is not production-grade.** Signing keys live encrypted in the
  database under a master key supplied by the environment. A managed KMS is the
  right answer and is not implemented.
- **No independent security review**, no published benchmarks, no HA story, no
  backup automation.

## Core concepts

| Concept | Meaning |
|---|---|
| **Ledger** | A book of accounts. A tenant may have many. |
| **Account** | A named node — `assets:cash`, `liabilities:customer:wallet` — with a type and a normal balance. |
| **Transaction** | A set of postings that must balance per asset. `draft` or `posted`. |
| **Posting** | One debit or credit against one account, in one asset. Immutable. |
| **Hold** | A **reservation**, not an accounting entry. It reduces `available` and leaves `posted` untouched. Capture links the hold to a separately posted transaction; it does not create one. |
| **Reversal** | A new posted transaction that compensates an earlier one. |

Money is always `{"asset": "USD", "amount": "10000"}` — a string-encoded integer
in minor units, with the exponent held in the tenant asset registry.

## Architecture

Four Go services over one PostgreSQL instance, one schema each, with NATS
JetStream carrying events published through a transactional outbox.

```mermaid
flowchart LR
  C[Client / SDK] --> G[Traefik gateway :8080]
  G --> L[ledger-core :8081]
  G --> I[identity :8082]
  G --> R[reconciliation :8083]
  G --> W[webhooks :8084]
  L --- DB[(PostgreSQL — schemas: ledger, identity, recon, webhooks)]
  I --- DB
  R --- DB
  W --- DB
  L -- outbox poller --> N[(NATS JetStream)]
  N --> R
  N --> W
  I -. JWKS .-> L
```

A client exchanges an API key for a 15-minute Ed25519 JWT at `identity`; the
other services validate it against identity's JWKS, so no service ever reads
another service's tables.

Detail: [`docs/architecture.md`](docs/architecture.md) ·
[`docs/invariants.md`](docs/invariants.md) ·
[`docs/concurrency.md`](docs/concurrency.md) ·
[`docs/adr/`](docs/adr/)

## SDKs

| Language | Package | Repository |
|---|---|---|
| TypeScript | [`@ledgercore/sdk`](https://www.npmjs.com/package/@ledgercore/sdk) | [ledgercore-sdk-typescript](https://github.com/ASanchezT85/ledgercore-sdk-typescript) |
| PHP | [`ledgercore/sdk`](https://packagist.org/packages/ledgercore/sdk) | [ledgercore-sdk-php](https://github.com/ASanchezT85/ledgercore-sdk-php) |

Both take a configurable base URL pointing at your own deployment. Neither
depends on any hosted service.

## Running locally

```bash
docker compose -f infra/compose/docker-compose.yml up -d --build          # core stack
docker compose -f infra/compose/docker-compose.yml --profile web up -d    # + console on :3000
docker compose -f infra/compose/docker-compose.yml --profile obs up -d    # + Grafana on :3100
```

Every published port is overridable, so the stack coexists with whatever else you
already run:

```bash
LEDGERCORE_GATEWAY_PORT=18080 LEDGERCORE_POSTGRES_PORT=55432 \
  docker compose -f infra/compose/docker-compose.yml up -d
```

See [`.env.example`](.env.example) for the full list; copy it to
`infra/compose/.env` to make the values stick.

**If the first `up` fails partway** (a port clash, a Ctrl-C), PostgreSQL is left
with a half-initialised data directory and skips its init script on every later
start, producing `no schema has been selected to create in`. Reset with:

```bash
docker compose -f infra/compose/docker-compose.yml down -v
```

## Testing

```bash
make test-go
```

Runs vet plus unit tests for every Go module, no database required.
Integration, RLS-isolation and purge tests need PostgreSQL; point them at a
superuser DSN and each service provisions its own roles and schema:

```bash
LEDGERCORE_TEST_ADMIN_URL='postgres://postgres:postgres@localhost:5432/ledgercore' make test-go
```

`scripts/ci-local.sh` runs the whole gate against a clean `git archive` checkout.
See [`docs/oss-transition/TEST_STRATEGY.md`](docs/oss-transition/TEST_STRATEGY.md).

## Security

See [`SECURITY.md`](SECURITY.md). Short version: report privately, never open a
public issue for a vulnerability, and assume this project has never been
independently audited.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). One rule above the rest: **a
contribution may not weaken a financial invariant.** A change touching the ledger
schema, the money package or the guard triggers needs a test that fails without
it.

## Limitations

The honest list of what is unfinished is in
[`docs/oss-transition/FINAL_REPORT.md`](docs/oss-transition/FINAL_REPORT.md).
Highlights: no KMS, no HA, no global rate limiting, OpenTelemetry wired but
no-op, holds have no accounting representation of their own, and CI has never
actually run on GitHub Actions for this repository.

## License

[Apache-2.0](LICENSE) — see
[`docs/oss-transition/LICENSE_DECISION.md`](docs/oss-transition/LICENSE_DECISION.md).
