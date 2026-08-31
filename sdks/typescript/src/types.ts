/**
 * API types mirrored by hand from contracts/openapi/ledger.v1.yaml,
 * identity.v1.yaml and webhooks.v1.yaml. All monetary amounts are int64
 * minor units, string-encoded.
 */

export type Direction = "DEBIT" | "CREDIT";
export type Environment = "sandbox" | "live";
export type TransactionStatus = "draft" | "posted" | "reversed";
export type AccountType = "asset" | "liability" | "equity" | "revenue" | "expense";
export type AccountStatus = "active" | "frozen" | "closed";
export type HoldStatus = "active" | "captured" | "released" | "expired";
export type Metadata = Record<string, string>;

export interface MoneyValue {
  asset: string;
  /** int64 in minor units, string-encoded (e.g. "10000" = USD 100.00). */
  amount: string;
}

/**
 * A page of a collection. Default page size 50, maximum 200. `next_cursor`
 * is always present and null on the last page; a malformed cursor is a
 * 400 `invalid_cursor`.
 */
export interface Page<T> {
  data: T[];
  next_cursor: string | null;
}

// ---- Ledgers --------------------------------------------------------------

export interface Ledger {
  id: string;
  name: string;
  description?: string;
  environment: Environment;
  metadata: Metadata;
  created_at: string;
}

export interface LedgerCreateParams {
  name: string;
  description?: string;
  metadata?: Metadata;
}

// ---- Accounts -------------------------------------------------------------

export interface Account {
  id: string;
  ledger_id: string;
  name: string;
  type: AccountType;
  normal_balance: Direction;
  status: AccountStatus;
  metadata?: Metadata;
  created_at: string;
}

export interface AccountCreateParams {
  ledger_id: string;
  name: string;
  type: AccountType;
  normal_balance: Direction;
  metadata?: Metadata;
}

export interface Balance {
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

// ---- Transactions ---------------------------------------------------------

export interface Posting {
  id?: string;
  account_id: string;
  direction: Direction;
  amount: MoneyValue;
}

export interface Transaction {
  id: string;
  ledger_id: string;
  idempotency_key: string;
  reference?: string;
  description?: string;
  status: TransactionStatus;
  postings: Posting[];
  metadata: Metadata;
  reversed_by: string | null;
  reverses: string | null;
  effective_at: string;
  created_at: string;
  posted_at: string | null;
}

export interface TransactionCreateParams {
  ledger_id: string;
  /** Generated (UUID v4) by the SDK when omitted. */
  idempotencyKey?: string;
  reference?: string;
  description?: string;
  status?: "draft" | "posted";
  postings: Array<{ account_id: string; direction: Direction; amount: MoneyValue }>;
  effective_at?: string;
  metadata?: Metadata;
}

// ---- Reports --------------------------------------------------------------

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

export interface TrialBalance {
  ledger_id: string;
  as_of: string;
  rows: TrialBalanceRow[];
  totals: TrialBalanceTotal[];
}

export interface AssetBalance {
  asset: string;
  exponent: number;
  amount: string;
}

export interface StatementEntry {
  id: string;
  transaction_id: string;
  reference?: string;
  direction: Direction;
  amount: MoneyValue;
  effective_at: string;
  running_balance: string;
}

export interface Statement {
  account: Account;
  period: { from: string; to: string };
  opening_balance: AssetBalance[];
  entries: StatementEntry[];
  closing_balance: AssetBalance[];
  next_cursor: string | null;
}

export interface ProviderPosition {
  asset: string;
  exponent: number;
  net: string;
  direction: "they_owe_us" | "we_owe_them" | "flat";
  accounts: string[];
}

export interface ProviderPositions {
  data: Array<{ provider: string; positions: ProviderPosition[] }>;
  hint?: string;
}

// ---- Webhooks -------------------------------------------------------------

export type WebhookEventType =
  | "ledger.transaction.posted"
  | "ledger.transaction.reversed"
  | "ledger.hold.created"
  | "ledger.hold.captured"
  | "ledger.hold.released"
  | "recon.discrepancy.detected"
  | "*";

export interface WebhookSubscription {
  id: string;
  url: string;
  event_types: WebhookEventType[];
  active: boolean;
  created_at: string;
}

export interface WebhookSubscriptionWithSecret extends WebhookSubscription {
  /** Returned exactly once, on creation. */
  secret: string;
}

export interface WebhookSubscriptionCreateParams {
  url: string;
  event_types: WebhookEventType[];
}

export interface WebhookSubscriptionUpdateParams {
  url?: string;
  event_types?: WebhookEventType[];
  active?: boolean;
}

/** Response of POST /v1/webhook-subscriptions/{id}/rotate-secret. */
export interface RotatedSecret {
  id: string;
  /** The new lcwh_... secret, shown exactly once. */
  secret: string;
  /** Until this instant deliveries are also signed with the previous secret (24h grace). */
  previous_secret_expires_at: string;
}

export type WebhookDeliveryStatus = "pending" | "delivered" | "failed" | "dead";

export interface WebhookDelivery {
  id: string;
  subscription_id: string;
  event_id: string;
  event_type: WebhookEventType;
  payload: Record<string, unknown>;
  status: WebhookDeliveryStatus;
  attempts: number;
  next_attempt_at: string;
  last_status_code: number | null;
  last_error: string | null;
  created_at: string;
  delivered_at: string | null;
}

// ---- Sandbox signup -------------------------------------------------------

export interface SandboxSignup {
  tenant_id: string;
  slug: string;
  /** The lk_sandbox_... key, shown exactly once. */
  api_key: string;
  key_prefix: string;
  expires_at: string;
}
