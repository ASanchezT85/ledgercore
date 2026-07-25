package domain

import (
	"testing"

	"github.com/google/uuid"
)

func ext(ref string, amount int64, asset string) ExternalTransaction {
	return ExternalTransaction{ID: uuid.New(), ExternalRef: ref, Amount: amount, Asset: asset}
}

func entry(ref string, amount int64, asset string) MirrorEntry {
	return MirrorEntry{ID: uuid.New(), Reference: ref, Amount: amount, Asset: asset, Direction: "DEBIT"}
}

func TestMatchNaiveExactMatch(t *testing.T) {
	e := ext("dep-001", 1050, "USD")
	m := entry("dep-001", 1050, "USD")

	res := MatchNaive([]ExternalTransaction{e}, []MirrorEntry{m})

	if len(res.Matches) != 1 || len(res.Findings) != 0 {
		t.Fatalf("expected 1 match / 0 findings, got %d / %d", len(res.Matches), len(res.Findings))
	}
	if res.Matches[0].ExternalID != e.ID || res.Matches[0].EntryID != m.ID {
		t.Errorf("match pair links wrong ids: %+v", res.Matches[0])
	}
}

func TestMatchNaiveMissingInternal(t *testing.T) {
	e := ext("bank-tx-072", 5000, "USD")

	res := MatchNaive([]ExternalTransaction{e}, []MirrorEntry{entry("other-ref", 5000, "USD")})

	if len(res.Matches) != 0 || len(res.Findings) != 1 {
		t.Fatalf("expected 0 matches / 1 finding, got %d / %d", len(res.Matches), len(res.Findings))
	}
	f := res.Findings[0]
	if f.Kind != KindMissingInternal {
		t.Errorf("expected missing_internal, got %s", f.Kind)
	}
	if f.External.ID != e.ID || f.Entry != nil {
		t.Errorf("finding should carry the external and no entry: %+v", f)
	}
}

func TestMatchNaiveAmountMismatch(t *testing.T) {
	e := ext("dep-002", 1050, "USD")
	m := entry("dep-002", 1000, "USD")

	res := MatchNaive([]ExternalTransaction{e}, []MirrorEntry{m})

	if len(res.Findings) != 1 || res.Findings[0].Kind != KindAmountMismatch {
		t.Fatalf("expected 1 amount_mismatch finding, got %+v", res.Findings)
	}
	if res.Findings[0].Entry == nil || res.Findings[0].Entry.ID != m.ID {
		t.Errorf("mismatch finding should reference the candidate entry")
	}
}

func TestMatchNaiveAssetMismatchIsAmountMismatch(t *testing.T) {
	e := ext("dep-003", 1050, "USD")
	m := entry("dep-003", 1050, "MXN")

	res := MatchNaive([]ExternalTransaction{e}, []MirrorEntry{m})

	if len(res.Findings) != 1 || res.Findings[0].Kind != KindAmountMismatch {
		t.Fatalf("same ref with different asset must be amount_mismatch, got %+v", res.Findings)
	}
}

func TestMatchNaiveEntryConsumedOnlyOnce(t *testing.T) {
	e1 := ext("dup-ref", 1050, "USD")
	e2 := ext("dup-ref", 1050, "USD")
	m := entry("dup-ref", 1050, "USD")

	res := MatchNaive([]ExternalTransaction{e1, e2}, []MirrorEntry{m})

	if len(res.Matches) != 1 {
		t.Fatalf("expected exactly 1 match (entry consumed once), got %d", len(res.Matches))
	}
	if len(res.Findings) != 1 || res.Findings[0].Kind != KindMissingInternal {
		t.Fatalf("second external must be missing_internal after consumption, got %+v", res.Findings)
	}
}

func TestMatchNaivePicksExactAmongCandidates(t *testing.T) {
	e := ext("multi-ref", 2000, "USD")
	wrong := entry("multi-ref", 1000, "USD")
	right := entry("multi-ref", 2000, "USD")

	res := MatchNaive([]ExternalTransaction{e}, []MirrorEntry{wrong, right})

	if len(res.Matches) != 1 || res.Matches[0].EntryID != right.ID {
		t.Fatalf("matcher must pick the exact-amount candidate, got %+v", res)
	}
}

func TestMatchNaiveEveryExternalAccountedFor(t *testing.T) {
	externals := []ExternalTransaction{
		ext("a", 100, "USD"),
		ext("b", 200, "USD"),
		ext("c", 300, "USD"),
	}
	mirror := []MirrorEntry{
		entry("a", 100, "USD"), // match
		entry("b", 999, "USD"), // mismatch
		// "c" missing entirely
	}

	res := MatchNaive(externals, mirror)

	if len(res.Matches)+len(res.Findings) != len(externals) {
		t.Fatalf("every external must yield exactly one outcome: %d matches + %d findings != %d externals",
			len(res.Matches), len(res.Findings), len(externals))
	}
	if len(res.Matches) != 1 || len(res.Findings) != 2 {
		t.Fatalf("expected 1 match and 2 findings, got %d / %d", len(res.Matches), len(res.Findings))
	}
}

func TestMatchNaiveEmptyInputs(t *testing.T) {
	res := MatchNaive(nil, nil)
	if len(res.Matches) != 0 || len(res.Findings) != 0 {
		t.Fatalf("empty inputs must produce empty result: %+v", res)
	}
}
