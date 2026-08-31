package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/app"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/domain"
)

// testPool skips the test unless LEDGERCORE_TEST_DATABASE_URL is set, per
// the platform convention for integration tests.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("LEDGERCORE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LEDGERCORE_TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxutil.NewPool(ctx, url, SchemaName)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	// When TestMain provisioned the real role model, migrations already ran as
	// the migrator and the grants were applied as superuser; `pool` is the
	// (DDL-less) runtime role, which can neither migrate nor reassign function
	// ownership. Skip both — they are already done.
	if !roleModelProvisioned {
		if err := Migrate(ctx, pool); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		applyMaintGrants(t, pool)
	}
	// Clean slate for this run. The cross-tenant DELETE runs under the
	// maintenance identity (no permissive policy exists anymore, R-004).
	if err := withMaintTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM deliveries"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM subscriptions")
		return err
	}); err != nil {
		t.Fatalf("clean tables: %v", err)
	}
	return pool
}

// maintRoleExists reports whether the ledgercore_maint role is provisioned in
// the connected cluster (it is under the production/CI role model; a bare dev
// superuser cluster may lack it).
func maintRoleExists(ctx context.Context, pool *pgxpool.Pool) bool {
	var exists bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ledgercore_maint')").Scan(&exists); err != nil {
		return false
	}
	return exists
}

// applyMaintGrants simulates the infra post-migration step
// (infra/postgres/migrate/grants.sql): it reassigns every SECURITY DEFINER
// function in the webhooks schema to ledgercore_maint and grants the privileges
// the maintenance functions need (EXECUTE to the runtime roles, plus UPDATE on
// the two tables — init grants maint only SELECT + DELETE). Without this the
// R-004 functions correctly fail closed. No-op when the role model is absent
// (e.g. a superuser dev cluster that bypasses RLS anyway).
func applyMaintGrants(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if !maintRoleExists(ctx, pool) {
		return
	}
	stmts := []string{
		`DO $$
		 DECLARE fn text;
		 BEGIN
		     FOR fn IN
		         SELECT p.oid::regprocedure::text
		         FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		         WHERE n.nspname = 'webhooks' AND p.prosecdef
		     LOOP
		         EXECUTE format('ALTER FUNCTION %s OWNER TO ledgercore_maint', fn);
		         EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO ledgercore_webhooks_rt', fn);
		     END LOOP;
		 END $$;`,
		`GRANT UPDATE ON webhooks.subscriptions, webhooks.deliveries TO ledgercore_maint`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("apply maint grants: %v", err)
		}
	}
}

// withMaintTx runs fn inside a transaction under the ledgercore_maint identity
// (SET LOCAL ROLE), so cross-tenant maintenance reads/writes in tests match the
// sanctioned SECURITY DEFINER path. Falls back to a plain system transaction
// when the role model is absent.
func withMaintTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	if !maintRoleExists(ctx, pool) {
		return pgxutil.WithSystemTx(ctx, pool, fn)
	}
	return pgxutil.WithSystemTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE ledgercore_maint"); err != nil {
			return err
		}
		return fn(tx)
	})
}

