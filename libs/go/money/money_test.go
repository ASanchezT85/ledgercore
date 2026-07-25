package money

import (
	"errors"
	"math"
	"testing"
)

func TestValidateAssetCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{"usd", "USD", false},
		{"stablecoin", "USDC", false},
		{"btc", "BTC", false},
		{"alphanumeric", "POINTS2", false},
		{"max length 12", "ABCDEFGHIJKL", false},
		{"empty", "", true},
		{"too long", "ABCDEFGHIJKLM", true},
		{"lowercase", "usd", true},
		{"symbol", "US$", true},
		{"space", "US D", true},
		{"unicode", "ÚSD", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAssetCode(tt.code)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateAssetCode(%q) error = %v, wantErr %v", tt.code, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidAsset) {
				t.Fatalf("error should wrap ErrInvalidAsset, got %v", err)
			}
		})
	}
}

func TestNew(t *testing.T) {
	a, err := New("USD", 1050)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Asset != "USD" || a.Units != 1050 {
		t.Fatalf("unexpected amount: %+v", a)
	}
	if _, err := New("usd", 1); err == nil {
		t.Fatal("New with invalid asset should fail")
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		a, b    Amount
		want    int64
		wantErr error
	}{
		{"simple", Amount{"USD", 100}, Amount{"USD", 250}, 350, nil},
		{"negative operand", Amount{"USD", 100}, Amount{"USD", -30}, 70, nil},
		{"both negative", Amount{"USD", -100}, Amount{"USD", -30}, -130, nil},
		{"zero", Amount{"USD", 0}, Amount{"USD", 0}, 0, nil},
		{"asset mismatch", Amount{"USD", 1}, Amount{"EUR", 1}, 0, ErrAssetMismatch},
		{"positive overflow", Amount{"USD", math.MaxInt64}, Amount{"USD", 1}, 0, ErrOverflow},
		{"negative overflow", Amount{"USD", math.MinInt64}, Amount{"USD", -1}, 0, ErrOverflow},
		{"max no overflow", Amount{"USD", math.MaxInt64 - 1}, Amount{"USD", 1}, math.MaxInt64, nil},
		{"min no overflow", Amount{"USD", math.MinInt64 + 1}, Amount{"USD", -1}, math.MinInt64, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Add(tt.b)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Add error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			if got.Units != tt.want || got.Asset != tt.a.Asset {
				t.Fatalf("Add = %+v, want units %d", got, tt.want)
			}
		})
	}
}

func TestSub(t *testing.T) {
	tests := []struct {
		name    string
		a, b    Amount
		want    int64
		wantErr error
	}{
		{"simple", Amount{"USD", 350}, Amount{"USD", 100}, 250, nil},
		{"goes negative", Amount{"USD", 100}, Amount{"USD", 350}, -250, nil},
		{"subtract negative", Amount{"USD", 100}, Amount{"USD", -50}, 150, nil},
		{"asset mismatch", Amount{"USD", 1}, Amount{"EUR", 1}, 0, ErrAssetMismatch},
		{"overflow subtracting min", Amount{"USD", 0}, Amount{"USD", math.MinInt64}, 0, ErrOverflow},
		{"min minus min is zero", Amount{"USD", math.MinInt64}, Amount{"USD", math.MinInt64}, 0, nil},
		{"negative overflow", Amount{"USD", math.MinInt64}, Amount{"USD", 1}, 0, ErrOverflow},
		{"positive overflow", Amount{"USD", math.MaxInt64}, Amount{"USD", -1}, 0, ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Sub(tt.b)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Sub error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Sub: %v", err)
			}
			if got.Units != tt.want {
				t.Fatalf("Sub = %+v, want units %d", got, tt.want)
			}
		})
	}
}

func TestNeg(t *testing.T) {
	got, err := Amount{"USD", 150}.Neg()
	if err != nil || got.Units != -150 {
		t.Fatalf("Neg = %+v, %v", got, err)
	}
	got, err = Amount{"USD", -150}.Neg()
	if err != nil || got.Units != 150 {
		t.Fatalf("Neg = %+v, %v", got, err)
	}
	if _, err = (Amount{"USD", math.MinInt64}).Neg(); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Neg(MinInt64) error = %v, want ErrOverflow", err)
	}
}

func TestIsZero(t *testing.T) {
	if !(Amount{"USD", 0}).IsZero() {
		t.Fatal("0 should be zero")
	}
	if (Amount{"USD", 1}).IsZero() || (Amount{"USD", -1}).IsZero() {
		t.Fatal("non-zero amounts reported as zero")
	}
}

