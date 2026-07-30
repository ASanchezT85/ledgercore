#!/usr/bin/env bash
# ci-local.sh — reproducible CI on a CLEAN CLONE, no GitHub Actions required.
#
# Runs the same gates as .github/workflows/ci.yml against a fresh `git archive`
# checkout, so the result is evidence that the tree is green from a clean state
# (Anexo B, punto 7). Postgres-dependent gates run only if a database is
# reachable; otherwise they are SKIPPED with a loud note (never silently green).
#
# Usage:
#   scripts/ci-local.sh                 # build+vet+test+lint+drift from clean clone
#   LEDGERCORE_CI_PG_ADMIN_URL=postgres://postgres:postgres@localhost:5432/ledgercore \
#     scripts/ci-local.sh               # ALSO run the 4-service Postgres gates
#
# Requires: go 1.26, node/npx, git. Optional (for the DB gates): a reachable
# empty Postgres 17 at LEDGERCORE_CI_PG_ADMIN_URL (a superuser DSN). NO psql and
# NO goose CLI are needed — each service's Go TestMain provisions the real role
# model itself via pgx (roles + schema + goose migrate as migrator + grants), then
# runs its integration + RLS + purge tests as the NOBYPASSRLS runtime role. This
# is the exact mechanism the CI `pg-integration` matrix uses (one superuser DSN,
# `go test` per service), so ci-local and CI are byte-for-byte the same gate.
set -uo pipefail

ROOT="$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)"
REF="${1:-HEAD}"
WORK="$(mktemp -d)"
LOG="${CI_LOCAL_LOG:-$ROOT/ci-local-evidence.txt}"
fail=0

say() { printf '\n=== %s ===\n' "$*" | tee -a "$LOG"; }
run() { echo "\$ $*" | tee -a "$LOG"; "$@" 2>&1 | tee -a "$LOG"; return "${PIPESTATUS[0]}"; }

: > "$LOG"
say "CLEAN CLONE of $REF -> $WORK"
git -C "$ROOT" archive "$REF" | tar -x -C "$WORK"
echo "commit: $(git -C "$ROOT" rev-parse "$REF")" | tee -a "$LOG"
cd "$WORK"

# Mirrors the CI `go` matrix job: build + vet + test per module WITHOUT a
# Postgres. Integration tests self-skip when LEDGERCORE_TEST_DATABASE_URL is
# unset (the DB-backed gates run as their own jobs, below). We intentionally do
# NOT export one DB URL across all modules — each service owns its own schema.
for mod in libs/go services/ledger-core services/identity services/reconciliation services/webhooks; do
  say "go build/vet/test — $mod"
  ( cd "$mod" && env -u LEDGERCORE_TEST_DATABASE_URL go build ./... && \
                 env -u LEDGERCORE_TEST_DATABASE_URL go vet ./... && \
                 env -u LEDGERCORE_TEST_DATABASE_URL go test ./... -count=1 ) 2>&1 | tee -a "$LOG"
  [ "${PIPESTATUS[0]}" -eq 0 ] || fail=1
done

say "OpenAPI lint (redocly)"
for f in contracts/openapi/*.yaml; do
  run npx --yes @redocly/cli@1 lint "$f" --format=stylish || fail=1
done

say "OpenAPI anti-drift (contracts vs console copies)"
for src in contracts/openapi/*.yaml; do
  copy="apps/console/public/openapi/$(basename "$src")"
  if [ ! -f "$copy" ]; then echo "::MISSING $copy" | tee -a "$LOG"; fail=1
  elif ! diff -q "$src" "$copy" >/dev/null; then echo "::DRIFT $copy" | tee -a "$LOG"; fail=1
  else echo "ok: $copy in sync" | tee -a "$LOG"; fi
done

say "Postgres integration gates (4 services)"
# Unified harness convention (R-009): every service's TestMain, given a SUPERUSER
# DSN in LEDGERCORE_TEST_ADMIN_URL, provisions the REAL role model entirely in Go
# via pgx — it creates the migrator/maint/<svc>_rt roles + the service schema
# (owner = migrator, the equivalent of infra/postgres/init/01-init.sql), runs the
# goose migrations AS THE MIGRATOR, applies the infra/postgres/migrate/grants.sql
# step (SECURITY DEFINER functions reassigned to ledgercore_maint + EXECUTE to the
# runtime role), and points LEDGERCORE_TEST_DATABASE_URL at the service's RUNTIME
# role (NOBYPASSRLS) before running the suite. So the gate needs NO psql, NO goose
# CLI and NO init/grants SQL executed here — just a superuser DSN and `go test`.
# Every service manages only its own schema + runtime role, so all four can share
# one database (each DROPs+recreates its own schema idempotently).
PG_ADMIN="${LEDGERCORE_CI_PG_ADMIN_URL:-}"
if [ -z "$PG_ADMIN" ]; then
  echo "SKIPPED — set LEDGERCORE_CI_PG_ADMIN_URL to a superuser DSN of an empty" | tee -a "$LOG"
  echo "Postgres 17 to run these locally, e.g." | tee -a "$LOG"
  echo "  LEDGERCORE_CI_PG_ADMIN_URL=postgres://postgres:postgres@localhost:5432/ledgercore" | tee -a "$LOG"
  echo "Each service's TestMain then provisions the full role model (roles +"     | tee -a "$LOG"
  echo "schema + migrate + grants) and runs its integration/RLS/purge tests as"   | tee -a "$LOG"
  echo "the NOBYPASSRLS runtime role. In CI the pg-integration matrix does the"    | tee -a "$LOG"
  echo "same. NOT counted as green here."                                          | tee -a "$LOG"
else
  for pair in "ledger-core:ledger" "identity:identity" "reconciliation:recon" "webhooks:webhooks"; do
    svc="${pair%%:*}"; schema="${pair##*:}"
    say "provision + migrate + grants + integration/RLS/purge — $svc ($schema)"
    ( cd "services/$svc" && \
      LEDGERCORE_TEST_ADMIN_URL="$PG_ADMIN" \
      go test ./internal/adapters/postgres/... -count=1 ) 2>&1 | tee -a "$LOG"
    [ "${PIPESTATUS[0]}" -eq 0 ] || fail=1
  done
fi

say "RESULT"
if [ "$fail" -eq 0 ]; then echo "GREEN — all runnable gates passed. Evidence: $LOG" | tee -a "$LOG"
else echo "RED — see failures above. Evidence: $LOG" | tee -a "$LOG"; fi
rm -rf "$WORK"
exit "$fail"
