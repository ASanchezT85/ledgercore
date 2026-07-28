<?php

declare(strict_types=1);

namespace LedgerCore;

/**
 * Helpers to build API `Money` values (['asset' => ..., 'amount' => ...])
 * from decimal strings and back, without ever touching floats. Amounts are
 * int64 minor units, string-encoded, exactly as the API expects.
 */
final class Money
{
    /**
     * Converts a decimal string into a Money value in minor units.
     *
     *   Money::fromDecimal('100.50', 'USD', 2) // ['asset' => 'USD', 'amount' => '10050']
     *
     * Throws on malformed input or when the decimal has more places than
     * `$exponent` (no silent rounding of money).
     *
     * @return array{asset: string, amount: string}
     */
    public static function fromDecimal(string $decimal, string $asset, int $exponent = 2): array
    {
        if ($exponent < 0 || $exponent > 18) {
            throw new \InvalidArgumentException("exponent must be between 0 and 18, got {$exponent}");
        }
        if (!preg_match('/^(-?)(\d+)(?:\.(\d+))?$/', trim($decimal), $m)) {
            throw new \InvalidArgumentException("invalid decimal amount: \"{$decimal}\"");
        }
        [, $sign, $whole] = $m;
        $fraction = $m[3] ?? '';
        if (strlen($fraction) > $exponent) {
            throw new \InvalidArgumentException(
                "amount {$decimal} has " . strlen($fraction) . " decimal places but {$asset} uses {$exponent}"
            );
        }
        $minor = ltrim($whole . str_pad($fraction, $exponent, '0'), '0');
        if ($minor === '') {
            $minor = '0';
        }
        $amount = ($sign === '-' && $minor !== '0') ? '-' . $minor : $minor;

        return ['asset' => $asset, 'amount' => $amount];
    }

    /**
     * Converts minor units back into a decimal string.
     *
     *   Money::toDecimal('10050', 2)                              // "100.50"
     *   Money::toDecimal(['asset' => 'USD', 'amount' => '10050']) // "100.50"
     *
     * @param string|array{asset: string, amount: string} $amount
     */
    public static function toDecimal(string|array $amount, int $exponent = 2): string
    {
        $raw = is_array($amount) ? $amount['amount'] : $amount;
        if (!preg_match('/^-?\d+$/', $raw)) {
            throw new \InvalidArgumentException("invalid minor-units amount: \"{$raw}\"");
        }
        if ($exponent < 0 || $exponent > 18) {
            throw new \InvalidArgumentException("exponent must be between 0 and 18, got {$exponent}");
        }
        $negative = str_starts_with($raw, '-');
        $digits = str_pad($negative ? substr($raw, 1) : $raw, $exponent + 1, '0', STR_PAD_LEFT);
        $whole = substr($digits, 0, strlen($digits) - $exponent);
        $fraction = $exponent > 0 ? '.' . substr($digits, -$exponent) : '';

        return ($negative ? '-' : '') . $whole . $fraction;
    }
}
