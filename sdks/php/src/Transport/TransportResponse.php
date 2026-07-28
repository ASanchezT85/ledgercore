<?php

declare(strict_types=1);

namespace LedgerCore\Transport;

final class TransportResponse
{
    /**
     * @param array<string, string> $headers Response headers, lowercase names.
     */
    public function __construct(
        public readonly int $status,
        public readonly string $body,
        public readonly array $headers = [],
    ) {
    }

    public function header(string $name): ?string
    {
        return $this->headers[strtolower($name)] ?? null;
    }
}
