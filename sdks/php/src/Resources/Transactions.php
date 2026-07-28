<?php

declare(strict_types=1);

namespace LedgerCore\Resources;

use LedgerCore\LedgerCore;

final class Transactions
{
    public function __construct(private readonly LedgerCore $client)
    {
    }

    /**
     * Creates a balanced transaction. `idempotency_key` is first-class: pass
     * your own to make retries safe, or the SDK generates a UUID v4.
     *
     * @param array{ledger_id: string, idempotency_key?: string, reference?: string, description?: string, status?: string, postings: list<array{account_id: string, direction: string, amount: array{asset: string, amount: string}}>, effective_at?: string, metadata?: array<string, string>} $params
     *
     * @return array<string, mixed> The created (posted) transaction.
     */
    public function create(array $params): array
    {
        $params['idempotency_key'] ??= LedgerCore::uuid();

        return $this->client->request('POST', '/v1/transactions', [], $params);
    }

    /**
     * @return array<string, mixed> Page: ['data' => Transaction[], 'next_cursor' => ?string]
     */
    public function list(?string $ledgerId = null, ?string $status = null, ?int $limit = null, ?string $cursor = null): array
    {
        return $this->client->request('GET', '/v1/transactions', [
            'ledger_id' => $ledgerId,
            'status' => $status,
            'limit' => $limit,
            'cursor' => $cursor,
        ]);
    }

    /**
     * Iterates every matching transaction, following next_cursor automatically.
     *
     * @return \Generator<int, array<string, mixed>>
     */
    public function listAll(?string $ledgerId = null, ?string $status = null, ?int $limit = null): \Generator
    {
        $cursor = null;
        do {
            $page = $this->list($ledgerId, $status, $limit, $cursor);
            yield from $page['data'];
            $cursor = $page['next_cursor'] ?? null;
        } while ($cursor !== null);
    }

    /** @return array<string, mixed> */
    public function get(string $id): array
    {
        return $this->client->request('GET', '/v1/transactions/' . rawurlencode($id));
    }

    /** Posts a draft transaction. @return array<string, mixed> */
    public function post(string $id): array
    {
        return $this->client->request('POST', '/v1/transactions/' . rawurlencode($id) . '/post');
    }

    /**
     * Reverses a posted transaction with a compensating one.
     *
     * @param array{idempotency_key?: string, reason?: string} $params
     *
     * @return array<string, mixed> The reversal transaction.
     */
    public function reverse(string $id, array $params = []): array
    {
        $params['idempotency_key'] ??= LedgerCore::uuid();

        return $this->client->request('POST', '/v1/transactions/' . rawurlencode($id) . '/reverse', [], $params);
    }
}
