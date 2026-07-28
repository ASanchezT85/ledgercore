<?php

declare(strict_types=1);

namespace LedgerCore\Transport;

/** Minimal HTTP transport port so tests (and PSR-18 adapters) can replace cURL. */
interface TransportInterface
{
    /**
     * @param array<string, string> $headers
     */
    public function request(string $method, string $url, array $headers, ?string $body): TransportResponse;
}
