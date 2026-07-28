"use client";

/**
 * Live data layer: typed fetchers against the real LedgerCore API using the
 * session JWT. Shapes follow contracts/openapi/ledger.v1.yaml — Money is
 * { asset, amount: string } in minor units, Direction is DEBIT/CREDIT.
 */

import { AuthError, apiBaseUrl, getAccessToken } from "./session";

// ---- API shapes (mirror of the OpenAPI contract) --------------------------

export interface Money {
  asset: string;
  amount: string; // int64 minor units, string-encoded
}

export type Direction = "DEBIT" | "CREDIT";
export type ApiTxnStatus = "draft" | "posted" | "reversed";
export type AccountType =
  | "asset"
  | "liability"
  | "equity"
  | "revenue"
  | "expense";

export interface ApiLedger {
  id: string;
  name: string;
  description?: string;
  environment: "sandbox" | "live";
  created_at: string;
}

export interface ApiAccount {
  id: string;
  ledger_id: string;
  name: string;
  type: AccountType;
  normal_balance: Direction;
  status: "active" | "frozen" | "closed";
  created_at: string;
}

export interface ApiBalance {
  asset: string;
  exponent: number;
  posted: string;
  pending: string;
  available: string;
  held: string;
  posted_debits: string;
  posted_credits: string;
  version: number;
}

export interface ApiPosting {
  id?: string;
  account_id: string;
  direction: Direction;
  amount: Money;
}

export interface ApiTransaction {
  id: string;
  ledger_id: string;
  idempotency_key: string;
  reference?: string;
  description?: string;
  status: ApiTxnStatus;
  postings: ApiPosting[];
  metadata: Record<string, string>;
  reversed_by: string | null;
  reverses: string | null;
  effective_at: string;
  created_at: string;
  posted_at: string | null;
}

export interface TrialBalanceRow {
  account_id: string;
  account_name: string;
  account_type: AccountType;
  asset: string;
  debits: string;
  credits: string;
  net: string;
}

export interface TrialBalanceTotal {
  asset: string;
  debits: string;
  credits: string;
  balanced: boolean;
}

export interface ApiTrialBalance {
  ledger_id: string;
  as_of: string;
  rows: TrialBalanceRow[];
  totals: TrialBalanceTotal[];
}

interface Page<T> {
  data: T[];
  next_cursor?: string | null;
}

// ---- Errors ---------------------------------------------------------------

export class ApiDownError extends Error {
  constructor() {
    super("api_unreachable");
    this.name = "ApiDownError";
  }
}

export { AuthError };

// ---- Core fetch -----------------------------------------------------------

async function apiFetch<T>(path: string): Promise<T> {
  const token = await getAccessToken();
  let res: Response;
  try {
    res = await fetch(`${apiBaseUrl()}${path}`, {
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: "application/json",
      },
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });
  } catch (err) {
    if (err instanceof AuthError) throw err;
    throw new ApiDownError();
  }
  if (res.status === 401) throw new AuthError("token_rejected");
  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as {
        error?: { code?: string; message?: string };
      };
      if (body.error?.code) message = body.error.code;
    } catch {
      /* non-JSON error body */
    }
    throw new Error(message);
  }
  return (await res.json()) as T;
}

// ---- Fetchers -------------------------------------------------------------

export function listLedgers(): Promise<Page<ApiLedger>> {
  return apiFetch<Page<ApiLedger>>("/v1/ledgers?limit=100");
}

export function listAccounts(ledgerId?: string): Promise<Page<ApiAccount>> {
  const q = ledgerId ? `&ledger_id=${encodeURIComponent(ledgerId)}` : "";
  return apiFetch<Page<ApiAccount>>(`/v1/accounts?limit=100${q}`);
}

export function getBalances(accountId: string): Promise<{ data: ApiBalance[] }> {
  return apiFetch<{ data: ApiBalance[] }>(
    `/v1/accounts/${encodeURIComponent(accountId)}/balances`,
  );
}

export function listTransactions(
  cursor?: string,
  limit = 25,
): Promise<Page<ApiTransaction>> {
  const q = cursor ? `&cursor=${encodeURIComponent(cursor)}` : "";
  return apiFetch<Page<ApiTransaction>>(`/v1/transactions?limit=${limit}${q}`);
}

export function getTrialBalance(ledgerId: string): Promise<ApiTrialBalance> {
  return apiFetch<ApiTrialBalance>(
    `/v1/trial-balance?ledger_id=${encodeURIComponent(ledgerId)}`,
  );
}

// ---- Helpers --------------------------------------------------------------

