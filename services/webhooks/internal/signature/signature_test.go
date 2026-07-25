package signature

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Known vectors computed with an independent HMAC-SHA256 implementation
// (.NET System.Security.Cryptography.HMACSHA256). They freeze the signing
// scheme: any change that breaks these breaks every client SDK.
var knownVectors = []struct {
	name   string
	secret string
	t      int64
	body   string
	hexSig string
}{
	{
		name:   "json body",
		secret: "whsec_8Zt5xVcbXkO2qWm1nJp3rTyUuIoPaSdF",
		t:      1700000000,
		body:   `{"hello":"world"}`,
		hexSig: "8d6e0eb1234ad908a934b80aa71636746c86da516c1d12f3a66edb3e9bc537ec",
	},
	{
		name:   "empty object",
		secret: "whsec_00000000000000000000000000000000",
		t:      1234567890,
		body:   `{}`,
		hexSig: "b988dac0a23f72be1f0f5fd147fd90e558dbf3d5d7e0730f1499d25286b65dd8",
	},
	{
		name:   "empty body",
		secret: "whsec_abc",
		t:      1700000001,
		body:   "",
		hexSig: "42f72bb46eeb920af976bd4e57d8635840971c464e6e42d71ee488efdb55ef2a",
	},
}

func TestSignKnownVectors(t *testing.T) {
	for _, v := range knownVectors {
		t.Run(v.name, func(t *testing.T) {
			got := Sign(v.secret, v.t, []byte(v.body))
			if got != v.hexSig {
				t.Fatalf("Sign(%q, %d, %q) = %s, want %s", v.secret, v.t, v.body, got, v.hexSig)
			}
		})
	}
}

func TestHeaderFormat(t *testing.T) {
	v := knownVectors[0]
	got := Header(v.secret, v.t, []byte(v.body))
	want := fmt.Sprintf("t=%d,v1=%s", v.t, v.hexSig)
	if got != want {
		t.Fatalf("Header = %s, want %s", got, want)
	}
}

func TestVerifyAcceptsValidSignature(t *testing.T) {
	for _, v := range knownVectors {
		t.Run(v.name, func(t *testing.T) {
			header := Header(v.secret, v.t, []byte(v.body))
			now := time.Unix(v.t, 0).Add(time.Minute)
			if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); err != nil {
				t.Fatalf("VerifyAt rejected a valid signature: %v", err)
			}
		})
	}
}

func TestVerifyRejectsTamperedBody(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	now := time.Unix(v.t, 0)
	tampered := []byte(`{"hello":"w0rld"}`)
	if err := VerifyAt(v.secret, header, tampered, now, DefaultTolerance); !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("want ErrSignatureMismatch for tampered body, got %v", err)
	}
}

func TestVerifyRejectsTamperedTimestamp(t *testing.T) {
	v := knownVectors[0]
	// Signature was computed for v.t but the header claims v.t+1.
	header := fmt.Sprintf("t=%d,v1=%s", v.t+1, Sign(v.secret, v.t, []byte(v.body)))
	now := time.Unix(v.t, 0)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("want ErrSignatureMismatch for tampered timestamp, got %v", err)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	now := time.Unix(v.t, 0)
	if err := VerifyAt("whsec_other", header, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("want ErrSignatureMismatch for wrong secret, got %v", err)
	}
}

func TestVerifyRejectsStaleTimestamp(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	now := time.Unix(v.t, 0).Add(DefaultTolerance + time.Second)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrTimestampTooOld) {
		t.Fatalf("want ErrTimestampTooOld, got %v", err)
	}
}

func TestVerifyToleranceDisabled(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	// Ten years later, tolerance 0 disables the freshness check.
	now := time.Unix(v.t, 0).Add(10 * 365 * 24 * time.Hour)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, 0); err != nil {
		t.Fatalf("tolerance 0 must skip freshness check, got %v", err)
	}
}

func TestVerifyAcceptsAnyMatchingV1(t *testing.T) {
	v := knownVectors[0]
	valid := Sign(v.secret, v.t, []byte(v.body))
	header := fmt.Sprintf("t=%d,v1=%s,v1=%s", v.t, strings.Repeat("0", 64), valid)
	now := time.Unix(v.t, 0)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); err != nil {
		t.Fatalf("VerifyAt must accept when any v1 matches, got %v", err)
	}
}

func TestVerifyIgnoresUnknownSchemes(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body)) + ",v2=deadbeef"
	now := time.Unix(v.t, 0)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); err != nil {
		t.Fatalf("unknown schemes must be ignored, got %v", err)
	}
}

func TestVerifyMalformedHeaders(t *testing.T) {
	v := knownVectors[0]
	now := time.Unix(v.t, 0)
	cases := []string{
		"",
		"t=notanumber,v1=abc",
		"v1=abc",       // missing t
		"t=1700000000", // missing v1
		"totally-not-a-header",
	}
	for _, h := range cases {
		if err := VerifyAt(v.secret, h, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrInvalidHeader) {
			t.Fatalf("header %q: want ErrInvalidHeader, got %v", h, err)
		}
	}
}
