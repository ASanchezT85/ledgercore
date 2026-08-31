package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/money"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/app"
	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/domain"
)

// newTestStore skips unless LEDGERCORE_TEST_DATABASE_URL is set, then
// migrates the ledger schema and returns a Store. Each test isolates itself
// with a fresh tenant id, so no cleanup of shared tables is needed.
func newTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("LEDGERCORE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LEDGERCORE_TEST_DATABASE_URL is not set; skipping integration test")
	}
	// When TestMain provisioned the real role model, migrations already ran as
	// the migrator and `url` points at the (DDL-less) runtime role; migrating
	// again would fail. Just open the runtime pool.
	if roleModelProvisioned {
		return runtimeStorePool(t, url)
	}
	ctx := context.Background()
	pool, err := pgxutil.NewPool(ctx, url, "ledger")
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(pool), pool
}

type fixture struct {
	tenantID uuid.UUID
	ledger   domain.Ledger
	cash     domain.Account // asset, debit-normal
	wallet   domain.Account // liability, credit-normal
	fees     domain.Account // revenue, credit-normal
}

func newFixture(t *testing.T, s *Store) fixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	f := fixture{tenantID: uuid.New()}

	f.ledger = domain.Ledger{
		ID: uuid.New(), TenantID: f.tenantID, Name: "main",
		Environment: "sandbox", CreatedAt: now,
	}
	if err := s.CreateLedger(ctx, f.ledger); err != nil {
		t.Fatalf("create ledger: %v", err)
	}

	mk := func(name string, typ domain.AccountType, nb domain.NormalBalance) domain.Account {
		a := domain.Account{
			ID: uuid.New(), TenantID: f.tenantID, LedgerID: f.ledger.ID,
			Name: name, Path: name, Type: typ, NormalBalance: nb,
			Status: "active", CreatedAt: now,
		}
		if err := s.CreateAccount(ctx, a); err != nil {
			t.Fatalf("create account %s: %v", name, err)
		}
		return a
	}
	f.cash = mk("assets:cash", domain.AccountAsset, domain.NormalDebit)
	f.wallet = mk("customer:c1:wallet", domain.AccountLiability, domain.NormalCredit)
	f.fees = mk("revenue:fees", domain.AccountRevenue, domain.NormalCredit)
	return f
}

func postedTx(f fixture, key string, amount int64) domain.Transaction {
	now := time.Now().UTC()
	fee := amount * 3 / 100
	return domain.Transaction{
		ID: uuid.New(), TenantID: f.tenantID, LedgerID: f.ledger.ID,
		IdempotencyKey: key, Status: domain.TransactionPosted,
		EffectiveAt: now, PostedAt: &now, CreatedAt: now,
		Postings: []domain.Posting{
			{ID: uuid.New(), AccountID: f.cash.ID, Direction: domain.DirectionDebit,
				Amount: money.Amount{Asset: "USD", Units: amount}},
			{ID: uuid.New(), AccountID: f.wallet.ID, Direction: domain.DirectionCredit,
				Amount: money.Amount{Asset: "USD", Units: amount - fee}},
			{ID: uuid.New(), AccountID: f.fees.ID, Direction: domain.DirectionCredit,
				Amount: money.Amount{Asset: "USD", Units: fee}},
		},
	}
}

func balanceFor(t *testing.T, s *Store, tenantID, accountID uuid.UUID, asset string) domain.Balance {
	t.Helper()
	_, balances, err := s.GetBalances(context.Background(), tenantID, accountID)
	if err != nil {
		t.Fatalf("get balances: %v", err)
	}
	for _, b := range balances {
		if b.Asset == asset {
			return b
		}
	}
	return domain.Balance{Asset: asset}
}

