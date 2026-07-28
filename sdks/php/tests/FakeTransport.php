<?php

declare(strict_types=1);

namespace LedgerCore\Tests;

use LedgerCore\Transport\TransportInterface;
use LedgerCore\Transport\TransportResponse;

/** Scripted transport: each queued response answers one request in order. */
final class FakeTransport implements TransportInterface
{
    /** @var list<array{method: string, url: string, headers: array<string, string>, body: mixed}> */
    public array $calls = [];

    /** @var list<TransportResponse> */
    private array $queue = [];

    public function queue(int $status, array $body, array $headers = []): void
    {
        $this->queue[] = new TransportResponse($status, json_encode($body, JSON_THROW_ON_ERROR), $headers);
    }

    public function queueToken(string $token = 'jwt-1', int $expiresIn = 900): void
    {
        $this->queue(200, ['access_token' => $token, 'token_type' => 'Bearer', 'expires_in' => $expiresIn]);
    }

    public function request(string $method, string $url, array $headers, ?string $body): TransportResponse
    {
        $this->calls[] = [
            'method' => $method,
            'url' => $url,
            'headers' => $headers,
            'body' => $body === null ? null : json_decode($body, true),
        ];
        $response = array_shift($this->queue);
        if ($response === null) {
            throw new \LogicException("unexpected request: {$method} {$url}");
        }

        return $response;
    }
}
