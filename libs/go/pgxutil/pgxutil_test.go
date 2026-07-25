package pgxutil

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestWithTenantTxRejectsNilTenant(t *testing.T) {
	// Pool is never touched when the tenant id is nil, so nil is fine here.
	err := WithTenantTx(context.Background(), nil, uuid.Nil, func(pgx.Tx) error { return nil })
	if err == nil {
		t.Fatal("expected error for nil tenant id")
	}
}

func TestNewPoolRejectsBadURL(t *testing.T) {
	if _, err := NewPool(context.Background(), "://not-a-url", "ledger"); err == nil {
		t.Fatal("expected error for malformed url")
	}
}

// Integration test: requires a live PostgreSQL. Skipped unless
// LEDGERCORE_TEST_DATABASE_URL is set.
func TestWithTenantTxIntegration(t *testing.T) {
	url := os.Getenv("LEDGERCORE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LEDGERCORE_TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := NewPool(ctx, url, "public")
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	tenant := uuid.New()
	err = WithTenantTx(ctx, pool, tenant, func(tx pgx.Tx) error {
		var got string
		if err := tx.QueryRow(ctx, "SELECT current_setting('app.tenant_id', true)").Scan(&got); err != nil {
			return err
		}
		if got != tenant.String() {
			t.Fatalf("app.tenant_id = %q, want %q", got, tenant)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTenantTx: %v", err)
	}

	// Outside the transaction the setting must be gone.
	var after *string
	if err := pool.QueryRow(ctx, "SELECT nullif(current_setting('app.tenant_id', true), '')").Scan(&after); err != nil {
		t.Fatalf("query after tx: %v", err)
	}
	if after != nil {
		t.Fatalf("app.tenant_id leaked outside the tx: %q", *after)
	}
}
