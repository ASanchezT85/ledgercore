package postgres

// P0 security-audit hardening tests (branch hardening/p0-auditoria).
//
//   - LC-001: PostgreSQL refuses to COMMIT an unbalanced firm transaction even
//     when inserted by direct SQL that bypasses the Go domain.
//   - LC-006: an idempotency key reused with a different payload is a conflict.
//   - LC-004: the runtime role cannot bypass the append-only guards, even with
//     ledger.allow_maintenance set; the sanctioned purge function is the only
//     path (exercised when ledgercore_maint owns it).
//   - Verifier: account_balances drift from postings is detected.
//
// All require LEDGERCORE_TEST_DATABASE_URL (skips otherwise).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ledgercore/ledgercore/libs/go/money"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"

	"github.com/ledgercore/ledgercore/services/ledger-core/internal/adapters/natsconsumer"
	"github.com/ledgercore/ledgercore/services/ledger-core/internal/app"
	"github.com/ledgercore/ledgercore/services/ledger-core/internal/domain"
)

// ---- LC-001: DB-enforced double-entry ---------------------------------------

func TestDoubleEntryConstraintRejectsUnbalancedDirectSQL(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	// Direct SQL insert of a POSTED transaction whose postings do NOT balance
	// (debit 100, credit 50). This bypasses domain.ValidateBalanced entirely.
	// The DEFERRABLE constraint trigger must fail the COMMIT.
	err := pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(tx pgx.Tx) error {
		txID := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO transactions (id, tenant_id, ledger_id, idempotency_key, status, effective_at, created_at)
			VALUES ($1, $2, $3, $4, 'posted', now(), now())`,
			txID, f.tenantID, f.ledger.ID, "direct-unbalanced-"+uuid.NewString()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO postings (id, tenant_id, transaction_id, account_id, direction, asset, amount, created_at)
			VALUES ($1, $2, $3, $4, 'DEBIT', 'USD', 100, now())`,
			uuid.New(), f.tenantID, txID, f.cash.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO postings (id, tenant_id, transaction_id, account_id, direction, asset, amount, created_at)
			VALUES ($1, $2, $3, $4, 'CREDIT', 'USD', 50, now())`,
			uuid.New(), f.tenantID, txID, f.wallet.ID); err != nil {
			return err
		}
		return nil // let WithTenantTx COMMIT — the deferred trigger fires here.
	})
	if err == nil {
		t.Fatal("COMMIT of an unbalanced posted transaction must be rejected by the double-entry constraint")
	}
	t.Logf("LC-001 negative rejected as expected: %v", err)

	// Positive control: a balanced direct insert commits cleanly.
	err = pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(tx pgx.Tx) error {
		txID := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO transactions (id, tenant_id, ledger_id, idempotency_key, status, effective_at, created_at)
			VALUES ($1, $2, $3, $4, 'posted', now(), now())`,
			txID, f.tenantID, f.ledger.ID, "direct-balanced-"+uuid.NewString()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO postings (id, tenant_id, transaction_id, account_id, direction, asset, amount, created_at)
			VALUES ($1, $2, $3, $4, 'DEBIT', 'USD', 100, now())`,
			uuid.New(), f.tenantID, txID, f.cash.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO postings (id, tenant_id, transaction_id, account_id, direction, asset, amount, created_at)
			VALUES ($1, $2, $3, $4, 'CREDIT', 'USD', 100, now())`,
			uuid.New(), f.tenantID, txID, f.wallet.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("balanced direct insert must commit, got: %v", err)
	}
}

// ---- LC-006: idempotency fingerprint ----------------------------------------

func TestIdempotencyFingerprintConflict(t *testing.T) {
	s := mustStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	key := "idem-fp-" + uuid.NewString()

	// First create.
	first := postedTx(f, key, 10000)
	if _, replayed, err := s.CreateTransaction(ctx, first); err != nil || replayed {
		t.Fatalf("first create: replayed=%v err=%v", replayed, err)
	}

	// Same key, SAME payload (new ids/timestamps only) -> replay.
	if _, replayed, err := s.CreateTransaction(ctx, postedTx(f, key, 10000)); err != nil || !replayed {
		t.Fatalf("same-payload replay: replayed=%v err=%v (want replayed=true)", replayed, err)
	}

	// Same key, DIFFERENT payload -> conflict.
	if _, _, err := s.CreateTransaction(ctx, postedTx(f, key, 20000)); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatalf("different-payload reuse: err=%v, want ErrIdempotencyConflict", err)
	}

	// Holds: same key, different amount -> conflict.
	holdKey := "idem-hold-" + uuid.NewString()
	// Fund the wallet first so the hold can be placed.
	if _, _, err := s.CreateTransaction(ctx, postedTx(f, "fund-"+uuid.NewString(), 10000)); err != nil {
		t.Fatalf("fund: %v", err)
	}
	mkHold := func(units int64) domain.Hold {
		return domain.Hold{
			ID: uuid.New(), TenantID: f.tenantID, AccountID: f.wallet.ID,
			IdempotencyKey: holdKey, Amount: money.Amount{Asset: "USD", Units: units},
			Status: domain.HoldActive, ExpiresAt: time.Now().UTC().Add(time.Hour),
			CreatedAt: time.Now().UTC(),
		}
	}
	if _, replayed, err := s.CreateHold(ctx, mkHold(500)); err != nil || replayed {
		t.Fatalf("first hold: replayed=%v err=%v", replayed, err)
	}
	if _, replayed, err := s.CreateHold(ctx, mkHold(500)); err != nil || !replayed {
		t.Fatalf("same hold replay: replayed=%v err=%v", replayed, err)
	}
	if _, _, err := s.CreateHold(ctx, mkHold(600)); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatalf("hold key reuse with different amount: err=%v, want ErrIdempotencyConflict", err)
	}
}