func countOutbox(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, topic string) int {
	t.Helper()
	var n int
	err := pgxutil.WithSystemTx(context.Background(), pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM outbox WHERE tenant_id = $1 AND topic = $2`,
			tenantID, topic).Scan(&n)
	})
	if err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}

func TestCreateTransactionIdempotencyAndBalances(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	tx := postedTx(f, "dep-0001", 10000) // 10000 in, 9700 wallet, 300 fees
	created, replayed, err := s.CreateTransaction(ctx, tx)
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	if replayed {
		t.Fatal("first creation must not be a replay")
	}
	if created.ID != tx.ID {
		t.Fatalf("created id = %s, want %s", created.ID, tx.ID)
	}

	// Balances applied atomically with the postings.
	cash := balanceFor(t, s, f.tenantID, f.cash.ID, "USD")
	if cash.PostedDebits != 10000 || cash.PostedCredits != 0 {
		t.Errorf("cash balance = %+v, want debits 10000 credits 0", cash)
	}
	if got := cash.Available(f.cash.NormalBalance); got != 10000 {
		t.Errorf("cash available = %d, want 10000", got)
	}
	wallet := balanceFor(t, s, f.tenantID, f.wallet.ID, "USD")
	if wallet.PostedCredits != 9700 || wallet.PostedDebits != 0 {
		t.Errorf("wallet balance = %+v, want credits 9700 debits 0", wallet)
	}
	if wallet.Version == 0 {
		t.Error("wallet balance version must be bumped")
	}

	// The outbox row committed with the same transaction.
	if n := countOutbox(t, pool, f.tenantID, events.TopicTransactionPosted); n != 1 {
		t.Errorf("outbox posted events = %d, want 1", n)
	}

	// Idempotent replay: same key returns the stored transaction, writes nothing.
	replay, replayed, err := s.CreateTransaction(ctx, postedTx(f, "dep-0001", 10000))
	if err != nil {
		t.Fatalf("replay create: %v", err)
	}
	if !replayed {
		t.Fatal("second creation with the same key must be a replay")
	}
	if replay.ID != created.ID {
		t.Errorf("replay id = %s, want original %s", replay.ID, created.ID)
	}
	if b := balanceFor(t, s, f.tenantID, f.cash.ID, "USD"); b.PostedDebits != 10000 {
		t.Errorf("replay must not double-apply balances: cash debits = %d", b.PostedDebits)
	}
	if n := countOutbox(t, pool, f.tenantID, events.TopicTransactionPosted); n != 1 {
		t.Errorf("replay must not add outbox events: got %d", n)
	}

	// GetTransaction round-trips postings.
	got, err := s.GetTransaction(ctx, f.tenantID, created.ID)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}
	if len(got.Postings) != 3 {
		t.Errorf("postings = %d, want 3", len(got.Postings))
	}
}

func TestDraftPostFlow(t *testing.T) {
	s, _ := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	draft := postedTx(f, "draft-0001", 5000)
	draft.Status = domain.TransactionDraft
	draft.PostedAt = nil
	if _, _, err := s.CreateTransaction(ctx, draft); err != nil {
		t.Fatalf("create draft: %v", err)
	}

	cash := balanceFor(t, s, f.tenantID, f.cash.ID, "USD")
	if cash.PendingDebits != 5000 || cash.PostedDebits != 0 {
		t.Fatalf("draft must hit pending only, got %+v", cash)
	}

	posted, err := s.PostTransaction(ctx, f.tenantID, draft.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("post draft: %v", err)
	}
	if posted.Status != domain.TransactionPosted || posted.PostedAt == nil {
		t.Fatalf("posted status = %+v", posted.Status)
	}
	cash = balanceFor(t, s, f.tenantID, f.cash.ID, "USD")
	if cash.PostedDebits != 5000 || cash.PendingDebits != 0 {
		t.Fatalf("post must move pending to posted, got %+v", cash)
	}

	// Posting again is an idempotent no-op.
	if _, err := s.PostTransaction(ctx, f.tenantID, draft.ID, time.Now().UTC()); err != nil {
		t.Fatalf("re-post must be idempotent: %v", err)
	}
	cash = balanceFor(t, s, f.tenantID, f.cash.ID, "USD")
	if cash.PostedDebits != 5000 {
		t.Fatalf("re-post must not double-apply, got %+v", cash)
	}
}

func TestReverseTransaction(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	tx := postedTx(f, "dep-0002", 10000)
	if _, _, err := s.CreateTransaction(ctx, tx); err != nil {
		t.Fatalf("create: %v", err)
	}

	rev, replayed, err := s.ReverseTransaction(ctx, f.tenantID, tx.ID, "", "chargeback", time.Now().UTC())
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if replayed {
		t.Fatal("first reverse must not be a replay")
	}
	if rev.ReversesID == nil || *rev.ReversesID != tx.ID {
		t.Errorf("reversal must link the original")
	}

	original, err := s.GetTransaction(ctx, f.tenantID, tx.ID)
	if err != nil {
		t.Fatalf("get original: %v", err)
	}
	if original.Status != domain.TransactionReversed {
		t.Errorf("original status = %s, want reversed", original.Status)
	}
	if original.ReversedByID == nil || *original.ReversedByID != rev.ID {
		t.Errorf("original must link the reversal")
	}

	// Net balances return to zero on both sides.
	cash := balanceFor(t, s, f.tenantID, f.cash.ID, "USD")
	if cash.PostedNet(f.cash.NormalBalance) != 0 {
		t.Errorf("cash net after reversal = %d, want 0", cash.PostedNet(f.cash.NormalBalance))
	}
	wallet := balanceFor(t, s, f.tenantID, f.wallet.ID, "USD")
	if wallet.PostedNet(f.wallet.NormalBalance) != 0 {
		t.Errorf("wallet net after reversal = %d, want 0", wallet.PostedNet(f.wallet.NormalBalance))
	}

	if n := countOutbox(t, pool, f.tenantID, events.TopicTransactionReversed); n != 1 {
		t.Errorf("outbox reversed events = %d, want 1", n)
	}

	// Retrying the SAME reversal (same default key + same reason) replays the
	// stored reversal rather than erroring (R-008 reverse idempotency).
	if again, replayed, err := s.ReverseTransaction(ctx, f.tenantID, tx.ID, "", "chargeback", time.Now().UTC()); err != nil || !replayed || again.ID != rev.ID {
		t.Errorf("same-reversal retry = (%v, replayed=%v, %v), want original id and replayed=true", again.ID, replayed, err)
	}

	// A reversal under a DIFFERENT key bypasses idempotency and hits the
	// already-reversed guard: invalid state.
	if _, _, err := s.ReverseTransaction(ctx, f.tenantID, tx.ID, "second-reverse-"+uuid.NewString(), "", time.Now().UTC()); !errors.Is(err, app.ErrInvalidState) {
		t.Errorf("second reverse under a new key = %v, want ErrInvalidState", err)
	}

	// The trial balance stays balanced after the full cycle.
	tb, err := s.TrialBalance(ctx, f.tenantID, f.ledger.ID, nil)
	if err != nil {
		t.Fatalf("trial balance: %v", err)
	}
	if len(tb.Totals) != 1 || !tb.Totals[0].Balanced {
		t.Errorf("trial balance totals = %+v, want balanced USD", tb.Totals)
	}
}

func TestHoldLifecycle(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	if _, _, err := s.CreateTransaction(ctx, postedTx(f, "dep-0003", 10000)); err != nil {
		t.Fatalf("fund wallet: %v", err)
	}
	// Wallet now has 9700 available.

	newHold := func(key string, units int64) domain.Hold {
		return domain.Hold{
			ID: uuid.New(), TenantID: f.tenantID, AccountID: f.wallet.ID,
			IdempotencyKey: key,
			Amount:         money.Amount{Asset: "USD", Units: units},
			Status:         domain.HoldActive,
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
			CreatedAt:      time.Now().UTC(),
		}
	}

	hold, replayed, err := s.CreateHold(ctx, newHold("auth-1", 600))
	if err != nil {
		t.Fatalf("create hold: %v", err)
	}
	if replayed {
		t.Fatal("first hold must not be a replay")
	}
	if hold.LedgerID != f.ledger.ID {
		t.Errorf("hold ledger = %s, want %s (derived from account)", hold.LedgerID, f.ledger.ID)
	}
	wallet := balanceFor(t, s, f.tenantID, f.wallet.ID, "USD")
	if wallet.Held != 600 {
		t.Errorf("held = %d, want 600", wallet.Held)
	}
	if got := wallet.Available(f.wallet.NormalBalance); got != 9100 {
		t.Errorf("available = %d, want 9100", got)
	}
	if n := countOutbox(t, pool, f.tenantID, events.TopicHoldCreated); n != 1 {
		t.Errorf("hold.created events = %d, want 1", n)
	}

	// Idempotent replay of the same hold key.
	again, replayed, err := s.CreateHold(ctx, newHold("auth-1", 600))
	if err != nil || !replayed || again.ID != hold.ID {
		t.Fatalf("hold replay = (%v, %v, %v), want original id and replayed=true", again.ID, replayed, err)
	}

	// Insufficient funds beyond available.
	if _, _, err := s.CreateHold(ctx, newHold("auth-too-big", 9500)); !errors.Is(err, app.ErrInsufficientFunds) {
		t.Errorf("oversized hold = %v, want ErrInsufficientFunds", err)
	}

	// Capture frees the held funds.
	captured, err := s.CaptureHold(ctx, f.tenantID, hold.ID, nil, nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if captured.Status != domain.HoldCaptured {
		t.Errorf("status = %s, want captured", captured.Status)
	}
	wallet = balanceFor(t, s, f.tenantID, f.wallet.ID, "USD")
	if wallet.Held != 0 {
		t.Errorf("held after capture = %d, want 0", wallet.Held)
	}
	if n := countOutbox(t, pool, f.tenantID, events.TopicHoldCaptured); n != 1 {
		t.Errorf("hold.captured events = %d, want 1", n)
	}

	// Capturing again is an invalid state.
	if _, err := s.CaptureHold(ctx, f.tenantID, hold.ID, nil, nil, time.Now().UTC()); !errors.Is(err, app.ErrInvalidState) {
		t.Errorf("double capture = %v, want ErrInvalidState", err)
	}

	// Release path.
	hold2, _, err := s.CreateHold(ctx, newHold("auth-2", 500))
	if err != nil {
		t.Fatalf("create hold 2: %v", err)
	}
	released, err := s.ReleaseHold(ctx, f.tenantID, hold2.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Status != domain.HoldReleased {
		t.Errorf("status = %s, want released", released.Status)
	}
	wallet = balanceFor(t, s, f.tenantID, f.wallet.ID, "USD")
	if wallet.Held != 0 {
		t.Errorf("held after release = %d, want 0", wallet.Held)
	}
}

func TestTenantIsolation(t *testing.T) {
	s, _ := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	// Another tenant must not see this tenant's ledger through any read path.
	otherTenant := uuid.New()
	if _, err := s.GetLedger(ctx, otherTenant, f.ledger.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("cross-tenant GetLedger = %v, want ErrNotFound", err)
	}
	if _, err := s.GetAccount(ctx, otherTenant, f.cash.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("cross-tenant GetAccount = %v, want ErrNotFound", err)
	}
	ledgers, err := s.ListLedgers(ctx, otherTenant, app.Page{Limit: 100})
	if err != nil {
		t.Fatalf("list ledgers as other tenant: %v", err)
	}
	for _, l := range ledgers {
		if l.ID == f.ledger.ID {
			t.Error("RLS leak: other tenant listed this tenant's ledger")
		}
	}
}

func TestPostingsAreAppendOnly(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	tx := postedTx(f, "immutable-1", 1000)
	if _, _, err := s.CreateTransaction(ctx, tx); err != nil {
		t.Fatalf("create: %v", err)
	}

	err := pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(dbtx pgx.Tx) error {
		_, err := dbtx.Exec(ctx, `UPDATE postings SET amount = amount + 1 WHERE transaction_id = $1`, tx.ID)
		return err
	})
	if err == nil {
		t.Fatal("updating postings must be rejected by the append-only trigger")
	}
	err = pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(dbtx pgx.Tx) error {
		_, err := dbtx.Exec(ctx, `DELETE FROM postings WHERE transaction_id = $1`, tx.ID)
		return err
	})
	if err == nil {
		t.Fatal("deleting postings must be rejected by the append-only trigger")
	}
	// Arbitrary transaction mutations are rejected too.
	err = pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(dbtx pgx.Tx) error {
		_, err := dbtx.Exec(ctx, `UPDATE transactions SET reference = 'tampered' WHERE id = $1`, tx.ID)
		return err
	})
	if err == nil {
		t.Fatal("mutating transaction fields must be rejected by the guard trigger")
	}
}
