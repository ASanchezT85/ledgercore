<?php

declare(strict_types=1);

namespace LedgerCore\Resources;

use LedgerCore\LedgerCore;

final class TrialBalance
{
    public function __construct(private readonly LedgerCore $client)
    {
    }

    /**
     * Trial balance of a ledger, optionally reconstructed as of an instant.
     *
     * @return array<string, mixed> ['ledger_id', 'as_of', 'rows', 'totals']
     */
    public function get(string $ledgerId, ?string $asOf = null): array
    {
        return $this->client->request('GET', '/v1/trial-balance', [
            'ledger_id' => $ledgerId,
            'as_of' => $asOf,
        ]);
    }
}
