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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ASanchezT85/ledgercore/libs/go/money"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/adapters/natsconsumer"
	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/app"
	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/domain"
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

// TestBalanceGuardIsSearchPathIndependent pins the fix from migration 0009.
//
// The guard functions used to resolve `transactions` / `postings` and
// `check_transaction_balance` through the CALLER's search_path. From a session
// that did not have `ledger` on its path — a psql session, a restore, a
// maintenance script — the COMMIT failed with
// `function check_transaction_balance(uuid) does not exist` instead of running
// the balance check, and a caller able to create objects earlier on the path
// could have had the guard validate the wrong tables.
//
// TestDoubleEntryConstraintRejectsUnbalancedDirectSQL cannot catch this: it
// runs as the runtime role, whose search_path already contains `ledger`. This
// test forces the opposite condition and asserts the guard still reports the
// real domain violation.
func TestBalanceGuardIsSearchPathIndependent(t *testing.T) {
	s, pool := newTestStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	err := pgxutil.WithTenantTx(ctx, pool, f.tenantID, func(tx pgx.Tx) error {
		// Take `ledger` off the path for this transaction. Every statement
		// below is schema-qualified, so only the guard's own resolution is
		// under test.
		if _, err := tx.Exec(ctx, `SET LOCAL search_path = pg_catalog`); err != nil {
			return err
		}
		txID := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger.transactions (id, tenant_id, ledger_id, idempotency_key, status, effective_at, created_at)
			VALUES ($1, $2, $3, $4, 'posted', now(), now())`,
			txID, f.tenantID, f.ledger.ID, "searchpath-unbalanced-"+uuid.NewString()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger.postings (id, tenant_id, transaction_id, account_id, direction, asset, amount, created_at)
			VALUES ($1, $2, $3, $4, 'DEBIT', 'USD', 100, now())`,
			uuid.New(), f.tenantID, txID, f.cash.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger.postings (id, tenant_id, transaction_id, account_id, direction, asset, amount, created_at)
			VALUES ($1, $2, $3, $4, 'CREDIT', 'USD', 50, now())`,
			uuid.New(), f.tenantID, txID, f.wallet.ID); err != nil {
			return err
		}
		return nil // COMMIT — the deferred guard fires here.
	})
	if err == nil {
		t.Fatal("unbalanced COMMIT must be rejected regardless of the caller's search_path")
	}

	// The point of the test: it must fail for the RIGHT reason. An
	// undefined_function (42883) here means the guard never ran the check.
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected a PostgreSQL error, got %T: %v", err, err)
	}
	if pgErr.Code == "42883" {
		t.Fatalf("guard did not resolve its own function (search_path leak): %v", pgErr)
	}
	if pgErr.Code != "23514" { // check_violation, raised by check_transaction_balance
		t.Fatalf("expected check_violation (23514) from the balance guard, got %s: %v", pgErr.Code, pgErr)
	}
	t.Logf("balance guard reported the real violation with ledger off the path: %s", pgErr.Message)
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

// ---- R-008: effective_at / expires_at are part of the fingerprint ----------

func TestIdempotencyFingerprintSemanticFields(t *testing.T) {
	s := mustStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	t1 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := time.Date(2026, 6, 7, 8, 9, 10, 0, time.UTC)

	// txWith builds an otherwise-identical posted transaction with a specific,
	// client-PROVIDED effective_at.
	txWith := func(key string, effAt time.Time) domain.Transaction {
		tx := postedTx(f, key, 10000)
		tx.EffectiveAt = effAt
		tx.EffectiveAtProvided = true
		return tx
	}

	// (a) same key + same explicit effective_at -> replay.
	key := "idem-eff-" + uuid.NewString()
	if _, replayed, err := s.CreateTransaction(ctx, txWith(key, t1)); err != nil || replayed {
		t.Fatalf("first create with explicit effective_at: replayed=%v err=%v", replayed, err)
	}
	if _, replayed, err := s.CreateTransaction(ctx, txWith(key, t1)); err != nil || !replayed {
		t.Fatalf("same explicit effective_at must replay: replayed=%v err=%v", replayed, err)
	}
	// (b) same key + DIFFERENT explicit effective_at -> 409.
	if _, _, err := s.CreateTransaction(ctx, txWith(key, t2)); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatalf("different effective_at reuse: err=%v, want ErrIdempotencyConflict", err)
	}
	// (c) same key, one OMITTED vs one PROVIDED -> 409 (distinguishes absent/value).
	keyO := "idem-eff-omit-" + uuid.NewString()
	if _, _, err := s.CreateTransaction(ctx, postedTx(f, keyO, 10000)); err != nil { // omitted
		t.Fatalf("create with omitted effective_at: %v", err)
	}
	if _, _, err := s.CreateTransaction(ctx, txWith(keyO, t1)); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatalf("omitted-then-provided effective_at: err=%v, want ErrIdempotencyConflict", err)
	}

	// Holds: expires_at is part of the fingerprint too.
	if _, _, err := s.CreateTransaction(ctx, postedTx(f, "fund-"+uuid.NewString(), 10000)); err != nil {
		t.Fatalf("fund: %v", err)
	}
	holdWith := func(key string, exp time.Time, provided bool) domain.Hold {
		return domain.Hold{
			ID: uuid.New(), TenantID: f.tenantID, AccountID: f.wallet.ID,
			IdempotencyKey: key, Amount: money.Amount{Asset: "USD", Units: 500},
			Status: domain.HoldActive, ExpiresAt: exp, ExpiresAtProvided: provided,
			CreatedAt: time.Now().UTC(),
		}
	}
	hk := "idem-hold-exp-" + uuid.NewString()
	e1 := time.Now().UTC().Add(2 * time.Hour)
	e2 := time.Now().UTC().Add(48 * time.Hour)
	if _, replayed, err := s.CreateHold(ctx, holdWith(hk, e1, true)); err != nil || replayed {
		t.Fatalf("first hold with explicit expires_at: replayed=%v err=%v", replayed, err)
	}
	if _, replayed, err := s.CreateHold(ctx, holdWith(hk, e1, true)); err != nil || !replayed {
		t.Fatalf("same explicit expires_at must replay: replayed=%v err=%v", replayed, err)
	}
	if _, _, err := s.CreateHold(ctx, holdWith(hk, e2, true)); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatalf("different expires_at reuse: err=%v, want ErrIdempotencyConflict", err)
	}
}

// ---- R-008: reverse is idempotent by fingerprint ----------------------------

func TestReverseIdempotencyFingerprint(t *testing.T) {
	s := mustStore(t)
	f := newFixture(t, s)
	ctx := context.Background()

	tx := postedTx(f, "rev-idem-"+uuid.NewString(), 10000)
	if _, _, err := s.CreateTransaction(ctx, tx); err != nil {
		t.Fatalf("create: %v", err)
	}

	key := "rev-key-" + uuid.NewString()
	rev, replayed, err := s.ReverseTransaction(ctx, f.tenantID, tx.ID, key, "customer refund", time.Now().UTC())
	if err != nil || replayed {
		t.Fatalf("first reverse: replayed=%v err=%v", replayed, err)
	}
	// Same key + same reason -> replay the stored reversal.
	if again, replayed, err := s.ReverseTransaction(ctx, f.tenantID, tx.ID, key, "customer refund", time.Now().UTC()); err != nil || !replayed || again.ID != rev.ID {
		t.Fatalf("same reversal retry = (%v, replayed=%v, %v), want original id and replayed=true", again.ID, replayed, err)
	}
	// Same key + DIFFERENT reason -> 409 conflict.
	if _, _, err := s.ReverseTransaction(ctx, f.tenantID, tx.ID, key, "different reason", time.Now().UTC()); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatalf("reversal key reused with different reason: err=%v, want ErrIdempotencyConflict", err)
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

	// The sanctioned purge is only meaningful under the real role model:
	// ledgercore_maint must OWN the SECURITY DEFINER function (so current_user
	// becomes ledgercore_maint inside it and the append-only bypass engages),
	// and grants.sql must have granted EXECUTE to the runtime role. We do NOT
	// silently skip when this is missing (that is exactly how R-005 hid): if
	// the environment is expected to be provisioned we FAIL with actionable
	// guidance. Provision it by running with LEDGERCORE_TEST_ADMIN_URL set (see
	// main_test.go) or by applying infra/postgres/init + migrate/grants.sql.
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
		t.Fatal("purge_expired_sandbox_tenant is not owned by ledgercore_maint: the real role model is not provisioned. " +
			"Run the integration suite with LEDGERCORE_TEST_ADMIN_URL set to a superuser DSN (see main_test.go), " +
			"or apply infra/postgres/init/01-init.sql + infra/postgres/migrate/grants.sql before testing. Refusing to skip (R-005).")
	}

	f := newFixture(t, s) // sandbox ledger
	// Seed a hold too, so the purge has to clear more than transactions.
	if _, _, err := s.CreateTransaction(ctx, postedTx(f, "purge-"+uuid.NewString(), 10000)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, _, err := s.CreateHold(ctx, domain.Hold{
		ID: uuid.New(), TenantID: f.tenantID, AccountID: f.wallet.ID,
		IdempotencyKey: "purge-hold-" + uuid.NewString(),
		Amount:         money.Amount{Asset: "USD", Units: 100},
		Status:         domain.HoldActive, ExpiresAt: time.Now().UTC().Add(time.Hour),
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed hold: %v", err)
	}

	// Count real rows before the purge, under this tenant's RLS context.
	before := countTenantRows(t, pool, f.tenantID)
	if before == 0 {
		t.Fatal("precondition: expected seeded rows for the tenant, found none")
	}

	deleted, err := natsconsumer.PurgeTenant(ctx, pool, f.tenantID)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted == 0 {
		t.Fatal("purge deleted 0 rows, want > 0 — the sanctioned purge is a no-op under FORCE RLS (R-005 regression)")
	}

	// Prove the rows are actually GONE (not merely a non-zero return value).
	if after := countTenantRows(t, pool, f.tenantID); after != 0 {
		t.Fatalf("after purge the tenant still owns %d rows across ledger tables, want 0 (R-005)", after)
	}
	t.Logf("R-005 purge deleted %d rows; tenant now owns 0 rows across all ledger tables", deleted)

	// Idempotent: a second purge finds nothing.
	if again, err := natsconsumer.PurgeTenant(ctx, pool, f.tenantID); err != nil || again != 0 {
		t.Errorf("second purge = (%d, %v), want (0, nil)", again, err)
	}
}

// countTenantRows totals the rows a tenant owns across every ledger table,
// read under that tenant's RLS context so it reflects exactly what the purge
// is meant to remove.
func countTenantRows(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) int64 {
	t.Helper()
	var total int64
	err := pgxutil.WithTenantTx(context.Background(), pool, tenantID, func(tx pgx.Tx) error {
		for _, table := range []string{
			"ledgers", "accounts", "transactions", "postings",
			"account_balances", "holds", "idempotency_keys", "outbox",
		} {
			var n int64
			if err := tx.QueryRow(context.Background(),
				`SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil {
				return err
			}
			total += n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("count tenant rows: %v", err)
	}
	return total
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