/** Display exponents for assets when the balance row is not at hand. */
const DEFAULT_EXPONENTS: Record<string, number> = {
  USD: 2,
  EUR: 2,
  MXN: 2,
  GBP: 2,
  JPY: 0,
  BTC: 8,
  USDC: 6,
};

export function assetExponent(asset: string, known?: number): number {
  return known ?? DEFAULT_EXPONENTS[asset] ?? 2;
}

/** Total moved per transaction = sum of DEBIT legs, grouped by asset. */
export function transactionTotals(txn: ApiTransaction): Money[] {
  const sums = new Map<string, bigint>();
  for (const p of txn.postings) {
    if (p.direction !== "DEBIT") continue;
    sums.set(p.amount.asset, (sums.get(p.amount.asset) ?? 0n) + BigInt(p.amount.amount));
  }
  return [...sums.entries()].map(([asset, amount]) => ({
    asset,
    amount: amount.toString(),
  }));
}

// ---- Aggregated page loaders ---------------------------------------------

export interface LedgerWithAccounts {
  ledger: ApiLedger;
  accounts: Array<{ account: ApiAccount; balances: ApiBalance[] }>;
}

export async function loadLedgersTree(): Promise<LedgerWithAccounts[]> {
  const ledgers = (await listLedgers()).data;
  return Promise.all(
    ledgers.map(async (ledger) => {
      const accounts = (await listAccounts(ledger.id)).data;
      const withBalances = await Promise.all(
        accounts.map(async (account) => ({
          account,
          balances: (await getBalances(account.id)).data,
        })),
      );
      return { ledger, accounts: withBalances };
    }),
  );
}

export interface LiveTransactionsPage {
  transactions: ApiTransaction[];
  nextCursor: string | null;
  /** account id -> human name, for rendering postings. */
  accountNames: Record<string, string>;
  /** ledger id -> name. */
  ledgerNames: Record<string, string>;
}

export async function loadTransactionsPage(
  cursor?: string,
): Promise<LiveTransactionsPage> {
  const [txns, ledgers] = await Promise.all([
    listTransactions(cursor, 25),
    listLedgers(),
  ]);
  const ledgerNames = Object.fromEntries(
    ledgers.data.map((l) => [l.id, l.name]),
  );
  const accountPages = await Promise.all(
    ledgers.data.map((l) => listAccounts(l.id).catch(() => ({ data: [] }))),
  );
  const accountNames = Object.fromEntries(
    accountPages.flatMap((p) => p.data.map((a) => [a.id, a.name])),
  );
  return {
    transactions: txns.data,
    nextCursor: txns.next_cursor ?? null,
    accountNames,
    ledgerNames,
  };
}

export interface LiveDashboard {
  ledgers: ApiLedger[];
  accountCount: number;
  /** Net posted balance per asset over asset-type accounts (custody view). */
  custody: Array<{ asset: string; exponent: number; units: bigint }>;
  transactions: ApiTransaction[];
  transactionsToday: number;
  trial: ApiTrialBalance | null;
}

export async function loadDashboard(): Promise<LiveDashboard> {
  const ledgers = (await listLedgers()).data;
  const first = ledgers[0];
  if (!first) {
    return {
      ledgers,
      accountCount: 0,
      custody: [],
      transactions: [],
      transactionsToday: 0,
      trial: null,
    };
  }
  const [accountsPages, txns, trial] = await Promise.all([
    Promise.all(ledgers.map((l) => listAccounts(l.id))),
    listTransactions(undefined, 10),
    getTrialBalance(first.id).catch(() => null),
  ]);
  const accounts = accountsPages.flatMap((p) => p.data);
  const assetAccounts = accounts.filter((a) => a.type === "asset");
  const custodyMap = new Map<string, { exponent: number; units: bigint }>();
  const balancePages = await Promise.all(
    assetAccounts.map((a) => getBalances(a.id).catch(() => ({ data: [] }))),
  );
  for (const page of balancePages) {
    for (const b of page.data) {
      const prev = custodyMap.get(b.asset);
      custodyMap.set(b.asset, {
        exponent: b.exponent,
        units: (prev?.units ?? 0n) + BigInt(b.posted),
      });
    }
  }
  const dayAgo = Date.now() - 24 * 3600 * 1000;
  return {
    ledgers,
    accountCount: accounts.length,
    custody: [...custodyMap.entries()].map(([asset, v]) => ({
      asset,
      exponent: v.exponent,
      units: v.units,
    })),
    transactions: txns.data,
    transactionsToday: txns.data.filter(
      (t) => new Date(t.created_at).getTime() >= dayAgo,
    ).length,
    trial,
  };
}
