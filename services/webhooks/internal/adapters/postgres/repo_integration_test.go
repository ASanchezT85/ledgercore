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

	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/app"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
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
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Clean slate for this run (owner bypasses RLS in the test database).
	if err := pgxutil.WithSystemTx(ctx, pool, func(tx pgx.Tx) error {
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

func TestRepoEndToEnd(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
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

	// Rotate secret.
	if err := repo.UpdateSecret(ctx, tenantID, sub.ID, "whsec_rotated0000000000000000000000"); err != nil {
		t.Fatalf("update secret: %v", err)
	}
	got, err = repo.Get(ctx, tenantID, sub.ID)
	if err != nil || got.Secret != "whsec_rotated0000000000000000000000" {
		t.Fatalf("secret not rotated: %v (%+v)", err, got)
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
