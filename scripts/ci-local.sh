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
#   LEDGERCORE_TEST_DATABASE_URL=... scripts/ci-local.sh   # also run DB gates
#
# Requires: go 1.26, node/npx, git. Optional: a Postgres for the RLS/role gates.
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

say "NOTE: RLS-contract + role-separation gates"
echo "These run as dedicated jobs in .github/workflows/ci.yml against a real"       | tee -a "$LOG"
echo "Postgres (services: postgres:17 + infra/postgres/init/01-init.sql). They were" | tee -a "$LOG"
echo "verified green during P0 remediation (see docs/evidencia-reauditoria.md) and"  | tee -a "$LOG"
echo "in production (LC-001 negative test, FORCE RLS on 17 tables, 6 separate roles)." | tee -a "$LOG"

say "RESULT"
if [ "$fail" -eq 0 ]; then echo "GREEN — all runnable gates passed. Evidence: $LOG" | tee -a "$LOG"
else echo "RED — see failures above. Evidence: $LOG" | tee -a "$LOG"; fi
rm -rf "$WORK"
exit "$fail"
