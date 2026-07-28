package domain

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/money"
)

func p(dir Direction, asset string, units int64) Posting {
	return Posting{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		Direction: dir,
		Amount:    money.Amount{Asset: asset, Units: units},
	}
}

func TestValidateBalanced(t *testing.T) {
	tests := []struct {
		name     string
		postings []Posting
		wantErr  error // nil means valid
	}{
		{
			name: "balanced simple pair",
			postings: []Posting{
				p(DirectionDebit, "USD", 10000),
				p(DirectionCredit, "USD", 10000),
			},
		},
		{
			name: "balanced one debit split into two credits",
			postings: []Posting{
				p(DirectionDebit, "USD", 10000),
				p(DirectionCredit, "USD", 9700),
				p(DirectionCredit, "USD", 300),
			},
		},
		{
			name: "balanced multi-asset",
			postings: []Posting{
				p(DirectionDebit, "USD", 500),
				p(DirectionCredit, "USD", 500),
				p(DirectionDebit, "BTC", 21),
				p(DirectionCredit, "BTC", 21),
			},
		},
		{
			name: "balanced multi-asset with splits",
			postings: []Posting{
				p(DirectionDebit, "USD", 100),
				p(DirectionDebit, "USD", 200),
				p(DirectionCredit, "USD", 300),
				p(DirectionCredit, "EUR", 50),
				p(DirectionDebit, "EUR", 30),
				p(DirectionDebit, "EUR", 20),
			},
		},
		{
			name: "balanced at max int64",
			postings: []Posting{
				p(DirectionDebit, "USD", math.MaxInt64),
				p(DirectionCredit, "USD", math.MaxInt64),
			},
		},
		{
			name: "unbalanced same asset",
			postings: []Posting{
				p(DirectionDebit, "USD", 10000),
				p(DirectionCredit, "USD", 9999),
			},
			wantErr: ErrUnbalanced,
		},
		{
			name: "unbalanced multi-asset (one asset off)",
			postings: []Posting{
				p(DirectionDebit, "USD", 500),
				p(DirectionCredit, "USD", 500),
				p(DirectionDebit, "BTC", 21),
				p(DirectionCredit, "BTC", 20),
			},
			wantErr: ErrUnbalanced,
		},
		{
			name: "asset with only debits",
			postings: []Posting{
				p(DirectionDebit, "USD", 500),
				p(DirectionCredit, "USD", 500),
				p(DirectionDebit, "BTC", 21),
			},
			wantErr: ErrUnbalanced,
		},
		{
			name: "asset with only credits",
			postings: []Posting{
				p(DirectionDebit, "USD", 500),
				p(DirectionCredit, "USD", 500),
				p(DirectionCredit, "BTC", 21),
			},
			wantErr: ErrUnbalanced,
		},
		{
			name:     "single posting",
			postings: []Posting{p(DirectionDebit, "USD", 100)},
			wantErr:  ErrTooFewPostings,
		},
		{
			name:     "empty postings",
			postings: nil,
			wantErr:  ErrTooFewPostings,
		},
		{
			name: "zero amount",
			postings: []Posting{
				p(DirectionDebit, "USD", 0),
				p(DirectionCredit, "USD", 0),
			},
			wantErr: ErrNonPositiveAmount,
		},
		{
			name: "negative amount",
			postings: []Posting{
				p(DirectionDebit, "USD", -100),
				p(DirectionCredit, "USD", -100),
			},
			wantErr: ErrNonPositiveAmount,
		},
		{
			name: "invalid direction",
			postings: []Posting{
				p(Direction("SIDEWAYS"), "USD", 100),
				p(DirectionCredit, "USD", 100),
			},
			wantErr: ErrInvalidDirection,
		},
		{
			name: "lowercase direction is invalid",
			postings: []Posting{
				p(Direction("debit"), "USD", 100),
				p(DirectionCredit, "USD", 100),
			},
			wantErr: ErrInvalidDirection,
		},
		{
			name: "invalid asset code",
			postings: []Posting{
				p(DirectionDebit, "usd", 100),
				p(DirectionCredit, "usd", 100),
			},
			wantErr: money.ErrInvalidAsset,
		},
		{
			name: "empty asset code",
			postings: []Posting{
				p(DirectionDebit, "", 100),
				p(DirectionCredit, "", 100),
			},
			wantErr: money.ErrInvalidAsset,
		},
		{
			name: "int64 overflow while summing debits",
			postings: []Posting{
				p(DirectionDebit, "USD", math.MaxInt64),
				p(DirectionDebit, "USD", 1),
				p(DirectionCredit, "USD", math.MaxInt64),
				p(DirectionCredit, "USD", 1),
			},
			wantErr: money.ErrOverflow,
		},
		{
			name: "int64 overflow while summing credits only",
			postings: []Posting{
				p(DirectionDebit, "USD", 1),
				p(DirectionCredit, "USD", math.MaxInt64),
				p(DirectionCredit, "USD", math.MaxInt64),
			},
			wantErr: money.ErrOverflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBalanced(tt.postings)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateBalanced() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateBalanced() = %v, want errors.Is(..., %v)", err, tt.wantErr)
			}
		})
	}
}

