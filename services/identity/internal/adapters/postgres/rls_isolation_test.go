package postgres

// LC-002/LC-014 anti-leak RLS contract test for the identity schema.
//
// identity.api_keys is the one tenant-scoped table in this schema (tenants,
// signing_keys, sandbox_signups and outbox are legitimately system-level and
// carry no RLS by design). Because the app role OWNS the schema, RLS only
// bites with FORCE ROW LEVEL SECURITY (migration 0003) — this test proves it
// does:
//   - a tenant context cannot SELECT another tenant's keys,
//   - a tenant context cannot INSERT a row for another tenant (WITH CHECK),
//   - the system path (no tenant context) still sees keys, so token issuance
//     and admin bootstrap keep working.
//
// Requires LEDGERCORE_TEST_DATABASE_URL and a NOBYPASSRLS role (skips
// otherwise).

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

func TestAPIKeyRLSCrossTenantIsolation(t *testing.T) {
	url := os.Getenv("LEDGERCORE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LEDGERCORE_TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := pgxutil.NewPool(ctx, url, "identity")
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var bypass bool
	if err := pool.QueryRow(ctx,
		"SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&bypass); err != nil {
		t.Fatalf("check role: %v", err)
	}
	if bypass {
		t.Skip("connected role bypasses RLS; run as ledgercore_app (NOBYPASSRLS) to verify isolation")
	}

	store := NewStore(pool)

	// Tenant A owns one API key. Created via the store (system path, no tenant
	// context) exactly like the real service does.
	tenantA := mustV7(t)
	tenantB := mustV7(t)
	secret, err := domain.NewAPIKeySecret(domain.EnvironmentSandbox)
	if err != nil {
		t.Fatal(err)
	}
	tenant := domain.Tenant{
		ID: tenantA, Name: "rls-A", Slug: "rls-a-" + uuid.NewString()[:8],
		Status: domain.TenantStatusActive, CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	keyID := mustV7(t)
	key := domain.APIKey{
		ID: keyID, TenantID: tenantA, Environment: domain.EnvironmentSandbox,
		Name: "rls-key", KeyPrefix: domain.PrefixOf(secret), SecretHash: domain.HashSecret(secret),
		Scopes: []string{"ledger:read"}, CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	t.Cleanup(func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		_, _ = pool.Exec(c, "DELETE FROM api_keys WHERE id = $1", keyID)
		_, _ = pool.Exec(c, "DELETE FROM tenants WHERE id = $1", tenantA)
	})

	count := func(t *testing.T, tenantCtx uuid.UUID) int {
		t.Helper()
		var n int
		if err := pgxutil.WithTenantTx(ctx, pool, tenantCtx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM api_keys WHERE id = $1", keyID).Scan(&n)
		}); err != nil {
			t.Fatalf("count under tenant %s: %v", tenantCtx, err)
		}
		return n
	}

	t.Run("tenant B cannot read tenant A key", func(t *testing.T) {
		if n := count(t, tenantB); n != 0 {
			t.Errorf("tenant B saw %d of tenant A's keys, want 0", n)
		}
	})

	t.Run("tenant A sees its own key", func(t *testing.T) {
		if n := count(t, tenantA); n != 1 {
			t.Errorf("tenant A saw %d of its own keys, want 1", n)
		}
	})

	t.Run("system path sees the key (issuance still works)", func(t *testing.T) {
		found, err := store.FindAPIKeysByPrefix(ctx, key.KeyPrefix)
		if err != nil {
			t.Fatalf("find by prefix: %v", err)
		}
		var seen bool
		for _, k := range found {
			if k.ID == keyID {
				seen = true
			}
		}
		if !seen {
			t.Error("system path could not see the key; token issuance would break")
		}
	})

	t.Run("tenant B cannot insert a row for tenant A (WITH CHECK)", func(t *testing.T) {
		intruderID := mustV7(t)
		err := pgxutil.WithTenantTx(ctx, pool, tenantB, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx,
				`INSERT INTO api_keys (id, tenant_id, environment, name, key_prefix, secret_hash, scopes, created_at)
				 VALUES ($1, $2, 'sandbox', 'intruder', 'lk_sandbox_x', $3, '{}', now())`,
				intruderID, tenantA, domain.HashSecret("x"))
			return e
		})
		if err == nil {
			_, _ = pool.Exec(ctx, "DELETE FROM api_keys WHERE id = $1", intruderID)
			t.Fatal("tenant B inserted a key for tenant A; WITH CHECK is not enforced")
		}
	})
}

func mustV7(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
