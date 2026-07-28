<?php

declare(strict_types=1);

namespace LedgerCore\Tests;

use LedgerCore\Webhook;
use PHPUnit\Framework\TestCase;

final class WebhookTest extends TestCase
{
    private const SECRET = 'whsec_4f3e2d1c0b9a';
    private const T = 1753000000;

    private string $body;

    protected function setUp(): void
    {
        $this->body = json_encode(['event_id' => '01890a5d', 'type' => 'ledger.transaction.posted'], JSON_THROW_ON_ERROR);
    }

    private static function sign(string $secret, int $t, string $body): string
    {
        return hash_hmac('sha256', $t . '.' . $body, $secret);
    }

    public function testAcceptsValidSignature(): void
    {
        $header = 't=' . self::T . ',v1=' . self::sign(self::SECRET, self::T, $this->body);
        self::assertTrue(Webhook::verifySignature($this->body, $header, self::SECRET, now: self::T));
    }

    public function testAcceptsAnyMatchingCandidateDuringRotation(): void
    {
        $header = 't=' . self::T
            . ',v1=' . self::sign('whsec_old', self::T, $this->body)
            . ',v1=' . self::sign(self::SECRET, self::T, $this->body);
        self::assertTrue(Webhook::verifySignature($this->body, $header, self::SECRET, now: self::T));
    }

    public function testRejectsTamperedBody(): void
    {
        $header = 't=' . self::T . ',v1=' . self::sign(self::SECRET, self::T, $this->body);
        self::assertFalse(Webhook::verifySignature($this->body . 'x', $header, self::SECRET, now: self::T));
    }

    public function testRejectsTamperedTimestamp(): void
    {
        $header = 't=' . (self::T + 1) . ',v1=' . self::sign(self::SECRET, self::T, $this->body);
        self::assertFalse(Webhook::verifySignature($this->body, $header, self::SECRET, now: self::T));
    }

    public function testRejectsWrongSecret(): void
    {
        $header = 't=' . self::T . ',v1=' . self::sign(self::SECRET, self::T, $this->body);
        self::assertFalse(Webhook::verifySignature($this->body, $header, 'whsec_other', now: self::T));
    }

    public function testRejectsStaleTimestampOutsideTolerance(): void
    {
        $header = 't=' . self::T . ',v1=' . self::sign(self::SECRET, self::T, $this->body);
        self::assertFalse(Webhook::verifySignature($this->body, $header, self::SECRET, now: self::T + 301));
        self::assertTrue(Webhook::verifySignature($this->body, $header, self::SECRET, toleranceSeconds: 0, now: self::T + 301));
    }

    public function testRejectsMalformedOrMissingHeaders(): void
    {
        self::assertFalse(Webhook::verifySignature($this->body, null, self::SECRET));
        self::assertFalse(Webhook::verifySignature($this->body, '', self::SECRET));
        self::assertFalse(Webhook::verifySignature($this->body, 'garbage', self::SECRET));
        self::assertFalse(Webhook::verifySignature($this->body, 'v1=' . self::sign(self::SECRET, self::T, $this->body), self::SECRET));
        self::assertFalse(Webhook::verifySignature($this->body, 't=' . self::T, self::SECRET, now: self::T));
    }
}