func TestReverse(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	postedAt := now.Add(-time.Hour)
	original := Transaction{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		LedgerID:       uuid.New(),
		Reference:      "dep-1",
		IdempotencyKey: "dep-1",
		Status:         TransactionPosted,
		EffectiveAt:    postedAt,
		PostedAt:       &postedAt,
		CreatedAt:      postedAt,
		Postings: []Posting{
			p(DirectionDebit, "USD", 10000),
			p(DirectionCredit, "USD", 9700),
			p(DirectionCredit, "USD", 300),
		},
	}

	revID := uuid.New()
	rev, err := Reverse(original, revID, now)
	if err != nil {
		t.Fatalf("Reverse() error = %v", err)
	}
	if rev.ID != revID {
		t.Errorf("rev.ID = %s, want %s", rev.ID, revID)
	}
	if rev.TenantID != original.TenantID || rev.LedgerID != original.LedgerID {
		t.Error("reversal must keep tenant and ledger")
	}
	wantRef := ReversalReferencePrefix + original.ID.String()
	if rev.Reference != wantRef {
		t.Errorf("rev.Reference = %q, want %q", rev.Reference, wantRef)
	}
	if rev.IdempotencyKey != wantRef {
		t.Errorf("rev.IdempotencyKey = %q, want %q", rev.IdempotencyKey, wantRef)
	}
	if rev.Status != TransactionPosted {
		t.Errorf("rev.Status = %q, want posted", rev.Status)
	}
	if rev.PostedAt == nil || !rev.PostedAt.Equal(now) {
		t.Errorf("rev.PostedAt = %v, want %v", rev.PostedAt, now)
	}
	if rev.ReversesID == nil || *rev.ReversesID != original.ID {
		t.Errorf("rev.ReversesID = %v, want %s", rev.ReversesID, original.ID)
	}
	if len(rev.Postings) != len(original.Postings) {
		t.Fatalf("len(rev.Postings) = %d, want %d", len(rev.Postings), len(original.Postings))
	}
	for i, rp := range rev.Postings {
		op := original.Postings[i]
		if rp.Direction != op.Direction.Opposite() {
			t.Errorf("posting %d direction = %q, want %q", i, rp.Direction, op.Direction.Opposite())
		}
		if rp.Amount != op.Amount {
			t.Errorf("posting %d amount = %+v, want %+v", i, rp.Amount, op.Amount)
		}
		if rp.AccountID != op.AccountID {
			t.Errorf("posting %d account = %s, want %s", i, rp.AccountID, op.AccountID)
		}
		if rp.ID == op.ID || rp.ID == uuid.Nil {
			t.Errorf("posting %d must get a fresh id", i)
		}
	}
	// The mirror must itself balance.
	if err := ValidateBalanced(rev.Postings); err != nil {
		t.Errorf("reversal postings are unbalanced: %v", err)
	}

	t.Run("draft cannot be reversed", func(t *testing.T) {
		draft := original
		draft.Status = TransactionDraft
		draft.PostedAt = nil
		if _, err := Reverse(draft, uuid.New(), now); !errors.Is(err, ErrNotPosted) {
			t.Fatalf("Reverse(draft) = %v, want ErrNotPosted", err)
		}
	})

	t.Run("already reversed cannot be reversed again", func(t *testing.T) {
		done := original
		otherID := uuid.New()
		done.Status = TransactionReversed
		done.ReversedByID = &otherID
		if _, err := Reverse(done, uuid.New(), now); !errors.Is(err, ErrAlreadyReversed) {
			t.Fatalf("Reverse(reversed) = %v, want ErrAlreadyReversed", err)
		}
	})

	t.Run("posted but already linked to a reversal", func(t *testing.T) {
		linked := original
		otherID := uuid.New()
		linked.ReversedByID = &otherID
		if _, err := Reverse(linked, uuid.New(), now); !errors.Is(err, ErrAlreadyReversed) {
			t.Fatalf("Reverse(linked) = %v, want ErrAlreadyReversed", err)
		}
	})
}

