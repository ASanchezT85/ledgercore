import type { MoneyValue } from "./types.js";

/**
 * Helpers to build API `Money` values ({ asset, amount }) from decimal
 * strings and back, without ever touching floats. Amounts are int64 minor
 * units, string-encoded, exactly as the API expects.
 */
export const Money = {
  /**
   * Converts a decimal string into a Money value in minor units.
   *
   *   Money.fromDecimal("100.50", "USD", 2) // { asset: "USD", amount: "10050" }
   *
   * Throws on malformed input or when the decimal has more places than
   * `exponent` (no silent rounding of money).
   */
  fromDecimal(decimal: string, asset: string, exponent = 2): MoneyValue {
    if (!Number.isInteger(exponent) || exponent < 0 || exponent > 18) {
      throw new RangeError(`exponent must be an integer between 0 and 18, got ${exponent}`);
    }
    const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(decimal.trim());
    if (!match) {
      throw new RangeError(`invalid decimal amount: ${JSON.stringify(decimal)}`);
    }
    const [, sign, whole, fraction = ""] = match;
    if (fraction.length > exponent) {
      throw new RangeError(
        `amount ${decimal} has ${fraction.length} decimal places but ${asset} uses ${exponent}`,
      );
    }
    const minor = BigInt(whole + fraction.padEnd(exponent, "0"));
    const amount = (sign === "-" && minor !== 0n ? -minor : minor).toString();
    return { asset, amount };
  },

  /**
   * Converts minor units back into a decimal string.
   *
   *   Money.toDecimal("10050", 2) // "100.50"
   *   Money.toDecimal({ asset: "USD", amount: "10050" }, 2) // "100.50"
   */
  toDecimal(amount: string | MoneyValue, exponent = 2): string {
    const raw = typeof amount === "string" ? amount : amount.amount;
    if (!/^-?\d+$/.test(raw)) {
      throw new RangeError(`invalid minor-units amount: ${JSON.stringify(raw)}`);
    }
    if (!Number.isInteger(exponent) || exponent < 0 || exponent > 18) {
      throw new RangeError(`exponent must be an integer between 0 and 18, got ${exponent}`);
    }
    const negative = raw.startsWith("-");
    const digits = (negative ? raw.slice(1) : raw).padStart(exponent + 1, "0");
    const whole = digits.slice(0, digits.length - exponent);
    const fraction = exponent > 0 ? `.${digits.slice(digits.length - exponent)}` : "";
    return `${negative ? "-" : ""}${whole}${fraction}`;
  },
};
