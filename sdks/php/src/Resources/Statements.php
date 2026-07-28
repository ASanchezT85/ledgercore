<?php

declare(strict_types=1);

namespace LedgerCore\Resources;

use LedgerCore\LedgerCore;

final class Statements
{
    public function __construct(private readonly LedgerCore $client)
    {
    }

    /**
     * Account statement: opening balance, running-balance entries, closing
     * balance over a period (defaults: last 30 days).
     *
     * @return array<string, mixed>
     */
    public function get(string $accountId, ?string $from = null, ?string $to = null, ?int $limit = null, ?string $cursor = null): array
    {
        return $this->client->request('GET', '/v1/statements', [
            'account_id' => $accountId,
            'from' => $from,
            'to' => $to,
            'limit' => $limit,
            'cursor' => $cursor,
        ]);
    }
}
