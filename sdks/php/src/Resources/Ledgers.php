<?php

declare(strict_types=1);

namespace LedgerCore\Resources;

use LedgerCore\LedgerCore;

final class Ledgers
{
    public function __construct(private readonly LedgerCore $client)
    {
    }

    /**
     * @param array{name: string, description?: string, metadata?: array<string, string>} $params
     *
     * @return array<string, mixed> The created ledger.
     */
    public function create(array $params): array
    {
        return $this->client->request('POST', '/v1/ledgers', [], $params);
    }

    /**
     * @return array<string, mixed> Page: ['data' => Ledger[], 'next_cursor' => ?string]
     */
    public function list(?int $limit = null, ?string $cursor = null): array
    {
        return $this->client->request('GET', '/v1/ledgers', ['limit' => $limit, 'cursor' => $cursor]);
    }

    /** @return array<string, mixed> */
    public function get(string $id): array
    {
        return $this->client->request('GET', '/v1/ledgers/' . rawurlencode($id));
    }

    /**
     * Iterates every ledger, following next_cursor automatically.
     *
     * @return \Generator<int, array<string, mixed>>
     */
    public function listAll(?int $limit = null): \Generator
    {
        $cursor = null;
        do {
            $page = $this->list($limit, $cursor);
            yield from $page['data'];
            $cursor = $page['next_cursor'] ?? null;
        } while ($cursor !== null);
    }
}
