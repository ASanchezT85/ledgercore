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
# empty Postgres 17 reachable at LEDGERCORE_CI_PG_ADMIN_URL (a superuser DSN)
# plus goose (auto-installed if absent). The DB gates mirror the CI
# `pg-integration` matrix: init SQL -> goose migrate -> grants.sql ->
# per-service integration + RLS + purge tests, each on its own schema/role.
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
PG_ADMIN="${LEDGERCORE_CI_PG_ADMIN_URL:-}"
if [ -z "$PG_ADMIN" ]; then
  echo "SKIPPED — set LEDGERCORE_CI_PG_ADMIN_URL to a superuser DSN of an empty" | tee -a "$LOG"
  echo "Postgres 17 to run these locally. In CI they run as the pg-integration" | tee -a "$LOG"
  echo "matrix (.github/workflows/ci.yml): init SQL + goose migrate + grants.sql" | tee -a "$LOG"
  echo "+ per-service integration/RLS/purge tests. NOT counted as green here."    | tee -a "$LOG"
else
  # Dev role passwords consumed by init/01-init.sql via psql \getenv (R-003).
  export LEDGERCORE_MIGRATOR_PASSWORD="${LEDGERCORE_MIGRATOR_PASSWORD:-ledgercore_migrator_dev}"
  export LEDGERCORE_LEDGER_RT_PASSWORD="${LEDGERCORE_LEDGER_RT_PASSWORD:-ledgercore_ledger_rt_dev}"
  export LEDGERCORE_IDENTITY_RT_PASSWORD="${LEDGERCORE_IDENTITY_RT_PASSWORD:-ledgercore_identity_rt_dev}"
  export LEDGERCORE_RECON_RT_PASSWORD="${LEDGERCORE_RECON_RT_PASSWORD:-ledgercore_recon_rt_dev}"
  export LEDGERCORE_WEBHOOKS_RT_PASSWORD="${LEDGERCORE_WEBHOOKS_RT_PASSWORD:-ledgercore_webhooks_rt_dev}"
  MIG_BASE="postgres://ledgercore_migrator:${LEDGERCORE_MIGRATOR_PASSWORD}@$(echo "$PG_ADMIN" | sed 's#^[a-z]*://[^@]*@##')"

  command -v goose >/dev/null 2>&1 || run go install github.com/pressly/goose/v3/cmd/goose@v3.24.1
  export PATH="$PATH:$(go env GOPATH)/bin"

  say "provision roles + schemas (init SQL)"
  run psql "$PG_ADMIN" -v ON_ERROR_STOP=1 -f infra/postgres/init/01-init.sql || fail=1

  for pair in "ledger-core:ledger" "identity:identity" "reconciliation:recon" "webhooks:webhooks"; do
    svc="${pair%%:*}"; schema="${pair##*:}"
    say "migrate + grants + integration — $svc ($schema)"
    run goose -dir "services/$svc/internal/adapters/postgres/migrations" postgres \
      "${MIG_BASE}?sslmode=disable&search_path=${schema}" up || fail=1
    run psql "$MIG_BASE" -v ON_ERROR_STOP=1 -f infra/postgres/migrate/grants.sql || fail=1
    ( cd "services/$svc" && \
      LEDGERCORE_TEST_DATABASE_URL="${MIG_BASE}?search_path=${schema}" \
      go test ./internal/adapters/postgres/... -count=1 ) 2>&1 | tee -a "$LOG"
    [ "${PIPESTATUS[0]}" -eq 0 ] || fail=1
  done
fi

say "RESULT"
if [ "$fail" -eq 0 ]; then echo "GREEN — all runnable gates passed. Evidence: $LOG" | tee -a "$LOG"
else echo "RED — see failures above. Evidence: $LOG" | tee -a "$LOG"; fi
rm -rf "$WORK"
exit "$fail"