// ---- LC-004: runtime role cannot bypass append-only -------------------------

func TestRuntimeRoleCannotBypassAppendOnly(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	tx := postedTx(f, "no-bypass-"+uuid.NewString(), 1000)
	if _, _, err := s.CreateTransaction(ctx, tx); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Setting the maintenance GUC as the runtime role must NOT enable deletes
	// or edits: the guard now also requires current_user = ledgercore_maint.
	attempts := []struct {
		name string
		sql  string
	}{
		{"delete postings", `DELETE FROM postings WHERE transaction_id = $1`},
		{"update postings", `UPDATE postings SET amount = amount + 1 WHERE transaction_id = $1`},
		{"delete transactions", `DELETE FROM transactions WHERE id = $1`},
	}
	for _, a := range attempts {
		err := pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(dbtx pgx.Tx) error {
			if _, err := dbtx.Exec(ctx, `SET LOCAL ledger.allow_maintenance = '1'`); err != nil {
				return err
			}
			_, err := dbtx.Exec(ctx, a.sql, tx.ID)
			return err
		})
		if err == nil {
			t.Errorf("%s: runtime role bypassed the append-only guard via the GUC (LC-004 regression)", a.name)
		}
	}
}

// ---- LC-004: sanctioned purge (runs only when infra provisioned maint) ------

func TestPurgeExpiredSandboxTenant(t *testing.T) {
	s, pool := newTestStore(t)
	ctx := context.Background()

	// Skip unless ledgercore_maint owns the purge function (infra step).
	var configured bool
	if err := pgxutil.WithSystemTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_proc p
				JOIN pg_roles r ON r.oid = p.proowner
				WHERE p.proname = 'purge_expired_sandbox_tenant'
				  AND r.rolname = 'ledgercore_maint'
			)`).Scan(&configured)
	}); err != nil {
		t.Fatalf("probe purge function owner: %v", err)
	}
	if !configured {
		t.Skip("ledgercore_maint does not yet own purge_expired_sandbox_tenant; infra coordination pending")
	}

	f := newFixture(t, s) // sandbox ledger
	if _, _, err := s.CreateTransaction(ctx, postedTx(f, "purge-"+uuid.NewString(), 10000)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	deleted, err := natsconsumer.PurgeTenant(ctx, pool, f.tenantID)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted == 0 {
		t.Error("purge deleted 0 rows, want > 0")
	}
	// Idempotent: a second purge finds nothing.
	if again, err := natsconsumer.PurgeTenant(ctx, pool, f.tenantID); err != nil || again != 0 {
		t.Errorf("second purge = (%d, %v), want (0, nil)", again, err)
	}
}

// ---- Verifier: postings vs account_balances ---------------------------------

func TestVerifyBalancesDetectsDrift(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	if _, _, err := s.CreateTransaction(ctx, postedTx(f, "verify-"+uuid.NewString(), 10000)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Healthy: no discrepancies.
	disc, err := s.VerifyBalances(ctx, f.tenantID, f.ledger.ID)
	if err != nil {
		t.Fatalf("verify (healthy): %v", err)
	}
	if len(disc) != 0 {
		t.Fatalf("healthy ledger reports %d discrepancies: %+v", len(disc), disc)
	}

	// Corrupt the derived table directly (account_balances has no append-only
	// guard). The verifier must catch the drift from the source postings.
	if err := pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE account_balances SET posted_debits = posted_debits + 999
			WHERE account_id = $1 AND asset = 'USD'`, f.cash.ID)
		return err
	}); err != nil {
		t.Fatalf("inject drift: %v", err)
	}

	disc, err = s.VerifyBalances(ctx, f.tenantID, f.ledger.ID)
	if err != nil {
		t.Fatalf("verify (drifted): %v", err)
	}
	if len(disc) != 1 || disc[0].AccountID != f.cash.ID {
		t.Fatalf("want exactly 1 discrepancy on cash, got %+v", disc)
	}
	if disc[0].StoredPostedDebits-disc[0].ComputedPostedDebits != 999 {
		t.Errorf("expected stored-computed drift of 999, got stored=%d computed=%d",
			disc[0].StoredPostedDebits, disc[0].ComputedPostedDebits)
	}
}

// mustStore returns a store, skipping if no test DB is configured.
func mustStore(t *testing.T) *Store {
	t.Helper()
	s, _ := newTestStore(t)
	return s
}
