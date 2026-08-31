# ADR-008 — Immutable history: append-only and compensating corrections

**Status:** accepted · **Date:** 2026-07-24

## Context
An auditable ledger has to be able to show what happened and when — corrections
included. That rules out fixing history with `UPDATE` or `DELETE`, because a
corrected row destroys the evidence of what it previously said. In systems the
team has operated, database-level append-only triggers with a narrow, audited
maintenance escape hatch proved to be the defence that survives any application
bug, precisely because they do not depend on application code being correct.

## Decision
- `postings` is **absolutely append-only**: a `BEFORE UPDATE OR DELETE` trigger
  raises an exception.
- `transactions` only allows the transitions `draft→posted→reversed`; a trigger
  validates which columns may change.
- Every correction is a **compensating transaction**, posted and linked to the
  original (`reversed_by`, reference `reversal-of:<id>`). A partial reversal is a
  compensating transaction for the sub-amount.
- The maintenance escape hatch (`ledger.allow_maintenance`) is documented,
  restricted to a dedicated database role, and every use leaves an audit record.

## Consequences
- (+) The balance at any historical date is reconstructible by aggregating
  postings over their transaction's `effective_at`.
- (+) For audit and dispute purposes the evidence is never destroyed.
- (−) "Fixing a value" always costs an extra transaction. That is the feature.
