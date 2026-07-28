<?php

declare(strict_types=1);

namespace LedgerCore;

/**
 * Verification of the LedgerCore webhook signature header:
 *
 *   X-LedgerCore-Signature: t=<unix seconds>,v1=<hex hmac-sha256>
 *
 * The MAC input is "<t>." . raw body, keyed with the subscription secret
 * (whsec_...). Mirrors services/webhooks/internal/signature (the reference
 * implementation).
 */
final class Webhook
{
    public const SIGNATURE_HEADER = 'X-LedgerCore-Signature';

    /** Recommended replay-protection window, in seconds. */
    public const DEFAULT_TOLERANCE_SECONDS = 300;

    /**
     * Verifies a webhook delivery. Returns true only when the header carries
     * a fresh timestamp and at least one v1 signature matches the payload.
     *
     *   $ok = Webhook::verifySignature($rawBody, $_SERVER['HTTP_X_LEDGERCORE_SIGNATURE'] ?? null, 'whsec_...');
     *
     * `$payload` must be the exact raw request body as received — do not
     * re-serialize parsed JSON. `$toleranceSeconds` 0 disables the freshness
     * check; `$now` (epoch seconds) is for deterministic tests.
     */
    public static function verifySignature(
        string $payload,
        ?string $header,
        string $secret,
        int $toleranceSeconds = self::DEFAULT_TOLERANCE_SECONDS,
        ?int $now = null,
    ): bool {
        if ($header === null || $header === '' || $secret === '') {
            return false;
        }

        $timestamp = null;
        $candidates = [];
        foreach (explode(',', $header) as $part) {
            $eq = strpos($part, '=');
            if ($eq === false) {
                return false; // malformed header
            }
            $key = trim(substr($part, 0, $eq));
            $value = trim(substr($part, $eq + 1));
            if ($key === 't') {
                if (!preg_match('/^\d+$/', $value)) {
                    return false;
                }
                $timestamp = (int) $value;
            } elseif ($key === 'v1') {
                $candidates[] = strtolower($value);
            }
            // Unknown keys (e.g. v2) are ignored for forward compatibility.
        }
        if ($timestamp === null || $candidates === []) {
            return false;
        }

        if ($toleranceSeconds > 0) {
            $reference = $now ?? time();
            if (abs($reference - $timestamp) > $toleranceSeconds) {
                return false;
            }
        }

        $expected = hash_hmac('sha256', $timestamp . '.' . $payload, $secret);
        $ok = false;
        foreach ($candidates as $candidate) {
            if (hash_equals($expected, $candidate)) {
                $ok = true;
            }
        }

        return $ok;
    }
}
