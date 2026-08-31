# ADR-005 — Money as integers in minor units

**Status:** accepted · **Date:** 2026-07-24

## Context
Decimal-string money handling is a well-known source of silent corruption. Two
failure modes motivated this decision, both observed in production systems the
team has operated:

1. A "normalisation" helper that treated `"1.500"` as a thousands-separated
   value and produced `1500` instead of `1.50` — a ×1000 error, with a unit test
   that asserted the wrong result and so froze the bug in place.
2. Amounts stored as wide decimals in string columns, with the currency as an
   integer foreign key into a separate catalogue, so no single value carried
   both the number and its scale.

Floating point is excluded for the usual reason: `0.1 + 0.2 != 0.3`.

## Decision
- Every amount is a **`BIGINT` in minor units** plus an `asset VARCHAR(12)` code,
  with the exponent held in the tenant's asset registry (USD→2, JPY→0, BTC→8).
- Over the API amounts travel as **string-encoded integers**, avoiding
  JavaScript's 2^53 limit.
- `money.ParseUnits` rejects more decimal places than the asset's exponent
  allows — it never silently rounds.
- Floats are forbidden on every money path, including the console.

## Consequences
- (+) Exact arithmetic, trivial comparisons, no locale surprises (`,` vs `.`).
- (+) Balancing per asset is an integer sum with an explicit overflow check.
- (−) Display needs the exponent, so each surface carries one `formatMoney`
  helper.
