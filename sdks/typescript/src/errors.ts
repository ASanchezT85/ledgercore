/** Typed errors raised by the SDK. */

/**
 * Stable machine-readable error codes answered by the API
 * (`{"error":{"code","message","request_id"}}`).
 */
export const ERROR_CODES = [
  "validation_failed",
  "invalid_cursor",
  "unauthorized",
  "forbidden",
  "not_found",
  "conflict",
  "idempotency_conflict",
  "unbalanced_transaction",
  "insufficient_funds",
  "rate_limited",
  "service_unavailable",
  "internal",
] as const;

export type ErrorCode = (typeof ERROR_CODES)[number];

/**
 * An error answered by the LedgerCore API (any non-2xx response).
 * `code` is the stable snake_case machine code from the error envelope
 * (see ERROR_CODES); `requestId` comes from `error.request_id` (also in
 * the X-Request-Id header).
 */
export class LedgerCoreError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | null;

  constructor(options: {
    status: number;
    code: string;
    message: string;
    requestId?: string | null;
  }) {
    super(options.message);
    this.name = "LedgerCoreError";
    this.status = options.status;
    this.code = options.code;
    this.requestId = options.requestId ?? null;
  }
}

/** The API key was rejected during token exchange (401 unauthorized). */
export class AuthenticationError extends LedgerCoreError {
  constructor(options: { status: number; code: string; message: string; requestId?: string | null }) {
    super(options);
    this.name = "AuthenticationError";
  }
}

/** The API could not be reached (network failure or timeout). */
export class ConnectionError extends Error {
  readonly cause?: unknown;
  constructor(message: string, cause?: unknown) {
    super(message);
    this.name = "ConnectionError";
    this.cause = cause;
  }
}
