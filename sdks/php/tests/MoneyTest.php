<?php

declare(strict_types=1);

namespace LedgerCore\Tests;

use LedgerCore\Money;
use PHPUnit\Framework\TestCase;

final class MoneyTest extends TestCase
{
    public function testFromDecimalConvertsToMinorUnits(): void
    {
        self::assertSame(['asset' => 'USD', 'amount' => '10050'], Money::fromDecimal('100.50', 'USD', 2));
        self::assertSame(['asset' => 'USD', 'amount' => '10000'], Money::fromDecimal('100', 'USD', 2));
        self::assertSame(['asset' => 'USD', 'amount' => '3'], Money::fromDecimal('0.03', 'USD', 2));
        self::assertSame(['asset' => 'JPY', 'amount' => '1234'], Money::fromDecimal('1234', 'JPY', 0));
        self::assertSame(['asset' => 'BTC', 'amount' => '1'], Money::fromDecimal('0.00000001', 'BTC', 8));
    }

    public function testFromDecimalHandlesNegativesAndZero(): void
    {
        self::assertSame(['asset' => 'USD', 'amount' => '-1234'], Money::fromDecimal('-12.34', 'USD', 2));
        self::assertSame(['asset' => 'USD', 'amount' => '0'], Money::fromDecimal('-0.00', 'USD', 2));
    }

    public function testRegressionThreeDecimalsIsNotTimesThousandOff(): void
    {
        // "1.500" with exponent 3 must be exactly 1500 minor units.
        self::assertSame(['asset' => 'KWD', 'amount' => '1500'], Money::fromDecimal('1.500', 'KWD', 3));
    }

    public function testFromDecimalRejectsTooManyDecimals(): void
    {
        $this->expectException(\InvalidArgumentException::class);
        Money::fromDecimal('1.005', 'USD', 2);
    }

    /** @dataProvider malformedDecimals */
    public function testFromDecimalRejectsMalformedInput(string $bad): void
    {
        $this->expectException(\InvalidArgumentException::class);
        Money::fromDecimal($bad, 'USD', 2);
    }

    /** @return iterable<array{string}> */
    public static function malformedDecimals(): iterable
    {
        foreach (['', 'abc', '1.2.3', '1,50', '.', '1e3'] as $bad) {
            yield [$bad];
        }
    }

    public function testInt64ScaleSurvivesWithoutFloatPrecisionLoss(): void
    {
        self::assertSame(
            ['asset' => 'USD', 'amount' => '9223372036854775807'],
            Money::fromDecimal('92233720368547758.07', 'USD', 2),
        );
    }

    public function testToDecimal(): void
    {
        self::assertSame('100.50', Money::toDecimal('10050', 2));
        self::assertSame('0.03', Money::toDecimal('3', 2));
        self::assertSame('-12.34', Money::toDecimal('-1234', 2));
        self::assertSame('1234', Money::toDecimal('1234', 0));
        self::assertSame('97.00', Money::toDecimal(['asset' => 'USD', 'amount' => '9700'], 2));
    }

    public function testRoundTrip(): void
    {
        foreach ([['100.50', 2], ['0.03', 2], ['-12.34', 2], ['0.00000001', 8]] as [$decimal, $exponent]) {
            self::assertSame($decimal, Money::toDecimal(Money::fromDecimal($decimal, 'X', $exponent), $exponent));
        }
    }

    public function testToDecimalRejectsMalformedMinorUnits(): void
    {
        $this->expectException(\InvalidArgumentException::class);
        Money::toDecimal('1.5', 2);
    }
}
