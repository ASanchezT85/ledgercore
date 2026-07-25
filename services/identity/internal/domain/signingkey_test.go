package domain

import (
	"crypto/ed25519"
	"testing"
)

func TestGenerateSigningKeyRoundTrip(t *testing.T) {
	key, err := GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if key.Algorithm != AlgorithmEdDSA {
		t.Errorf("algorithm = %q, want %q", key.Algorithm, AlgorithmEdDSA)
	}
	if !key.Active {
		t.Error("new key should be active")
	}

	priv, err := key.Ed25519PrivateKey()
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	pub, err := key.Ed25519PublicKey()
	if err != nil {
		t.Fatalf("decode public key: %v", err)
	}

	msg := []byte("ledgercore signing key round trip")
	sig := ed25519.Sign(priv, msg)
	if !ed25519.Verify(pub, msg, sig) {
		t.Error("signature from decoded private key does not verify with decoded public key")
	}
}

func TestSigningKeyMalformedPEM(t *testing.T) {
	k := SigningKey{PrivateKeyPEM: "not pem", PublicKeyPEM: "not pem"}
	if _, err := k.Ed25519PrivateKey(); err == nil {
		t.Error("expected error for malformed private PEM")
	}
	if _, err := k.Ed25519PublicKey(); err == nil {
		t.Error("expected error for malformed public PEM")
	}
}
