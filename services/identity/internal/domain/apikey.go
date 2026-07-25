package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Secret format: "lk_<env>_<SecretRandomLength base62 chars>".
const (
	// SecretRandomLength is the number of random base62 characters in a secret.
	SecretRandomLength = 32
	// KeyPrefixLength is how many leading characters of the full secret are
	// stored in clear for lookup (the rest is only ever stored as a hash).
	KeyPrefixLength = 12
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// APIKey is a long-lived credential a tenant exchanges for short-lived JWTs.
// The plaintext secret is shown exactly once at creation time; only its
// SHA-256 hash and a short lookup prefix are persisted.
type APIKey struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Environment string
	Name        string
	KeyPrefix   string
	SecretHash  []byte
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

// Revoked reports whether the key has been revoked.
func (k APIKey) Revoked() bool { return k.RevokedAt != nil }

// VerifySecret compares the SHA-256 hash of the candidate secret against the
// stored hash in constant time.
func (k APIKey) VerifySecret(secret string) bool {
	candidate := HashSecret(secret)
	return subtle.ConstantTimeCompare(candidate, k.SecretHash) == 1
}

// NewAPIKeySecret generates a fresh plaintext secret for the given
// environment: lk_<env>_<32 random base62 chars> from crypto/rand.
func NewAPIKeySecret(environment string) (string, error) {
	if !ValidEnvironment(environment) {
		return "", fmt.Errorf("domain: invalid environment %q", environment)
	}
	random, err := randomBase62(SecretRandomLength)
	if err != nil {
		return "", fmt.Errorf("domain: generate api key secret: %w", err)
	}
	return "lk_" + environment + "_" + random, nil
}

// randomBase62 returns n uniformly random base62 characters using rejection
// sampling (bytes >= 248 are discarded so the modulo introduces no bias:
// 248 = 62 * 4, each symbol maps to exactly 4 byte values).
func randomBase62(n int) (string, error) {
	out := make([]byte, 0, n)
	buf := make([]byte, 2*n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b >= 248 {
				continue
			}
			out = append(out, base62Alphabet[int(b)%62])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}

// HashSecret returns the SHA-256 digest of the plaintext secret. SHA-256 is
// appropriate here (rather than a slow KDF) because secrets are 32 chars of
// crypto/rand base62 (~190 bits of entropy), not human-chosen passwords.
func HashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// PrefixOf returns the lookup prefix (first KeyPrefixLength characters) of a
// plaintext secret.
func PrefixOf(secret string) string {
	if len(secret) < KeyPrefixLength {
		return secret
	}
	return secret[:KeyPrefixLength]
}