func TestRepoEndToEnd(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool, nil)
	ctx := context.Background()
	tenantID := uuid.New()

	// Create + read back a subscription.
	sub := domain.Subscription{
		ID:         uuid.New(),
		TenantID:   tenantID,
		URL:        "https://client.example.com/hooks",
		Secret:     "whsec_integrationtestsecret00000000",
		EventTypes: []string{"ledger.transaction.posted", "*"},
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}
	if err := repo.Create(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	got, err := repo.Get(ctx, tenantID, sub.ID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if got.URL != sub.URL || len(got.EventTypes) != 2 || !got.Active {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Tenant isolation at the query level: another tenant cannot see it.
	if _, err := repo.Get(ctx, uuid.New(), sub.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant read must be ErrNotFound, got %v", err)
	}

	// Idempotent enqueue: the same (subscription, event) inserts once.
	eventID := uuid.New()
	payload, _ := json.Marshal(map[string]string{"type": "ledger.transaction.posted"})
	mkDelivery := func() domain.Delivery {
		now := time.Now().UTC()
		return domain.Delivery{
			ID:             uuid.New(),
			TenantID:       tenantID,
			SubscriptionID: sub.ID,
			EventID:        eventID,
			EventType:      "ledger.transaction.posted",
			Payload:        payload,
			Status:         domain.StatusPending,
			NextAttemptAt:  now,
			CreatedAt:      now,
		}
	}
	if err := repo.InsertPending(ctx, tenantID, []domain.Delivery{mkDelivery()}); err != nil {
		t.Fatalf("insert pending: %v", err)
	}
	if err := repo.InsertPending(ctx, tenantID, []domain.Delivery{mkDelivery()}); err != nil {
		t.Fatalf("second insert must be a no-op, got: %v", err)
	}
	page, _, err := repo.ListDeliveries(ctx, tenantID, app.DeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("expected exactly 1 delivery after duplicate insert, got %d", len(page))
	}
	deliveryID := page[0].ID

	// Claim leases the due delivery and hides it from a second claim.
	claimed, err := repo.ClaimDue(ctx, 10, 2*time.Minute)
	if err != nil {
		t.Fatalf("claim due: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != deliveryID {
		t.Fatalf("claim mismatch: %+v", claimed)
	}
	if claimed[0].URL != sub.URL || claimed[0].Secret != sub.Secret {
		t.Fatalf("claim must join endpoint data: %+v", claimed[0])
	}
	again, err := repo.ClaimDue(ctx, 10, 2*time.Minute)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("leased delivery must not be claimable again, got %d", len(again))
	}

	// A failed attempt reschedules; then requeue makes it pending again.
	code := 500
	if err := repo.MarkFailed(ctx, tenantID, deliveryID, 1, &code, "endpoint returned status 500",
		time.Now().UTC().Add(time.Minute), false); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	failedPage, _, err := repo.ListDeliveries(ctx, tenantID, app.DeliveryFilter{Status: domain.StatusFailed, Limit: 10})
	if err != nil || len(failedPage) != 1 {
		t.Fatalf("expected 1 failed delivery, got %d (err %v)", len(failedPage), err)
	}
	requeued, err := repo.Requeue(ctx, tenantID, deliveryID)
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if requeued.Status != domain.StatusPending || requeued.Attempts != 0 {
		t.Fatalf("requeue must reset status/attempts: %+v", requeued)
	}

	// Requeueing a pending delivery conflicts.
	if _, err := repo.Requeue(ctx, tenantID, deliveryID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("requeue of pending must be ErrConflict, got %v", err)
	}

	// Delivered path.
	claimed, err = repo.ClaimDue(ctx, 10, 2*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim after requeue: %v (%d)", err, len(claimed))
	}
	if err := repo.MarkDelivered(ctx, tenantID, deliveryID, 200); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	done, _, err := repo.ListDeliveries(ctx, tenantID, app.DeliveryFilter{Status: domain.StatusDelivered, Limit: 10})
	if err != nil || len(done) != 1 {
		t.Fatalf("expected 1 delivered, got %d (err %v)", len(done), err)
	}
	if done[0].DeliveredAt == nil || done[0].LastStatusCode == nil || *done[0].LastStatusCode != 200 {
		t.Fatalf("delivered bookkeeping incomplete: %+v", done[0])
	}

	// Rotate secret: the old one must survive as previous_secret until the
	// grace deadline, and the purge must clear it once expired.
	oldSecret := got.Secret
	prevExpires := time.Now().UTC().Add(domain.RotationGrace)
	if err := repo.RotateSecret(ctx, tenantID, sub.ID, "whsec_rotated0000000000000000000000", prevExpires); err != nil {
		t.Fatalf("rotate secret: %v", err)
	}
	got, err = repo.Get(ctx, tenantID, sub.ID)
	if err != nil || got.Secret != "whsec_rotated0000000000000000000000" {
		t.Fatalf("secret not rotated: %v (%+v)", err, got)
	}
	if got.PreviousSecret == nil || *got.PreviousSecret != oldSecret || got.PreviousSecretExpiresAt == nil {
		t.Fatalf("previous secret not retained: %+v", got)
	}
	if secrets := got.SigningSecrets(time.Now()); len(secrets) != 2 {
		t.Fatalf("expected 2 signing secrets during grace, got %v", secrets)
	}
	// Not yet expired: the purge must leave it alone.
	if _, err := repo.PurgeExpiredPreviousSecrets(ctx); err != nil {
		t.Fatalf("purge: %v", err)
	}
	got, _ = repo.Get(ctx, tenantID, sub.ID)
	if got.PreviousSecret == nil {
		t.Fatalf("purge removed a still-valid previous secret")
	}
	// Force-expire and purge.
	if err := repo.RotateSecret(ctx, tenantID, sub.ID, got.Secret, time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatalf("re-rotate: %v", err)
	}
	if n, err := repo.PurgeExpiredPreviousSecrets(ctx); err != nil || n != 1 {
		t.Fatalf("purge expired: n=%d err=%v", n, err)
	}
	got, _ = repo.Get(ctx, tenantID, sub.ID)
	if got.PreviousSecret != nil || got.PreviousSecretExpiresAt != nil {
		t.Fatalf("expired previous secret not purged: %+v", got)
	}

	// Deactivated subscriptions are excluded from claims.
	inactive := false
	if _, err := repo.Update(ctx, tenantID, sub.ID, nil, nil, &inactive); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := repo.Requeue(ctx, tenantID, deliveryID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("delivered must not requeue, got %v", err)
	}
}
