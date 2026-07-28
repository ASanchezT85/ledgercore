/**
 * Verification of the LedgerCore webhook signature header:
 *
 *   X-LedgerCore-Signature: t=<unix seconds>,v1=<hex hmac-sha256>
 *
 * The MAC input is "<t>." + raw body, keyed with the subscription secret
 * (whsec_...). Mirrors services/webhooks/internal/signature (the reference
 * implementation). Uses WebCrypto, so it works on Node 18+, workers and
 * browsers.
 */

export const SIGNATURE_HEADER = "X-LedgerCore-Signature";

/** Recommended replay-protection window, in seconds. */
export const DEFAULT_TOLERANCE_SECONDS = 300;

export interface VerifySignatureOptions {
  /** Replay window in seconds; 0 disables the freshness check. Default 300. */
  toleranceSeconds?: number;
  /** Reference clock (epoch seconds) for deterministic tests. */
  now?: number;
}

const encoder = new TextEncoder();

function toBytes(payload: string | Uint8Array): Uint8Array {
  return typeof payload === "string" ? encoder.encode(payload) : payload;
}

function toHex(buf: ArrayBuffer): string {
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** Constant-time comparison of two equal-purpose hex strings. */
function timingSafeEqualHex(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

async function hmacHex(secret: string, message: Uint8Array): Promise<string> {
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  // Copy into a plain ArrayBuffer-backed view for WebCrypto typings.
  const data = new Uint8Array(message);
  return toHex(await crypto.subtle.sign("HMAC", key, data));
}

/**
 * Verifies a webhook delivery. Returns true only when the header carries a
 * fresh timestamp and at least one v1 signature matches the payload.
 *
 *   const ok = await verifySignature(rawBody, req.headers["x-ledgercore-signature"], secret);
 *
 * `payload` must be the exact raw request body bytes/string as received —
 * do not re-serialize parsed JSON.
 */
export async function verifySignature(
  payload: string | Uint8Array,
  header: string | null | undefined,
  secret: string,
  options: VerifySignatureOptions = {},
): Promise<boolean> {
  if (!header || !secret) return false;

  let timestamp: number | null = null;
  const candidates: string[] = [];
  for (const part of header.split(",")) {
    const eq = part.indexOf("=");
    if (eq < 0) return false; // malformed header
    const key = part.slice(0, eq).trim();
    const value = part.slice(eq + 1).trim();
    if (key === "t") {
      if (!/^\d+$/.test(value)) return false;
      timestamp = Number(value);
    } else if (key === "v1") {
      candidates.push(value.toLowerCase());
    }
    // Unknown keys (e.g. v2) are ignored for forward compatibility.
  }
  if (timestamp === null || candidates.length === 0) return false;

  const tolerance = options.toleranceSeconds ?? DEFAULT_TOLERANCE_SECONDS;
  if (tolerance > 0) {
    const now = options.now ?? Math.floor(Date.now() / 1000);
    if (Math.abs(now - timestamp) > tolerance) return false;
  }

  const body = toBytes(payload);
  const message = new Uint8Array(String(timestamp).length + 1 + body.length);
  message.set(encoder.encode(`${timestamp}.`), 0);
  message.set(body, String(timestamp).length + 1);
  const expected = await hmacHex(secret, message);

  let ok = false;
  for (const candidate of candidates) {
    if (timingSafeEqualHex(candidate, expected)) ok = true;
  }
  return ok;
}
