"use client";

/**
 * Client-side session for the console.
 *
 * The user signs in with an API key (lk_sandbox_... / lk_live_...). We
 * exchange it for a short-lived (15 min) EdDSA JWT via POST /v1/auth/token
 * and keep the JWT in memory + sessionStorage. The API key itself stays in
 * sessionStorage (or localStorage when the user opted into "remember on
 * this device") so we can silently re-exchange it when the JWT expires.
 */

const TOKEN_KEY = "lc_token";
const TOKEN_EXP_KEY = "lc_token_exp"; // epoch ms
const API_KEY_KEY = "lc_api_key";
const REMEMBER_KEY = "lc_api_key_remember";

/** Seconds of remaining validity below which we proactively re-exchange. */
const REFRESH_SKEW_MS = 60_000;

let memToken: string | null = null;
let memTokenExp = 0;

export interface SessionClaims {
  tenantId: string;
  env: "sandbox" | "live" | string;
  scopes: string[];
  keyId: string;
  exp: number;
}

export class AuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AuthError";
  }
}

export function apiBaseUrl(): string {
  return process.env.NEXT_PUBLIC_API_URL ?? "";
}

function storage(kind: "session" | "local"): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return kind === "session" ? window.sessionStorage : window.localStorage;
  } catch {
    return null;
  }
}

/** The stored API key, wherever the user chose to keep it. */
export function storedApiKey(): string | null {
  return (
    storage("session")?.getItem(API_KEY_KEY) ??
    storage("local")?.getItem(REMEMBER_KEY) ??
    null
  );
}

/** Whether a real (non-demo) session exists on this device. */
export function hasSession(): boolean {
  return storedApiKey() !== null;
}

/** Base64url-decode the JWT payload. Returns null on any malformed input. */
export function decodeClaims(token: string): SessionClaims | null {
  try {
    const payload = token.split(".")[1];
    if (!payload) return null;
    const json = JSON.parse(
      atob(payload.replace(/-/g, "+").replace(/_/g, "/")),
    ) as Record<string, unknown>;
    return {
      tenantId: String(json.tenant_id ?? ""),
      env: String(json.env ?? "sandbox"),
      scopes: Array.isArray(json.scopes) ? json.scopes.map(String) : [],
      keyId: String(json.sub ?? ""),
      exp: Number(json.exp ?? 0),
    };
  } catch {
    return null;
  }
}

async function exchangeKey(apiKey: string): Promise<string> {
  const base = apiBaseUrl();
  if (!base) throw new Error("NEXT_PUBLIC_API_URL is not configured");
  let res: Response;
  try {
    res = await fetch(`${base}/v1/auth/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ api_key: apiKey }),
      cache: "no-store",
    });
  } catch {
    throw new Error("network");
  }
  if (res.status === 401) throw new AuthError("invalid_credentials");
  if (!res.ok) throw new Error(`token exchange failed (${res.status})`);
  const body = (await res.json()) as {
    access_token: string;
    expires_in: number;
  };
  memToken = body.access_token;
  memTokenExp = Date.now() + body.expires_in * 1000;
  const ss = storage("session");
  ss?.setItem(TOKEN_KEY, memToken);
  ss?.setItem(TOKEN_EXP_KEY, String(memTokenExp));
  return memToken;
}

/**
 * Starts a session: validates the key by exchanging it for a JWT and stores
 * it. With `remember`, the key ALSO goes to localStorage (survives closing
 * the browser) — the UI must warn about that explicitly.
 */
export async function login(apiKey: string, remember: boolean): Promise<void> {
  await exchangeKey(apiKey.trim());
  storage("session")?.setItem(API_KEY_KEY, apiKey.trim());
  if (remember) storage("local")?.setItem(REMEMBER_KEY, apiKey.trim());
  else storage("local")?.removeItem(REMEMBER_KEY);
}

export function logout(): void {
  memToken = null;
  memTokenExp = 0;
  const ss = storage("session");
  ss?.removeItem(TOKEN_KEY);
  ss?.removeItem(TOKEN_EXP_KEY);
  ss?.removeItem(API_KEY_KEY);
  storage("local")?.removeItem(REMEMBER_KEY);
}

/**
 * Returns a valid access token, silently re-exchanging the stored API key
 * when the current one is expired or about to expire. Throws AuthError when
 * no session exists or the key stopped being valid.
 */
export async function getAccessToken(): Promise<string> {
  if (!memToken) {
    const ss = storage("session");
    memToken = ss?.getItem(TOKEN_KEY) ?? null;
    memTokenExp = Number(ss?.getItem(TOKEN_EXP_KEY) ?? 0);
  }
  if (memToken && Date.now() < memTokenExp - REFRESH_SKEW_MS) {
    return memToken;
  }
  const key = storedApiKey();
  if (!key) throw new AuthError("no_session");
  return exchangeKey(key);
}

/** Claims of the current session token, if any. */
export async function sessionClaims(): Promise<SessionClaims | null> {
  if (!hasSession()) return null;
  try {
    return decodeClaims(await getAccessToken());
  } catch {
    return null;
  }
}
