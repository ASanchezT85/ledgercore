#!/usr/bin/env bash
# Golden scenarios — every claim the README makes, executed against a running
# LedgerCore, asserted, and reported.
#
# This is the script behind docs/oss-transition/VERIFICATION.md. If you want to
# know whether this project does what it says, run it rather than trusting the
# prose.
#
#   docker compose -f infra/compose/docker-compose.yml up -d --build
#   bash examples/golden-scenarios.sh
#
# Point it somewhere else with:
#   API=http://localhost:18081 bash examples/golden-scenarios.sh
#
# Requires: curl, python3 (for JSON field extraction). Development mode only —
# it uses X-Tenant-Id, which real deployments reject.

set -uo pipefail

API="${API:-http://localhost:8081}"
TENANT="${TENANT:-11111111-1111-1111-1111-111111111111}"
H_TENANT="X-Tenant-Id: $TENANT"
H_JSON="Content-Type: application/json"
RUN="$RANDOM$RANDOM"   # keeps idempotency keys unique across repeated runs

pass=0; fail=0

ok()   { printf '  \033[32mPASS\033[0m  %s\n' "$1"; pass=$((pass+1)); }
bad()  { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; printf '        %s\n' "$2"; fail=$((fail+1)); }
head1(){ printf '\n\033[1m%s\033[0m\n' "$1"; }

