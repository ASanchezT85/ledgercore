package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/libs/go/money"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/app"
	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/domain"
)

// txAt builds a posted deposit transaction with an explicit effective_at.
func txAt(f fixture, key string, amount int64, effectiveAt time.Time) domain.Transaction {
	tx := postedTx(f, key, amount)
	tx.EffectiveAt = effectiveAt
	return tx
}

func TestStatementOpeningRunningClosing(t *testing.T) {
	s, _ := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// Two deposits before the period, two inside, one after.
	for i, spec := range []struct {
		key    string
		amount int64
		at     time.Time
	}{
		{"st-before-1", 10000, base.Add(-72 * time.Hour)},
		{"st-before-2", 5000, base.Add(-48 * time.Hour)},
		{"st-in-1", 2000, base.Add(1 * time.Hour)},
		{"st-in-2", 1000, base.Add(2 * time.Hour)},
		{"st-after-1", 3000, base.Add(100 * time.Hour)},
	} {
		if _, _, err := s.CreateTransaction(ctx, txAt(f, spec.key, spec.amount, spec.at)); err != nil {
			t.Fatalf("create tx %d: %v", i, err)
		}
	}

	from := base
	to := base.Add(24 * time.Hour)
	st, err := s.Statement(ctx, f.tenantID, f.wallet.ID, from, to, app.Page{Limit: 100})
	if err != nil {
		t.Fatalf("statement: %v", err)
	}

	// Wallet is credit-normal: opening = 9700 + 4850 = 14550.
	if len(st.Opening) != 1 || st.Opening[0].Asset != "USD" || st.Opening[0].Units != 14550 {
		t.Fatalf("opening = %+v, want USD 14550", st.Opening)
	}
	// Two entries inside the period: +1940, +970 -> running 16490, 17460.
	if len(st.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(st.Entries))
	}
	if st.Entries[0].Running != 16490 || st.Entries[1].Running != 17460 {
		t.Errorf("running = %d, %d, want 16490, 17460", st.Entries[0].Running, st.Entries[1].Running)
	}
	if !st.Entries[0].EffectiveAt.Before(st.Entries[1].EffectiveAt) {
		t.Error("entries must be ordered by effective_at asc")
	}
	// Closing excludes the transaction after the period.
	if len(st.Closing) != 1 || st.Closing[0].Units != 17460 {
		t.Fatalf("closing = %+v, want USD 17460", st.Closing)
	}

	// Pagination must not break the running balance accumulation.
	p1, err := s.Statement(ctx, f.tenantID, f.wallet.ID, from, to, app.Page{Limit: 1})
	if err != nil {
		t.Fatalf("statement page1: %v", err)
	}
	if len(p1.Entries) != 1 || p1.Entries[0].Running != 16490 {
		t.Fatalf("page1 = %+v, want one entry running 16490", p1.Entries)
	}
	p2, err := s.Statement(ctx, f.tenantID, f.wallet.ID, from, to, app.Page{
		Limit:  1,
		Cursor: httpx.Cursor{CreatedAt: p1.Entries[0].EffectiveAt, ID: p1.Entries[0].PostingID},
	})
	if err != nil {
		t.Fatalf("statement page2: %v", err)
	}
	if len(p2.Entries) != 1 || p2.Entries[0].Running != 17460 {
		t.Fatalf("page2 = %+v, want one entry running 17460", p2.Entries)
	}

	// Debit-normal cash account: opening 15000, closing 18000.
	stCash, err := s.Statement(ctx, f.tenantID, f.cash.ID, from, to, app.Page{Limit: 100})
	if err != nil {
		t.Fatalf("statement cash: %v", err)
	}
	if stCash.Opening[0].Units != 15000 || stCash.Closing[0].Units != 18000 {
		t.Fatalf("cash opening/closing = %d/%d, want 15000/18000",
			stCash.Opening[0].Units, stCash.Closing[0].Units)
	}
}

// mustTxID finds a transaction id by idempotency key through the list API.
func mustTxID(t *testing.T, s *Store, f fixture, key string) uuid.UUID {
	t.Helper()
	txs, err := s.ListTransactions(context.Background(), f.tenantID,
		app.TransactionFilter{LedgerID: &f.ledger.ID}, app.Page{Limit: 100})
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	for _, tx := range txs {
		if tx.IdempotencyKey == key {
			return tx.ID
		}
	}
	t.Fatalf("transaction %q not found", key)
	return uuid.Nil
}

