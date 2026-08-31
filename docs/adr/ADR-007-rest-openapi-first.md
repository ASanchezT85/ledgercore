# ADR-007 — Public REST API, OpenAPI-first, generated SDKs

**Status:** accepted · **Date:** 2026-07-24

## Context
The API *is* the product surface. Integrators value fast, boring integration
over protocol elegance, and an externally exposed gRPC interface adds adoption
friction for the languages most of them use.

## Decision
- Public API is **REST + JSON**; the contracts are **OpenAPI 3.1** documents in
  `contracts/openapi/`, written before the handlers.
- Versioned by path (`/v1`). Additive changes are allowed in place; a breaking
  change means `/v2` with an overlap period and a `Sunset` header.
- `Idempotency-Key` is required on every mutation; pagination is keyset-based
  with an opaque cursor; errors are `{"error":{"code","message"}}` with a stable
  code catalogue (see [`docs/errores-api.md`](../errores-api.md)).
- SDKs are generated from the OpenAPI documents. PHP and TypeScript exist today.
- gRPC remains a possible internal transport, never a requirement for clients.

## Consequences
- (+) One artifact — the YAML — feeds the docs, the SDKs, the drift check in CI
  and the API reference.
- (−) Contract-first requires discipline: CI has to fail when a handler diverges
  from the document, otherwise the contract quietly becomes fiction.