# jget FILE FIELD  — dotted path into a JSON document
jget() { python3 -c "
import json,sys
d=json.load(open(sys.argv[1]))
for k in sys.argv[2].split('.'):
    d = d[int(k)] if k.isdigit() else d.get(k)
    if d is None: print(''); raise SystemExit
print(d)" "$1" "$2" 2>/dev/null; }

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

post() { # post PATH BODY OUTFILE -> echoes status code, newline-terminated
  # The trailing newline matters: these codes get concatenated across parallel
  # runs, and curl's -w output has none of its own.
  curl -s -X POST "$API$1" -H "$H_TENANT" -H "$H_JSON" -d "$2" -o "$3" -w '%{http_code}
'
}

expect_code() { # expect_code DESCRIPTION EXPECTED_ERROR_CODE FILE
  local got; got="$(jget "$3" error.code)"
  [ "$got" = "$2" ] && ok "$1" || bad "$1" "expected error.code=$2, got '${got:-<none>}': $(head -c 200 "$3")"
}

# ---------------------------------------------------------------------------
head1 "Setup"

curl -sf "$API/healthz" >/dev/null 2>&1 || {
  echo "  ledger-core is not answering at $API/healthz"
  echo "  start it with: docker compose -f infra/compose/docker-compose.yml up -d --build"
  exit 1
}
ok "ledger-core is up at $API"

post /v1/ledgers "{\"name\":\"golden-$RUN\"}" "$TMP/l.json" >/dev/null
LEDGER="$(jget "$TMP/l.json" id)"
[ -n "$LEDGER" ] && ok "ledger created" || { bad "ledger created" "$(cat "$TMP/l.json")"; exit 1; }

mkacct() {
  post /v1/accounts \
    "{\"ledger_id\":\"$LEDGER\",\"name\":\"$1\",\"type\":\"$2\",\"normal_balance\":\"$3\"}" \
    "$TMP/a.json" >/dev/null
  jget "$TMP/a.json" id
}
CASH="$(mkacct assets:cash asset DEBIT)"
WALLET="$(mkacct liabilities:customer:wallet liability CREDIT)"
FEES="$(mkacct revenue:fees revenue CREDIT)"
[ -n "$CASH" ] && [ -n "$WALLET" ] && [ -n "$FEES" ] \
  && ok "three accounts created" || { bad "accounts" "$(cat "$TMP/a.json")"; exit 1; }

balance() { curl -s "$API/v1/accounts/$1/balances" -H "$H_TENANT" -o "$TMP/b.json"; jget "$TMP/b.json" "data.0.$2"; }

# ---------------------------------------------------------------------------
head1 "A · Deposit — a balanced transaction with a fee"

DEPOSIT="{\"ledger_id\":\"$LEDGER\",\"idempotency_key\":\"dep-$RUN\",
  \"description\":\"Customer USD deposit\",\"postings\":[
  {\"account_id\":\"$CASH\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"10000\"}},
  {\"account_id\":\"$WALLET\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"9700\"}},
  {\"account_id\":\"$FEES\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"300\"}}]}"

code="$(post /v1/transactions "$DEPOSIT" "$TMP/tx.json" | tr -d "[:space:]")"
TXID="$(jget "$TMP/tx.json" id)"
[ "$code" = "201" ] && ok "posted (201)" || bad "posted" "HTTP $code: $(head -c 200 "$TMP/tx.json")"
[ "$(jget "$TMP/tx.json" status)" = "posted" ] && ok "status is posted" || bad "status" "$(jget "$TMP/tx.json" status)"
[ "$(balance "$WALLET" posted)" = "9700" ] && ok "wallet balance is 9700" || bad "wallet balance" "got $(balance "$WALLET" posted)"
[ "$(balance "$FEES" posted)" = "300" ] && ok "fee revenue is 300" || bad "fee revenue" "got $(balance "$FEES" posted)"

# ---------------------------------------------------------------------------
head1 "B · Invalid transactions are refused"

post /v1/transactions "{\"ledger_id\":\"$LEDGER\",\"idempotency_key\":\"unbal-$RUN\",\"postings\":[
  {\"account_id\":\"$CASH\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"10000\"}},
  {\"account_id\":\"$WALLET\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"5000\"}}]}" \
  "$TMP/e.json" >/dev/null
expect_code "debits != credits -> unbalanced_transaction" unbalanced_transaction "$TMP/e.json"

post /v1/transactions "{\"ledger_id\":\"$LEDGER\",\"idempotency_key\":\"fx-$RUN\",\"postings\":[
  {\"account_id\":\"$CASH\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"10000\"}},
  {\"account_id\":\"$WALLET\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"EUR\",\"amount\":\"10000\"}}]}" \
  "$TMP/e.json" >/dev/null
expect_code "USD against EUR -> unbalanced (no implicit FX)" unbalanced_transaction "$TMP/e.json"

for v in '1.005' '-100' '99999999999999999999999' '9223372036854775808'; do
  post /v1/transactions "{\"ledger_id\":\"$LEDGER\",\"idempotency_key\":\"amt-$RUN-$v\",\"postings\":[
    {\"account_id\":\"$CASH\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"$v\"}},
    {\"account_id\":\"$WALLET\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"$v\"}}]}" \
    "$TMP/e.json" >/dev/null
  expect_code "amount '$v' -> validation_failed" validation_failed "$TMP/e.json"
done

# ---------------------------------------------------------------------------
head1 "C · Idempotency"

code="$(curl -s -D "$TMP/hdr" -X POST "$API/v1/transactions" -H "$H_TENANT" -H "$H_JSON" \
        -d "$DEPOSIT" -o "$TMP/rep.json" -w '%{http_code}')"
[ "$code" = "200" ] && ok "replay returns 200" || bad "replay code" "got $code"
grep -qi 'X-Idempotent-Replay: true' "$TMP/hdr" && ok "X-Idempotent-Replay: true" || bad "replay header" "absent"
[ "$(jget "$TMP/rep.json" id)" = "$TXID" ] && ok "same transaction id" || bad "replay id" "differs from original"

post /v1/transactions "{\"ledger_id\":\"$LEDGER\",\"idempotency_key\":\"dep-$RUN\",\"postings\":[
  {\"account_id\":\"$CASH\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"111\"}},
  {\"account_id\":\"$WALLET\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"111\"}}]}" \
  "$TMP/e.json" >/dev/null
expect_code "same key + different payload -> idempotency_conflict" idempotency_conflict "$TMP/e.json"

# ---------------------------------------------------------------------------
head1 "D · Holds — reserve, over-reserve, capture"

post /v1/holds "{\"ledger_id\":\"$LEDGER\",\"account_id\":\"$WALLET\",\"idempotency_key\":\"hold-$RUN\",
  \"amount\":{\"asset\":\"USD\",\"amount\":\"5000\"}}" "$TMP/h.json" >/dev/null
HOLD="$(jget "$TMP/h.json" id)"
[ -n "$HOLD" ] && ok "hold created" || bad "hold" "$(head -c 200 "$TMP/h.json")"
[ "$(balance "$WALLET" posted)"    = "9700" ] && ok "posted unchanged by the hold"  || bad "posted"    "got $(balance "$WALLET" posted)"
[ "$(balance "$WALLET" held)"      = "5000" ] && ok "held is 5000"                  || bad "held"      "got $(balance "$WALLET" held)"
[ "$(balance "$WALLET" available)" = "4700" ] && ok "available dropped to 4700"     || bad "available" "got $(balance "$WALLET" available)"

post /v1/holds "{\"ledger_id\":\"$LEDGER\",\"account_id\":\"$WALLET\",\"idempotency_key\":\"hold-big-$RUN\",
  \"amount\":{\"asset\":\"USD\",\"amount\":\"999999\"}}" "$TMP/e.json" >/dev/null
expect_code "hold beyond available -> insufficient_funds" insufficient_funds "$TMP/e.json"

# Capture links the hold to a transaction the caller posts; it does not create one.
post /v1/transactions "{\"ledger_id\":\"$LEDGER\",\"idempotency_key\":\"cap-move-$RUN\",\"postings\":[
  {\"account_id\":\"$WALLET\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"5000\"}},
  {\"account_id\":\"$CASH\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"5000\"}}]}" \
  "$TMP/m.json" >/dev/null
MOVE="$(jget "$TMP/m.json" id)"

post "/v1/holds/$HOLD/capture" "{\"transaction_id\":\"$MOVE\"}" "$TMP/c.json" >/dev/null
[ "$(jget "$TMP/c.json" status)" = "captured" ] && ok "hold captured" || bad "capture" "$(head -c 200 "$TMP/c.json")"
[ "$(balance "$WALLET" held)" = "0" ] && ok "held released to 0" || bad "held after capture" "got $(balance "$WALLET" held)"

post "/v1/holds/$HOLD/capture" "{\"transaction_id\":\"$MOVE\"}" "$TMP/e.json" >/dev/null
expect_code "double capture -> conflict" conflict "$TMP/e.json"
post "/v1/holds/$HOLD/release" '{}' "$TMP/e.json" >/dev/null
expect_code "release after capture -> conflict" conflict "$TMP/e.json"

# ---------------------------------------------------------------------------
head1 "E · Reversal — a correction is a new entry"

before_d="$(balance "$WALLET" posted_debits)"; before_c="$(balance "$WALLET" posted_credits)"
post "/v1/transactions/$TXID/reverse" '{}' "$TMP/r.json" >/dev/null
REVID="$(jget "$TMP/r.json" id)"
[ -n "$REVID" ] && [ "$REVID" != "$TXID" ] && ok "reversal is a NEW transaction" || bad "reversal" "$(head -c 200 "$TMP/r.json")"
case "$(jget "$TMP/r.json" reference)" in
  reversal-of:*) ok "references the original" ;;
  *) bad "reference" "got '$(jget "$TMP/r.json" reference)'" ;;
esac

after_d="$(balance "$WALLET" posted_debits)"; after_c="$(balance "$WALLET" posted_credits)"
[ "$after_d" -gt "$before_d" ] || [ "$after_c" -gt "$before_c" ] \
  && ok "gross movement grew — nothing was erased ($before_d/$before_c -> $after_d/$after_c)" \
  || bad "append-only" "gross totals did not grow"

# Reversing again WITHOUT a key derives the same key (reversal-of:<id>), so it
# is an idempotent replay of the first reversal — a safe retry, not a second
# reversal.
code="$(curl -s -D "$TMP/hdr2" -X POST "$API/v1/transactions/$TXID/reverse"         -H "$H_TENANT" -H "$H_JSON" -d '{}' -o "$TMP/r2.json" -w '%{http_code}')"
[ "$code" = "200" ] && grep -qi 'X-Idempotent-Replay: true' "$TMP/hdr2"   && ok "retrying the reversal replays it (200 + replay header)"   || bad "reversal retry" "HTTP $code, replay header $(grep -ci 'X-Idempotent-Replay: true' "$TMP/hdr2")"
[ "$(jget "$TMP/r2.json" id)" = "$REVID" ] && ok "replay returns the same reversal"   || bad "reversal replay id" "differs"

# A DISTINCT key means "reverse it again", which must be refused.
curl -s -X POST "$API/v1/transactions/$TXID/reverse" -H "$H_TENANT" -H "$H_JSON"      -H "Idempotency-Key: second-reversal-$RUN" -d '{}' -o "$TMP/e.json" >/dev/null
expect_code "a second, distinct reversal -> conflict" conflict "$TMP/e.json"

# ---------------------------------------------------------------------------
head1 "F · Concurrency"

# 20 identical requests, one key: exactly one accounting effect.
CKEY="conc-idem-$RUN"
CBODY="{\"ledger_id\":\"$LEDGER\",\"idempotency_key\":\"$CKEY\",\"postings\":[
  {\"account_id\":\"$CASH\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"700\"}},
  {\"account_id\":\"$FEES\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"700\"}}]}"
fees_before="$(balance "$FEES" posted)"
for i in $(seq 1 20); do ( post /v1/transactions "$CBODY" "$TMP/c$i.json" > "$TMP/code.$i" ) & done
wait
cat "$TMP"/code.* > "$TMP/codes"
created="$(grep -c '^201$' "$TMP/codes" || true)"; replayed="$(grep -c '^200$' "$TMP/codes" || true)"
[ "$created" = "1" ] && [ "$replayed" = "19" ] \
  && ok "20 duplicate requests -> 1 created, 19 replayed" \
  || bad "idempotency race" "created=$created replayed=$replayed"
[ "$(balance "$FEES" posted)" = "$((fees_before + 700))" ] \
  && ok "exactly one accounting effect" \
  || bad "idempotency race balance" "expected $((fees_before + 700)), got $(balance "$FEES" posted)"

# 20 holds against funds for 10: nothing over-reserved.
RL="$(post /v1/ledgers "{\"name\":\"race-$RUN\"}" "$TMP/rl.json" >/dev/null; jget "$TMP/rl.json" id)"
LEDGER_SAVED="$LEDGER"; LEDGER="$RL"
RC="$(mkacct assets:cash asset DEBIT)"; RW="$(mkacct liabilities:w liability CREDIT)"
post /v1/transactions "{\"ledger_id\":\"$RL\",\"idempotency_key\":\"race-fund-$RUN\",\"postings\":[
  {\"account_id\":\"$RC\",\"direction\":\"DEBIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"10000\"}},
  {\"account_id\":\"$RW\",\"direction\":\"CREDIT\",\"amount\":{\"asset\":\"USD\",\"amount\":\"10000\"}}]}" \
  "$TMP/rf.json" >/dev/null
for i in $(seq 1 20); do
  ( post /v1/holds "{\"ledger_id\":\"$RL\",\"account_id\":\"$RW\",\"idempotency_key\":\"race-h-$RUN-$i\",
    \"amount\":{\"asset\":\"USD\",\"amount\":\"1000\"}}" "$TMP/rh$i.json" > "$TMP/hcode.$i" ) &
done
wait
cat "$TMP"/hcode.* > "$TMP/hcodes"
acc="$(grep -c '^201$' "$TMP/hcodes" || true)"
[ "$acc" = "10" ] && ok "20 holds against funds for 10 -> exactly 10 accepted" \
                  || bad "hold race" "accepted=$acc (expected 10)"
[ "$(balance "$RW" held)" = "10000" ] && [ "$(balance "$RW" available)" = "0" ] \
  && ok "held=10000, available=0 — nothing over-reserved" \
  || bad "hold race balance" "held=$(balance "$RW" held) available=$(balance "$RW" available)"
LEDGER="$LEDGER_SAVED"

# ---------------------------------------------------------------------------
head1 "G · Trial balance"

curl -s "$API/v1/trial-balance?ledger_id=$LEDGER" -H "$H_TENANT" -o "$TMP/tb.json"
python3 - "$TMP/tb.json" <<'PY' && ok "trial balance: total debits == total credits" || bad "trial balance" "debits != credits"
import json,sys
rows=json.load(open(sys.argv[1]))["rows"]
d=sum(int(r["debits"]) for r in rows); c=sum(int(r["credits"]) for r in rows)
print(f"        debits={d} credits={c}")
raise SystemExit(0 if d==c else 1)
PY

# ---------------------------------------------------------------------------
printf '\n\033[1m%s\033[0m\n' "Result"
printf '  passed: %d\n  failed: %d\n' "$pass" "$fail"
[ "$fail" -eq 0 ] || exit 1
