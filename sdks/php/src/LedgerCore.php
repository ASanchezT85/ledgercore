<?php

declare(strict_types=1);

namespace LedgerCore;

use LedgerCore\Exception\AuthenticationException;
use LedgerCore\Exception\LedgerCoreException;
use LedgerCore\Resources\Accounts;
use LedgerCore\Resources\Ledgers;
use LedgerCore\Resources\ProviderPositions;
use LedgerCore\Resources\Statements;
use LedgerCore\Resources\Transactions;
use LedgerCore\Resources\TrialBalance;
use LedgerCore\Resources\Webhooks;
use LedgerCore\Transport\CurlTransport;
use LedgerCore\Transport\TransportInterface;

/**
 * LedgerCore API client.
 *
 *   $lc = new LedgerCore(['api_key' => 'lk_sandbox_...']);
 *   $ledger = $lc->ledgers->create(['name' => 'main']);
 *
 * Authentication is handled internally: the API key is exchanged for a
 * short-lived (15 min) JWT on first use, cached, renewed before expiry
 * (<60 s left) and re-exchanged once on an unexpected 401.
 */
final class LedgerCore
{
    private const DEFAULT_BASE_URL = 'http://localhost:8080';

    /** Renew the JWT when it has less than this many seconds of validity left. */
    private const REFRESH_SKEW_SECONDS = 60;

    public readonly Ledgers $ledgers;
    public readonly Accounts $accounts;
    public readonly Transactions $transactions;
    public readonly Statements $statements;
    public readonly TrialBalance $trialBalance;
    public readonly ProviderPositions $providerPositions;
    public readonly Webhooks $webhooks;

    private readonly string $apiKey;
    private readonly string $baseUrl;
    private readonly TransportInterface $transport;

    /** @var callable(): int */
    private $clock;

    private ?string $token = null;
    private int $tokenExpiresAt = 0; // epoch seconds

    /**
     * @param array{api_key: string, base_url?: string, timeout?: int, transport?: TransportInterface, clock?: callable(): int} $options
     */
    public function __construct(array $options)
    {
        if (($options['api_key'] ?? '') === '') {
            throw new \InvalidArgumentException('api_key is required');
        }
        $this->apiKey = $options['api_key'];
        $this->baseUrl = rtrim($options['base_url'] ?? self::DEFAULT_BASE_URL, '/');
        $this->transport = $options['transport'] ?? new CurlTransport($options['timeout'] ?? 10);
        $this->clock = $options['clock'] ?? static fn (): int => time();

        $this->ledgers = new Ledgers($this);
        $this->accounts = new Accounts($this);
        $this->transactions = new Transactions($this);
        $this->statements = new Statements($this);
        $this->trialBalance = new TrialBalance($this);
        $this->providerPositions = new ProviderPositions($this);
        $this->webhooks = new Webhooks($this);
    }

    // ---- Auth ---------------------------------------------------------------

    private function getToken(bool $fresh = false): string
    {
        $now = ($this->clock)();
        if (!$fresh && $this->token !== null && $now < $this->tokenExpiresAt - self::REFRESH_SKEW_SECONDS) {
            return $this->token;
        }

        $response = $this->transport->request(
            'POST',
            $this->baseUrl . '/v1/auth/token',
            ['Content-Type' => 'application/json', 'Accept' => 'application/json'],
            json_encode(['api_key' => $this->apiKey], JSON_THROW_ON_ERROR),
        );
        if ($response->status !== 200) {
            $error = $this->toException($response);
            if ($response->status === 401) {
                throw new AuthenticationException(
                    $error->getMessage(),
                    $error->status,
                    $error->errorCode,
                    $error->requestId,
                );
            }
            throw $error;
        }

        /** @var array{access_token: string, expires_in: int} $body */
        $body = json_decode($response->body, true, 512, JSON_THROW_ON_ERROR);
        $this->token = $body['access_token'];
        $this->tokenExpiresAt = ($this->clock)() + (int) $body['expires_in'];

        return $this->token;
    }

    // ---- Transport ----------------------------------------------------------

    private function toException(Transport\TransportResponse $response): LedgerCoreException
    {
        $code = "http_{$response->status}";
        $message = "HTTP {$response->status}";
        $requestId = null;
        $decoded = json_decode($response->body, true);
        if (is_array($decoded) && isset($decoded['error']) && is_array($decoded['error'])) {
            $code = (string) ($decoded['error']['code'] ?? $code);
            $message = (string) ($decoded['error']['message'] ?? $message);
            if (isset($decoded['error']['request_id'])) {
                $requestId = (string) $decoded['error']['request_id'];
            }
        }

        return new LedgerCoreException($message, $response->status, $code, $requestId ?? $response->header('X-Request-Id'));
    }

    /**
     * Performs an authenticated API request. Internal: used by the resource
     * classes; not part of the supported public surface.
     *
     * @param array<string, string|int|null> $query
     * @param array<string, mixed>|null      $body
     *
     * @return array<string, mixed>
     */
    public function request(string $method, string $path, array $query = [], ?array $body = null, bool $freshToken = false): array
    {
        $token = $this->getToken($freshToken);

        $url = $this->baseUrl . $path;
        $filtered = array_filter($query, static fn ($v) => $v !== null);
        if ($filtered !== []) {
            $url .= '?' . http_build_query($filtered);
        }

        $headers = [
            'Authorization' => "Bearer {$token}",
            'Accept' => 'application/json',
        ];
        $encodedBody = null;
        if ($body !== null) {
            $headers['Content-Type'] = 'application/json';
            $encodedBody = json_encode($body === [] ? new \stdClass() : $body, JSON_THROW_ON_ERROR);
        }

        $response = $this->transport->request($method, $url, $headers, $encodedBody);

        if ($response->status === 401 && !$freshToken) {
            // The cached token was rejected (revoked key, clock skew...):
            // force a fresh exchange and retry exactly once.
            return $this->request($method, $path, $query, $body, true);
        }
        if ($response->status < 200 || $response->status >= 300) {
            throw $this->toException($response);
        }
        if ($response->status === 204 || $response->body === '') {
            return [];
        }

        /** @var array<string, mixed> */
        return json_decode($response->body, true, 512, JSON_THROW_ON_ERROR);
    }

    /** Generates an RFC 4122 UUID v4 (used for default idempotency keys). */
    public static function uuid(): string
    {
        $bytes = random_bytes(16);
        $bytes[6] = chr((ord($bytes[6]) & 0x0F) | 0x40);
        $bytes[8] = chr((ord($bytes[8]) & 0x3F) | 0x80);
        $hex = bin2hex($bytes);

        return sprintf(
            '%s-%s-%s-%s-%s',
            substr($hex, 0, 8),
            substr($hex, 8, 4),
            substr($hex, 12, 4),
            substr($hex, 16, 4),
            substr($hex, 20, 12),
        );
    }
}
