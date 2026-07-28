/**
 * LedgerCore TypeScript SDK — quickstart / smoke test against a local stack.
 *
 * Run (from sdks/typescript, stack up via infra/compose):
 *   npx tsx examples/quickstart.ts
 *
 * Env:
 *   LEDGERCORE_API_URL  gateway base URL (default http://localhost:8080)
 *   LEDGERCORE_API_KEY  existing key; when unset, a sandbox tenant is created
 */
import { LedgerCore, Money } from "../src/index.js";

const baseUrl = process.env.LEDGERCORE_API_URL ?? "http://localhost:8080";

async function getApiKey(): Promise<string> {
  if (process.env.LEDGERCORE_API_KEY) return process.env.LEDGERCORE_API_KEY;
  // Public self-service sandbox signup (one per email, rate limited).
  const res = await fetch(`${baseUrl}/v1/sandbox/signups`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email: `sdk-ts-${Date.now()}@example.com`,
      company_name: `SDK TS Smoke ${Date.now()}`,
    }),
  });
  if (!res.ok) throw new Error(`sandbox signup failed: ${res.status} ${await res.text()}`);
  const signup = (await res.json()) as { api_key: string; tenant_id: string; slug: string };
  console.log(`signed up sandbox tenant ${signup.slug} (${signup.tenant_id})`);
  return signup.api_key;
}

const lc = new LedgerCore({ apiKey: await getApiKey(), baseUrl });

// 1. Ledger
const ledger = await lc.ledgers.create({ name: "main", description: "Primary operating ledger" });
console.log(`ledger:       ${ledger.id} (${ledger.name}, ${ledger.environment})`);

// 2. Accounts
const cash = await lc.accounts.create({
  ledger_id: ledger.id,
  name: "assets:cash",
  type: "asset",
  normal_balance: "DEBIT",
});
const wallet = await lc.accounts.create({
  ledger_id: ledger.id,
  name: "customer:cust_42:wallet",
  type: "liability",
  normal_balance: "CREDIT",
});
const fees = await lc.accounts.create({
  ledger_id: ledger.id,
  name: "revenue:fees",
  type: "revenue",
  normal_balance: "CREDIT",
});
console.log(`accounts:     cash=${cash.id} wallet=${wallet.id} fees=${fees.id}`);

// 3. Balanced 100 / 97 / 3 deposit (amounts as minor-unit strings via Money)
const txn = await lc.transactions.create({
  ledger_id: ledger.id,
  description: "Customer USD deposit",
  postings: [
    { account_id: cash.id, direction: "DEBIT", amount: Money.fromDecimal("100.00", "USD", 2) },
    { account_id: wallet.id, direction: "CREDIT", amount: Money.fromDecimal("97.00", "USD", 2) },
    { account_id: fees.id, direction: "CREDIT", amount: Money.fromDecimal("3.00", "USD", 2) },
  ],
});
console.log(`transaction:  ${txn.id} status=${txn.status} idempotency_key=${txn.idempotency_key}`);

// 4. Statement of the wallet
const statement = await lc.statements.get({ accountId: wallet.id });
const closing = statement.closing_balance.map(
  (b) => `${Money.toDecimal(b.amount, b.exponent)} ${b.asset}`,
);
console.log(
  `statement:    ${statement.entries.length} entr${statement.entries.length === 1 ? "y" : "ies"}, closing balance ${closing.join(", ") || "0"}`,
);

// 5. Trial balance must close
const trial = await lc.trialBalance.get({ ledgerId: ledger.id });
for (const total of trial.totals) {
  console.log(
    `trialbalance: ${total.asset} debits=${total.debits} credits=${total.credits} balanced=${total.balanced}`,
  );
  if (!total.balanced) throw new Error("trial balance does not close");
}

// 6. Pagination: list accounts with a small limit and follow next_cursor to null
let cursor: string | undefined;
let pages = 0;
let seen = 0;
do {
  const page = await lc.accounts.list({ ledgerId: ledger.id, limit: 2, cursor });
  pages += 1;
  seen += page.data.length;
  console.log(
    `pagination:   page ${pages}: ${page.data.length} account(s), next_cursor=${page.next_cursor}`,
  );
  cursor = page.next_cursor ?? undefined;
} while (cursor !== undefined);
if (seen !== 3) throw new Error(`pagination saw ${seen} accounts, expected 3`);

console.log("OK: ledger balances");
