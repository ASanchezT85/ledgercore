<?php

declare(strict_types=1);

namespace LedgerCore\Tests;

use LedgerCore\Exception\AuthenticationException;
use LedgerCore\Exception\LedgerCoreException;
use LedgerCore\LedgerCore;
use PHPUnit\Framework\TestCase;

final class LedgerCoreTest extends TestCase
{
    private const KEY = 'lk_sandbox_test';
    private const BASE = 'http://api.test';

    private FakeTransport $transport;
    private int $now = 1_753_000_000;

    protected function setUp(): void
    {
        $this->transport = new FakeTransport();
    }

    private function client(): LedgerCore
    {
        return new LedgerCore([
            'api_key' => self::KEY,
            'base_url' => self::BASE,
            'transport' => $this->transport,
            'clock' => fn (): int => $this->now,
        ]);
    }

    public function testExchangesApiKeyOnceAndCachesJwt(): void
    {
        $this->transport->queueToken('jwt-1');
        $this->transport->queue(200, ['data' => []]);
        $this->transport->queue(200, ['data' => []]);

        $lc = $this->client();
        $lc->ledgers->list();
        $lc->ledgers->list();

        self::assertCount(3, $this->transport->calls);
        self::assertSame(self::BASE . '/v1/auth/token', $this->transport->calls[0]['url']);
        self::assertSame(['api_key' => self::KEY], $this->transport->calls[0]['body']);
        self::assertSame('Bearer jwt-1', $this->transport->calls[1]['headers']['Authorization']);
        self::assertSame('Bearer jwt-1', $this->transport->calls[2]['headers']['Authorization']);
    }

    public function testRenewsTokenWhenUnderSixtySecondsRemain(): void
    {
        $this->transport->queueToken('jwt-1', 900);
        $this->transport->queue(200, ['data' => []]);
        $this->transport->queueToken('jwt-2', 900);
        $this->transport->queue(200, ['data' => []]);

        $lc = $this->client();
        $lc->ledgers->list();
        $this->now += 841; // 59 s of validity left -> under the 60 s skew
        $lc->ledgers->list();

        self::assertSame(self::BASE . '/v1/auth/token', $this->transport->calls[2]['url']);
        self::assertSame('Bearer jwt-2', $this->transport->calls[3]['headers']['Authorization']);
    }

    public function testRetriesExactlyOnceWithFreshTokenOn401(): void
    {
        $this->transport->queueToken('jwt-1');
        $this->transport->queue(401, ['error' => ['code' => 'unauthorized', 'message' => 'expired']]);
        $this->transport->queueToken('jwt-2');
        $this->transport->queue(200, ['data' => [['id' => 'l1']]]);

        $page = $this->client()->ledgers->list();

        self::assertCount(1, $page['data']);
        self::assertSame('Bearer jwt-2', $this->transport->calls[3]['headers']['Authorization']);
    }

    public function testDoesNotLoopOnRepeated401s(): void
    {
        $this->transport->queueToken('jwt-1');
        $this->transport->queue(401, ['error' => ['code' => 'unauthorized', 'message' => 'nope']]);
        $this->transport->queueToken('jwt-2');
        $this->transport->queue(401, ['error' => ['code' => 'unauthorized', 'message' => 'nope']]);

        try {
            $this->client()->ledgers->list();
            self::fail('expected LedgerCoreException');
        } catch (LedgerCoreException $e) {
            self::assertSame(401, $e->status);
            self::assertSame('unauthorized', $e->errorCode);
        }
        self::assertCount(4, $this->transport->calls);
    }

    public function testRaisesAuthenticationExceptionWhenKeyIsRejected(): void
    {
        $this->transport->queue(401, ['error' => ['code' => 'unauthorized', 'message' => 'bad key']]);

        $this->expectException(AuthenticationException::class);
        $this->client()->ledgers->list();
    }

    public function testSendsCallerIdempotencyKeyAsIs(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(201, ['id' => 't1']);

        $this->client()->transactions->create([
            'ledger_id' => 'l1',
            'idempotency_key' => 'dep-2026-0001',
            'postings' => [],
        ]);

        self::assertSame('dep-2026-0001', $this->transport->calls[1]['body']['idempotency_key']);
    }