func TestTrialBalanceAsOfReconstructsHistory(t *testing.T) {
	s, _ := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := s.CreateTransaction(ctx, txAt(f, "asof-1", 10000, base)); err != nil {
		t.Fatalf("create tx1: %v", err)
	}
	if _, _, err := s.CreateTransaction(ctx, txAt(f, "asof-2", 4000, base.Add(48*time.Hour))); err != nil {
		t.Fatalf("create tx2: %v", err)
	}

	// as_of between the two: only the first transaction counts.
	asOf := base.Add(24 * time.Hour)
	tb, err := s.TrialBalance(ctx, f.tenantID, f.ledger.ID, &asOf)
	if err != nil {
		t.Fatalf("trial balance as_of: %v", err)
	}
	if !tb.AsOf.Equal(asOf) {
		t.Errorf("as_of = %v, want %v", tb.AsOf, asOf)
	}
	byAccount := map[uuid.UUID]domain.TrialBalanceRow{}
	for _, r := range tb.Rows {
		byAccount[r.AccountID] = r
	}
	if r := byAccount[f.cash.ID]; r.Debits != 10000 || r.Credits != 0 {
		t.Errorf("cash as_of = %+v, want debits 10000", r)
	}
	if r := byAccount[f.wallet.ID]; r.Credits != 9700 {
		t.Errorf("wallet as_of = %+v, want credits 9700", r)
	}
	if len(tb.Totals) != 1 || tb.Totals[0].Debits != 10000 || !tb.Totals[0].Balanced {
		t.Errorf("totals as_of = %+v, want balanced USD 10000", tb.Totals)
	}

	// Without as_of the report reflects both transactions.
	now, err := s.TrialBalance(ctx, f.tenantID, f.ledger.ID, nil)
	if err != nil {
		t.Fatalf("trial balance now: %v", err)
	}
	if len(now.Totals) != 1 || now.Totals[0].Debits != 14000 {
		t.Errorf("current totals = %+v, want debits 14000", now.Totals)
	}

	// Reverse tx1 today: a historical as_of BEFORE the reversal must still
	// count the original postings (status 'reversed' means it WAS posted).
	if _, _, err := s.ReverseTransaction(ctx, f.tenantID, mustTxID(t, s, f, "asof-1"), "", "undo", time.Now().UTC()); err != nil {
		t.Fatalf("reverse tx1: %v", err)
	}
	histor, err := s.TrialBalance(ctx, f.tenantID, f.ledger.ID, &asOf)
	if err != nil {
		t.Fatalf("trial balance as_of post-reversal: %v", err)
	}
	if len(histor.Totals) != 1 || histor.Totals[0].Debits != 10000 || !histor.Totals[0].Balanced {
		t.Errorf("historical totals after reversal = %+v, want balanced USD 10000", histor.Totals)
	}
}

func TestProviderAccountBalances(t *testing.T) {
	s, _ := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()
	now := time.Now().UTC()

	mk := func(name string, typ domain.AccountType, nb domain.NormalBalance, metadata map[string]string) domain.Account {
		a := domain.Account{
			ID: uuid.New(), TenantID: f.tenantID, LedgerID: f.ledger.ID,
			Name: name, Path: name, Type: typ, NormalBalance: nb,
			Status: "active", Metadata: metadata, CreatedAt: now,
		}
		if err := s.CreateAccount(ctx, a); err != nil {
			t.Fatalf("create account %s: %v", name, err)
		}
		return a
	}
	acmepayUSD := mk("assets:provider:acmepay:usd", domain.AccountAsset, domain.NormalDebit, nil)
	acmepayMXN := mk("assets:provider:acmepay:mxn", domain.AccountAsset, domain.NormalDebit, nil)
	nordpay := mk("liabilities:payables:nordpay", domain.AccountLiability, domain.NormalCredit,
		map[string]string{"provider": "nordpay"})

	post := func(key string, from, to uuid.UUID, asset string, units int64) {
		tx := domain.Transaction{
			ID: uuid.New(), TenantID: f.tenantID, LedgerID: f.ledger.ID,
			IdempotencyKey: key, Status: domain.TransactionPosted,
			EffectiveAt: now, PostedAt: &now, CreatedAt: now,
			Postings: []domain.Posting{
				{ID: uuid.New(), AccountID: from, Direction: domain.DirectionDebit,
					Amount: money.Amount{Asset: asset, Units: units}},
				{ID: uuid.New(), AccountID: to, Direction: domain.DirectionCredit,
					Amount: money.Amount{Asset: asset, Units: units}},
			},
		}
		if _, _, err := s.CreateTransaction(ctx, tx); err != nil {
			t.Fatalf("post %s: %v", key, err)
		}
	}
	// Fund acmepay USD 5000 and MXN 30000; accrue a 1200 USD payable to nordpay.
	post("pp-1", acmepayUSD.ID, f.wallet.ID, "USD", 5000)
	post("pp-2", acmepayMXN.ID, f.wallet.ID, "MXN", 30000)
	post("pp-3", f.cash.ID, nordpay.ID, "USD", 1200)

	rows, err := s.ProviderAccountBalances(ctx, f.tenantID)
	if err != nil {
		t.Fatalf("provider account balances: %v", err)
	}

	// Group like the service does and assert the nets per provider/asset.
	type key struct{ provider, asset string }
	nets := map[key]int64{}
	for _, r := range rows {
		provider, ok := domain.ProviderOf(r.Path, r.Metadata)
		if !ok {
			continue
		}
		nets[key{provider, r.Asset}] += r.PostedDebits - r.PostedCredits
	}
	if got := nets[key{"acmepay", "USD"}]; got != 5000 {
		t.Errorf("acmepay USD = %d, want 5000 (they_owe_us)", got)
	}
	if got := nets[key{"acmepay", "MXN"}]; got != 30000 {
		t.Errorf("acmepay MXN = %d, want 30000", got)
	}
	if got := nets[key{"nordpay", "USD"}]; got != -1200 {
		t.Errorf("nordpay USD = %d, want -1200 (we_owe_them)", got)
	}
	if len(nets) != 3 {
		t.Errorf("provider/asset pairs = %d, want 3 (accounts without convention excluded)", len(nets))
	}
}
