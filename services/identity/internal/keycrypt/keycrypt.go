// Package keycrypt implements pragmatic envelope encryption for the JWT
// signing keys stored in Postgres: AES-256-GCM under a single master key
// provided via LEDGERCORE_MASTER_KEY (32 bytes, hex-encoded).
//
// This is the interim "KMS" for the shared-VPS deployment: the master key
// lives only in the host environment, never in the repo or the database.
// The migration to a managed KMS (AWS/GCP) is deferred to the cloud stage
// (see docs/legal/politicas-seguridad.md, secrets policy).
package keycrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// prefix marks an encrypted blob (versioned so the format can evolve).
const prefix = "enc:v1:"

// ErrNoMasterKey is returned when a stored key is encrypted but no master
// key was provided to decrypt it.
var ErrNoMasterKey = errors.New("keycrypt: signing key is encrypted at rest but LEDGERCORE_MASTER_KEY is not set")

// Cipher encrypts/decrypts signing-key PEM blobs with AES-256-GCM.
// A nil *Cipher is a valid "encryption disabled" value.
type Cipher struct {
	aead cipher.AEAD
}

// New parses a 32-byte hex master key and builds the AEAD.
func New(hexKey string) (*Cipher, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, errors.New("keycrypt: LEDGERCORE_MASTER_KEY must be hex-encoded")
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("keycrypt: LEDGERCORE_MASTER_KEY must be 32 bytes (64 hex chars), got %d bytes", len(raw))
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("keycrypt: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("keycrypt: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// IsEncrypted reports whether a stored value is an encrypted blob (as
// opposed to a legacy plaintext PEM).
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, prefix)
}

// Encrypt seals a plaintext PEM. The kid is bound as AAD so a blob cannot
// be swapped between key records. A fresh random nonce is used per call.
func (c *Cipher) Encrypt(plaintextPEM, kid string) (string, error) {
	if c == nil {
		return "", errors.New("keycrypt: encryption requested but no master key configured")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("keycrypt: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintextPEM), []byte(kid))
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a blob produced by Encrypt, verifying the kid AAD.
func (c *Cipher) Decrypt(stored, kid string) (string, error) {
	if !IsEncrypted(stored) {
		return "", errors.New("keycrypt: value is not an encrypted blob")
	}
	if c == nil {
		return "", ErrNoMasterKey
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", errors.New("keycrypt: malformed encrypted blob (base64)")
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("keycrypt: malformed encrypted blob (too short)")
	}
	plain, err := c.aead.Open(nil, raw[:ns], raw[ns:], []byte(kid))
	if err != nil {
		return "", errors.New("keycrypt: decryption failed (wrong master key or tampered blob)")
	}
	return string(plain), nil
}
