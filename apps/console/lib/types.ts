/** Shared domain types used by the console pages and the API layer. */

export type TransactionStatus = "posted" | "pending" | "reversed";

export interface AssetBalance {
  asset: string;
  exponent: number;
  units: number;
}

export interface Posting {
  account: string;
  direction: "debit" | "credit";
  amountUnits: number | bigint;
  asset: string;
  exponent: number;
}

export interface Transaction {
  id: string;
  ledger: string;
  description: string;
  status: TransactionStatus;
  amountUnits: number | bigint;
  asset: string;
  exponent: number;
  createdAt: string;
  idempotencyKey: string;
  postings: Posting[];
}

export interface LedgerAccount {
  path: string;
  balances: AssetBalance[];
}

export interface LedgerSummary {
  id: string;
  name: string;
  description: string;
  accounts: LedgerAccount[];
}

export type ReconSourceStatus = "healthy" | "degraded" | "critical";

export interface ReconSourceHealth {
  source: string;
  label: string;
  matchedPct: number;
  openDiscrepancies: number;
  lastRunAt: string;
  status: ReconSourceStatus;
}

export type ReconRunStatus = "completed" | "running" | "failed";

export interface ReconRun {
  id: string;
  source: string;
  windowStart: string;
  windowEnd: string;
  status: ReconRunStatus;
  matched: number;
  unmatched: number;
}

export type DiscrepancyType =
  | "missing_internal"
  | "missing_external"
  | "amount_mismatch"
  | "duplicate";

export type DiscrepancyStatus = "open" | "investigating" | "resolved";

export interface Discrepancy {
  id: string;
  source: string;
  type: DiscrepancyType;
  status: DiscrepancyStatus;
  externalRef: string;
  amountUnits: number;
  asset: string;
  exponent: number;
  detectedAt: string;
}

export interface DashboardStats {
  custodyUnits: number;
  custodyAsset: string;
  custodyExponent: number;
  transactionsToday: number;
  openDiscrepancies: number;
  unbalancedProviders: number;
}

export interface DashboardData {
  stats: DashboardStats;
  recentTransactions: Transaction[];
  reconHealth: ReconSourceHealth[];
}

export interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  createdAt: string;
  lastUsedAt: string | null;
}

export interface WebhookSubscription {
  id: string;
  url: string;
  topics: string[];
  status: "active" | "paused";
  createdAt: string;
}

export interface DeveloperData {
  apiKeys: ApiKey[];
  webhooks: WebhookSubscription[];
}

export interface ReconciliationData {
  runs: ReconRun[];
  discrepancies: Discrepancy[];
}
