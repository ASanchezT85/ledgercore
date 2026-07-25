package domain

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AlgorithmEdDSA is the only signing algorithm the platform uses.
const AlgorithmEdDSA = "EdDSA"

// SigningKey is an Ed25519 key pair used to sign the JWTs this service
// issues. The public half is served through /.well-known/jwks.json.
//
// TODO(kms): in production the private key must live in a KMS/HSM and never
// touch the database; storing PEM here is a dev-bootstrap convenience only.
type SigningKey struct {
	Kid           uuid.UUID
	PrivateKeyPEM string
	PublicKeyPEM  string
	Algorithm     string
	CreatedAt     time.Time
	Active        bool
}

// GenerateSigningKey creates a fresh active Ed25519 signing key with both
// halves PEM-encoded (PKCS#8 private, PKIX public).
func GenerateSigningKey() (SigningKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SigningKey{}, fmt.Errorf("domain: generate ed25519 key: %w", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return SigningKey{}, fmt.Errorf("domain: marshal private key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return SigningKey{}, fmt.Errorf("domain: marshal public key: %w", err)
	}
	kid, err := uuid.NewV7()
	if err != nil {
		return SigningKey{}, fmt.Errorf("domain: generate kid: %w", err)
	}
	return SigningKey{
		Kid:           kid,
		PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})),
		PublicKeyPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})),
		Algorithm:     AlgorithmEdDSA,
		CreatedAt:     time.Now().UTC(),
		Active:        true,
	}, nil
}

// Ed25519PrivateKey decodes the PEM private half.
func (k SigningKey) Ed25519PrivateKey() (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(k.PrivateKeyPEM))
	if block == nil {
		return nil, errors.New("domain: signing key private PEM is malformed")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("domain: parse private key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("domain: signing key is not Ed25519")
	}
	return priv, nil
}

// Ed25519PublicKey decodes the PEM public half.
func (k SigningKey) Ed25519PublicKey() (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(k.PublicKeyPEM))
	if block == nil {
		return nil, errors.New("domain: signing key public PEM is malformed")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("domain: parse public key: %w", err)
	}
	pub, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("domain: signing key is not Ed25519")
	}
	return pub, nil
}
