package keycrypt

import (
	"strings"
	"testing"
)

const testMasterKey = "3f8a2c1d9e4b7a6053c2e1f0b9d8a7c6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a1b0"

func newCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := New(testMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRoundTrip(t *testing.T) {
	c := newCipher(t)
	const secret = "lcwh_8Zt5xVcbXkO2qWm1nJp3rTyUuIoPaSdF"
	const aad = "11111111-1111-1111-1111-111111111111"

	blob, err := c.Encrypt(secret, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(blob) {
		t.Fatalf("blob missing enc:v1: prefix: %q", blob)
	}
	if strings.Contains(blob, secret) {
		t.Fatal("plaintext secret leaks into the encrypted blob")
	}
	got, err := c.Decrypt(blob, aad)
	if err != nil {
		t.Fatal(err)
	}
	if got != secret {
		t.Fatalf("round-trip mismatch: %q != %q", got, secret)
	}
}

func TestDecryptWrongAAD(t *testing.T) {
	c := newCipher(t)
	blob, err := c.Encrypt("lcwh_x", "sub-a")
	if err != nil {
		t.Fatal(err)
	}
	// A blob bound to sub-a must not open under sub-b (anti-swap).
	if _, err := c.Decrypt(blob, "sub-b"); err == nil {
		t.Fatal("decrypt succeeded with the wrong AAD")
	}
}

func TestDecryptWrongMasterKey(t *testing.T) {
	c := newCipher(t)
	blob, err := c.Encrypt("lcwh_x", "sub")
	if err != nil {
		t.Fatal(err)
	}
	other, err := New(strings.Repeat("11", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(blob, "sub"); err == nil {
		t.Fatal("decrypt succeeded with the wrong master key")
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	c := newCipher(t)
	// Legacy plaintext (no prefix) is returned verbatim.
	got, err := c.Decrypt("lcwh_legacy", "sub")
	if err != nil {
		t.Fatal(err)
	}
	if got != "lcwh_legacy" {
		t.Fatalf("plaintext passthrough failed: %q", got)
	}
}

func TestDecryptEncryptedWithoutKeyFails(t *testing.T) {
	c := newCipher(t)
	blob, err := c.Encrypt("lcwh_x", "sub")
	if err != nil {
		t.Fatal(err)
	}
	var nilCipher *Cipher
	if _, err := nilCipher.Decrypt(blob, "sub"); err != ErrNoMasterKey {
		t.Fatalf("want ErrNoMasterKey, got %v", err)
	}
}

func TestNewRejectsBadKey(t *testing.T) {
	if _, err := New("zzz"); err == nil {
		t.Fatal("want error for non-hex key")
	}
	if _, err := New("aabb"); err == nil {
		t.Fatal("want error for short key")
	}
}
