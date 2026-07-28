import { describe, expect, it } from "vitest";
import { Money } from "../src/money.js";

describe("Money.fromDecimal", () => {
  it("converts decimals to minor units", () => {
    expect(Money.fromDecimal("100.50", "USD", 2)).toEqual({ asset: "USD", amount: "10050" });
    expect(Money.fromDecimal("100", "USD", 2)).toEqual({ asset: "USD", amount: "10000" });
    expect(Money.fromDecimal("0.03", "USD", 2)).toEqual({ asset: "USD", amount: "3" });
    expect(Money.fromDecimal("1234", "JPY", 0)).toEqual({ asset: "JPY", amount: "1234" });
    expect(Money.fromDecimal("0.00000001", "BTC", 8)).toEqual({ asset: "BTC", amount: "1" });
  });

  it("handles negatives and zero", () => {
    expect(Money.fromDecimal("-12.34", "USD", 2)).toEqual({ asset: "USD", amount: "-1234" });
    expect(Money.fromDecimal("-0.00", "USD", 2)).toEqual({ asset: "USD", amount: "0" });
  });

  it("regression: '1.500' with exponent 3 is 1500, never x1000 off", () => {
    expect(Money.fromDecimal("1.500", "KWD", 3)).toEqual({ asset: "KWD", amount: "1500" });
  });

  it("rejects more decimals than the exponent (no silent rounding)", () => {
    expect(() => Money.fromDecimal("1.005", "USD", 2)).toThrow(RangeError);
  });

  it("rejects malformed input", () => {
    for (const bad of ["", "abc", "1.2.3", "1,50", ".", "1e3", "NaN"]) {
      expect(() => Money.fromDecimal(bad, "USD", 2)).toThrow(RangeError);
    }
    expect(() => Money.fromDecimal("1", "USD", -1)).toThrow(RangeError);
  });

  it("survives int64-scale values without float precision loss", () => {
    expect(Money.fromDecimal("92233720368547758.07", "USD", 2)).toEqual({
      asset: "USD",
      amount: "9223372036854775807",
    });
  });
});

describe("Money.toDecimal", () => {
  it("converts minor units back to decimals", () => {
    expect(Money.toDecimal("10050", 2)).toBe("100.50");
    expect(Money.toDecimal("3", 2)).toBe("0.03");
    expect(Money.toDecimal("-1234", 2)).toBe("-12.34");
    expect(Money.toDecimal("1234", 0)).toBe("1234");
    expect(Money.toDecimal({ asset: "USD", amount: "9700" }, 2)).toBe("97.00");
  });

  it("round-trips", () => {
    for (const [dec, exp] of [
      ["100.50", 2],
      ["0.03", 2],
      ["-12.34", 2],
      ["0.00000001", 8],
    ] as const) {
      expect(Money.toDecimal(Money.fromDecimal(dec, "X", exp), exp)).toBe(dec);
    }
  });

  it("rejects malformed minor units", () => {
    expect(() => Money.toDecimal("1.5", 2)).toThrow(RangeError);
    expect(() => Money.toDecimal("abc", 2)).toThrow(RangeError);
  });
});
