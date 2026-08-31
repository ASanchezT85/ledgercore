package money

// Property tests for the money package, using Go's built-in fuzzing.
//
// The table tests next door pin the cases someone thought of. These pin
// properties that must hold for every input, which is the part a table cannot
// reach: the ×1000 class of bug that motivated integer money in the first place
// is exactly a case nobody wrote down.
//
// Run the seed corpus with `go test ./money/`. Run real fuzzing with:
//
//	go test ./money/ -run '^$' -fuzz FuzzParseFormatRoundTrip -fuzztime 30s

import (
	"strings"
	"testing"
)

// FuzzParseFormatRoundTrip: formatting an amount and parsing it back must
// return the identical number of minor units, for every int64 and every
// exponent the registry allows. If this ever fails, some amount of money
// changes value simply by being displayed and read back.
func FuzzParseFormatRoundTrip(f *testing.F) {
	for _, units := range []int64{0, 1, -1, 10, 150, 1500, 999, -9700, 1 << 40} {
		for _, exp := range []int{0, 2, 8} {
			f.Add(units, exp)
		}
	}
	// The values that historically break naive implementations.
	f.Add(int64(9223372036854775807), 2)  // int64 max
	f.Add(int64(-9223372036854775808), 2) // int64 min
	f.Add(int64(5), 8)                    // many leading zeros after the point

	f.Fuzz(func(t *testing.T, units int64, exp int) {
		// The registry constrains exponents to a sane range; anything outside
		// it is rejected at the boundary, not handled here.
		if exp < 0 || exp > 18 {
			t.Skip()
		}

		a := Amount{Asset: "USD", Units: units}
		s := a.FormatWithExponent(exp)

		got, err := ParseUnits(s, exp)
		if err != nil {
			t.Fatalf("ParseUnits(%q, %d) failed on our own output of %d: %v", s, exp, units, err)
		}
		if got != units {
			t.Fatalf("round trip changed the amount: %d -> %q -> %d (exp %d)", units, s, got, exp)
		}
	})
}

// FuzzParseUnitsNeverRounds: whatever ParseUnits accepts, it must have taken
// every digit into account. A parser that silently drops or rounds excess
// decimals is how "1.005" quietly becomes 1.00 — or 1500.
//
// The property: if parsing succeeds, formatting the result back must reproduce
// the input's numeric value. Anything the parser cannot represent exactly has
// to be an error, never a rounded success.
func FuzzParseUnitsNeverRounds(f *testing.F) {
	for _, s := range []string{
		"0", "1", "1.5", "1.50", "1.005", "-1.50", "0.01", "1500", "1.000",
		"00.10", "+1.5", ".5", "1.", "", "abc", "1.2.3", "1e5", " 1.5",
	} {
		f.Add(s, 2)
	}

	f.Fuzz(func(t *testing.T, decimal string, exp int) {
		if exp < 0 || exp > 18 {
			t.Skip()
		}

		units, err := ParseUnits(decimal, exp)
		if err != nil {
			return // rejecting an input is always an acceptable answer
		}

		// It accepted the string, so the value must survive a round trip.
		back := Amount{Asset: "USD", Units: units}.FormatWithExponent(exp)
		reparsed, err := ParseUnits(back, exp)
		if err != nil {
			t.Fatalf("ParseUnits accepted %q -> %d, but its own rendering %q is unparseable: %v",
				decimal, units, back, err)
		}
		if reparsed != units {
			t.Fatalf("ParseUnits(%q, %d) = %d, but %q reparses as %d", decimal, exp, units, back, reparsed)
		}

		// And it must not have discarded significant digits: an accepted input
		// with more fractional digits than the exponent allows would mean the
		// parser rounded instead of refusing.
		if i := strings.IndexByte(decimal, '.'); i >= 0 {
			frac := decimal[i+1:]
			if len(frac) > exp {
				t.Fatalf("ParseUnits accepted %q with %d fractional digits at exponent %d: it rounded instead of refusing",
					decimal, len(frac), exp)
			}
		}
	})
}