func TestDirectionOpposite(t *testing.T) {
	if DirectionDebit.Opposite() != DirectionCredit {
		t.Error("DEBIT.Opposite() should be CREDIT")
	}
	if DirectionCredit.Opposite() != DirectionDebit {
		t.Error("CREDIT.Opposite() should be DEBIT")
	}
}

func TestBalanceMath(t *testing.T) {
	b := Balance{Asset: "USD", PostedDebits: 300, PostedCredits: 10300, PendingDebits: 50, PendingCredits: 150, Held: 600}

	if got := b.PostedNet(NormalCredit); got != 10000 {
		t.Errorf("PostedNet(credit) = %d, want 10000", got)
	}
	if got := b.PostedNet(NormalDebit); got != -10000 {
		t.Errorf("PostedNet(debit) = %d, want -10000", got)
	}
	if got := b.PendingNet(NormalCredit); got != 100 {
		t.Errorf("PendingNet(credit) = %d, want 100", got)
	}
	if got := b.Available(NormalCredit); got != 9400 {
		t.Errorf("Available(credit) = %d, want 9400", got)
	}
	if got := b.Available(NormalDebit); got != -10600 {
		t.Errorf("Available(debit) = %d, want -10600", got)
	}
}

func TestAssetExponent(t *testing.T) {
	if AssetExponent("USD") != 2 {
		t.Error("USD exponent should be 2")
	}
	if AssetExponent("JPY") != 0 {
		t.Error("JPY exponent should be 0")
	}
	if AssetExponent("BTC") != 8 {
		t.Error("BTC exponent should be 8")
	}
	if AssetExponent("UNKNOWN9") != DefaultAssetExponent {
		t.Error("unknown assets should fall back to the default exponent")
	}
}

// ---- Statement running balance -------------------------------------------------

func TestRunningBalance(t *testing.T) {
	// A credit-normal wallet: opening raw is debits-credits, so a wallet that
	// received 9700 in credits has raw -9700 and signed opening 9700.
	openingRaw := int64(-9700)
	if got := SignRaw(NormalCredit, openingRaw); got != 9700 {
		t.Fatalf("signed opening = %d, want 9700", got)
	}
	// Entries: +500 credit, -200 debit, +100 credit (raw cumulative window
	// values as SQL would produce them: debit positive, credit negative).
	steps := []struct {
		dir  Direction
		amt  int64
		want int64 // running balance after the entry, credit-normal signed
	}{
		{DirectionCredit, 500, 10200},
		{DirectionDebit, 200, 10000},
		{DirectionCredit, 100, 10100},
	}
	cum := int64(0)
	for i, s := range steps {
		if s.dir == DirectionDebit {
			cum += s.amt
		} else {
			cum -= s.amt
		}
		if got := RunningBalance(NormalCredit, openingRaw, cum); got != s.want {
			t.Errorf("step %d: running = %d, want %d", i, got, s.want)
		}
	}
	// Debit-normal accounts keep the raw sign.
	if got := RunningBalance(NormalDebit, 1000, 250); got != 1250 {
		t.Errorf("debit-normal running = %d, want 1250", got)
	}
	if got := SignRaw(NormalDebit, -300); got != -300 {
		t.Errorf("debit-normal negative = %d, want -300", got)
	}
}

// ---- Provider convention parser ------------------------------------------------

func TestProviderOf(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		metadata map[string]string
		want     string
		ok       bool
	}{
		{"colon convention", "assets:provider:thunes:usd", nil, "thunes", true},
		{"slash convention", "liabilities/provider/dlocal/mxn", nil, "dlocal", true},
		{"mixed separators", "assets:provider/uniteller:usd", nil, "uniteller", true},
		{"metadata override wins", "assets:cash", map[string]string{"provider": "Thunes"}, "thunes", true},
		{"metadata on conventional path wins", "assets:provider:thunes:usd", map[string]string{"provider": "other"}, "other", true},
		{"no convention", "customer:c1:wallet", nil, "", false},
		{"provider segment last, no name", "assets:provider", nil, "", false},
		{"empty path", "", nil, "", false},
		{"case-insensitive segment", "Assets:Provider:Thunes", nil, "thunes", true},
		{"whitespace-only metadata ignored", "assets:cash", map[string]string{"provider": "  "}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ProviderOf(c.path, c.metadata)
			if got != c.want || ok != c.ok {
				t.Errorf("ProviderOf(%q, %v) = (%q, %v), want (%q, %v)",
					c.path, c.metadata, got, ok, c.want, c.ok)
			}
		})
	}
}
