import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LedgerCore } from "../src/client.js";
import { AuthenticationError, LedgerCoreError } from "../src/errors.js";

interface Call {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: unknown;
}

/** Scripted fetch mock: each queued responder handles one request in order. */
function makeFetch(responders: Array<(call: Call) => Response>) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const call: Call = {
      url: String(input),
      method: init?.method ?? "GET",
      headers: Object.fromEntries(
        Object.entries((init?.headers ?? {}) as Record<string, string>),
      ),
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    };
    calls.push(call);
    const responder = responders.shift();
    if (!responder) throw new Error(`unexpected request: ${call.method} ${call.url}`);
    return responder(call);
  });
  return { fetchMock: fetchMock as unknown as typeof fetch, calls };
}

function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

function tokenResponse(token = "jwt-1", expiresIn = 900): Response {
  return json(200, { access_token: token, token_type: "Bearer", expires_in: expiresIn });
}

const KEY = "lk_sandbox_test";

describe("auth", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("exchanges the API key once and caches the JWT", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse("jwt-1"),
      () => json(200, { data: [] }),
      () => json(200, { data: [] }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await lc.ledgers.list();
    await lc.ledgers.list();
    expect(calls).toHaveLength(3);
    expect(calls[0]!.url).toBe("http://api.test/v1/auth/token");
    expect(calls[0]!.body).toEqual({ api_key: KEY });
    expect(calls[1]!.headers.Authorization).toBe("Bearer jwt-1");
    expect(calls[2]!.headers.Authorization).toBe("Bearer jwt-1");
  });

  it("renews the token when less than 60s of validity remain", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse("jwt-1", 900),
      () => json(200, { data: [] }),
      () => tokenResponse("jwt-2", 900),
      () => json(200, { data: [] }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await lc.ledgers.list();
    vi.advanceTimersByTime(841_000); // 900s - 59s left -> under the 60s skew
    await lc.ledgers.list();
    expect(calls.map((c) => c.url)).toEqual([
      "http://api.test/v1/auth/token",
      "http://api.test/v1/ledgers",
      "http://api.test/v1/auth/token",
      "http://api.test/v1/ledgers",
    ]);
    expect(calls[3]!.headers.Authorization).toBe("Bearer jwt-2");
  });

  it("retries exactly once with a fresh token on 401", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse("jwt-1"),
      () => json(401, { error: { code: "unauthorized", message: "expired", request_id: "r1" } }),
      () => tokenResponse("jwt-2"),
      () => json(200, { data: [{ id: "l1" }] }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    const page = await lc.ledgers.list();
    expect(page.data).toHaveLength(1);
    expect(calls[3]!.headers.Authorization).toBe("Bearer jwt-2");
  });

  it("does not loop on repeated 401s", async () => {
    const reject = () =>
      json(401, { error: { code: "unauthorized", message: "nope", request_id: "r2" } });
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse("jwt-1"),
      reject,
      () => tokenResponse("jwt-2"),
      reject,
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await expect(lc.ledgers.list()).rejects.toMatchObject({ status: 401, code: "unauthorized" });
    expect(calls).toHaveLength(4);
  });

  it("raises AuthenticationError when the key itself is rejected", async () => {
    const { fetchMock } = makeFetch([
      () => json(401, { error: { code: "unauthorized", message: "bad key", request_id: "r3" } }),
    ]);
    const lc = new LedgerCore({ apiKey: "lk_bad", baseUrl: "http://api.test", fetch: fetchMock });
    await expect(lc.ledgers.list()).rejects.toBeInstanceOf(AuthenticationError);
  });
});

describe("idempotency", () => {
  it("sends the caller's idempotency key as-is", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse(),
      (call) => json(201, { id: "t1", ...(call.body as object) }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await lc.transactions.create({
      ledger_id: "l1",
      idempotencyKey: "dep-2026-0001",
      postings: [],
    });
    expect((calls[1]!.body as { idempotency_key: string }).idempotency_key).toBe("dep-2026-0001");
  });

  it("generates a UUID v4 when no key is given", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse(),
      () => json(201, { id: "t1" }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await lc.transactions.create({ ledger_id: "l1", postings: [] });
    const sent = (calls[1]!.body as { idempotency_key: string }).idempotency_key;
    expect(sent).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });
});

