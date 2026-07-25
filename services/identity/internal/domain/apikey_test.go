package domain

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

var secretPattern = regexp.MustCompile(`^lk_(sandbox|live)_[0-9A-Za-z]{32}$`)

func TestNewAPIKeySecretFormat(t *testing.T) {
	for _, env := range []string{EnvironmentSandbox, EnvironmentLive} {
		secret, err := NewAPIKeySecret(env)
		if err != nil {
			t.Fatalf("NewAPIKeySecret(%q): %v", env, err)
		}
		if !secretPattern.MatchString(secret) {
			t.Errorf("secret %q does not match %v", secret, secretPattern)
		}
		if !strings.HasPrefix(secret, "lk_"+env+"_") {
			t.Errorf("secret %q missing env prefix for %q", secret, env)
		}
		if got := len(PrefixOf(secret)); got != KeyPrefixLength {
			t.Errorf("prefix length = %d, want %d", got, KeyPrefixLength)
		}
	}
}

func TestNewAPIKeySecretRejectsUnknownEnvironment(t *testing.T) {
	if _, err := NewAPIKeySecret("production"); err == nil {
		t.Fatal("expected error for unknown environment")
	}
}

func TestNewAPIKeySecretUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 256)
	for range 256 {
		secret, err := NewAPIKeySecret(EnvironmentSandbox)
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[secret]; dup {
			t.Fatalf("duplicate secret generated: %s", secret)
		}
		seen[secret] = struct{}{}
	}
}

func TestVerifySecret(t *testing.T) {
	secret, err := NewAPIKeySecret(EnvironmentSandbox)
	if err != nil {
		t.Fatal(err)
	}
	key := APIKey{KeyPrefix: PrefixOf(secret), SecretHash: HashSecret(secret)}

	if !key.VerifySecret(secret) {
		t.Error("correct secret did not verify")
	}
	if key.VerifySecret(secret + "x") {
		t.Error("tampered secret verified")
	}
	if key.VerifySecret("") {
		t.Error("empty secret verified")
	}

	other, err := NewAPIKeySecret(EnvironmentSandbox)
	if err != nil {
		t.Fatal(err)
	}
	if key.VerifySecret(other) {
		t.Error("a different secret verified")
	}
}

func TestHashSecretIsDeterministicSHA256(t *testing.T) {
	a := HashSecret("lk_sandbox_abc")
	b := HashSecret("lk_sandbox_abc")
	if !bytes.Equal(a, b) {
		t.Error("hash is not deterministic")
	}
	if len(a) != 32 {
		t.Errorf("hash length = %d, want 32 (SHA-256)", len(a))
	}
	if bytes.Equal(a, HashSecret("lk_sandbox_abd")) {
		t.Error("distinct inputs produced identical hashes")
	}
}

func TestPrefixOfShortInput(t *testing.T) {
	if got := PrefixOf("lk_x"); got != "lk_x" {
		t.Errorf("PrefixOf short input = %q, want %q", got, "lk_x")
	}
}
