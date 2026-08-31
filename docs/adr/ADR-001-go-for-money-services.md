# ADR-001 — Go for the money-handling services

**Status:** accepted · **Date:** 2026-07-24

## Context
The services that touch money need predictable latency, cheap concurrency for
pollers and workers, and a small deployable footprint. The team's background is
PHP/Laravel, so the choice is between staying on a familiar dynamic runtime and
moving to a statically compiled one. Reference points in the category: Formance
(Go, open source), TigerBeetle (Zig), Modern Treasury (Rails + Postgres).

## Decision
Every service that touches money (`ledger-core`, `identity`, `reconciliation`,
`webhooks`) is written in **Go**, using the standard library `net/http`
(`ServeMux` with method patterns), `pgx/v5`, `log/slog`, and as few
dependencies as possible.

## Consequences
- (+) Static binaries, instant start-up, native concurrency for the outbox
  pollers and the webhook dispatcher.
- (+) A small dependency surface is easier to audit — relevant for code that
  moves money.
- (+) The language is simple enough that the step up from PHP is short, unlike
  Rust or the JVM.
- (−) No prior Go experience on the team. Mitigated by scaffolding that fixes
  the patterns once, and by keeping the standard library as the default answer.
- The console and the SDKs are TypeScript/PHP. Go is the backend choice, not a
  house rule for everything.
