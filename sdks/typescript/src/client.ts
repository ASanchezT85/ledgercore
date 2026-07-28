import { AuthenticationError, ConnectionError, LedgerCoreError } from "./errors.js";
import { verifySignature, type VerifySignatureOptions } from "./webhook-signature.js";
import type {
  Account,
  AccountCreateParams,
  AccountType,
  Balance,
  Ledger,
  LedgerCreateParams,
  Page,
  ProviderPositions,
  RotatedSecret,
  Statement,
  Transaction,
  TransactionCreateParams,
  TransactionStatus,
  TrialBalance,
  WebhookDelivery,
  WebhookDeliveryStatus,
  WebhookSubscription,
  WebhookSubscriptionCreateParams,
  WebhookSubscriptionUpdateParams,
  WebhookSubscriptionWithSecret,
} from "./types.js";

export interface LedgerCoreOptions {
  /** Tenant API key (lk_sandbox_... / lk_live_...). */
  apiKey: string;
  /** API gateway base URL. Defaults to http://localhost:8080 (local stack). */
  baseUrl?: string;
  /** Per-request timeout in milliseconds. Default 10 000. */
  timeoutMs?: number;
  /** Custom fetch (for tests / non-standard runtimes). Defaults to globalThis.fetch. */
  fetch?: typeof fetch;
}

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  query?: Record<string, string | number | undefined>;
  body?: unknown;
  /** Set on 401 retry to force a fresh token exchange. */
  freshToken?: boolean;
}

const DEFAULT_BASE_URL = "http://localhost:8080";

/** Renew the JWT when it has less than this many ms of validity left. */
const REFRESH_SKEW_MS = 60_000;

