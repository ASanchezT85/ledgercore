/**
 * Formatting helpers for the console.
 *
 * Monetary amounts across LedgerCore are integers in minor units plus an
 * asset code and an exponent from the asset registry. Formatting NEVER goes
 * through floats: everything is done with BigInt and string padding.
 */

const ASSET_SYMBOLS: Record<string, string> = {
  USD: "$",
  EUR: "€",
  MXN: "MX$",
  GBP: "£",
};

/**
 * Formats an amount expressed in integer minor units into a human string,
 * e.g. formatMoney(1284503217n, "USD", 2) => "$12,845,032.17".
 */
export function formatMoney(
  units: bigint | number,
  asset: string,
  exponent: number,
): string {
  const value = typeof units === "bigint" ? units : BigInt(Math.trunc(units));
  const negative = value < 0n;
  const abs = negative ? -value : value;

  const digits = abs.toString().padStart(exponent + 1, "0");
  const intPart = exponent > 0 ? digits.slice(0, -exponent) : digits;
  const fracPart = exponent > 0 ? digits.slice(-exponent) : "";

  const grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const symbol = ASSET_SYMBOLS[asset] ?? "";
  const body = fracPart.length > 0 ? `${grouped}.${fracPart}` : grouped;

  return `${negative ? "-" : ""}${symbol}${body}`;
}

/** Formats an amount together with its asset code: "$97.00 USD". */
export function formatMoneyWithAsset(
  units: bigint | number,
  asset: string,
  exponent: number,
): string {
  return `${formatMoney(units, asset, exponent)} ${asset}`;
}

const MONTHS_ES = [
  "ene",
  "feb",
  "mar",
  "abr",
  "may",
  "jun",
  "jul",
  "ago",
  "sep",
  "oct",
  "nov",
  "dic",
] as const;

/**
 * Deterministic UTC date-time formatter ("24 jul 2026, 14:32 UTC").
 * Manual formatting keeps server and client output identical regardless of
 * locale data, avoiding hydration mismatches.
 */
export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const month = MONTHS_ES[d.getUTCMonth()] ?? "";
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  return `${d.getUTCDate()} ${month} ${d.getUTCFullYear()}, ${hh}:${mm} UTC`;
}

/** Short date without time ("24 jul 2026"). */
export function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const month = MONTHS_ES[d.getUTCMonth()] ?? "";
  return `${d.getUTCDate()} ${month} ${d.getUTCFullYear()}`;
}

/** Integer with thousands separators: 18942 => "18,942". */
export function formatInt(value: number): string {
  return Math.trunc(value)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/** Percentage with two decimals: 99.98 => "99.98%" (recon precision matters). */
export function formatPct(value: number): string {
  return `${value.toFixed(2)}%`;
}
