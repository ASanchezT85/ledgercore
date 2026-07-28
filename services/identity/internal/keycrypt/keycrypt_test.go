package keycrypt

import (
	"strings"
	"testing"
)

const testKey = "3f8a2c1d9e4b7a6053c2e1f0b9d8a7c6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a1b0"

func TestNewRejectsBadKeys(t *testing.T) {
	for _, bad := range []string{"", "zz", "abcd", strings.Repeat("ab", 16), strings.Repeat("ab", 33)} {
		if _, err := New(bad); err == nil {
			t.Errorf("New(%q): expected error", bad)
		}
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := New(testKey)
	if err != nil {
		t.Fatal(err)
	}
	plain := "-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIA==\n-----END PRIVATE KEY-----\n"
	sealed, err := c.Encrypt(plain, "kid-1")
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(sealed) {
		t.Fatalf("sealed blob lacks prefix: %q", sealed)
	}
	if IsEncrypted(plain) {
		t.Error("plaintext PEM must not look encrypted")
	}
	got, err := c.Decrypt(sealed, "kid-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("round-trip mismatch: %q != %q", got, plain)
	}

	// Nonce per record: same input encrypts to different blobs.
	sealed2, err := c.Encrypt(plain, "kid-1")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == sealed2 {
		t.Error("two encryptions produced identical blobs (nonce reuse?)")
	}

	// AAD binding: decrypting under another kid must fail.
	if _, err := c.Decrypt(sealed, "kid-2"); err == nil {
		t.Error("decrypt with wrong kid AAD must fail")
	}

	// Wrong master key must fail.
	other, err := New(strings.Repeat("00", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(sealed, "kid-1"); err == nil {
		t.Error("decrypt with wrong master key must fail")
	}
}

func TestNilCipherFailsClosed(t *testing.T) {
	var c *Cipher
	if _, err := c.Encrypt("pem", "kid"); err == nil {
		t.Error("nil cipher Encrypt must fail")
	}
	if _, err := c.Decrypt("enc:v1:AAAA", "kid"); err == nil {
		t.Error("nil cipher Decrypt must fail")
	}
}