function uuid(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  // Fallback RFC 4122 v4 for runtimes without randomUUID (e.g. non-secure contexts).
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  bytes[6] = (bytes[6]! & 0x0f) | 0x40;
  bytes[8] = (bytes[8]! & 0x3f) | 0x80;
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

/**
 * LedgerCore API client.
 *
 *   const lc = new LedgerCore({ apiKey: "lk_sandbox_..." });
 *   const ledger = await lc.ledgers.create({ name: "main" });
 *
 * Authentication is handled internally: the API key is exchanged for a
 * short-lived (15 min) JWT on first use, cached, renewed before expiry and
 * re-exchanged once on an unexpected 401.
 */
export class LedgerCore {
  private readonly apiKey: string;
  private readonly baseUrl: string;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;

  private token: string | null = null;
  private tokenExpiresAt = 0; // epoch ms
  private tokenExchange: Promise<string> | null = null;

  constructor(options: LedgerCoreOptions) {
    if (!options.apiKey) throw new TypeError("apiKey is required");
    this.apiKey = options.apiKey;
    this.baseUrl = (options.baseUrl ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
    this.timeoutMs = options.timeoutMs ?? 10_000;
    this.fetchImpl = options.fetch ?? fetch;
  }

  // ---- Auth ---------------------------------------------------------------

  private async getToken(fresh = false): Promise<string> {
    if (!fresh && this.token && Date.now() < this.tokenExpiresAt - REFRESH_SKEW_MS) {
      return this.token;
    }
    // Deduplicate concurrent exchanges.
    this.tokenExchange ??= this.exchangeApiKey().finally(() => {
      this.tokenExchange = null;
    });
    return this.tokenExchange;
  }

  private async exchangeApiKey(): Promise<string> {
    const res = await this.rawFetch(`${this.baseUrl}/v1/auth/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ api_key: this.apiKey }),
    });
    if (!res.ok) {
      const err = await this.toError(res);
      throw res.status === 401
        ? new AuthenticationError({ status: err.status, code: err.code, message: err.message, requestId: err.requestId })
        : err;
    }
    const body = (await res.json()) as { access_token: string; expires_in: number };
    this.token = body.access_token;
    this.tokenExpiresAt = Date.now() + body.expires_in * 1000;
    return this.token;
  }

  // ---- Transport ----------------------------------------------------------

  private async rawFetch(url: string, init: RequestInit): Promise<Response> {
    try {
      return await this.fetchImpl(url, {
        ...init,
        signal: AbortSignal.timeout(this.timeoutMs),
      });
    } catch (cause) {
      throw new ConnectionError(`could not reach LedgerCore at ${this.baseUrl}`, cause);
    }
  }

  private async toError(res: Response): Promise<LedgerCoreError> {
    let code = `http_${res.status}`;
    let message = `HTTP ${res.status}`;
    let requestId: string | null = null;
    try {
      const body = (await res.json()) as {
        error?: { code?: string; message?: string; request_id?: string };
      };
      if (body.error?.code) code = body.error.code;
      if (body.error?.message) message = body.error.message;
      if (body.error?.request_id) requestId = body.error.request_id;
    } catch {
      /* non-JSON error body */
    }
    return new LedgerCoreError({
      status: res.status,
      code,
      message,
      requestId: requestId ?? res.headers.get("X-Request-Id"),
    });
  }

  private async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const token = await this.getToken(options.freshToken ?? false);
    const url = new URL(this.baseUrl + path);
    for (const [key, value] of Object.entries(options.query ?? {})) {
      if (value !== undefined) url.searchParams.set(key, String(value));
    }
    const headers: Record<string, string> = {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
    };
    if (options.body !== undefined) headers["Content-Type"] = "application/json";

    const res = await this.rawFetch(url.toString(), {
      method: options.method ?? "GET",
      headers,
      body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
    });

    if (res.status === 401 && !options.freshToken) {
      // The cached token was rejected (revoked key, clock skew...): force a
      // fresh exchange and retry exactly once.
      return this.request<T>(path, { ...options, freshToken: true });
    }
    if (!res.ok) throw await this.toError(res);
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  // ---- Pagination ----------------------------------------------------------

  /**
   * Auto-pagination: walks `{data, next_cursor}` pages until next_cursor is
   * null, yielding each item. Backs every `listAll()` helper.
   */
  private async *paginate<T>(
    fetchPage: (cursor: string | undefined) => Promise<Page<T>>,
  ): AsyncGenerator<T, void, undefined> {
    let cursor: string | undefined;
    do {
      const page = await fetchPage(cursor);
      yield* page.data;
      cursor = page.next_cursor ?? undefined;
    } while (cursor !== undefined);
  }

  // ---- Resources ----------------------------------------------------------

  readonly ledgers = {
    create: (params: LedgerCreateParams): Promise<Ledger> =>
      this.request("/v1/ledgers", { method: "POST", body: params }),
    list: (params: { limit?: number; cursor?: string } = {}): Promise<Page<Ledger>> =>
      this.request("/v1/ledgers", { query: { limit: params.limit, cursor: params.cursor } }),
    get: (id: string): Promise<Ledger> =>
      this.request(`/v1/ledgers/${encodeURIComponent(id)}`),
    /** Iterates every ledger, following next_cursor automatically. */
    listAll: (params: { limit?: number } = {}): AsyncGenerator<Ledger, void, undefined> =>
      this.paginate((cursor) => this.ledgers.list({ limit: params.limit, cursor })),
  };

  readonly accounts = {
    create: (params: AccountCreateParams): Promise<Account> =>
      this.request("/v1/accounts", { method: "POST", body: params }),
    list: (
      params: { ledgerId?: string; type?: AccountType; limit?: number; cursor?: string } = {},
    ): Promise<Page<Account>> =>
      this.request("/v1/accounts", {
        query: {
          ledger_id: params.ledgerId,
          type: params.type,
          limit: params.limit,
          cursor: params.cursor,
        },
      }),
    get: (id: string): Promise<Account> =>
      this.request(`/v1/accounts/${encodeURIComponent(id)}`),
    /** Iterates every matching account, following next_cursor automatically. */
    listAll: (
      params: { ledgerId?: string; type?: AccountType; limit?: number } = {},
    ): AsyncGenerator<Account, void, undefined> =>
      this.paginate((cursor) => this.accounts.list({ ...params, cursor })),
    balances: (id: string): Promise<{ data: Balance[] }> =>
      this.request(`/v1/accounts/${encodeURIComponent(id)}/balances`),
  };

  readonly transactions = {
    /**
     * Creates a balanced transaction. `idempotencyKey` is first-class: pass
     * your own to make retries safe, or let the SDK generate a UUID v4.
     */
    create: (params: TransactionCreateParams): Promise<Transaction> => {
      const { idempotencyKey, ...rest } = params;
      return this.request("/v1/transactions", {
        method: "POST",
        body: { ...rest, idempotency_key: idempotencyKey ?? uuid() },
      });
    },
    list: (
      params: { ledgerId?: string; status?: TransactionStatus; limit?: number; cursor?: string } = {},
    ): Promise<Page<Transaction>> =>
      this.request("/v1/transactions", {
        query: {
          ledger_id: params.ledgerId,
          status: params.status,
          limit: params.limit,
          cursor: params.cursor,
        },
      }),
    get: (id: string): Promise<Transaction> =>
      this.request(`/v1/transactions/${encodeURIComponent(id)}`),
    /** Iterates every matching transaction, following next_cursor automatically. */
    listAll: (
      params: { ledgerId?: string; status?: TransactionStatus; limit?: number } = {},
    ): AsyncGenerator<Transaction, void, undefined> =>
      this.paginate((cursor) => this.transactions.list({ ...params, cursor })),
    post: (id: string): Promise<Transaction> =>
      this.request(`/v1/transactions/${encodeURIComponent(id)}/post`, { method: "POST" }),
    reverse: (
      id: string,
      params: { idempotencyKey?: string; reason?: string } = {},
    ): Promise<Transaction> =>
      this.request(`/v1/transactions/${encodeURIComponent(id)}/reverse`, {
        method: "POST",
        body: { idempotency_key: params.idempotencyKey ?? uuid(), reason: params.reason },
      }),
  };

  readonly statements = {
    get: (params: {
      accountId: string;
      from?: string;
      to?: string;
      limit?: number;
      cursor?: string;
    }): Promise<Statement> =>
      this.request("/v1/statements", {
        query: {
          account_id: params.accountId,
          from: params.from,
          to: params.to,
          limit: params.limit,
          cursor: params.cursor,
        },
      }),
  };

  readonly trialBalance = {
    get: (params: { ledgerId: string; asOf?: string }): Promise<TrialBalance> =>
      this.request("/v1/trial-balance", {
        query: { ledger_id: params.ledgerId, as_of: params.asOf },
      }),
  };

  readonly providerPositions = {
    get: (): Promise<ProviderPositions> => this.request("/v1/provider-positions"),
  };

  readonly webhooks = {
    create: (params: WebhookSubscriptionCreateParams): Promise<WebhookSubscriptionWithSecret> =>
      this.request("/v1/webhook-subscriptions", { method: "POST", body: params }),
    list: (params: { limit?: number; cursor?: string } = {}): Promise<Page<WebhookSubscription>> =>
      this.request("/v1/webhook-subscriptions", {
        query: { limit: params.limit, cursor: params.cursor },
      }),
    /** Iterates every subscription, following next_cursor automatically. */
    listAll: (params: { limit?: number } = {}): AsyncGenerator<WebhookSubscription, void, undefined> =>
      this.paginate((cursor) => this.webhooks.list({ limit: params.limit, cursor })),
    get: (id: string): Promise<WebhookSubscription> =>
      this.request(`/v1/webhook-subscriptions/${encodeURIComponent(id)}`),
    update: (id: string, params: WebhookSubscriptionUpdateParams): Promise<WebhookSubscription> =>
      this.request(`/v1/webhook-subscriptions/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body: params,
      }),
    /**
     * Rotates the signing secret. The previous secret keeps verifying until
     * `previous_secret_expires_at` (24h grace): deliveries carry two `v1`
     * entries in that window, and verifySignature accepts either.
     */
    rotateSecret: (id: string): Promise<RotatedSecret> =>
      this.request(`/v1/webhook-subscriptions/${encodeURIComponent(id)}/rotate-secret`, {
        method: "POST",
      }),
    deliveries: {
      list: (
        params: {
          subscriptionId?: string;
          status?: WebhookDeliveryStatus;
          limit?: number;
          cursor?: string;
        } = {},
      ): Promise<Page<WebhookDelivery>> =>
        this.request("/v1/webhook-deliveries", {
          query: {
            subscription_id: params.subscriptionId,
            status: params.status,
            limit: params.limit,
            cursor: params.cursor,
          },
        }),
      /** Iterates every matching delivery, following next_cursor automatically. */
      listAll: (
        params: {
          subscriptionId?: string;
          status?: WebhookDeliveryStatus;
          limit?: number;
        } = {},
      ): AsyncGenerator<WebhookDelivery, void, undefined> =>
        this.paginate((cursor) => this.webhooks.deliveries.list({ ...params, cursor })),
      retry: (id: string): Promise<WebhookDelivery> =>
        this.request(`/v1/webhook-deliveries/${encodeURIComponent(id)}/retry`, {
          method: "POST",
        }),
    },
    /**
     * Verifies an incoming webhook signature (X-LedgerCore-Signature). This
     * is a static helper: it needs no API key and never talks to the network.
     */
    verifySignature: (
      payload: string | Uint8Array,
      header: string | null | undefined,
      secret: string,
      options?: VerifySignatureOptions,
    ): Promise<boolean> => verifySignature(payload, header, secret, options),
  };
}