describe("errors and queries", () => {
  it("surfaces LedgerCoreError with status, code and requestId from the error body", async () => {
    const { fetchMock } = makeFetch([
      () => tokenResponse(),
      () =>
        json(422, {
          error: {
            code: "unbalanced_transaction",
            message: "debits != credits",
            request_id: "req-42",
          },
        }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    const err = await lc.transactions
      .create({ ledger_id: "l1", postings: [] })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(LedgerCoreError);
    expect(err).toMatchObject({
      status: 422,
      code: "unbalanced_transaction",
      message: "debits != credits",
      requestId: "req-42",
    });
  });

  it("falls back to the X-Request-Id header when the error body is not JSON", async () => {
    const { fetchMock } = makeFetch([
      () => tokenResponse(),
      () => new Response("boom", { status: 503, headers: { "X-Request-Id": "req-h" } }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await expect(lc.ledgers.list()).rejects.toMatchObject({
      status: 503,
      code: "http_503",
      requestId: "req-h",
    });
  });

  it("surfaces invalid_cursor for malformed pagination cursors", async () => {
    const { fetchMock } = makeFetch([
      () => tokenResponse(),
      () =>
        json(400, {
          error: { code: "invalid_cursor", message: "malformed cursor", request_id: "req-c" },
        }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await expect(lc.ledgers.list({ cursor: "garbage" })).rejects.toMatchObject({
      status: 400,
      code: "invalid_cursor",
      requestId: "req-c",
    });
  });

  it("builds query strings from camelCase params", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse(),
      () => json(200, { ledger_id: "l1", as_of: "x", rows: [], totals: [] }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    await lc.trialBalance.get({ ledgerId: "l1", asOf: "2026-07-01T00:00:00Z" });
    expect(calls[1]!.url).toBe(
      "http://api.test/v1/trial-balance?ledger_id=l1&as_of=2026-07-01T00%3A00%3A00Z",
    );
  });
});

describe("pagination", () => {
  it("passes limit and cursor and returns next_cursor", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse(),
      () => json(200, { data: [{ id: "s1" }], next_cursor: "c2" }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    const page = await lc.webhooks.list({ limit: 1, cursor: "c1" });
    expect(calls[1]!.url).toBe("http://api.test/v1/webhook-subscriptions?limit=1&cursor=c1");
    expect(page.next_cursor).toBe("c2");
  });

  it("listAll follows next_cursor until null", async () => {
    const { fetchMock, calls } = makeFetch([
      () => tokenResponse(),
      () => json(200, { data: [{ id: "l1" }, { id: "l2" }], next_cursor: "c2" }),
      () => json(200, { data: [{ id: "l3" }], next_cursor: null }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    const ids: string[] = [];
    for await (const ledger of lc.ledgers.listAll({ limit: 2 })) ids.push(ledger.id);
    expect(ids).toEqual(["l1", "l2", "l3"]);
    expect(calls[1]!.url).toBe("http://api.test/v1/ledgers?limit=2");
    expect(calls[2]!.url).toBe("http://api.test/v1/ledgers?limit=2&cursor=c2");
  });
});

describe("webhooks", () => {
  it("rotateSecret returns previous_secret_expires_at typed", async () => {
    const { fetchMock } = makeFetch([
      () => tokenResponse(),
      () =>
        json(200, {
          id: "ws1",
          secret: "whsec_new",
          previous_secret_expires_at: "2026-07-29T12:00:00Z",
        }),
    ]);
    const lc = new LedgerCore({ apiKey: KEY, baseUrl: "http://api.test", fetch: fetchMock });
    const rotated = await lc.webhooks.rotateSecret("ws1");
    expect(rotated.previous_secret_expires_at).toBe("2026-07-29T12:00:00Z");
    expect(rotated.secret).toBe("whsec_new");
  });
});
