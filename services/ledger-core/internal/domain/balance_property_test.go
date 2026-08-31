package domain

// Property tests for the double-entry check, using Go's built-in fuzzing.
//
// Run the seed corpus with `go test ./internal/domain/`. Run real fuzzing with:
//
//	go test ./internal/domain/ -run '^$' -fuzz FuzzValidateBalancedIsOrderIndependent -fuzztime 30s

import (
	"errors"
	"testing"

	"github.com/ASanchezT85/ledgercore/libs/go/money"
)

// FuzzValidateBalancedIsOrderIndependent: whether a set of postings is ACCEPTED
// must not depend on the order they arrive in.
//
// This is not as obvious as it looks. ValidateBalanced accumulates a running
// sum per (asset, direction) with an overflow-checked Add, and a running sum is
// exactly the kind of thing that overflows in one order and not another —
// [max, -max, max] overflows first if you start from the left, never if you
// reorder it. What rules that out here is the requirement that every posting
// amount be strictly positive: each side's sum is monotonically increasing, so
// if any ordering overflows, all of them do.
//
// The property pins that reasoning in place. If someone ever relaxes the
// positive-amount rule — to allow a "negative debit", say — this test starts
// failing, which is the correct moment to find out.
func FuzzValidateBalancedIsOrderIndependent(f *testing.F) {
	// amounts are packed as four int64s; the seeds cover balanced, unbalanced,
	// mixed-asset and near-overflow shapes.
	f.Add(int64(10000), int64(9700), int64(300), int64(0), uint8(0))
	f.Add(int64(10000), int64(5000), int64(0), int64(0), uint8(0))
	f.Add(int64(1), int64(1), int64(0), int64(0), uint8(1))
	f.Add(int64(9223372036854775807), int64(9223372036854775807), int64(1), int64(1), uint8(0))
	f.Add(int64(-5), int64(5), int64(0), int64(0), uint8(0))

	f.Fuzz(func(t *testing.T, d1, c1, c2, c3 int64, assetMix uint8) {
		asset := func(i int) string {
			// assetMix decides whether the credits share the debit's asset.
			if assetMix&(1<<uint(i)) != 0 {
				return "EUR"
			}
			return "USD"
		}
		mk := func(dir Direction, a string, units int64) Posting {
			return Posting{Direction: dir, Amount: money.Amount{Asset: a, Units: units}}
		}

		postings := []Posting{
			mk(DirectionDebit, "USD", d1),
			mk(DirectionCredit, asset(0), c1),
			mk(DirectionCredit, asset(1), c2),
			mk(DirectionCredit, asset(2), c3),
		}

		base := verdict(ValidateBalanced(postings))

		// Every permutation of the same four postings must agree on ACCEPTANCE.
		//
		// Only acceptance, not the specific error. ValidateBalanced returns on
		// the first problem it meets, so a set that is wrong in two independent
		// ways — say a negative amount AND a sum that overflows — reports
		// whichever it reaches first, and that does depend on order. Both
		// orderings still reject it, which is the part that protects the
		// ledger. Fuzzing found exactly this case:
		//
		//   DEBIT USD max, CREDIT USD max, CREDIT USD -73, CREDIT USD 32
		//     -> "non-positive" in the given order (-73 comes first)
		//     -> "overflow"     when 32 is summed before -73 is seen
		//
		// Asserting the stronger property would be asserting something the API
		// has never promised.
		for _, perm := range permutations4() {
			shuffled := make([]Posting, 4)
			for i, j := range perm {
				shuffled[i] = postings[j]
			}
			if got := verdict(ValidateBalanced(shuffled)); accepted(got) != accepted(base) {
				t.Fatalf("order changed acceptance: %v gives %q, permutation %v gives %q",
					summarise(postings), base, perm, got)
			}
		}
	})
}

// accepted reports whether the verdict means the transaction was allowed.
func accepted(v string) bool { return v == "valid" }

// verdict collapses an error into a comparable class. The message carries a
// posting index, which legitimately differs between orderings; the class does
// not.
func verdict(err error) string {
	switch {
	case err == nil:
		return "valid"
	case errors.Is(err, ErrUnbalanced):
		return "unbalanced"
	case errors.Is(err, ErrNonPositiveAmount):
		return "non-positive"
	case errors.Is(err, ErrTooFewPostings):
		return "too-few"
	case errors.Is(err, money.ErrOverflow):
		return "overflow"
	case errors.Is(err, money.ErrInvalidAsset):
		return "invalid-asset"
	default:
		return "other: " + err.Error()
	}
}

func summarise(ps []Posting) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p.Direction) + " " + p.Amount.Asset + " " + p.Amount.FormatWithExponent(0)
	}
	return out
}

// permutations4 returns every ordering of four indices.
func permutations4() [][4]int {
	var out [][4]int
	idx := []int{0, 1, 2, 3}
	var rec func(k int)
	rec = func(k int) {
		if k == len(idx) {
			out = append(out, [4]int{idx[0], idx[1], idx[2], idx[3]})
			return
		}
		for i := k; i < len(idx); i++ {
			idx[k], idx[i] = idx[i], idx[k]
			rec(k + 1)
			idx[k], idx[i] = idx[i], idx[k]
		}
	}
	rec(0)
	return out
}
