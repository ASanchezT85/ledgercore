<?php

/**
 * LedgerCore PHP SDK — quickstart / smoke test against a local stack.
 *
 * Run (from sdks/php, stack up via infra/compose, PHP 8.1+):
 *   php examples/quickstart.php
 *
 * Env:
 *   LEDGERCORE_API_URL  gateway base URL (default http://localhost:8080)
 *   LEDGERCORE_API_KEY  existing key; when unset, a sandbox tenant is created
 */

declare(strict_types=1);

require __DIR__ . '/../vendor/autoload.php';

use LedgerCore\LedgerCore;
use LedgerCore\Money;

$baseUrl = getenv('LEDGERCORE_API_URL') ?: 'http://localhost:8080';

function getApiKey(string $baseUrl): string
{
    $existing = getenv('LEDGERCORE_API_KEY');
    if ($existing !== false && $existing !== '') {
        return $existing;
    }
    // Public self-service sandbox signup (one per email, rate limited).
    $ch = curl_init("{$baseUrl}/v1/sandbox/signups");
    curl_setopt_array($ch, [
        CURLOPT_POST => true,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_HTTPHEADER => ['Content-Type: application/json'],
        CURLOPT_POSTFIELDS => json_encode([
            'email' => sprintf('sdk-php-%d@example.com', time()),
            'company_name' => sprintf('SDK PHP Smoke %d', time()),
        ], JSON_THROW_ON_ERROR),
    ]);
    $body = curl_exec($ch);
    $status = (int) curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
    curl_close($ch);
    if ($body === false || $status !== 201) {
        throw new RuntimeException("sandbox signup failed: {$status} {$body}");
    }
    $signup = json_decode((string) $body, true, 512, JSON_THROW_ON_ERROR);
    echo "signed up sandbox tenant {$signup['slug']} ({$signup['tenant_id']})\n";

    return $signup['api_key'];
}

$lc = new LedgerCore(['api_key' => getApiKey($baseUrl), 'base_url' => $baseUrl]);

// 1. Ledger
$ledger = $lc->ledgers->create(['name' => 'main', 'description' => 'Primary operating ledger']);
echo "ledger:       {$ledger['id']} ({$ledger['name']}, {$ledger['environment']})\n";

// 2. Accounts
$cash = $lc->accounts->create([
    'ledger_id' => $ledger['id'],
    'name' => 'assets:cash',
    'type' => 'asset',
    'normal_balance' => 'DEBIT',
]);
$wallet = $lc->accounts->create([
    'ledger_id' => $ledger['id'],
    'name' => 'customer:cust_42:wallet',
    'type' => 'liability',
    'normal_balance' => 'CREDIT',
]);
$fees = $lc->accounts->create([
    'ledger_id' => $ledger['id'],
    'name' => 'revenue:fees',
    'type' => 'revenue',
    'normal_balance' => 'CREDIT',
]);
echo "accounts:     cash={$cash['id']} wallet={$wallet['id']} fees={$fees['id']}\n";

// 3. Balanced 100 / 97 / 3 deposit (amounts as minor-unit strings via Money)
$txn = $lc->transactions->create([
    'ledger_id' => $ledger['id'],
    'description' => 'Customer USD deposit',
    'postings' => [
        ['account_id' => $cash['id'], 'direction' => 'DEBIT', 'amount' => Money::fromDecimal('100.00', 'USD', 2)],
        ['account_id' => $wallet['id'], 'direction' => 'CREDIT', 'amount' => Money::fromDecimal('97.00', 'USD', 2)],
        ['account_id' => $fees['id'], 'direction' => 'CREDIT', 'amount' => Money::fromDecimal('3.00', 'USD', 2)],
    ],
]);
echo "transaction:  {$txn['id']} status={$txn['status']} idempotency_key={$txn['idempotency_key']}\n";

// 4. Statement of the wallet
$statement = $lc->statements->get($wallet['id']);
$closing = array_map(
    static fn (array $b): string => Money::toDecimal($b['amount'], $b['exponent']) . ' ' . $b['asset'],
    $statement['closing_balance'],
);
$entries = count($statement['entries']);
echo "statement:    {$entries} " . ($entries === 1 ? 'entry' : 'entries')
    . ', closing balance ' . (implode(', ', $closing) ?: '0') . "\n";

// 5. Trial balance must close
$trial = $lc->trialBalance->get($ledger['id']);
foreach ($trial['totals'] as $total) {
    $balanced = $total['balanced'] ? 'true' : 'false';
    echo "trialbalance: {$total['asset']} debits={$total['debits']} credits={$total['credits']} balanced={$balanced}\n";
    if (!$total['balanced']) {
        throw new RuntimeException('trial balance does not close');
    }
}

// 6. Pagination: list accounts with a small limit and follow next_cursor to null
$cursor = null;
$pages = 0;
$seen = 0;
do {
    $page = $lc->accounts->list(ledgerId: $ledger['id'], limit: 2, cursor: $cursor);
    ++$pages;
    $seen += count($page['data']);
    $next = $page['next_cursor'] ?? null;
    echo "pagination:   page {$pages}: " . count($page['data']) . ' account(s), next_cursor=' . ($next ?? 'null') . "\n";
    $cursor = $next;
} while ($cursor !== null);
if ($seen !== 3) {
    throw new RuntimeException("pagination saw {$seen} accounts, expected 3");
}

echo "OK: ledger balances\n";
