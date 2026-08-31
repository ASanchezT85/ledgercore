# ADR-003 — NATS JetStream + transactional outbox for events

**Status:** accepted · **Date:** 2026-07-24

## Context
Every ledger state change has to reach reconciliation, webhooks and any future
consumer reliably. Kafka is the de-facto standard, but its operational cost
(brokers, partitions, rebalancing) is high for a small system. Publishing
directly from application code after a commit — "publish and pray" — loses
events whenever the process dies between the two steps.

## Decision
- **Transactional outbox, always:** the event row is inserted into `outbox` in
  the same database transaction as the state change; a poller
  (`FOR UPDATE SKIP LOCKED`) publishes it and stamps `published_at`.
- **NATS JetStream** as the bus: stream `LEDGERCORE`, subjects `ledger.>` and
  `recon.>`, one durable consumer per service.
- Delivery is **at-least-once**; every consumer is idempotent (deterministic IDs
  plus `ON CONFLICT DO NOTHING`).

## Consequences
- (+) NATS is a single binary — trivial to run in development and in production.
- (+) The outbox decouples the bus from the architecture: moving to Kafka or
  Redpanda later is an adapter change.
- (−) At-least-once pushes the idempotency burden onto consumers. That is
  deliberate; see [`docs/invariants.md`](../invariants.md).
