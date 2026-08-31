# API design

The contract lives in [`../contracts/openapi/`](../contracts/openapi/) — four
OpenAPI 3.1 documents, one per service, written before the handlers. This page
explains the conventions they encode and why.

## Shape

REST over JSON. Path-versioned: every route is under `/v1`. Resources are
plural nouns; state changes that are not CRUD are sub-resource verbs
(`POST /v1/transactions/{id}/reverse`, `POST /v1/holds/{id}/capture`) rather than
a `PATCH` with a magic field, because "reverse" and "set status to reversed" are
not the same operation and should not look alike.

| Service | Base | Operations |
|---|---|---|
| ledger-core | `/v1/ledgers`, `/v1/accounts`, `/v1/transactions`, `/v1/holds`, `/v1/trial-balance`, `/v1/statements`, `/v1/provider-positions` | 20 |
| reconciliation | `/v1/reconciliation/...` | 8 |
| identity | `/v1/auth`, `/v1/tenants`, `/v1/api-keys`, `/.well-known/jwks.json` | 7 |
| webhooks | `/v1/webhook-subscriptions`, `/v1/webhook-deliveries` | 7 |

All four are reachable through one gateway on port 8080, which routes by path
prefix. A client sees a single API.

## Versioning

`v1` is **additive-only**. A new optional field or a new endpoint ships in place;
anything that would break an existing client means `v2`, served alongside `v1`
with a `Sunset` header on the old one.

Pre-1.0 caveat, stated plainly: **`v1` here means the API shape, not a stability
promise backed by a track record.** This project is early-stage. The intent is
additive-only, the discipline is in CI as an anti-drift check, but there is no
history of honouring a deprecation window yet because there has been nothing to
deprecate.

## Money on the wire

```json
{"asset": "USD", "amount": "10000"}
```

An integer in minor units, **encoded as a string**. Two reasons, both learned the
hard way in systems this design reacts to:

- A JSON number in JavaScript is a float. `9007199254740993` does not survive
  `JSON.parse`. A string does.
- A decimal string like `"100.00"` invites a client to do arithmetic on it as a
  float. Minor units make that mistake harder to reach for.

The exponent is not on the amount — it lives in the tenant's asset registry, so
`USD` is 2, `JPY` is 0, `BTC` is 8, and nothing has to agree on a per-request
basis. Formatting happens at the display boundary only.

## Errors

One shape, everywhere:

```json
{"error": {"code": "unbalanced_transaction",
           "message": "debits do not equal credits: asset USD has debits 10000 and credits 5000",
           "request_id": "e0464ae2209d9d17cbc108fa15bd834e"}}
```

- **`code`** is a stable, machine-readable string. It is part of the contract:
  clients branch on it, so renaming one is a breaking change.
- **`message`** is for a human reading a log. It is *not* stable and clients must
  not parse it — but it is written to be worth reading. `"debits 10000 and
  credits 5000"` tells you what to fix; `"validation error"` does not.
- **`request_id`** is on every error and every response, so a support
  conversation can start with an identifier instead of a timestamp.

The catalogue is in [`errores-api.md`](errores-api.md). The codes exercised in
the verification transcript:

| Code | HTTP | Meaning |
|---|---|---|
| `unbalanced_transaction` | 422 | Debits ≠ credits for some asset |
| `idempotency_conflict` | 409 | Key reused with a different payload |
| `insufficient_funds` | 422 | Hold exceeds available balance |
| `conflict` | 409 | Already reversed, already captured, already released |
| `validation_failed` | 422 | Malformed input; the message names the field |
| `not_found` | 404 | Absent, or belongs to another tenant — indistinguishable on purpose |

`not_found` covering both absence and another tenant's resource is deliberate: a
`403` would confirm the id exists, which is an enumeration oracle across tenants.

## Idempotency

Required on every mutation, as an `Idempotency-Key` header or an
`idempotency_key` body field (the body wins if both are present). Behaviour:

| Situation | Response |
|---|---|
| First use | `201`, resource created |
| Same key, same payload | `200` + `X-Idempotent-Replay: true`, the original resource |
| Same key, different payload | `409 idempotency_conflict` |

Keys are scoped per tenant and do not expire. Backed by a unique constraint plus
a payload fingerprint, not by a check-then-insert — so concurrent duplicates
cannot both pass. See [`architecture.md`](architecture.md#idempotency).

## Strict request bodies

Every request schema is `additionalProperties: false`, and the handlers enforce
it:

```json
{"error":{"code":"validation_failed","message":"unknown field \"counter_account_id\""}}
```

A typo in a field name is a rejection, not a silently ignored value. For an API
where an ignored field can mean money going somewhere unintended, failing loudly
is the correct trade against convenience.

## Pagination

Keyset with an opaque cursor. Responses are `{"data": [...], "next_cursor": "..."}`;
`next_cursor` is absent on the last page. Cursors are opaque by contract — they
encode a sort key, and clients must not construct or decode them.

Offset pagination was rejected: it skips or duplicates rows when the underlying
set changes between pages, which for a ledger means an export that silently
misses a transaction.

## Identifiers

UUIDv7 everywhere. Time-ordered, so they cluster well in an index, without
exposing a sequential count of the business the way an auto-increment does.
Ids are stable and never reused. External references belong in `reference` or
`metadata`, not in the id.

## Authentication

`Authorization: Bearer <jwt>`, where the JWT is obtained by exchanging an API key
at `POST /v1/auth/token` and lives 15 minutes. Services verify it against
identity's JWKS. See [ADR-006](adr/ADR-006-auth-by-token-exchange.md).

Development mode (`LEDGERCORE_AUTH_DISABLED=true`) accepts `X-Tenant-Id`
instead. The hardened compose overlay refuses to start with it.

## Webhooks

`X-LedgerCore-Signature: t=<unix>,v1=<hex hmac>` over the timestamped body.
Verification must reject a stale timestamp, not only a bad MAC — otherwise a
captured request can be replayed forever. Both SDKs ship a verifier; rotation
keeps the previous secret valid for a grace window so a receiver can roll over
without downtime.

Delivery is **at-least-once**. Retries use backoff and give up after a maximum
attempt count. Receivers must deduplicate on the event id.

## Keeping the contract honest

A contract nobody checks becomes documentation of an API that no longer exists.
Two guards:

- **`contracts` CI job** — lints the OpenAPI documents and fails if
  `contracts/openapi/` has drifted from the copies the console serves.
- **SDKs generated from the documents**, so a contract change that nobody
  implemented shows up as an SDK that does not compile.

Neither of these catches a *handler* that diverges from its document while the
document stays self-consistent. That gap is real and is listed in
[`TEST_STRATEGY.md`](oss-transition/TEST_STRATEGY.md#known-gaps): there is no
end-to-end SDK test against a running instance.

## Reading the contract locally

```bash
docker compose -f infra/compose/docker-compose.yml --profile web up -d
# http://localhost:3000/docs/api — Scalar reference, served from a vendored
# bundle, no external network access required.
```
