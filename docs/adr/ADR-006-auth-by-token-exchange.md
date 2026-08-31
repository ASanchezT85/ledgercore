# ADR-006 — Authentication by token exchange (API key → EdDSA JWT)

**Status:** accepted · **Date:** 2026-07-24

## Context
Four services must authenticate every request against the right tenant without
each one holding a connection to the credential store.

## Decision
- `identity` is the only service that knows about API keys. It stores a SHA-256
  hash plus a prefix; the secret is shown exactly once.
- The client exchanges `lk_<env>_…` for a **15-minute EdDSA (Ed25519) JWT**
  carrying `{tid, env, scope}` via `POST /v1/auth/token`.
- Every other service validates that JWT against identity's public **JWKS**
  (cached) — no cross-service database I/O on the hot path.
- For local development, `LEDGERCORE_AUTH_DISABLED=true` takes the tenant from
  an `X-Tenant-Id` header. This is local-only and refuses to start otherwise.

## Consequences
- (+) Revoking a key takes effect within the token TTL without a per-request
  round trip.
- (+) Rotating a signing key means publishing a new `kid` in the JWKS.
- (−) Signing keys are stored in the database, encrypted with a master key
  supplied by the environment. A managed KMS is the correct answer for a
  production deployment and is **not** part of this implementation — see
  [Limitations](../../README.md#limitations).
