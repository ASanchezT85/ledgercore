// Package money provides integer-based monetary amounts in minor units.
//
// Amounts are always stored as int64 minor units (e.g. cents, satoshis)
// together with an asset code. The exponent that maps minor units to the
// display representation lives in the asset registry of each service and is
// only needed at the formatting / parsing boundary. Floats are never used.
package money

import (
	"errors"
	"fmt"
	"strings"
)

// Errors returned by this package.
var (
	ErrOverflow       = errors.New("money: int64 overflow")
	ErrAssetMismatch  = errors.New("money: asset codes do not match")
	ErrInvalidAsset   = errors.New("money: invalid asset code")
	ErrInvalidDecimal = errors.New("money: invalid decimal string")
	ErrTooManyDigits  = errors.New("money: more decimal places than the asset exponent allows")
)

// Amount is a monetary value expressed in minor units of an asset.
// Units is the signed count of minor units (e.g. 1050 == USD 10.50 when the
// exponent is 2). The zero value is an invalid amount (empty asset).
type Amount struct {
	Asset string
	Units int64
}

// New builds an Amount after validating the asset code.
func New(asset string, units int64) (Amount, error) {
	if err := ValidateAssetCode(asset); err != nil {
		return Amount{}, err
	}
	return Amount{Asset: asset, Units: units}, nil
}

// ValidateAssetCode checks that the code is 1..12 chars of uppercase
// letters or digits (e.g. "USD", "USDC", "BTC", "POINTS2").
func ValidateAssetCode(code string) error {
	if len(code) == 0 || len(code) > 12 {
		return fmt.Errorf("%w: %q must be 1-12 characters", ErrInvalidAsset, code)
	}
	for _, r := range code {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return fmt.Errorf("%w: %q must contain only A-Z or 0-9", ErrInvalidAsset, code)
		}
	}
	return nil
}

// Add returns a + b. It fails if the assets differ or the sum overflows int64.
func (a Amount) Add(b Amount) (Amount, error) {
	if a.Asset != b.Asset {
		return Amount{}, fmt.Errorf("%w: %q vs %q", ErrAssetMismatch, a.Asset, b.Asset)
	}
	sum := a.Units + b.Units
	// Overflow iff both operands share a sign and the result flipped it.
	if (a.Units > 0 && b.Units > 0 && sum < 0) || (a.Units < 0 && b.Units < 0 && sum >= 0) {
		return Amount{}, fmt.Errorf("%w: %d + %d", ErrOverflow, a.Units, b.Units)
	}
	return Amount{Asset: a.Asset, Units: sum}, nil
}

// Sub returns a - b. It fails if the assets differ or the result overflows.
func (a Amount) Sub(b Amount) (Amount, error) {
	if a.Asset != b.Asset {
		return Amount{}, fmt.Errorf("%w: %q vs %q", ErrAssetMismatch, a.Asset, b.Asset)
	}
	if b.Units == minInt64 {
		// -minInt64 is not representable; handle explicitly.
		if a.Units >= 0 {
			return Amount{}, fmt.Errorf("%w: %d - %d", ErrOverflow, a.Units, b.Units)
		}
		return Amount{Asset: a.Asset, Units: a.Units - b.Units}, nil
	}
	neg := Amount{Asset: b.Asset, Units: -b.Units}
	res, err := a.Add(neg)
	if err != nil {
		return Amount{}, fmt.Errorf("%w: %d - %d", ErrOverflow, a.Units, b.Units)
	}
	return res, nil
}

const minInt64 = -1 << 63

// Neg returns the amount with the opposite sign.
// It fails only for math.MinInt64, whose negation is not representable.
func (a Amount) Neg() (Amount, error) {
	if a.Units == minInt64 {
		return Amount{}, fmt.Errorf("%w: -(%d)", ErrOverflow, a.Units)
	}
	return Amount{Asset: a.Asset, Units: -a.Units}, nil
}

// IsZero reports whether the amount is exactly zero units.
func (a Amount) IsZero() bool { return a.Units == 0 }

// FormatWithExponent renders the amount as a decimal string using the given
// exponent (number of decimal places of the asset). Examples with Units=150:
// exp 0 -> "150", exp 2 -> "1.50", exp 8 -> "0.00000150".
func (a Amount) FormatWithExponent(exp int) string {
	if exp <= 0 {
		return fmt.Sprintf("%d", a.Units)
	}
	u := a.Units
	sign := ""
	var abs uint64
	if u < 0 {
		sign = "-"
		abs = uint64(-(u + 1)) + 1 // safe for MinInt64
	} else {
		abs = uint64(u)
	}
	digits := fmt.Sprintf("%d", abs)
	if len(digits) <= exp {
		digits = strings.Repeat("0", exp-len(digits)+1) + digits
	}
	cut := len(digits) - exp
	return sign + digits[:cut] + "." + digits[cut:]
}

// ParseUnits converts a decimal string (e.g. "10.50") into minor units for an
// asset with the given exponent. It rejects: empty input, malformed numbers,
// more decimal places than the exponent allows, and values that overflow
// int64. A leading '-' or '+' is accepted; group separators are not.
func ParseUnits(decimal string, exp int) (int64, error) {
	if exp < 0 {
		return 0, fmt.Errorf("%w: negative exponent %d", ErrInvalidDecimal, exp)
	}
	s := decimal
	if s == "" {
		return 0, fmt.Errorf("%w: empty string", ErrInvalidDecimal)
	}
	negative := false
	switch s[0] {
	case '-':
		negative = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	if s == "" || s == "." {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDecimal, decimal)
	}

	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
		if strings.IndexByte(fracPart, '.') >= 0 {
			return 0, fmt.Errorf("%w: %q has multiple decimal points", ErrInvalidDecimal, decimal)
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	for _, part := range [2]string{intPart, fracPart} {
		for i := 0; i < len(part); i++ {
			if part[i] < '0' || part[i] > '9' {
				return 0, fmt.Errorf("%w: %q contains non-digit characters", ErrInvalidDecimal, decimal)
			}
		}
	}
	if len(fracPart) > exp {
		return 0, fmt.Errorf("%w: %q has %d decimals, asset exponent is %d",
			ErrTooManyDigits, decimal, len(fracPart), exp)
	}
	// Right-pad the fraction to exactly exp digits.
	fracPart += strings.Repeat("0", exp-len(fracPart))

	var units int64
	for i := 0; i < len(intPart)+len(fracPart); i++ {
		var d byte
		if i < len(intPart) {
			d = intPart[i] - '0'
		} else {
			d = fracPart[i-len(intPart)] - '0'
		}
		if units > (1<<63-1)/10 {
			return 0, fmt.Errorf("%w: %q", ErrOverflow, decimal)
		}
		units *= 10
		if units > (1<<63-1)-int64(d) {
			// The only in-range value past MaxInt64 is MinInt64 when negative.
			if negative && units == (1<<63-1)-int64(d)+1 && i == len(intPart)+len(fracPart)-1 {
				return minInt64, nil
			}
			return 0, fmt.Errorf("%w: %q", ErrOverflow, decimal)
		}
		units += int64(d)
	}
	if negative {
		units = -units
	}
	return units, nil
}
