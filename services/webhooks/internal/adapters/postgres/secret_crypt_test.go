package postgres

// LC-008 encryption-at-rest test for webhook signing secrets.
//
// With a cipher configured, the secret column must hold an enc:v1: blob (never
// the plaintext), ClaimDue must decrypt it back so deliveries still sign, and a
// rotation must keep both current and previous secrets encrypted. Also checks
// the ReencryptPlaintextSecrets startup migration. Requires
// LEDGERCORE_TEST_DATABASE_URL.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/keycrypt"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/signature"
)

const cryptTestMasterKey = "3f8a2c1d9e4b7a6053c2e1f0b9d8a7c6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a1b0"

func TestSecretEncryptedAtRest(t *testing.T) {
	pool := testPool(t)
	cipher, err := keycrypt.New(cryptTestMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(pool, cipher)
	ctx := context.Background()
	tenantID := uuid.New()

	const plainSecret = "whsec_plaintextsecretshouldbesealed0"
	sub := domain.Subscription{
		ID:         uuid.New(),
		TenantID:   tenantID,
		URL:        "https://client.example.com/hooks",
		Secret:     plainSecret,
		EventTypes: []string{"*"},
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}
	if err := repo.Create(ctx, sub); err != nil {
		t.Fatalf("create: %v", err)
	}

	// The stored column must be an enc:v1: blob, not the plaintext.
	stored := readColumn(t, pool, sub.ID, "secret")
	if !keycrypt.IsEncrypted(stored) {
		t.Fatalf("stored secret is not encrypted: %q", stored)
	}
	if strings.Contains(stored, plainSecret) || strings.Contains(stored, "whsec_") {
		t.Fatalf("plaintext secret leaked into the column: %q", stored)
	}

	// Get must decrypt transparently.
	got, err := repo.Get(ctx, tenantID, sub.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Secret != plainSecret {
		t.Fatalf("decrypted secret mismatch: %q != %q", got.Secret, plainSecret)
	}

	// A delivery must sign with the decrypted secret and verify.
	if err := repo.InsertPending(ctx, tenantID, []domain.Delivery{{
		ID: uuid.New(), TenantID: tenantID, SubscriptionID: sub.ID, EventID: uuid.New(),
		EventType: "ledger.transaction.posted", Payload: []byte(`{"x":1}`),
		Status: domain.StatusPending, NextAttemptAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("insert pending: %v", err)
	}
	claimed, err := repo.ClaimDue(ctx, 10, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim due: err=%v n=%d", err, len(claimed))
	}
	if claimed[0].Secret != plainSecret {
		t.Fatalf("ClaimDue did not decrypt the secret: %q", claimed[0].Secret)
	}
	body := []byte(`{"x":1}`)
	ts := time.Now().Unix()
	header := signature.HeaderMulti(claimed[0].SigningSecrets(time.Now()), ts, body)
	if err := signature.VerifyAt(plainSecret, header, body, time.Unix(ts, 0), signature.DefaultTolerance); err != nil {
		t.Fatalf("signature does not verify with the original secret: %v", err)
	}

	// Rotate: both current and previous columns must stay encrypted, and both
	// must decrypt back correctly during the grace window.
	const newSecret = "whsec_rotatednewsecret00000000000000"
	if err := repo.RotateSecret(ctx, tenantID, sub.ID, newSecret, time.Now().UTC().Add(domain.RotationGrace)); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	rotStored := readColumn(t, pool, sub.ID, "secret")
	rotPrev := readColumn(t, pool, sub.ID, "previous_secret")
	if !keycrypt.IsEncrypted(rotStored) || !keycrypt.IsEncrypted(rotPrev) {
		t.Fatalf("rotated columns not both encrypted: secret=%q prev=%q", rotStored, rotPrev)
	}
	got, err = repo.Get(ctx, tenantID, sub.ID)
	if err != nil {
		t.Fatalf("get after rotate: %v", err)
	}
	if got.Secret != newSecret {
		t.Fatalf("new secret not decrypted: %q", got.Secret)
	}
	if got.PreviousSecret == nil || *got.PreviousSecret != plainSecret {
		t.Fatalf("previous secret not decrypted to original: %+v", got.PreviousSecret)
	}
}

func TestReencryptPlaintextSecrets(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tenantID := uuid.New()

	// Legacy row: created WITHOUT a cipher, so the secret is plaintext.
	plainRepo := NewRepo(pool, nil)
	const legacy = "whsec_legacyplaintextsecret000000000"
	sub := domain.Subscription{
		ID: uuid.New(), TenantID: tenantID, URL: "https://l.example.com/h",
		Secret: legacy, EventTypes: []string{"*"}, Active: true, CreatedAt: time.Now().UTC(),
	}
	if err := plainRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if keycrypt.IsEncrypted(readColumn(t, pool, sub.ID, "secret")) {
		t.Fatal("precondition failed: legacy secret should be plaintext")
	}

	// Startup migration with a cipher re-encrypts it in place.
	cipher, _ := keycrypt.New(cryptTestMasterKey)
	encRepo := NewRepo(pool, cipher)
	n, err := encRepo.ReencryptPlaintextSecrets(ctx)
	if err != nil {
		t.Fatalf("reencrypt: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row re-encrypted, got %d", n)
	}
	if !keycrypt.IsEncrypted(readColumn(t, pool, sub.ID, "secret")) {
		t.Fatal("secret was not re-encrypted at rest")
	}
	got, err := encRepo.Get(ctx, tenantID, sub.ID)
	if err != nil || got.Secret != legacy {
		t.Fatalf("re-encrypted secret does not decrypt to original: %q err=%v", got.Secret, err)
	}
}

// readColumn reads a single text column of a subscription row directly,
// bypassing the repo decryption. It reads under the maintenance identity
// because the cross-tenant (tenant-context-free) read is no longer served by a
// permissive policy (R-004).
func readColumn(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, col string) string {
	t.Helper()
	ctx := context.Background()
	var v *string
	err := withMaintTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT "+col+" FROM subscriptions WHERE id = $1", id).Scan(&v)
	})
	if err != nil {
		t.Fatalf("read column %s: %v", col, err)
	}
	if v == nil {
		return ""
	}
	return *v
}