    public function testGeneratesUuidV4WhenNoIdempotencyKeyGiven(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(201, ['id' => 't1']);

        $this->client()->transactions->create(['ledger_id' => 'l1', 'postings' => []]);

        $sent = $this->transport->calls[1]['body']['idempotency_key'];
        self::assertMatchesRegularExpression(
            '/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/',
            $sent,
        );
    }

    public function testSurfacesTypedErrorWithStatusCodeAndRequestIdFromErrorBody(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(
            422,
            ['error' => ['code' => 'unbalanced_transaction', 'message' => 'debits != credits', 'request_id' => 'req-42']],
        );

        try {
            $this->client()->transactions->create(['ledger_id' => 'l1', 'postings' => []]);
            self::fail('expected LedgerCoreException');
        } catch (LedgerCoreException $e) {
            self::assertSame(422, $e->status);
            self::assertSame('unbalanced_transaction', $e->errorCode);
            self::assertSame('debits != credits', $e->getMessage());
            self::assertSame('req-42', $e->requestId);
        }
    }

    public function testFallsBackToRequestIdHeaderWhenBodyLacksIt(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(
            503,
            ['error' => ['code' => 'service_unavailable', 'message' => 'down']],
            ['x-request-id' => 'req-h'],
        );

        try {
            $this->client()->ledgers->list();
            self::fail('expected LedgerCoreException');
        } catch (LedgerCoreException $e) {
            self::assertSame('service_unavailable', $e->errorCode);
            self::assertSame('req-h', $e->requestId);
        }
    }

    public function testSurfacesInvalidCursorForMalformedPaginationCursor(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(
            400,
            ['error' => ['code' => 'invalid_cursor', 'message' => 'malformed cursor', 'request_id' => 'req-c']],
        );

        try {
            $this->client()->ledgers->list(cursor: 'garbage');
            self::fail('expected LedgerCoreException');
        } catch (LedgerCoreException $e) {
            self::assertSame(400, $e->status);
            self::assertSame('invalid_cursor', $e->errorCode);
            self::assertSame('req-c', $e->requestId);
        }
    }

    public function testWebhookListPassesLimitAndCursor(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(200, ['data' => [['id' => 's1']], 'next_cursor' => 'c2']);

        $page = $this->client()->webhooks->list(limit: 1, cursor: 'c1');

        self::assertSame(self::BASE . '/v1/webhook-subscriptions?limit=1&cursor=c1', $this->transport->calls[1]['url']);
        self::assertSame('c2', $page['next_cursor']);
    }

    public function testListAllFollowsNextCursorUntilNull(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(200, ['data' => [['id' => 'l1'], ['id' => 'l2']], 'next_cursor' => 'c2']);
        $this->transport->queue(200, ['data' => [['id' => 'l3']], 'next_cursor' => null]);

        $ids = [];
        foreach ($this->client()->ledgers->listAll(limit: 2) as $ledger) {
            $ids[] = $ledger['id'];
        }

        self::assertSame(['l1', 'l2', 'l3'], $ids);
        self::assertSame(self::BASE . '/v1/ledgers?limit=2', $this->transport->calls[1]['url']);
        self::assertSame(self::BASE . '/v1/ledgers?limit=2&cursor=c2', $this->transport->calls[2]['url']);
    }

    public function testRotateSecretExposesPreviousSecretExpiresAt(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(200, [
            'id' => 'ws1',
            'secret' => 'whsec_new',
            'previous_secret_expires_at' => '2026-07-29T12:00:00Z',
        ]);

        $rotated = $this->client()->webhooks->rotateSecret('ws1');

        self::assertSame('whsec_new', $rotated['secret']);
        self::assertSame('2026-07-29T12:00:00Z', $rotated['previous_secret_expires_at']);
    }

    public function testBuildsQueryStringsFromParams(): void
    {
        $this->transport->queueToken();
        $this->transport->queue(200, ['ledger_id' => 'l1', 'as_of' => 'x', 'rows' => [], 'totals' => []]);

        $this->client()->trialBalance->get('l1', '2026-07-01T00:00:00Z');

        self::assertSame(
            self::BASE . '/v1/trial-balance?ledger_id=l1&as_of=2026-07-01T00%3A00%3A00Z',
            $this->transport->calls[1]['url'],
        );
    }
}
