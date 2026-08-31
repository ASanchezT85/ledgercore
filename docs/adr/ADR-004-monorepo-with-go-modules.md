# ADR-004 — Monorepo with independent Go modules

**Status:** accepted · **Date:** 2026-07-24

## Context
The system is four well-bounded services — not a monolith, not fine-grained
microservices. How the repositories and modules are organised has to be decided
once.

## Decision
**One monorepo** containing:
- One Go module per service (`services/<x>/go.mod`) plus a shared `libs/go`
  module, wired with relative `replace` directives and a root `go.work`.
- TypeScript apps under `apps/` (pnpm).
- Contracts (`contracts/`) and infrastructure (`infra/`) versioned next to the
  code they describe.
- CI as a module matrix: each service builds, tests and containerises
  independently.

## Consequences
- (+) A single pull request can change the contract, the service and the console
  atomically — which matters most with a small number of maintainers.
- (+) Each service keeps an independent build and deploy (build context is the
  repo root, Dockerfile is its own).
- (−) A monorepo needs per-directory ownership discipline; CODEOWNERS covers it
  once there is more than one maintainer.

See [`REPOSITORY_STRUCTURE_DECISION.md`](../oss-transition/REPOSITORY_STRUCTURE_DECISION.md)
for why the SDKs live in their own repositories.
