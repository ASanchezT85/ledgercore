package postgres

// LC-002 anti-leak RLS contract test for the webhooks schema.
//
// Tenant A owns a subscription and a delivery; tenant B must not be able to
// read or modify either through the tenant-scoped repository. Mirrors the
// ledger-core RLS isolation gate. Requires LEDGERCORE_TEST_DATABASE_URL and a
// pool connected as the NOBYPASSRLS app role (the production setup); it is
// meaningful only because migration 0003 FORCEs RLS on both tables.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/app"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/domain"
)

func TestRLSCrossTenantIsolation(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool, nil)
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New() // intruder, zero data

	sub := domain.Subscription{
		ID:         uuid.New(),
		TenantID:   tenantA,
		URL:        "https://a.example.com/hooks",
		Secret:     "whsec_tenantAsecret0000000000000000",
		EventTypes: []string{"*"},
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}
	if err := repo.Create(ctx, sub); err != nil {
		t.Fatalf("tenant A create subscription: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{"type": "ledger.transaction.posted"})
	del := domain.Delivery{
		ID:             uuid.New(),
		TenantID:       tenantA,
		SubscriptionID: sub.ID,
		EventID:        uuid.New(),
		EventType:      "ledger.transaction.posted",
		Payload:        payload,
		Status:         domain.StatusPending,
		NextAttemptAt:  time.Now().UTC(),
		CreatedAt:      time.Now().UTC(),
	}
	if err := repo.InsertPending(ctx, tenantA, []domain.Delivery{del}); err != nil {
		t.Fatalf("tenant A insert delivery: %v", err)
	}

	t.Run("cross-tenant reads see nothing", func(t *testing.T) {
		if _, err := repo.Get(ctx, tenantB, sub.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Get: want ErrNotFound, got %v", err)
		}
		if got, err := repo.List(ctx, tenantB, 50, httpx.Cursor{}); err != nil || len(got) != 0 {
			t.Errorf("List: want 0 rows nil err, got %d rows err=%v", len(got), err)
		}
		if got, err := repo.ListActive(ctx, tenantB); err != nil || len(got) != 0 {
			t.Errorf("ListActive: want 0 rows nil err, got %d rows err=%v", len(got), err)
		}
		got, _, err := repo.ListDeliveries(ctx, tenantB, app.DeliveryFilter{Limit: 50})
		if err != nil || len(got) != 0 {
			t.Errorf("ListDeliveries: want 0 rows nil err, got %d rows err=%v", len(got), err)
		}
	})

	t.Run("cross-tenant writes fail closed", func(t *testing.T) {
		active := false
		if _, err := repo.Update(ctx, tenantB, sub.ID, nil, nil, &active); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Update foreign sub: want ErrNotFound, got %v", err)
		}
		if err := repo.RotateSecret(ctx, tenantB, sub.ID, "whsec_hijack00000000000000000000000", time.Now().UTC().Add(time.Hour)); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("RotateSecret foreign sub: want ErrNotFound, got %v", err)
		}
		if _, err := repo.Requeue(ctx, tenantB, del.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Requeue foreign delivery: want ErrNotFound, got %v", err)
		}
		if err := repo.MarkDelivered(ctx, tenantB, del.ID, 200); err != nil {
			t.Errorf("MarkDelivered foreign delivery: unexpected err %v", err)
		}
	})

	t.Run("no permissive cross-tenant policy for the runtime role", func(t *testing.T) {
		// R-004: with system_drain removed, a tenant-context-free query on the
		// runtime/owner connection (NOT the maintenance identity) must see zero
		// rows — deny by default — instead of every tenant's data. This is the
		// exact escalation path the old permissive policy allowed.
		var subCount, delCount int
		if err := pgxutil.WithSystemTx(ctx, pool, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM subscriptions").Scan(&subCount); err != nil {
				return err
			}
			return tx.QueryRow(ctx, "SELECT count(*) FROM deliveries").Scan(&delCount)
		}); err != nil {
			t.Fatalf("system-tx count: %v", err)
		}
		if subCount != 0 || delCount != 0 {
			t.Fatalf("cross-tenant leak without tenant context: %d subscriptions, %d deliveries visible (want 0/0)", subCount, delCount)
		}
	})

	t.Run("sanctioned maintenance path still spans tenants", func(t *testing.T) {
		// The dispatcher's ClaimDue goes through the SECURITY DEFINER function
		// (owned by ledgercore_maint), so it still sees tenant A's due delivery
		// even with no tenant context. This proves the cross-tenant capability
		// moved from a blanket policy to the auditable maintenance function.
		if !maintRoleExists(ctx, pool) {
			t.Skip("role model not provisioned; maintenance path exercised under the full role model")
		}
		claimed, err := repo.ClaimDue(ctx, 10, time.Minute)
		if err != nil {
			t.Fatalf("claim due via maintenance function: %v", err)
		}
		if len(claimed) != 1 || claimed[0].TenantID != tenantA {
			t.Fatalf("maintenance claim mismatch: %+v", claimed)
		}
	})

	// Sanity: tenant A is unaffected — the delivery is still pending and the
	// subscription still active (the foreign writes above changed nothing).
	got, err := repo.Get(ctx, tenantA, sub.ID)
	if err != nil {
		t.Fatalf("tenant A lost access to its own subscription: %v", err)
	}
	if !got.Active {
		t.Fatal("foreign Update leaked: tenant A subscription was deactivated")
	}
	page, _, err := repo.ListDeliveries(ctx, tenantA, app.DeliveryFilter{Status: domain.StatusPending, Limit: 10})
	if err != nil || len(page) != 1 {
		t.Fatalf("foreign MarkDelivered leaked: want 1 pending, got %d (err %v)", len(page), err)
	}
}
