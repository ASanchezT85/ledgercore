<?php

declare(strict_types=1);

namespace LedgerCore\Exception;

/**
 * An error answered by the LedgerCore API (any non-2xx response).
 * `code` is the stable snake_case machine code from the error envelope.
 */
class LedgerCoreException extends \RuntimeException
{
    public function __construct(
        string $message,
        public readonly int $status,
        public readonly string $errorCode,
        public readonly ?string $requestId = null,
    ) {
        parent::__construct($message);
    }
}
