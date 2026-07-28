<?php

declare(strict_types=1);

namespace LedgerCore\Resources;

use LedgerCore\LedgerCore;
use LedgerCore\Webhook;

final class Webhooks
{
    public function __construct(private readonly LedgerCore $client)
    {
    }

    /**
     * @param array{url: string, event_types: list<string>} $params
     *
     * @return array<string, mixed> Subscription including the one-time `secret`.
     */
    public function create(array $params): array
    {
        return $this->client->request('POST', '/v1/webhook-subscriptions', [], $params);
    }

    /**
     * @return array<string, mixed> Page: ['data' => Subscription[], 'next_cursor' => ?string]
     */
    public function list(?int $limit = null, ?string $cursor = null): array
    {
        return $this->client->request('GET', '/v1/webhook-subscriptions', [
            'limit' => $limit,
            'cursor' => $cursor,
        ]);
    }

    /**
     * Iterates every subscription, following next_cursor automatically.
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

    /** @return array<string, mixed> */
    public function get(string $id): array
    {
        return $this->client->request('GET', '/v1/webhook-subscriptions/' . rawurlencode($id));
    }

    /**
     * Partial update; deactivate with ['active' => false] (there is no delete).
     *
     * @param array{url?: string, event_types?: list<string>, active?: bool} $params
     *
     * @return array<string, mixed>
     */
    public function update(string $id, array $params): array
    {
        return $this->client->request('PATCH', '/v1/webhook-subscriptions/' . rawurlencode($id), [], $params);
    }

    /**
     * Rotates the signing secret. The previous secret keeps verifying until
     * `previous_secret_expires_at` (24h grace): deliveries carry two `v1`
     * entries in that window, and verifySignature accepts either.
     *
     * @return array<string, mixed> ['id', 'secret' (shown once), 'previous_secret_expires_at']
     */
    public function rotateSecret(string $id): array
    {
        return $this->client->request('POST', '/v1/webhook-subscriptions/' . rawurlencode($id) . '/rotate-secret');
    }

    /**
     * @return array<string, mixed> Page: ['data' => Delivery[], 'next_cursor' => ?string]
     */
    public function deliveries(?string $subscriptionId = null, ?string $status = null, ?int $limit = null, ?string $cursor = null): array
    {
        return $this->client->request('GET', '/v1/webhook-deliveries', [
            'subscription_id' => $subscriptionId,
            'status' => $status,
            'limit' => $limit,
            'cursor' => $cursor,
        ]);
    }

    /**
     * Iterates every matching delivery, following next_cursor automatically.
     *
     * @return \Generator<int, array<string, mixed>>
     */
    public function deliveriesAll(?string $subscriptionId = null, ?string $status = null, ?int $limit = null): \Generator
    {
        $cursor = null;
        do {
            $page = $this->deliveries($subscriptionId, $status, $limit, $cursor);
            yield from $page['data'];
            $cursor = $page['next_cursor'] ?? null;
        } while ($cursor !== null);
    }

    /** @return array<string, mixed> The delivery, re-enqueued as pending. */
    public function retryDelivery(string $id): array
    {
        return $this->client->request('POST', '/v1/webhook-deliveries/' . rawurlencode($id) . '/retry');
    }

    /**
     * Verifies an incoming webhook signature (X-LedgerCore-Signature).
     * Static helper: needs no API key and never talks to the network.
     */
    public static function verifySignature(
        string $payload,
        ?string $header,
        string $secret,
        int $toleranceSeconds = Webhook::DEFAULT_TOLERANCE_SECONDS,
        ?int $now = null,
    ): bool {
        return Webhook::verifySignature($payload, $header, $secret, $toleranceSeconds, $now);
    }
}
