<?php

declare(strict_types=1);

namespace LedgerCore;

/**
 * Stable machine-readable error codes answered by the API
 * (`{"error":{"code","message","request_id"}}`).
 */
final class ErrorCode
{
    public const VALIDATION_FAILED = 'validation_failed';
    public const INVALID_CURSOR = 'invalid_cursor';
    public const UNAUTHORIZED = 'unauthorized';
    public const FORBIDDEN = 'forbidden';
    public const NOT_FOUND = 'not_found';
    public const CONFLICT = 'conflict';
    public const IDEMPOTENCY_CONFLICT = 'idempotency_conflict';
    public const UNBALANCED_TRANSACTION = 'unbalanced_transaction';
    public const INSUFFICIENT_FUNDS = 'insufficient_funds';
    public const RATE_LIMITED = 'rate_limited';
    public const SERVICE_UNAVAILABLE = 'service_unavailable';
    public const INTERNAL = 'internal';

    /** @var list<string> The complete catalog. */
    public const ALL = [
        self::VALIDATION_FAILED,
        self::INVALID_CURSOR,
        self::UNAUTHORIZED,
        self::FORBIDDEN,
        self::NOT_FOUND,
        self::CONFLICT,
        self::IDEMPOTENCY_CONFLICT,
        self::UNBALANCED_TRANSACTION,
        self::INSUFFICIENT_FUNDS,
        self::RATE_LIMITED,
        self::SERVICE_UNAVAILABLE,
        self::INTERNAL,
    ];

    private function __construct()
    {
    }
}
