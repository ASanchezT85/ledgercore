/**
 * Data-access layer for the console.
 *
 * Every fetcher targets the LedgerCore gateway at NEXT_PUBLIC_API_URL. If the
 * variable is unset, the request fails or times out, the fetcher resolves
 * with realistic demo data and `demo: true` so pages can flag it visually.
 * The Go services are being built in parallel — the console never assumes
 * they are running.
 */

import {
  mockDashboard,
  mockDevelopers,
  mockLedgers,
  mockReconciliation,
  mockTransactions,
} from "./mock-data";
import type {
  DashboardData,
  DeveloperData,
  LedgerSummary,
  ReconciliationData,
  Transaction,
} from "./types";

export interface ApiResult<T> {
  data: T;
  /** True when the payload came from the demo dataset, not the live API. */
  demo: boolean;
}

const FETCH_TIMEOUT_MS = 2500;

async function fetchWithFallback<T>(
  path: string,
  fallback: T,
): Promise<ApiResult<T>> {
  const base = process.env.NEXT_PUBLIC_API_URL;
  if (!base) {
    return { data: fallback, demo: true };
  }
  try {
    const res = await fetch(`${base}${path}`, {
      signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    if (!res.ok) {
      throw new Error(`unexpected status ${res.status}`);
    }
    const data = (await res.json()) as T;
    return { data, demo: false };
  } catch {
    return { data: fallback, demo: true };
  }
}

export function getDashboard(): Promise<ApiResult<DashboardData>> {
  return fetchWithFallback<DashboardData>("/v1/console/dashboard", mockDashboard);
}

export function getLedgers(): Promise<ApiResult<LedgerSummary[]>> {
  return fetchWithFallback<LedgerSummary[]>("/v1/ledgers", mockLedgers);
}

export function getTransactions(): Promise<ApiResult<Transaction[]>> {
  return fetchWithFallback<Transaction[]>("/v1/transactions", mockTransactions);
}

export function getReconciliation(): Promise<ApiResult<ReconciliationData>> {
  return fetchWithFallback<ReconciliationData>(
    "/v1/recon/overview",
    mockReconciliation,
  );
}

export function getDeveloperData(): Promise<ApiResult<DeveloperData>> {
  return fetchWithFallback<DeveloperData>(
    "/v1/console/developers",
    mockDevelopers,
  );
}
