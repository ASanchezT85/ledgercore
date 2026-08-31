# ADR-002 — PostgreSQL with Row-Level Security as the ledger store

**Status:** accepted · **Date:** 2026-07-24

## Context
The ledger needs ACID transactions, immutability enforced by the database
rather than by application discipline, multi-tenant isolation that survives an
application bug, and efficient aggregation for balances and reporting.
MySQL has no native row-level security. TigerBeetle is a high-performance
primitive but a rigid one: no flexible metadata, no SQL for reporting.

## Decision
**PostgreSQL** for every service. Pooled multi-tenancy with **RLS**
(`POLICY ... USING (tenant_id = current_setting('app.tenant_id')::uuid)`) and an
application role that is `NOSUPERUSER NOBYPASSRLS`. **One schema per service**
(`ledger`, `identity`, `recon`, `webhooks`); crossing schemas is forbidden.

## Consequences
- (+) Tenant isolation is guaranteed by the database, not by remembering to add
  a `WHERE` clause.
- (+) Declarative partitioning and read replicas cover the scaling path without
  changing engines.
- (+) JSONB for metadata; full SQL for reporting and reconciliation.
- (−) `SET LOCAL app.tenant_id` means every access must go through
  `pgxutil.WithTenantTx`. One helper, covered by an isolation test per service.
- A specialised hot-path store behind the same API stays possible later; the
  domain's repository interface allows it.
