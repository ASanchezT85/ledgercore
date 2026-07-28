<?php

declare(strict_types=1);

namespace LedgerCore\Resources;

use LedgerCore\LedgerCore;

final class ProviderPositions
{
    public function __construct(private readonly LedgerCore $client)
    {
    }

    /**
     * Net position against every counterparty (provider), per asset.
     *
     * @return array<string, mixed> ['data' => [...], 'hint'?: string]
     */
    public function get(): array
    {
        return $this->client->request('GET', '/v1/provider-positions');
    }
}