func TestFormatWithExponent(t *testing.T) {
	tests := []struct {
		name  string
		units int64
		exp   int
		want  string
	}{
		{"exp0 positive", 150, 0, "150"},
		{"exp0 negative", -150, 0, "-150"},
		{"exp0 zero", 0, 0, "0"},
		{"exp2 typical", 1050, 2, "10.50"},
		{"exp2 sub-unit", 5, 2, "0.05"},
		{"exp2 exactly one unit", 100, 2, "1.00"},
		{"exp2 negative", -1050, 2, "-10.50"},
		{"exp2 negative sub-unit", -5, 2, "-0.05"},
		{"exp2 zero", 0, 2, "0.00"},
		{"exp8 satoshi", 150, 8, "0.00000150"},
		{"exp8 one btc", 100_000_000, 8, "1.00000000"},
		{"exp8 negative", -123_456_789, 8, "-1.23456789"},
		{"max int64 exp2", math.MaxInt64, 2, "92233720368547758.07"},
		{"min int64 exp2", math.MinInt64, 2, "-92233720368547758.08"},
		{"negative exponent treated as 0", 150, -1, "150"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Amount{Asset: "USD", Units: tt.units}.FormatWithExponent(tt.exp)
			if got != tt.want {
				t.Fatalf("FormatWithExponent(%d, %d) = %q, want %q", tt.units, tt.exp, got, tt.want)
			}
		})
	}
}

func TestParseUnits(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		exp     int
		want    int64
		wantErr error
	}{
		// exponent 0
		{"exp0 integer", "150", 0, 150, nil},
		{"exp0 negative", "-150", 0, -150, nil},
		{"exp0 explicit plus", "+7", 0, 7, nil},
		{"exp0 rejects decimals", "1.5", 0, 0, ErrTooManyDigits},
		{"exp0 trailing dot ok", "150.", 0, 150, nil},
		// exponent 2
		{"exp2 typical", "10.50", 2, 1050, nil},
		{"exp2 one decimal padded", "10.5", 2, 1050, nil},
		{"exp2 no decimals", "10", 2, 1000, nil},
		{"exp2 negative", "-10.50", 2, -1050, nil},
		{"exp2 leading dot", ".05", 2, 5, nil},
		{"exp2 zero", "0.00", 2, 0, nil},
		{"exp2 three decimals rejected", "1.005", 2, 0, ErrTooManyDigits},
		// exponent 8
		{"exp8 satoshis", "0.00000150", 8, 150, nil},
		{"exp8 one btc", "1", 8, 100_000_000, nil},
		{"exp8 full precision", "1.23456789", 8, 123_456_789, nil},
		{"exp8 nine decimals rejected", "0.000000001", 8, 0, ErrTooManyDigits},
		// malformed input
		{"empty", "", 2, 0, ErrInvalidDecimal},
		{"just sign", "-", 2, 0, ErrInvalidDecimal},
		{"just dot", ".", 2, 0, ErrInvalidDecimal},
		{"sign and dot", "-.", 2, 0, ErrInvalidDecimal},
		{"letters", "10a.50", 2, 0, ErrInvalidDecimal},
		{"two dots", "1.2.3", 2, 0, ErrInvalidDecimal},
		{"thousands separator", "1,000", 2, 0, ErrInvalidDecimal},
		{"internal space", "1 000", 2, 0, ErrInvalidDecimal},
		{"negative exponent", "1", -1, 0, ErrInvalidDecimal},
		// boundaries
		{"max int64 exp0", "9223372036854775807", 0, math.MaxInt64, nil},
		{"min int64 exp0", "-9223372036854775808", 0, math.MinInt64, nil},
		{"overflow exp0", "9223372036854775808", 0, 0, ErrOverflow},
		{"underflow exp0", "-9223372036854775809", 0, 0, ErrOverflow},
		{"max int64 exp2", "92233720368547758.07", 2, math.MaxInt64, nil},
		{"min int64 exp2", "-92233720368547758.08", 2, math.MinInt64, nil},
		{"overflow exp2", "92233720368547758.08", 2, 0, ErrOverflow},
		{"overflow via padding exp8", "92233720368547758.08", 8, 0, ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUnits(tt.in, tt.exp)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseUnits(%q, %d) error = %v, want %v", tt.in, tt.exp, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUnits(%q, %d): %v", tt.in, tt.exp, err)
			}
			if got != tt.want {
				t.Fatalf("ParseUnits(%q, %d) = %d, want %d", tt.in, tt.exp, got, tt.want)
			}
		})
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	cases := []struct {
		units int64
		exp   int
	}{
		{0, 2}, {1, 2}, {-1, 2}, {1050, 2}, {-1050, 2},
		{150, 0}, {-150, 0},
		{123_456_789, 8}, {-123_456_789, 8}, {1, 8},
		{math.MaxInt64, 2}, {math.MinInt64, 2},
	}
	for _, c := range cases {
		s := Amount{Asset: "USD", Units: c.units}.FormatWithExponent(c.exp)
		got, err := ParseUnits(s, c.exp)
		if err != nil {
			t.Fatalf("round trip ParseUnits(%q, %d): %v", s, c.exp, err)
		}
		if got != c.units {
			t.Fatalf("round trip %d -> %q -> %d (exp %d)", c.units, s, got, c.exp)
		}
	}
}
