package postgres

// Anti-leak RLS contract test (blueprint §14, Fase 1a — mandatory CI gate).
//
// Tenant A creates a ledger, accounts and a posted transaction; tenant B
// must not be able to see or touch any of it:
//   - GET by id -> app.ErrNotFound
//   - listings  -> empty
//   - writes against A's resources -> app.ErrNotFound (never success)
//
// Requires LEDGERCORE_TEST_DATABASE_URL (skips otherwise). In CI it runs
// against a postgres:17 service container provisioned with the init SQL, so
// the pool connects as the NOBYPASSRLS app role — the exact production setup.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/money"

	"github.com/ledgercore/ledgercore/services/ledger-core/internal/app"
	"github.com/ledgercore/ledgercore/services/ledger-core/internal/domain"
)

func TestRLSCrossTenantIsolation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// Tenant A owns everything created here.
	fx := newFixture(t, store)
	tx := postedTx(fx, "rls-iso-"+uuid.NewString(), 10_000)
	if _, _, err := store.CreateTransaction(ctx, tx); err != nil {
		t.Fatalf("tenant A create transaction: %v", err)
	}
	hold := domain.Hold{
		ID: uuid.New(), TenantID: fx.tenantID, LedgerID: fx.ledger.ID,
		AccountID: fx.cash.ID, IdempotencyKey: "rls-hold-" + uuid.NewString(),
		Amount: money.Amount{Asset: "USD", Units: 100}, Status: domain.HoldActive,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	if _, _, err := store.CreateHold(ctx, hold); err != nil {
		t.Fatalf("tenant A create hold: %v", err)
	}

	// Tenant B: a different, freshly minted tenant with zero data.
	intruder := uuid.New()
	page := app.Page{Limit: 50}

	t.Run("gets by id return not found", func(t *testing.T) {
		if _, err := store.GetLedger(ctx, intruder, fx.ledger.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("GetLedger: want ErrNotFound, got %v", err)
		}
		if _, err := store.GetAccount(ctx, intruder, fx.cash.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("GetAccount: want ErrNotFound, got %v", err)
		}
		if _, err := store.GetTransaction(ctx, intruder, tx.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("GetTransaction: want ErrNotFound, got %v", err)
		}
		if _, err := store.GetHold(ctx, intruder, hold.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("GetHold: want ErrNotFound, got %v", err)
		}
		if _, _, err := store.GetBalances(ctx, intruder, fx.wallet.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("GetBalances: want ErrNotFound, got %v", err)
		}
	})

	t.Run("listings are empty", func(t *testing.T) {
		if got, err := store.ListLedgers(ctx, intruder, page); err != nil || len(got) != 0 {
			t.Errorf("ListLedgers: want 0 rows nil err, got %d rows err=%v", len(got), err)
		}
		if got, err := store.ListAccounts(ctx, intruder, app.AccountFilter{LedgerID: &fx.ledger.ID}, page); err != nil || len(got) != 0 {
			t.Errorf("ListAccounts: want 0 rows nil err, got %d rows err=%v", len(got), err)
		}
		if got, err := store.ListTransactions(ctx, intruder, app.TransactionFilter{LedgerID: &fx.ledger.ID}, page); err != nil || len(got) != 0 {
			t.Errorf("ListTransactions: want 0 rows nil err, got %d rows err=%v", len(got), err)
		}
		// ListEntries is account-scoped: for a foreign account it fails
		// closed with not-found (also acceptable: zero rows).
		if got, err := store.ListEntries(ctx, intruder, fx.wallet.ID, "USD", page); !(errors.Is(err, app.ErrNotFound) || (err == nil && len(got) == 0)) {
			t.Errorf("ListEntries: want ErrNotFound or 0 rows, got %d rows err=%v", len(got), err)
		}
	})

	t.Run("writes against foreign resources fail closed", func(t *testing.T) {
		// Create an account inside A's ledger.
		alien := domain.Account{
			ID: uuid.New(), TenantID: intruder, LedgerID: fx.ledger.ID,
			Name: "intruder:acct", Path: "intruder:acct",
			Type: domain.AccountAsset, NormalBalance: domain.NormalDebit,
			Status: "active", CreatedAt: time.Now().UTC(),
		}
		if err := store.CreateAccount(ctx, alien); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("CreateAccount in foreign ledger: want ErrNotFound, got %v", err)
		}

		// Post a transaction against A's ledger/accounts.
		now := time.Now().UTC()
		foreignTx := domain.Transaction{
			ID: uuid.New(), TenantID: intruder, LedgerID: fx.ledger.ID,
			IdempotencyKey: "rls-foreign-" + uuid.NewString(),
			Status:         domain.TransactionPosted,
			EffectiveAt:    now, PostedAt: &now, CreatedAt: now,
			Postings: []domain.Posting{
				{ID: uuid.New(), AccountID: fx.cash.ID, Direction: domain.DirectionDebit,
					Amount: money.Amount{Asset: "USD", Units: 1}},
				{ID: uuid.New(), AccountID: fx.wallet.ID, Direction: domain.DirectionCredit,
					Amount: money.Amount{Asset: "USD", Units: 1}},
			},
		}
		if _, _, err := store.CreateTransaction(ctx, foreignTx); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("CreateTransaction against foreign ledger: want ErrNotFound, got %v", err)
		}

		// Lifecycle operations on A's transaction and hold.
		if _, _, err := store.ReverseTransaction(ctx, intruder, tx.ID, "rls-rev-"+uuid.NewString(), "intrusion", now); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("ReverseTransaction of foreign tx: want ErrNotFound, got %v", err)
		}
		if _, err := store.ReleaseHold(ctx, intruder, hold.ID, now); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("ReleaseHold of foreign hold: want ErrNotFound, got %v", err)
		}
	})

	// Sanity: tenant A still sees its own data (the isolation above is not
	// a side effect of broken reads).
	if _, err := store.GetLedger(ctx, fx.tenantID, fx.ledger.ID); err != nil {
		t.Fatalf("tenant A lost access to its own ledger: %v", err)
	}
}
