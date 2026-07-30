package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

// TestStoreIntegration exercises the real repositories against PostgreSQL.
// It is skipped unless LEDGERCORE_TEST_DATABASE_URL is set.
func TestStoreIntegration(t *testing.T) {
	url := os.Getenv("LEDGERCORE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LEDGERCORE_TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := pgxutil.NewPool(ctx, url, "identity")
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if !roleModelProvisioned {
		if err := Migrate(ctx, pool); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}

	store := NewStore(pool)
	suffix := uuid.NewString()[:8]

	// ---- Tenants ----
	tenantID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	tenant := domain.Tenant{
		ID:        tenantID,
		Name:      "Integration Tenant",
		Slug:      "it-" + suffix,
		Status:    domain.TenantStatusActive,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM api_keys WHERE tenant_id = $1", tenant.ID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM tenants WHERE id = $1", tenant.ID)
	})

	dupID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	dup := tenant
	dup.ID = dupID
	if err := store.CreateTenant(ctx, dup); !errors.Is(err, domain.ErrSlugConflict) {
		t.Errorf("duplicate slug: got %v, want ErrSlugConflict", err)
	}

	got, err := store.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.Slug != tenant.Slug || got.Status != domain.TenantStatusActive {
		t.Errorf("round-tripped tenant mismatch: %+v", got)
	}
	if _, err := store.GetTenant(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing tenant: got %v, want ErrNotFound", err)
	}
	tenants, err := store.ListTenants(ctx, 50, httpx.Cursor{})
	if err != nil || len(tenants) == 0 {
		t.Errorf("list tenants: len=%d err=%v", len(tenants), err)
	}

	// ---- API keys ----
	secret, err := domain.NewAPIKeySecret(domain.EnvironmentSandbox)
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	key := domain.APIKey{
		ID:          keyID,
		TenantID:    tenant.ID,
		Environment: domain.EnvironmentSandbox,
		Name:        "integration-key",
		KeyPrefix:   domain.PrefixOf(secret),
		SecretHash:  domain.HashSecret(secret),
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	found, err := store.FindAPIKeysByPrefix(ctx, key.KeyPrefix)
	if err != nil {
		t.Fatalf("find by prefix: %v", err)
	}
	var match *domain.APIKey
	for i := range found {
		if found[i].ID == key.ID {
			match = &found[i]
		}
	}
	if match == nil {
		t.Fatalf("key %s not found by prefix %s", key.ID, key.KeyPrefix)
	}
	if !match.VerifySecret(secret) {
		t.Error("persisted hash does not verify the secret")
	}
	if match.Revoked() {
		t.Error("fresh key reported as revoked")
	}

	if err := store.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := store.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Errorf("double revoke should be a no-op, got %v", err)
	}
	if err := store.RevokeAPIKey(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("revoke missing key: got %v, want ErrNotFound", err)
	}
	found, err = store.FindAPIKeysByPrefix(ctx, key.KeyPrefix)
	if err != nil {
		t.Fatal(err)
	}
	for i := range found {
		if found[i].ID == key.ID && !found[i].Revoked() {
			t.Error("revoked key still reported active")
		}
	}

	// ---- Signing keys ----
	sk, err := domain.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertSigningKey(ctx, sk); err != nil {
		t.Fatalf("insert signing key: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM signing_keys WHERE kid = $1", sk.Kid)
	})
	active, err := store.ListActiveSigningKeys(ctx)
	if err != nil {
		t.Fatalf("list signing keys: %v", err)
	}
	var seen bool
	for _, k := range active {
		if k.Kid == sk.Kid {
			seen = true
			if _, err := k.Ed25519PrivateKey(); err != nil {
				t.Errorf("persisted private PEM does not decode: %v", err)
			}
		}
	}
	if !seen {
		t.Errorf("signing key %s not listed as active", sk.Kid)
	}
}
