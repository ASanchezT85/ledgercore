package app

import (
	"context"
	"strings"
	"testing"

	"github.com/ASanchezT85/ledgercore/services/identity/internal/domain"
	"github.com/ASanchezT85/ledgercore/services/identity/internal/keycrypt"
)

const testMasterKey = "3f8a2c1d9e4b7a6053c2e1f0b9d8a7c6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a1b0"

func newTestCipher(t *testing.T) *keycrypt.Cipher {
	t.Helper()
	c, err := keycrypt.New(testMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestEnsureSigningKeyEncryptsAtRest: with a master key, the persisted
// private PEM is an encrypted blob while the in-memory key stays usable.
func TestEnsureSigningKeyEncryptsAtRest(t *testing.T) {
	store := newFakeStore()
	cipher := newTestCipher(t)

	key, err := EnsureSigningKey(context.Background(), store, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(key.PrivateKeyPEM, "PRIVATE KEY") {
		t.Error("returned key must carry plaintext PEM for signing")
	}
	if _, err := key.Ed25519PrivateKey(); err != nil {
		t.Fatalf("returned key does not decode: %v", err)
	}
	stored := store.signing[0]
	if !keycrypt.IsEncrypted(stored.PrivateKeyPEM) {
		t.Fatal("persisted private PEM is not encrypted")
	}
	if strings.Contains(stored.PrivateKeyPEM, "PRIVATE KEY") {
		t.Error("persisted value leaks plaintext PEM")
	}

	// Second startup: decrypts and reuses the same key.
	again, err := EnsureSigningKey(context.Background(), store, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if again.Kid != key.Kid || again.PrivateKeyPEM != key.PrivateKeyPEM {
		t.Error("restart did not decrypt back to the same key")
	}
}

// TestEnsureSigningKeyMigratesLegacyPlaintext: a key stored in the clear is
// transparently re-encrypted at startup once a master key exists.
func TestEnsureSigningKeyMigratesLegacyPlaintext(t *testing.T) {
	store := newFakeStore()

	// Legacy state: generated and stored without a master key (plaintext).
	legacy, err := EnsureSigningKey(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if keycrypt.IsEncrypted(store.signing[0].PrivateKeyPEM) {
		t.Fatal("precondition failed: key should be plaintext")
	}

	cipher := newTestCipher(t)
	migrated, err := EnsureSigningKey(context.Background(), store, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Kid != legacy.Kid {
		t.Errorf("migration must not rotate the key: %s != %s", migrated.Kid, legacy.Kid)
	}
	if migrated.PrivateKeyPEM != legacy.PrivateKeyPEM {
		t.Error("in-memory key changed during migration")
	}
	if !keycrypt.IsEncrypted(store.signing[0].PrivateKeyPEM) {
		t.Fatal("legacy key was not re-encrypted at rest")
	}

	// And the encrypted state keeps working on the next startup.
	next, err := EnsureSigningKey(context.Background(), store, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if next.PrivateKeyPEM != legacy.PrivateKeyPEM {
		t.Error("post-migration startup did not decrypt to the original key")
	}
}

// TestEnsureSigningKeyEncryptedWithoutMasterKeyFails: fail closed if the DB
// holds an encrypted key but no master key is configured.
func TestEnsureSigningKeyEncryptedWithoutMasterKeyFails(t *testing.T) {
	store := newFakeStore()
	if _, err := EnsureSigningKey(context.Background(), store, newTestCipher(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureSigningKey(context.Background(), store, nil); err == nil {
		t.Fatal("expected failure: encrypted key at rest with no master key")
	}
}

// TestEnsureSigningKeyWrongMasterKeyFails: a different master key must not
// silently produce garbage.
func TestEnsureSigningKeyWrongMasterKeyFails(t *testing.T) {
	store := newFakeStore()
	if _, err := EnsureSigningKey(context.Background(), store, newTestCipher(t)); err != nil {
		t.Fatal(err)
	}
	other, err := keycrypt.New(strings.Repeat("11", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureSigningKey(context.Background(), store, other); err == nil {
		t.Fatal("expected failure with the wrong master key")
	}
}

// Sanity: token issuance still works end to end with encryption on.
func TestTokenIssuanceWithEncryptedKey(t *testing.T) {
	store := newFakeStore()
	key, err := EnsureSigningKey(context.Background(), store, newTestCipher(t))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewTokenIssuer(key, TokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, store, store, issuer)
	tenant, err := svc.CreateTenant(context.Background(), "Acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	_, secret, err := svc.CreateAPIKey(context.Background(), tenant.ID, domain.EnvironmentSandbox, "ci", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IssueToken(context.Background(), secret); err != nil {
		t.Fatalf("IssueToken with encrypted-at-rest key: %v", err)
	}
}
