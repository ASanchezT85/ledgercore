<?php

declare(strict_types=1);

namespace LedgerCore\Resources;

use LedgerCore\LedgerCore;

final class Accounts
{
    public function __construct(private readonly LedgerCore $client)
    {
    }

    /**
     * @param array{ledger_id: string, name: string, type: string, normal_balance: string, metadata?: array<string, string>} $params
     *
     * @return array<string, mixed> The created account.
     */
    public function create(array $params): array
    {
        return $this->client->request('POST', '/v1/accounts', [], $params);
    }

    /**
     * @return array<string, mixed> Page: ['data' => Account[], 'next_cursor' => ?string]
     */
    public function list(?string $ledgerId = null, ?string $type = null, ?int $limit = null, ?string $cursor = null): array
    {
        return $this->client->request('GET', '/v1/accounts', [
            'ledger_id' => $ledgerId,
            'type' => $type,
            'limit' => $limit,
            'cursor' => $cursor,
        ]);
    }

    /**
     * Iterates every matching account, following next_cursor automatically.
     *
     * @return \Generator<int, array<string, mixed>>
     */
    public function listAll(?string $ledgerId = null, ?string $type = null, ?int $limit = null): \Generator
    {
        $cursor = null;
        do {
            $page = $this->list($ledgerId, $type, $limit, $cursor);
            yield from $page['data'];
            $cursor = $page['next_cursor'] ?? null;
        } while ($cursor !== null);
    }

    /** @return array<string, mixed> */
    public function get(string $id): array
    {
        return $this->client->request('GET', '/v1/accounts/' . rawurlencode($id));
    }

    /** @return array<string, mixed> ['data' => Balance[]] one row per asset. */
    public function balances(string $id): array
    {
        return $this->client->request('GET', '/v1/accounts/' . rawurlencode($id) . '/balances');
    }
}
